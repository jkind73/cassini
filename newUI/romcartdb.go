// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

// Package romcartdb identifies the two Sega Saturn ROM cartridges
// (King of Fighters '95 and Ultraman: Hikari no Kyojin Densetsu --
// the only titles ever released using a game-specific ROM cart rather
// than the general-purpose DRAM expansion cart; see biosdb-adjacent
// bus.go changes for how these differ in addressing).
//
// PROVENANCE: entries are sourced from Mednafen/Beetle Saturn's own
// cart filenames and MD5 hashes, quoted verbatim in
// github.com/StrikerX3/Ymir/issues/71 (Ymir being cassini's own
// reference emulator for hardware behavior cross-checks). Product
// numbers from github.com/StrikerX3/Ymir/issues/98, itself citing
// Mednafen's cart database.
//
// MD5, not CRC32+SHA1: this deliberately does not follow the MAME
// convention the rest of cassini's hash identification uses (biosdb,
// hashid) -- I do not have CRC32/SHA1 for these two specific dumps
// verified from any source, and computing them from an MD5 is not
// possible (independent hash functions). Storing what's actually
// confirmed is more honest than converting to a "consistent" format
// I'd have to fabricate values for.
package romcartdb

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Entry is one known ROM cartridge.
type Entry struct {
	ProductNumber string // the game's IP.BIN product number that requires this cart
	Label         string
	CartFilename  string // canonical filename, matching Mednafen/Beetle Saturn's own convention
	MD5           string // hex, lowercase, 32 chars
	Source        string
}

// Known is the full (currently two-entry) identification table.
var Known = []Entry{
	{
		ProductNumber: "T-3101G", // Japan; MK-81088 is the Europe SKU of the same game/cart, see below
		Label:         "King of Fighters '95, The (Japan)",
		CartFilename:  "mpr-18811-mx.ic1",
		MD5:           "255113ba943c92a54facd25a10fd780c",
		Source:        "Mednafen/Beetle Saturn filename+MD5 via github.com/StrikerX3/Ymir issue #71; product number via issue #98 (citing Mednafen's cart database)",
	},
	{
		ProductNumber: "MK-81088", // Europe SKU of King of Fighters '95 -- same cart/ROM as the Japan release
		Label:         "King of Fighters '95, The (Europe)",
		CartFilename:  "mpr-18811-mx.ic1",
		MD5:           "255113ba943c92a54facd25a10fd780c",
		Source:        "Mednafen/Beetle Saturn filename+MD5 via github.com/StrikerX3/Ymir issue #71; product number via issue #98 (citing Mednafen's cart database)",
	},
	{
		ProductNumber: "T-13308G",
		Label:         "Ultraman: Hikari no Kyojin Densetsu (Japan)",
		CartFilename:  "mpr-19367-mx.ic1",
		MD5:           "1cd19988d1d72a3e7caa0b73234c96b4",
		Source:        "Mednafen/Beetle Saturn filename+MD5 via github.com/StrikerX3/Ymir issue #71; product number via issue #98 (citing Mednafen's cart database)",
	},
}

// RequiresROMCart reports whether productNumber is one of the two
// titles needing a ROM cart at all, without needing a scanned Found
// list -- useful for a caller deciding whether to even bother
// searching cart directories.
func RequiresROMCart(productNumber string) (Entry, bool) {
	for _, e := range Known {
		if e.ProductNumber == productNumber {
			return e, true
		}
	}
	return Entry{}, false
}

// Found is one identified cart file located on disk.
type Found struct {
	Entry Entry
	Path  string
}

// Scan walks dirs looking for files matching Known by MD5.
func Scan(dirs []string) ([]Found, error) {
	var found []Found
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			sum, herr := hashMD5(path)
			if herr != nil {
				return nil // unreadable file, skip
			}
			for _, e := range Known {
				if strings.EqualFold(e.MD5, sum) {
					found = append(found, Found{Entry: e, Path: path})
				}
			}
			return nil
		})
		if err != nil {
			return found, err
		}
	}
	return found, nil
}

// SelectForProduct returns the cart file matching productNumber from
// a Scan result, if one was found. Unlike biosdb's region fallback
// chain, this is a direct, exact match -- each of these two games
// needs its own specific cart, there is no "close enough" substitute.
func SelectForProduct(available []Found, productNumber string) (Found, bool) {
	for _, f := range available {
		if f.Entry.ProductNumber == productNumber {
			return f, true
		}
	}
	return Found{}, false
}

func hashMD5(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("romcartdb: open %s: %w", path, err)
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("romcartdb: hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
