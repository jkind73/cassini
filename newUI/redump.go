// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package discimg

import (
	"fmt"

	"github.com/jkind73/cassini/hashid"
)

// FileHash is one physical file's identity, matching how Redump's own
// .dat files describe a disc: one <rom name="..."> entry per physical
// file the disc's cue sheet references, each with its own
// crc/md5/sha1 — never a hash of a synthesized/concatenated logical
// image, and never a hash of the .cue text itself (cue-sheet
// formatting — line endings, comment style, REM fields — varies
// between tools without the underlying disc being different, so
// hashing the .cue would make identical discs look different).
type FileHash struct {
	Name   string // as referenced in the cue sheet's FILE line, or the .iso's own filename
	Path   string
	Digest hashid.Digest
}

// HashCue hashes every physical file cuePath's FILE entries reference,
// each independently, following Redump convention. A disc is a full
// match against a Redump-style hash list only when every FileHash in
// the returned slice matches an expected entry — this function itself
// makes no matching decision, it only produces the per-file digests
// for a caller (or ScreenScraper's own hash-based lookup, item 5) to
// compare.
func HashCue(cuePath string) ([]FileHash, error) {
	sheet, err := ParseCue(cuePath)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool, len(sheet.Files))
	var out []FileHash
	for _, cf := range sheet.Files {
		if seen[cf.Path] {
			continue // a FILE referenced by name more than once (unusual, but not invalid) is hashed once
		}
		seen[cf.Path] = true

		d, err := hashid.HashFile(cf.Path)
		if err != nil {
			return nil, fmt.Errorf("discimg: %w", err)
		}
		out = append(out, FileHash{Name: filepathBase(cf.Path), Path: cf.Path, Digest: d})
	}
	return out, nil
}

// HashIso hashes a single .iso as one physical file — the only
// sensible interpretation of "Redump convention" for a format that's
// inherently single-file to begin with.
func HashIso(isoPath string) (FileHash, error) {
	d, err := hashid.HashFile(isoPath)
	if err != nil {
		return FileHash{}, fmt.Errorf("discimg: %w", err)
	}
	return FileHash{Name: filepathBase(isoPath), Path: isoPath, Digest: d}, nil
}

func filepathBase(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[i+1:]
		}
	}
	return p
}
