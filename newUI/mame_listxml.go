// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package cache

import (
	"encoding/xml"
	"fmt"
	"os"
	"strings"
)

// mameListXML mirrors the subset of `mame -listxml` (or a
// driver-scoped invocation like `mame stv -listxml`) output cassini
// needs. This is MAME's own primary machine-readable interface --
// every major frontend (Attract-Mode, HyperSpin, Pegasus) parses this
// exact schema, and it has been stable in shape (root <mame>, child
// <machine> elements with <description>/<year>/<manufacturer> and
// <rom> children carrying crc/sha1) across MAME's history. Extra
// fields MAME's real output includes (input, driver status, DIP
// switches, softwarelist references, etc) are simply ignored by
// encoding/xml's default behavior of skipping unmapped elements --
// they don't need to be declared here to parse correctly.
type mameListXML struct {
	Machines []mameMachine `xml:"machine"`
}

type mameMachine struct {
	Name         string    `xml:"name,attr"`
	Description  string    `xml:"description"`
	Year         string    `xml:"year"`
	Manufacturer string    `xml:"manufacturer"`
	ROMs         []mameROM `xml:"rom"`
	// Cloneof identifies a clone/variant set; not currently used by
	// the resolver (an ST-V region variant's ROMs still hash-match
	// their own <machine> entry directly), kept for a future pass
	// that might want to prefer a parent set's title for a clone.
	Cloneof string `xml:"cloneof,attr"`
}

type mameROM struct {
	Name string `xml:"name,attr"`
	Size int64  `xml:"size,attr"`
	CRC  string `xml:"crc,attr"`
	SHA1 string `xml:"sha1,attr"`
}

// ImportMameListXML parses a `mame -listxml` output file and loads it
// into mame_entries/mame_entry_roms, replacing any previous import.
// Pass a driver-scoped invocation's output (e.g. `mame stv -listxml >
// stv.xml`) to import only ST-V, or a full listxml for broader
// coverage -- either way, only <machine> elements are read, so
// scoping is entirely the caller's choice of what to feed in.
func (c *Cache) ImportMameListXML(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("cache: open MAME listxml %s: %w", path, err)
	}
	defer f.Close()

	var doc mameListXML
	if err := xml.NewDecoder(f).Decode(&doc); err != nil {
		return 0, fmt.Errorf("cache: parse MAME listxml %s: %w", path, err)
	}

	tx, err := c.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("cache: import MAME listxml: begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM mame_entry_roms`); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`DELETE FROM mame_entries`); err != nil {
		return 0, err
	}

	for _, m := range doc.Machines {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO mame_entries (shortname, description, year, manufacturer) VALUES (?, ?, ?, ?)`,
			m.Name, m.Description, m.Year, m.Manufacturer); err != nil {
			return 0, fmt.Errorf("cache: import MAME listxml: insert entry %s: %w", m.Name, err)
		}
		for _, r := range m.ROMs {
			if _, err := tx.Exec(`INSERT INTO mame_entry_roms (shortname, crc32, sha1, size) VALUES (?, ?, ?, ?)`,
				m.Name, strings.ToLower(r.CRC), strings.ToLower(r.SHA1), r.Size); err != nil {
				return 0, fmt.Errorf("cache: import MAME listxml: insert rom for %s: %w", m.Name, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("cache: import MAME listxml: commit: %w", err)
	}
	return len(doc.Machines), nil
}

// MameMatch is a resolved MAME listxml lookup result.
type MameMatch struct {
	Shortname    string
	Description  string
	Year         string
	Manufacturer string
}

// LookupMameByHash finds a machine with a rom whose sha1 or crc32
// matches. Since one ROM chip's hash can appear in multiple regional
// variants' <machine> entries only if they're bit-identical (rare for
// region-coded BIOS/program ROMs, but region-shared graphics/sound
// ROMs are common), this returns the first match; callers wanting
// every candidate machine should query mame_entry_roms directly.
func (c *Cache) LookupMameByHash(sha1, crc32 string) (MameMatch, bool, error) {
	m, ok, err := c.lookupMame("sha1", strings.ToLower(sha1))
	if err != nil || ok {
		return m, ok, err
	}
	return c.lookupMame("crc32", strings.ToLower(crc32))
}

func (c *Cache) lookupMame(column, value string) (MameMatch, bool, error) {
	if value == "" {
		return MameMatch{}, false, nil
	}
	var m MameMatch
	err := c.db.QueryRow(`
		SELECT e.shortname, e.description, e.year, e.manufacturer
		FROM mame_entry_roms r JOIN mame_entries e ON e.shortname = r.shortname
		WHERE r.`+column+` = ? LIMIT 1
	`, value).Scan(&m.Shortname, &m.Description, &m.Year, &m.Manufacturer)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return MameMatch{}, false, nil
		}
		return MameMatch{}, false, fmt.Errorf("cache: lookup MAME by %s: %w", column, err)
	}
	return m, true, nil
}
