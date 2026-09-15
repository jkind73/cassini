// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package screenscraper

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
)

// ErrGameNotFound is returned when ScreenScraper has no match for the
// given hashes/filename. Detected via an effectively-empty payload --
// see parseGame's implementation comment for exactly what's checked,
// and its accuracy caveat.
var ErrGameNotFound = errors.New("screenscraper: no matching game")

// LocalizedText is ScreenScraper's region-keyed text pattern (used
// for game names, synopses, and other per-region fields). This shape
// -- {"region": "...", "text": "..."} inside a named array -- is the
// one convention I found consistently described across every client
// surveyed (skyscraper, screech, the JS/Dart/PHP wrappers), unlike the
// rest of the `jeu` object's field names.
type LocalizedText struct {
	Region string `json:"region"`
	Text   string `json:"text"`
}

// Media is one entry in a game's media list -- type keys match the
// documented table in the package doc comment (box-2D, wheel-hd,
// sstitle, video, etc), optionally region-suffixed as
// "wheel-hd(wor)"-style keys per the API Playground's documented
// convention.
type Media struct {
	Type   string `json:"type"`
	Region string `json:"region"`
	URL    string `json:"url"`
	Format string `json:"format"`
}

// Game is a ScreenScraper game record. See the package doc comment
// for exactly which fields here are independently verified vs. a
// best-effort convenience subset; RawJSON is always the authoritative
// source.
type Game struct {
	ID      string          `json:"id"`
	Names   []LocalizedText `json:"noms"`
	Medias  []Media         `json:"medias"`
	RawJSON json.RawMessage `json:"-"`
}

// Name returns the best available display name: prefers "wor"
// (world) or "eu" region tags if present (the two region codes every
// surveyed client treats as sane defaults), else the first entry.
func (g Game) Name() string {
	for _, n := range g.Names {
		if n.Region == "wor" || n.Region == "eu" {
			return n.Text
		}
	}
	if len(g.Names) > 0 {
		return g.Names[0].Text
	}
	return ""
}

// MediaByType returns the first Media entry whose Type matches
// (case-sensitive, matching the documented type-key table exactly).
func (g Game) MediaByType(mediaType string) (Media, bool) {
	for _, m := range g.Medias {
		if m.Type == mediaType {
			return m, true
		}
	}
	return Media{}, false
}

// IdentifyParams identifies a game by the hash-first workflow the API
// Playground documents as the recommended path ("Can I match a real
// file using hash + size + filename?"). SystemID, ROMName, and at
// least one of CRC32/SHA1 should be set; ROMSize is optional but
// recommended (ScreenScraper's own docs, per every client surveyed,
// use it as a tie-breaker).
type IdentifyParams struct {
	SystemID int
	ROMType  string // "rom", "iso", or "folder" -- pass "iso" for cassini's disc-based scans
	ROMName  string
	ROMSize  int64
	CRC32    string // hex, matches hashid.Digest.CRC32 formatted as %08x
	SHA1     string // hex, matches hashid.Digest.SHA1 / discimg.FileHash.Digest.SHA1
	MD5      string // optional; cassini does not compute MD5 (see hashid's doc comment on MAME convention), left for callers with an MD5 from elsewhere
}

// IdentifyGame calls jeuInfos.php.
func (c *Client) IdentifyGame(ctx context.Context, p IdentifyParams) (Game, error) {
	v := url.Values{}
	if p.SystemID > 0 {
		v.Set("systemeid", strconv.Itoa(p.SystemID))
	}
	if p.ROMType != "" {
		v.Set("romtype", p.ROMType)
	}
	if p.ROMName != "" {
		v.Set("romnom", p.ROMName)
	}
	if p.ROMSize > 0 {
		v.Set("romtaille", strconv.FormatInt(p.ROMSize, 10))
	}
	if p.CRC32 != "" {
		v.Set("crc", p.CRC32)
	}
	if p.SHA1 != "" {
		v.Set("sha1", p.SHA1)
	}
	if p.MD5 != "" {
		v.Set("md5", p.MD5)
	}

	raw, err := c.do(ctx, "jeuInfos.php", v)
	if err != nil {
		// ScreenScraper is documented (across client error-handling
		// code) to report "not found" via the header error text
		// rather than a distinct HTTP status -- surfaced generically
		// as *APIError by (*Client).do. Without a verified stable
		// substring for "not found" specifically (vs. other failure
		// reasons sharing the same mechanism), this package does not
		// attempt to distinguish ErrGameNotFound from other APIErrors
		// at this layer; callers that need to tell them apart should
		// inspect the returned *APIError's Header.Error themselves
		// against what they observe from their own account, rather
		// than this package guessing a wrong substring match.
		return Game{}, err
	}

	return parseGame(raw)
}

// SearchParams identifies a game by title when no file hash match was
// found -- the documented fallback path.
type SearchParams struct {
	SystemID int
	Query    string
}

// SearchGames calls jeuRecherche.php.
func (c *Client) SearchGames(ctx context.Context, p SearchParams) ([]Game, error) {
	v := url.Values{}
	if p.SystemID > 0 {
		v.Set("systemeid", strconv.Itoa(p.SystemID))
	}
	v.Set("recherche", p.Query)

	raw, err := c.do(ctx, "jeuRecherche.php", v)
	if err != nil {
		return nil, err
	}

	// Same response-shape uncertainty as Systems(): try a direct array
	// first, then a wrapped {"jeux": [...]} shape.
	var direct []json.RawMessage
	if err := json.Unmarshal(raw, &direct); err == nil {
		return parseGames(direct)
	}
	var wrapped struct {
		Jeux []json.RawMessage `json:"jeux"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Jeux != nil {
		return parseGames(wrapped.Jeux)
	}
	return nil, errors.New("screenscraper: jeuRecherche.php: unrecognized response shape")
}

func parseGame(raw json.RawMessage) (Game, error) {
	// jeuInfos.php's response wraps the game under a "jeu" key per
	// screech's verified struct (Response.Jeu); handle both that and a
	// bare game object defensively, since (per the package doc
	// comment) I could not confirm this nesting with a live call.
	var wrapped struct {
		Jeu json.RawMessage `json:"jeu"`
	}
	target := raw
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Jeu != nil {
		target = wrapped.Jeu
	}

	if len(target) <= 2 { // "{}" / "[]" / "null" -- effectively empty
		return Game{}, ErrGameNotFound
	}

	var g Game
	if err := json.Unmarshal(target, &g); err != nil {
		return Game{}, err
	}
	g.RawJSON = target
	return g, nil
}

func parseGames(items []json.RawMessage) ([]Game, error) {
	out := make([]Game, 0, len(items))
	for _, item := range items {
		g, err := parseGame(item)
		if err != nil {
			continue
		}
		out = append(out, g)
	}
	return out, nil
}
