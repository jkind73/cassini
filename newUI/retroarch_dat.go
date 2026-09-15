// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package cache

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ClrMamePro DAT grammar, generically parsed:
//
//	game (
//		name "Display Name (Region)"
//		description "..."
//		serial "T-9904H"          <- present on some entries, not guaranteed on all
//		rom ( name "track01.bin" size 12345 crc abcd1234 md5 ... sha1 ... )
//		rom ( name "track02.bin" size 67890 crc ... )
//	)
//
// This is a decades-old, standardized format (predates libretro --
// used identically by MAME's own DAT tooling, RomCenter-compatible
// tools, ClrMamePro itself) and libretro-database's redump-sourced
// DATs (metadat/redump/Sega - Saturn.dat) are built in this format
// per the repository's own documented structure. I could not fetch
// the raw file bytes in the environment this was written in (GitHub's
// robots.txt blocks automated raw-file access, and I don't have a
// pre-authorized mirror URL), so rather than hardcode field names I'm
// not 100% certain are present on every entry (specifically "serial"),
// this parser is intentionally generic: it captures every key/value
// pair it encounters into a map and only reads out the keys it
// recognizes, so an entry missing "serial" (or having some other DAT's
// extra fields this parser has never seen) parses correctly rather
// than erroring.
//
// The reliable, load-bearing match key is ROM hash (crc/sha1/md5),
// which is unambiguously the DAT format's actual designed purpose and
// exactly how RetroArch's own database scanner works -- see
// resolver.go for why product number/serial is treated as a bonus
// field, not the primary lookup mechanism, for this fallback tier.

// ImportRetroArchDAT parses a ClrMamePro-format DAT file and loads its
// entries into retroarch_dat_entries/retroarch_dat_roms, replacing any
// previous import (a DAT import is a full refresh, not incremental --
// DAT files are small enough, and infrequent enough to update, that
// there's no benefit to incremental diffing here).
func (c *Cache) ImportRetroArchDAT(path string) (int, error) {
	entries, err := parseClrMamePro(path)
	if err != nil {
		return 0, err
	}

	tx, err := c.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("cache: import retroarch dat: begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM retroarch_dat_roms`); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`DELETE FROM retroarch_dat_entries`); err != nil {
		return 0, err
	}

	for _, e := range entries {
		res, err := tx.Exec(`INSERT INTO retroarch_dat_entries (name, description, serial) VALUES (?, ?, ?)`,
			e.fields["name"], e.fields["description"], e.fields["serial"])
		if err != nil {
			return 0, fmt.Errorf("cache: import retroarch dat: insert entry: %w", err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			return 0, err
		}
		for _, r := range e.roms {
			var size int64
			if s, ok := r["size"]; ok {
				size, _ = strconv.ParseInt(s, 10, 64)
			}
			if _, err := tx.Exec(`INSERT INTO retroarch_dat_roms (entry_id, crc32, sha1, md5, size) VALUES (?, ?, ?, ?, ?)`,
				id, strings.ToLower(r["crc"]), strings.ToLower(r["sha1"]), strings.ToLower(r["md5"]), size); err != nil {
				return 0, fmt.Errorf("cache: import retroarch dat: insert rom: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("cache: import retroarch dat: commit: %w", err)
	}
	return len(entries), nil
}

// RetroArchMatch is a resolved DAT lookup result.
type RetroArchMatch struct {
	Name        string
	Description string
	Serial      string // may be empty -- not every DAT entry has one, see the package-level doc comment
}

// LookupRetroArchByHash finds a DAT entry with a rom whose sha1 or
// crc32 matches (sha1 checked first, being the stronger identifier;
// falls back to crc32 for callers/DATs where only that's available).
func (c *Cache) LookupRetroArchByHash(sha1, crc32 string) (RetroArchMatch, bool, error) {
	m, ok, err := c.lookupRetroArch("sha1", strings.ToLower(sha1))
	if err != nil || ok {
		return m, ok, err
	}
	return c.lookupRetroArch("crc32", strings.ToLower(crc32))
}

func (c *Cache) lookupRetroArch(column, value string) (RetroArchMatch, bool, error) {
	if value == "" {
		return RetroArchMatch{}, false, nil
	}
	var m RetroArchMatch
	err := c.db.QueryRow(`
		SELECT e.name, e.description, e.serial
		FROM retroarch_dat_roms r JOIN retroarch_dat_entries e ON e.id = r.entry_id
		WHERE r.`+column+` = ? LIMIT 1
	`, value).Scan(&m.Name, &m.Description, &m.Serial)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return RetroArchMatch{}, false, nil
		}
		return RetroArchMatch{}, false, fmt.Errorf("cache: lookup retroarch by %s: %w", column, err)
	}
	return m, true, nil
}

// --- generic ClrMamePro grammar parser ---

type cmpEntry struct {
	fields map[string]string
	roms   []map[string]string
}

// parseClrMamePro tokenizes and parses the format's actual grammar:
// bare identifiers, "quoted strings" (which may contain spaces), and
// nested ( ... ) blocks. This is a real recursive-descent parser
// against the documented grammar, not a line-by-line regex scraper --
// DAT files routinely have values spanning unusual characters, and a
// proper tokenizer handles that correctly where a regex approach
// would silently mis-split on some entries.
func parseClrMamePro(path string) ([]cmpEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cache: open DAT %s: %w", path, err)
	}
	defer f.Close()

	toks, err := tokenizeCMP(f)
	if err != nil {
		return nil, fmt.Errorf("cache: tokenize DAT %s: %w", path, err)
	}

	var entries []cmpEntry
	i := 0
	for i < len(toks) {
		if toks[i] == "game" || toks[i] == "resource" {
			e, next, err := parseCMPGameBlock(toks, i)
			if err != nil {
				return nil, fmt.Errorf("cache: parse DAT %s: %w", path, err)
			}
			entries = append(entries, e)
			i = next
			continue
		}
		i++
	}
	return entries, nil
}

// tokenizeCMP splits the file into tokens: "(", ")", bare words, and
// quoted strings (quotes stripped, contents kept verbatim including
// internal spaces).
func tokenizeCMP(f *os.File) ([]string, error) {
	var toks []string
	r := bufio.NewReader(f)
	var cur strings.Builder
	inQuotes := false

	flush := func() {
		if cur.Len() > 0 {
			toks = append(toks, cur.String())
			cur.Reset()
		}
	}

	for {
		ch, _, err := r.ReadRune()
		if err != nil {
			break
		}
		switch {
		case ch == '"':
			if inQuotes {
				toks = append(toks, cur.String())
				cur.Reset()
				inQuotes = false
			} else {
				flush()
				inQuotes = true
			}
		case inQuotes:
			cur.WriteRune(ch)
		case ch == '(' || ch == ')':
			flush()
			toks = append(toks, string(ch))
		case ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n':
			flush()
		default:
			cur.WriteRune(ch)
		}
	}
	flush()
	return toks, nil
}

// parseCMPGameBlock parses one `game ( key "value" ... rom (...) ... )`
// block starting at toks[start] (which must be "game"/"resource"),
// returning the parsed entry and the index just past its closing ")".
func parseCMPGameBlock(toks []string, start int) (cmpEntry, int, error) {
	e := cmpEntry{fields: map[string]string{}}
	i := start + 1
	if i >= len(toks) || toks[i] != "(" {
		return e, i, fmt.Errorf("expected '(' after %q at token %d", toks[start], start)
	}
	i++

	for i < len(toks) {
		switch toks[i] {
		case ")":
			return e, i + 1, nil
		case "rom":
			rom, next, err := parseCMPRomBlock(toks, i)
			if err != nil {
				return e, i, err
			}
			e.roms = append(e.roms, rom)
			i = next
		default:
			key := toks[i]
			if i+1 >= len(toks) {
				return e, i, fmt.Errorf("key %q at token %d has no value", key, i)
			}
			e.fields[key] = toks[i+1]
			i += 2
		}
	}
	return e, i, fmt.Errorf("unterminated game block starting at token %d", start)
}

func parseCMPRomBlock(toks []string, start int) (map[string]string, int, error) {
	rom := map[string]string{}
	i := start + 1
	if i >= len(toks) || toks[i] != "(" {
		return rom, i, fmt.Errorf("expected '(' after 'rom' at token %d", start)
	}
	i++
	for i < len(toks) {
		if toks[i] == ")" {
			return rom, i + 1, nil
		}
		key := toks[i]
		if i+1 >= len(toks) {
			return rom, i, fmt.Errorf("rom key %q at token %d has no value", key, i)
		}
		rom[key] = toks[i+1]
		i += 2
	}
	return rom, i, fmt.Errorf("unterminated rom block starting at token %d", start)
}
