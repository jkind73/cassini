// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

// Package discimg reads Saturn disc images (.bin/.cue, .iso, .mds) for
// scanning purposes: per-file hashing (following Redump convention —
// see redump.go) and IP.BIN volume-header identification (via the
// existing, unmodified uiface.Factory.DiscInfo — see scan.go). This
// package does not implement CD-block emulation; it is a lightweight,
// read-only sector accessor for the scanner/browser, separate from
// whatever full disc-reading implementation the core CD block uses
// during actual emulation.
package discimg

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// TrackMode is a cue sheet TRACK type. Saturn's data track is always
// MODE1 (see adapter.go's ipHardwareID check assuming a 16-byte
// Mode-1 header); other modes are supported here for completeness of
// raw sector access (e.g. CD-DA audio tracks on multi-track discs)
// but are not exercised by IP.BIN identification.
type TrackMode int

const (
	ModeAudio TrackMode = iota // "AUDIO" — 2352 bytes/sector, no header
	Mode1                       // "MODE1/2352" or "MODE1/2048"
	Mode2                       // "MODE2/2352" or "MODE2/2336" or "MODE2/2048"
)

// rawSectorSize is the on-disc size for a mode/declared-size pair.
// "Cooked" (headerless) rips store fewer bytes per sector than a raw
// dump; ReadSector synthesizes the missing header for Mode1 cooked
// sectors so callers always see a canonical raw-shaped sector — see
// bincue.go.
type sectorFormat struct {
	mode       TrackMode
	declBytes  int // bytes per sector as declared in the cue (what's actually on disk)
	rawEquiv   int // 2352 for audio/mode1/mode2 raw forms; used to decide if synthesis is needed
}

func parseTrackType(s string) (sectorFormat, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "AUDIO":
		return sectorFormat{ModeAudio, 2352, 2352}, nil
	case "MODE1/2352":
		return sectorFormat{Mode1, 2352, 2352}, nil
	case "MODE1/2048":
		return sectorFormat{Mode1, 2048, 2352}, nil
	case "MODE2/2352":
		return sectorFormat{Mode2, 2352, 2352}, nil
	case "MODE2/2336":
		return sectorFormat{Mode2, 2336, 2352}, nil
	case "MODE2/2048":
		return sectorFormat{Mode2, 2048, 2352}, nil
	default:
		return sectorFormat{}, fmt.Errorf("discimg: unrecognized cue TRACK type %q", s)
	}
}

// CueFile is one FILE block: the physical file it points to, and the
// tracks stored within it, in file order.
type CueFile struct {
	Path   string // resolved absolute path (cue's FILE line, relative to the .cue's own directory)
	Tracks []*CueTrack
}

// CueTrack is one TRACK block.
type CueTrack struct {
	Number int
	Format sectorFormat
	File   *CueFile

	// byteOffset is this track's start offset within File.Path,
	// computed from preceding tracks' extents in the same file
	// (INDEX 01, ignoring the INDEX 00 pregap for LBA purposes — see
	// startLBA doc comment).
	byteOffset int64

	// startLBA is this track's INDEX 01 start, in absolute disc LBA
	// (LBA 0 = start of track 1). Saturn/Sega CD convention doesn't
	// use the Red Book "00:02:00" 2-second lead-in offset for data
	// LBA addressing the way audio-CD players do — CD block sector
	// addressing is 0-based from the start of track 1 — so no +150
	// bias is applied here.
	startLBA int
}

// CueSheet is a fully parsed and resolved .cue file.
type CueSheet struct {
	Files  []*CueFile
	Tracks []*CueTrack // flattened, in track-number order
}

// ParseCue reads and resolves a .cue file. FILE paths are resolved
// relative to the .cue's own directory, matching every cue-sheet tool
// and burning application's convention.
func ParseCue(path string) (*CueSheet, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("discimg: open %s: %w", path, err)
	}
	defer f.Close()

	dir := filepath.Dir(path)
	sheet := &CueSheet{}

	var curFile *CueFile
	var curTrack *CueTrack

	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		fields := splitCueLine(line)
		if len(fields) == 0 {
			continue
		}

		switch strings.ToUpper(fields[0]) {
		case "FILE":
			if len(fields) < 2 {
				return nil, fmt.Errorf("discimg: %s:%d: FILE missing name", path, lineNo)
			}
			name := fields[1]
			fp := name
			if !filepath.IsAbs(fp) {
				fp = filepath.Join(dir, name)
			}
			if _, err := os.Stat(fp); err != nil {
				return nil, fmt.Errorf("discimg: %s:%d: referenced file %q: %w", path, lineNo, name, err)
			}
			curFile = &CueFile{Path: fp}
			sheet.Files = append(sheet.Files, curFile)

		case "TRACK":
			if curFile == nil {
				return nil, fmt.Errorf("discimg: %s:%d: TRACK before any FILE", path, lineNo)
			}
			if len(fields) < 3 {
				return nil, fmt.Errorf("discimg: %s:%d: malformed TRACK line", path, lineNo)
			}
			num, err := strconv.Atoi(fields[1])
			if err != nil {
				return nil, fmt.Errorf("discimg: %s:%d: bad track number %q: %w", path, lineNo, fields[1], err)
			}
			fmtType, err := parseTrackType(fields[2])
			if err != nil {
				return nil, fmt.Errorf("discimg: %s:%d: %w", path, lineNo, err)
			}
			curTrack = &CueTrack{Number: num, Format: fmtType, File: curFile}
			curFile.Tracks = append(curFile.Tracks, curTrack)
			sheet.Tracks = append(sheet.Tracks, curTrack)

		case "INDEX":
			if curTrack == nil {
				return nil, fmt.Errorf("discimg: %s:%d: INDEX before any TRACK", path, lineNo)
			}
			if len(fields) < 3 {
				return nil, fmt.Errorf("discimg: %s:%d: malformed INDEX line", path, lineNo)
			}
			idxNum, err := strconv.Atoi(fields[1])
			if err != nil {
				return nil, fmt.Errorf("discimg: %s:%d: bad index number %q: %w", path, lineNo, fields[1], err)
			}
			// Only INDEX 01 (the actual track start) matters for LBA
			// computation; INDEX 00 (pregap) is parsed for validity
			// but intentionally not used to offset addressing — see
			// CueTrack.startLBA's doc comment.
			if idxNum != 1 {
				continue
			}
			m, s, fr, err := parseMSF(fields[2])
			if err != nil {
				return nil, fmt.Errorf("discimg: %s:%d: bad INDEX MSF %q: %w", path, lineNo, fields[2], err)
			}
			curTrack.byteOffset = msfToSectors(m, s, fr) * int64(curTrack.Format.declBytes)

		default:
			// PREGAP, POSTGAP, FLAGS, CATALOG, REM, etc. — not needed
			// for identification/hashing; explicitly ignored rather
			// than erroring, since a cue sheet with extra metadata
			// fields is still a perfectly valid cue sheet.
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("discimg: read %s: %w", path, err)
	}

	if err := resolveLBAs(sheet); err != nil {
		return nil, err
	}
	return sheet, nil
}

// resolveLBAs assigns each track's absolute startLBA. Tracks within
// the same FILE are contiguous (this track's LBA = previous track's
// LBA + previous track's sector count up to this track's byteOffset);
// the first track of a new FILE continues the absolute LBA sequence
// from where the previous FILE's last track ended, which is standard
// multi-FILE cue sheet semantics (each FILE is a contiguous
// continuation of the disc, not a separate addressing space).
func resolveLBAs(sheet *CueSheet) error {
	lba := 0
	for i, t := range sheet.Tracks {
		t.startLBA = lba
		// Determine this track's sector count to advance lba for the
		// next track: either up to the next track's byteOffset within
		// the same file, or (last track in file / last track overall)
		// the remainder of the file's size.
		var trackBytes int64
		st, err := os.Stat(t.File.Path)
		if err != nil {
			return fmt.Errorf("discimg: stat %s: %w", t.File.Path, err)
		}
		fileSize := st.Size()

		if i+1 < len(sheet.Tracks) && sheet.Tracks[i+1].File == t.File {
			trackBytes = sheet.Tracks[i+1].byteOffset - t.byteOffset
		} else {
			trackBytes = fileSize - t.byteOffset
		}
		if trackBytes < 0 {
			return fmt.Errorf("discimg: track %d: negative computed length (malformed cue or truncated file)", t.Number)
		}
		if t.Format.declBytes == 0 {
			return fmt.Errorf("discimg: track %d: zero sector size", t.Number)
		}
		lba += int(trackBytes / int64(t.Format.declBytes))
	}
	return nil
}

// splitCueLine tokenizes a cue-sheet line, honoring double-quoted
// strings (cue's convention for filenames containing spaces).
func splitCueLine(line string) []string {
	var out []string
	var cur strings.Builder
	inQuotes := false
	for _, r := range line {
		switch {
		case r == '"':
			inQuotes = !inQuotes
		case r == ' ' && !inQuotes:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// parseMSF parses a cue-sheet "MM:SS:FF" timestamp (frames, 75/sec).
func parseMSF(s string) (m, sec, fr int, err error) {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, 0, 0, fmt.Errorf("expected MM:SS:FF, got %q", s)
	}
	vals := make([]int, 3)
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("non-numeric MSF component %q: %w", p, err)
		}
		vals[i] = v
	}
	return vals[0], vals[1], vals[2], nil
}

func msfToSectors(m, s, fr int) int64 {
	return int64(m)*60*75 + int64(s)*75 + int64(fr)
}
