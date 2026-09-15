// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package screenscraper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// System is one entry from systemesListe.php.
//
// Field names are a best-effort mapping: I have high confidence in
// ID (every ScreenScraper-derived tool keys systems by a numeric ID,
// and this is the field jeuInfos.php's systemeid parameter expects)
// but lower confidence in the exact JSON key ScreenScraper uses for
// the display name, since -- same caveat as the package doc comment
// -- I don't have a live authenticated response to confirm against.
// RawJSON preserves everything for a caller that needs a field not
// mapped here.
type System struct {
	ID      int
	Nom     string
	RawJSON json.RawMessage
}

// Name returns the best available display name.
func (s System) Name() string { return s.Nom }

// Systems calls systemesListe.php and returns every known system.
// Cache this result (e.g. in the SQLite metadata cache, item 6) --
// there is no reason to call it more than roughly once per session.
func (c *Client) Systems(ctx context.Context) ([]System, error) {
	raw, err := c.do(ctx, "systemesListe.php", url.Values{})
	if err != nil {
		return nil, err
	}

	// The response shape for a list endpoint is not independently
	// verified here (same caveat as the package doc comment) -- try
	// the two shapes every list-style ScreenScraper endpoint is
	// reported to use across client source comments: a top-level
	// array, or {"systemes": [...]}. If neither parses, surface the
	// raw bytes in the error so a caller debugging against a real
	// account can see exactly what came back rather than a bare
	// "invalid JSON".
	var direct []json.RawMessage
	if err := json.Unmarshal(raw, &direct); err == nil {
		return parseSystems(direct)
	}
	var wrapped struct {
		Systemes []json.RawMessage `json:"systemes"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Systemes != nil {
		return parseSystems(wrapped.Systemes)
	}
	return nil, fmt.Errorf("screenscraper: systemesListe.php: unrecognized response shape (first 300 bytes: %.300s)", raw)
}

func parseSystems(items []json.RawMessage) ([]System, error) {
	out := make([]System, 0, len(items))
	for _, item := range items {
		var s System
		// Decode into a generic map first so ID (which may arrive as
		// either a JSON number or a numeric string depending on
		// endpoint -- ScreenScraper is documented across clients as
		// inconsistent about this) is handled either way, rather than
		// failing the whole list over one field's type.
		var m map[string]json.RawMessage
		if err := json.Unmarshal(item, &m); err != nil {
			continue
		}
		if idRaw, ok := m["id"]; ok {
			var idStr string
			if err := json.Unmarshal(idRaw, &idStr); err == nil {
				s.ID, _ = strconv.Atoi(strings.TrimSpace(idStr))
			} else {
				var idNum int
				if err := json.Unmarshal(idRaw, &idNum); err == nil {
					s.ID = idNum
				}
			}
		}
		if nomRaw, ok := m["nom"]; ok {
			json.Unmarshal(nomRaw, &s.Nom)
		}
		s.RawJSON = item
		out = append(out, s)
	}
	return out, nil
}

// ResolveSystemID finds a system by case-insensitive substring match
// against its display name. Returns the first match; if
// ScreenScraper's list has more than one plausible match for a query
// (unlikely for specific queries like "Saturn" or "ST-V", but
// possible for something generic), ok is still true for the first
// hit -- callers with a specific need should inspect the full
// Systems() result themselves rather than rely on this always picking
// the intended one.
func ResolveSystemID(systems []System, nameSubstring string) (System, bool) {
	q := strings.ToLower(nameSubstring)
	for _, s := range systems {
		if strings.Contains(strings.ToLower(s.Name()), q) {
			return s, true
		}
	}
	return System{}, false
}

// Ping calls ssinfraInfos.php -- the documented first step ("is the
// API up, and does my developer login work") -- and returns nil on
// success or the classified error otherwise. Intended for a "Test
// Connection" button in the config UI (item 8's ScreenScraper
// credentials panel).
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.do(ctx, "ssinfraInfos.php", url.Values{})
	return err
}

// UserInfo calls ssuserInfos.php ("does my member login work, what
// are my limits"). Same schema-confidence caveat as Systems: RawJSON
// is authoritative, this is intentionally just a thin wrapper.
type UserInfo struct {
	RawJSON json.RawMessage
}

func (c *Client) UserInfo(ctx context.Context) (UserInfo, error) {
	raw, err := c.do(ctx, "ssuserInfos.php", url.Values{})
	if err != nil {
		return UserInfo{}, err
	}
	return UserInfo{RawJSON: raw}, nil
}
