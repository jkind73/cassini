// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package biosdb

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/jkind73/cassini/hashid"
)

// Found is one identified BIOS file located on disk.
type Found struct {
	Entry Entry
	Path  string // filesystem path to the file (or to the .zip containing it)
	Inzip string // non-empty if Entry was found inside a zip archive, e.g. "epr-20091.ic8" within stvbios.zip
}

// Scan walks dirs (non-recursive is deliberately not offered — BIOS
// collections are routinely organized into per-system subfolders, and
// silently missing a nested file is a worse failure mode than the
// small extra cost of walking a few directory trees) looking for
// files whose hash matches Known. Both loose files and zip archives
// (e.g. stvbios.zip, which packs 15 IC8 dumps together) are scanned;
// a zip's own file extension is not required to be exactly ".zip"
// (case-insensitive check) since some BIOS packs ship as .ZIP.
//
// Unrecognized files are skipped, not errored — a BIOS directory
// legitimately containing unrelated files (readmes, other systems'
// BIOS) must not abort the whole scan.
func Scan(dirs []string) ([]Found, error) {
	var found []Found

	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				// A single unreadable file/dir (permissions, broken
				// symlink) shouldn't abort scanning the rest of the
				// tree — record nothing for it and continue.
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}

			if strings.EqualFold(filepath.Ext(path), ".zip") {
				entries, zerr := hashid.HashZipEntries(path)
				if zerr != nil {
					// Corrupt/unreadable zip: skip it, don't fail the
					// whole directory scan over one bad archive.
					return nil
				}
				for _, ze := range entries {
					if e, ok := Identify(ze.Digest.CRC32, ze.Digest.SHA1); ok {
						found = append(found, Found{Entry: e, Path: path, Inzip: ze.Name})
					}
				}
				return nil
			}

			d1, herr := hashid.HashFile(path)
			if herr != nil {
				return nil // unreadable file, skip
			}
			if e, ok := Identify(d1.CRC32, d1.SHA1); ok {
				found = append(found, Found{Entry: e, Path: path})
			}
			return nil
		})
		if err != nil {
			return found, err
		}
	}

	return found, nil
}

// SelectForRegion implements the auto-assignment fallback chain:
//
//  1. An exact region match that is also marked Recommended.
//  2. Any exact region match.
//  3. For SystemSaturnConsole only: RegionOverseas satisfies both
//     RegionUSA and RegionEurope requests, since one BIOS image
//     legitimately covers both (matches MAME's own "Overseas" naming
//     and MiSTer's documented boot.rom convention) — checked
//     Recommended-first, then any.
//  4. Any available entry for the requested System at all, so a
//     collection missing the exact region still boots something
//     rather than refusing outright — the caller is expected to
//     surface this as a "wrong region BIOS in use" notice, not treat
//     it as silently equivalent to step 1/2.
//
// Returns ok=false only if no BIOS for the requested System was found
// at all.
func SelectForRegion(available []Found, sys System, region Region) (Found, bool) {
	pick := func(pred func(Found) bool) (Found, bool) {
		var best Found
		haveBest := false
		for _, f := range available {
			if f.Entry.System != sys || !pred(f) {
				continue
			}
			if !haveBest || (f.Entry.Recommended && !best.Entry.Recommended) {
				best, haveBest = f, true
			}
		}
		return best, haveBest
	}

	// Step 1+2 combined: pick() itself already prefers Recommended
	// within matches via the tie-break above.
	if f, ok := pick(func(f Found) bool { return f.Entry.Region == region }); ok {
		return f, true
	}

	// Step 3: Overseas fallback, Saturn console only.
	if sys == SystemSaturnConsole && (region == RegionUSA || region == RegionEurope) {
		if f, ok := pick(func(f Found) bool { return f.Entry.Region == RegionOverseas }); ok {
			return f, true
		}
	}

	// Step 4: anything for this system at all.
	if f, ok := pick(func(Found) bool { return true }); ok {
		return f, true
	}

	return Found{}, false
}
