// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jkind73/cassini/discimg"
	"github.com/jkind73/cassini/screenscraper"
	"github.com/jkind73/cassini/uiface"
)

// Resolver runs the full identification fallback chain for a scanned
// disc: ScreenScraper (network, best metadata) -> RetroArch DAT (local
// hash match, good title/serial) -> MAME listxml (local hash match,
// arcade-oriented) -> IP.BIN-only (always available, least detail).
// Each tier is tried only if the previous one didn't produce a
// result, and every tier's result is written through UpsertGame's
// confidence ranking (schema.go/games.go), so calling Resolve
// repeatedly (e.g. re-running a scan after importing a DAT for the
// first time) can only ever improve a cached entry, never regress it.
type Resolver struct {
	Cache          *Cache
	ScreenScraper  *screenscraper.Client // nil is fine: network tier is skipped, not an error
	SaturnSystemID int                   // screenscraper.SaturnSystemID by default; see NewResolver
}

// NewResolver constructs a Resolver with cassini's known SaturnSystemID
// default. ss may be nil (e.g. no ScreenScraper credentials configured
// yet -- see config.ScreenScraperCreds, filled in via the UI) to run
// the local-only tiers (RetroArch/MAME/IP.BIN).
func NewResolver(c *Cache, ss *screenscraper.Client) *Resolver {
	return &Resolver{Cache: c, ScreenScraper: ss, SaturnSystemID: screenscraper.SaturnSystemID}
}

// ResolveDisc runs the fallback chain for one scanned Saturn disc
// (discimg.Found with HasDiscInfo true) and returns the Game that was
// written to the cache.
func (r *Resolver) ResolveDisc(ctx context.Context, found discimg.Found) (Game, error) {
	if !found.HasDiscInfo {
		return Game{}, fmt.Errorf("cache: resolve: %s has no IP.BIN volume header, nothing to key on", found.Path)
	}
	productNumber := found.DiscInfo.ProductNumber
	if productNumber == "" {
		return Game{}, fmt.Errorf("cache: resolve: %s: empty product number", found.Path)
	}

	// Tier 0: already cached at screenscraper confidence or above --
	// nothing to do. (A lower-confidence existing row is still worth
	// re-attempting the higher tiers for, since UpsertGame's ranking
	// means a successful higher-confidence result will still improve
	// it below.)
	if existing, err := r.Cache.GetGame(productNumber); err == nil {
		if sourceRank[existing.Source] >= sourceRank["screenscraper"] {
			return existing, nil
		}
	} else if !errors.Is(err, ErrNotFound) {
		return Game{}, err
	}

	sha1, crc32 := primaryHash(found)

	// Tier 1: ScreenScraper, by hash.
	if r.ScreenScraper != nil {
		g, err := r.ScreenScraper.IdentifyGame(ctx, screenscraper.IdentifyParams{
			SystemID: r.SaturnSystemID,
			ROMType:  "iso",
			SHA1:     sha1,
			CRC32:    crc32,
		})
		if err == nil {
			raw, _ := json.Marshal(g)
			game := Game{
				ProductNumber: productNumber, Title: g.Name(),
				Region:     regionString(found.DiscInfo.Region),
				DiscNumber: found.DiscInfo.DiscNumber, DiscTotal: found.DiscInfo.DiscTotal,
				Source: "screenscraper", ScreenScraperID: g.ID, ScreenScraperJSON: raw,
				Path: found.Path,
			}
			if err := r.Cache.UpsertGame(game); err != nil {
				return Game{}, err
			}
			r.cacheMedia(g, productNumber)
			return game, nil
		}
		// Not found (or any other API error) falls through to the
		// local tiers rather than aborting -- a rate limit, a
		// temporary outage, or a genuine "not in ScreenScraper's
		// database" should all still leave the disc with *some*
		// title rather than none.
	}

	// Tier 2: RetroArch DAT, by hash.
	if ra, ok, err := r.Cache.LookupRetroArchByHash(sha1, crc32); err != nil {
		return Game{}, err
	} else if ok {
		title := ra.Description
		if title == "" {
			title = ra.Name
		}
		game := Game{
			ProductNumber: productNumber, Title: title,
			Region:     regionString(found.DiscInfo.Region),
			DiscNumber: found.DiscInfo.DiscNumber, DiscTotal: found.DiscInfo.DiscTotal,
			Source: "retroarch", Path: found.Path,
		}
		// Cross-check: if the DAT entry carries a serial and it
		// disagrees with the IP.BIN-derived product number, that's
		// worth surfacing rather than silently preferring one -- most
		// likely explanation is a multi-disc set where the DAT's
		// serial reflects only disc 1, or a regional variant mismatch.
		// UpsertGame still uses the IP.BIN product number as the key
		// (it's what was actually read off this specific disc); this
		// is purely informational.
		if ra.Serial != "" && ra.Serial != productNumber {
			game.Title = fmt.Sprintf("%s [retroarch serial %s != ipbin %s]", title, ra.Serial, productNumber)
		}
		if err := r.Cache.UpsertGame(game); err != nil {
			return Game{}, err
		}
		return game, nil
	}

	// Tier 3: MAME listxml, by hash. Primarily meaningful for ST-V
	// (see ResolveArcadeSet for the cartridge path), but a Saturn
	// CD-ROM-based ST-V title (Sports Fishing 2's addon) could in
	// principle hash-match a MAME <machine>'s data track too, so this
	// tier is not skipped for discs.
	if mm, ok, err := r.Cache.LookupMameByHash(sha1, crc32); err != nil {
		return Game{}, err
	} else if ok {
		game := Game{
			ProductNumber: productNumber, Title: mm.Description,
			Region:     regionString(found.DiscInfo.Region),
			DiscNumber: found.DiscInfo.DiscNumber, DiscTotal: found.DiscInfo.DiscTotal,
			Source: "mame", Path: found.Path,
		}
		if err := r.Cache.UpsertGame(game); err != nil {
			return Game{}, err
		}
		return game, nil
	}

	// Tier 4: IP.BIN only -- always available, since HasDiscInfo was
	// already checked above.
	game := Game{
		ProductNumber: productNumber, Title: found.DiscInfo.Title,
		Region:     regionString(found.DiscInfo.Region),
		DiscNumber: found.DiscInfo.DiscNumber, DiscTotal: found.DiscInfo.DiscTotal,
		Source: "ipbin", Path: found.Path,
	}
	if err := r.Cache.UpsertGame(game); err != nil {
		return Game{}, err
	}
	return game, nil
}

// ResolveArcadeSet runs the MAME tier for an ST-V cartridge,
// identified by its own hash set (from a scan of the cartridge dump --
// ST-V per-title hash identification itself is not yet implemented in
// cassini; this function accepts the hashes as already computed by
// whatever calls it). ScreenScraper is not queried here: cassini does
// not yet have a verified ScreenScraper systemeid for ST-V (see
// screenscraper package's STVSystemID doc comment note) -- resolve
// that via screenscraper.Client.Systems() + ResolveSystemID("ST-V")
// before wiring a ScreenScraper tier in here.
func (r *Resolver) ResolveArcadeSet(sha1, crc32, path string) (ArcadeSet, error) {
	if mm, ok, err := r.Cache.LookupMameByHash(sha1, crc32); err != nil {
		return ArcadeSet{}, err
	} else if ok {
		set := ArcadeSet{
			MameShortname: mm.Shortname, Title: mm.Description,
			Year: mm.Year, Manufacturer: mm.Manufacturer,
			Source: "mame", Path: path,
		}
		return set, r.Cache.UpsertArcadeSet(set)
	}
	return ArcadeSet{}, ErrNotFound
}

// cacheMedia records every media entry ScreenScraper returned for g
// (URL only -- actually downloading bytes to disk is item 7/8
// territory, the browser UI decides what to prefetch vs. lazy-load).
func (r *Resolver) cacheMedia(g screenscraper.Game, productNumber string) {
	for _, m := range g.Medias {
		r.Cache.UpsertMedia(Media{
			ProductNumber: productNumber,
			MediaType:     m.Type,
			Region:        m.Region,
			SourceURL:     m.URL,
		})
	}
}

func primaryHash(found discimg.Found) (sha1, crc32 string) {
	if found.CHD != nil {
		return found.CHD.SHA1, ""
	}
	if len(found.Files) > 0 {
		d := found.Files[0].Digest
		return d.SHA1, fmt.Sprintf("%08x", d.CRC32)
	}
	return "", ""
}

func regionString(r uiface.Region) string {
	switch r {
	case uiface.RegionJapan:
		return "jp"
	case uiface.RegionUSA:
		return "us"
	case uiface.RegionEurope:
		return "eu"
	case uiface.RegionAsiaNTSC:
		return "asia"
	default:
		return ""
	}
}
