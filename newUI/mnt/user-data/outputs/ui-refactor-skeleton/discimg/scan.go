// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package discimg

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/jkind73/cassini/biosdb"
	"github.com/jkind73/cassini/chdlib"
	"github.com/jkind73/cassini/uiface"
)

// Found is one scanned disc: its Redump-style per-file hashes (for
// external catalog matching — item 5), its IP.BIN-derived identity
// (available immediately, offline, independent of any hash database),
// and enough to open it again later (Path/Kind).
type Found struct {
	Path  string
	Kind  Kind
	Files []FileHash // empty for .chd, which identifies via embedded SHA1 instead — see CHD field
	CHD   *chdlib.Header

	DiscInfo    uiface.DiscInfo
	HasDiscInfo bool // false if IP.BIN didn't parse (not a Saturn disc, corrupt, unsupported image layout, or a non-CD-ROM CHD)
}

type Kind int

const (
	KindCue Kind = iota
	KindIso
	KindMds
	KindChd
)

// discInfoFactory is the minimal surface Scan needs from
// uiface.Factory — just DiscInfo. Satisfied by *adapter.Factory
// unchanged; declared narrowly here so this package doesn't need to
// import adapter (which would be an import cycle risk if adapter ever
// needs disc-scanning helpers later) — the caller passes its own
// *adapter.Factory in.
type discInfoFactory interface {
	DiscInfo(disc uiface.DiscReader) (uiface.DiscInfo, bool)
}

// Scan walks dirs looking for .cue/.iso/.mds/.chd files (case
// insensitive extensions) and, for each, computes Redump-style file
// hashes (or reads the CHD's embedded SHA1) and reads the IP.BIN
// volume header via factory.DiscInfo — the same, unmodified function
// adapter.go already uses at emulation time, so identification here
// can never diverge from what actually boots.
//
// A bare .bin with no accompanying .cue is skipped (not an error):
// plenty of directories legitimately contain track .bin files awaiting
// their .cue, or leftover raw tracks with no disc description at all,
// and guessing a layout for a bin with no cue would be exactly the
// kind of unverifiable assumption this scanner avoids elsewhere.
func Scan(dirs []string, factory discInfoFactory) ([]Found, error) {
	var out []Found

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

			ext := strings.ToLower(filepath.Ext(path))
			switch ext {
			case ".cue":
				f, ferr := scanCue(path, factory)
				if ferr == nil {
					out = append(out, f)
				}
			case ".iso":
				f, ferr := scanIso(path, factory)
				if ferr == nil {
					out = append(out, f)
				}
			case ".mds":
				f, ferr := scanMds(path, factory)
				if ferr == nil {
					out = append(out, f)
				}
			case ".chd":
				f, ferr := scanChd(path, factory)
				if ferr == nil {
					out = append(out, f)
				}
			}
			return nil
		})
		if err != nil {
			return out, err
		}
	}

	return out, nil
}

func scanCue(path string, factory discInfoFactory) (Found, error) {
	files, err := HashCue(path)
	if err != nil {
		return Found{}, err
	}
	r, err := OpenBinCue(path)
	if err != nil {
		return Found{}, err
	}
	defer r.Close()

	info, ok := factory.DiscInfo(r)
	return Found{Path: path, Kind: KindCue, Files: files, DiscInfo: info, HasDiscInfo: ok}, nil
}

func scanIso(path string, factory discInfoFactory) (Found, error) {
	fh, err := HashIso(path)
	if err != nil {
		return Found{}, err
	}
	r, err := OpenIso(path)
	if err != nil {
		return Found{}, err
	}
	defer r.Close()

	info, ok := factory.DiscInfo(r)
	return Found{Path: path, Kind: KindIso, Files: []FileHash{fh}, DiscInfo: info, HasDiscInfo: ok}, nil
}

func scanMds(path string, factory discInfoFactory) (Found, error) {
	r, err := OpenMds(path)
	if err != nil {
		return Found{}, err
	}
	defer r.Close()

	// The .mdf backing an .mds is hashed the same way an .iso is
	// (single physical file) — OpenMds already established it's a
	// single-track image before we get here.
	mdfPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".mdf"
	fh, err := HashIso(mdfPath)
	if err != nil {
		return Found{}, err
	}

	info, ok := factory.DiscInfo(r)
	return Found{Path: path, Kind: KindMds, Files: []FileHash{fh}, DiscInfo: info, HasDiscInfo: ok}, nil
}

func scanChd(path string, factory discInfoFactory) (Found, error) {
	r, err := chdlib.OpenCD(path)
	if err != nil {
		// Not every CHD is a CD-ROM image (hard disk CHDs exist for
		// other MAME-emulated hardware) -- OpenCD reports that
		// distinctly via its "no CD track metadata found" error, and
		// scanning should skip such a file rather than fail the whole
		// directory walk over it. Header-only identification (SHA1)
		// still isn't lost in that case: fall back to a header-only
		// open so the CHD is still recorded, just without disc
		// contents.
		hf, herr := chdlib.Open(path)
		if herr != nil {
			return Found{}, herr
		}
		h := hf.Header()
		hf.Close()
		return Found{Path: path, Kind: KindChd, CHD: &h, HasDiscInfo: false}, nil
	}
	defer r.Close()

	h := r.Header()
	info, ok := factory.DiscInfo(r)
	return Found{Path: path, Kind: KindChd, CHD: &h, DiscInfo: info, HasDiscInfo: ok}, nil
}

// biosRegionFor bridges a disc's uiface.Region into biosdb.Region for
// BIOS auto-assignment. RegionAsiaNTSC maps to RegionJapan: Sega's own
// area-code scheme groups "T" (Asia NTSC) discs as Japan-hardware-
// compatible (ip_bin.md's area symbols are a compatibility list, and
// Asia-NTSC territories were serviced with Japan-region hardware/BIOS
// in practice), so this is the correct fallback rather than an
// arbitrary default.
func biosRegionFor(r uiface.Region) biosdb.Region {
	switch r {
	case uiface.RegionJapan, uiface.RegionAsiaNTSC:
		return biosdb.RegionJapan
	case uiface.RegionUSA:
		return biosdb.RegionUSA
	case uiface.RegionEurope:
		return biosdb.RegionEurope
	default:
		return biosdb.RegionUnknown
	}
}

// SelectBIOSForDisc is the convenience glue between a Scan result and
// biosdb.SelectForRegion: given a Found disc (with HasDiscInfo true)
// and the BIOS collection from biosdb.Scan, returns the auto-assigned
// BIOS per biosdb's documented fallback chain.
func SelectBIOSForDisc(disc Found, sys biosdb.System, available []biosdb.Found) (biosdb.Found, bool, error) {
	if !disc.HasDiscInfo {
		return biosdb.Found{}, false, fmt.Errorf("discimg: %s: no IP.BIN volume header was read (not a Saturn disc, or unsupported image layout)", disc.Path)
	}
	f, ok := biosdb.SelectForRegion(available, sys, biosRegionFor(disc.DiscInfo.Region))
	return f, ok, nil
}
