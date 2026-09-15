// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package cache

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrNotFound is returned by the Get* functions when no row matches.
var ErrNotFound = errors.New("cache: not found")

// Game is one cached row, keyed by product number.
type Game struct {
	ProductNumber     string
	Title             string
	Region            string
	DiscNumber        int
	DiscTotal         int
	Source            string // "screenscraper" | "retroarch" | "mame" | "ipbin" | "manual"
	ScreenScraperID   string
	ScreenScraperJSON json.RawMessage
	Path              string
}

// sourceRank orders Source values by confidence, used to decide
// whether a new write should be allowed to overwrite an existing row
// -- see UpsertGame's doc comment. Higher is more authoritative.
var sourceRank = map[string]int{
	"ipbin":         0,
	"mame":          1,
	"retroarch":     2,
	"screenscraper": 3,
	"manual":        4, // a user's own edit always wins
}

// UpsertGame inserts or updates a game row. If a row already exists
// for g.ProductNumber, the write is applied only when g.Source is the
// same or higher confidence than the existing row's source (per
// sourceRank) -- this is what lets the resolver chain (resolver.go)
// safely call UpsertGame at every fallback tier without a later,
// lower-confidence tier ever clobbering an earlier successful
// higher-confidence match, and without a "manual" user edit ever being
// silently overwritten by a subsequent re-scrape.
func (c *Cache) UpsertGame(g Game) error {
	existing, err := c.GetGame(g.ProductNumber)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	if err == nil && sourceRank[g.Source] < sourceRank[existing.Source] {
		return nil // lower-confidence write against a better existing row: no-op, not an error
	}

	_, err = c.db.Exec(`
		INSERT INTO games (product_number, title, region, disc_number, disc_total, source, screenscraper_id, screenscraper_json, path, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, unixepoch())
		ON CONFLICT(product_number) DO UPDATE SET
			title=excluded.title, region=excluded.region,
			disc_number=excluded.disc_number, disc_total=excluded.disc_total,
			source=excluded.source, screenscraper_id=excluded.screenscraper_id,
			screenscraper_json=excluded.screenscraper_json, path=excluded.path,
			updated_at=unixepoch()
	`, g.ProductNumber, g.Title, g.Region, g.DiscNumber, g.DiscTotal, g.Source, g.ScreenScraperID, []byte(g.ScreenScraperJSON), g.Path)
	if err != nil {
		return fmt.Errorf("cache: upsert game %s: %w", g.ProductNumber, err)
	}
	return nil
}

// GetGame looks up a game by product number.
func (c *Cache) GetGame(productNumber string) (Game, error) {
	var g Game
	var raw []byte
	err := c.db.QueryRow(`
		SELECT product_number, title, region, disc_number, disc_total, source, screenscraper_id, screenscraper_json, path
		FROM games WHERE product_number = ?
	`, productNumber).Scan(&g.ProductNumber, &g.Title, &g.Region, &g.DiscNumber, &g.DiscTotal, &g.Source, &g.ScreenScraperID, &raw, &g.Path)
	if errors.Is(err, sql.ErrNoRows) {
		return Game{}, ErrNotFound
	}
	if err != nil {
		return Game{}, fmt.Errorf("cache: get game %s: %w", productNumber, err)
	}
	g.ScreenScraperJSON = raw
	return g, nil
}

// ListGames returns every cached game, for the browser UI (item 7).
func (c *Cache) ListGames() ([]Game, error) {
	rows, err := c.db.Query(`SELECT product_number, title, region, disc_number, disc_total, source, screenscraper_id, screenscraper_json, path FROM games ORDER BY title`)
	if err != nil {
		return nil, fmt.Errorf("cache: list games: %w", err)
	}
	defer rows.Close()

	var out []Game
	for rows.Next() {
		var g Game
		var raw []byte
		if err := rows.Scan(&g.ProductNumber, &g.Title, &g.Region, &g.DiscNumber, &g.DiscTotal, &g.Source, &g.ScreenScraperID, &raw, &g.Path); err != nil {
			return nil, fmt.Errorf("cache: list games: scan: %w", err)
		}
		g.ScreenScraperJSON = raw
		out = append(out, g)
	}
	return out, rows.Err()
}

// ArcadeSet mirrors Game for ST-V cartridge titles (see schema.go for
// why these are separate tables).
type ArcadeSet struct {
	MameShortname     string
	Title             string
	Year              string
	Manufacturer      string
	Source            string
	ScreenScraperID   string
	ScreenScraperJSON json.RawMessage
	Path              string
}

func (c *Cache) UpsertArcadeSet(a ArcadeSet) error {
	existing, err := c.GetArcadeSet(a.MameShortname)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	if err == nil && sourceRank[a.Source] < sourceRank[existing.Source] {
		return nil
	}

	_, err = c.db.Exec(`
		INSERT INTO arcade_sets (mame_shortname, title, year, manufacturer, source, screenscraper_id, screenscraper_json, path, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, unixepoch())
		ON CONFLICT(mame_shortname) DO UPDATE SET
			title=excluded.title, year=excluded.year, manufacturer=excluded.manufacturer,
			source=excluded.source, screenscraper_id=excluded.screenscraper_id,
			screenscraper_json=excluded.screenscraper_json, path=excluded.path,
			updated_at=unixepoch()
	`, a.MameShortname, a.Title, a.Year, a.Manufacturer, a.Source, a.ScreenScraperID, []byte(a.ScreenScraperJSON), a.Path)
	if err != nil {
		return fmt.Errorf("cache: upsert arcade set %s: %w", a.MameShortname, err)
	}
	return nil
}

func (c *Cache) GetArcadeSet(shortname string) (ArcadeSet, error) {
	var a ArcadeSet
	var raw []byte
	err := c.db.QueryRow(`
		SELECT mame_shortname, title, year, manufacturer, source, screenscraper_id, screenscraper_json, path
		FROM arcade_sets WHERE mame_shortname = ?
	`, shortname).Scan(&a.MameShortname, &a.Title, &a.Year, &a.Manufacturer, &a.Source, &a.ScreenScraperID, &raw, &a.Path)
	if errors.Is(err, sql.ErrNoRows) {
		return ArcadeSet{}, ErrNotFound
	}
	if err != nil {
		return ArcadeSet{}, fmt.Errorf("cache: get arcade set %s: %w", shortname, err)
	}
	a.ScreenScraperJSON = raw
	return a, nil
}

// Media is one cached asset reference.
type Media struct {
	ProductNumber string // exactly one of ProductNumber/MameShortname is set
	MameShortname string
	MediaType     string
	Region        string
	LocalPath     string
	SourceURL     string
}

func (c *Cache) UpsertMedia(m Media) error {
	_, err := c.db.Exec(`
		INSERT INTO media (product_number, mame_shortname, media_type, region, local_path, source_url, cached_at)
		VALUES (?, ?, ?, ?, ?, ?, unixepoch())
		ON CONFLICT(product_number, mame_shortname, media_type, region) DO UPDATE SET
			local_path=excluded.local_path, source_url=excluded.source_url, cached_at=unixepoch()
	`, m.ProductNumber, m.MameShortname, m.MediaType, m.Region, m.LocalPath, m.SourceURL)
	if err != nil {
		return fmt.Errorf("cache: upsert media: %w", err)
	}
	return nil
}

// MediaForGame returns every cached media row for a product number.
func (c *Cache) MediaForGame(productNumber string) ([]Media, error) {
	rows, err := c.db.Query(`SELECT product_number, mame_shortname, media_type, region, local_path, source_url FROM media WHERE product_number = ?`, productNumber)
	if err != nil {
		return nil, fmt.Errorf("cache: media for %s: %w", productNumber, err)
	}
	defer rows.Close()

	var out []Media
	for rows.Next() {
		var m Media
		if err := rows.Scan(&m.ProductNumber, &m.MameShortname, &m.MediaType, &m.Region, &m.LocalPath, &m.SourceURL); err != nil {
			return nil, fmt.Errorf("cache: media for %s: scan: %w", productNumber, err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
