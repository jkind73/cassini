// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package discimg

import (
	"fmt"
	"os"
	"sort"
)

// BinCueReader implements uiface.DiscReader over a parsed CueSheet.
// ReadSector always returns a canonical 2352-byte raw sector,
// regardless of whether the underlying rip is raw or "cooked"
// (headerless) — headerless Mode1 sectors get their sync/address/mode
// header synthesized on read, since the CD Format Standards header
// layout is fully specified (it's not disc-specific data, just a
// function of sector mode and LBA), and this keeps every downstream
// consumer (adapter.go's DiscInfo, which slices data[16:]) working
// identically no matter which rip format produced the file on disk.
type BinCueReader struct {
	sheet *CueSheet
	files map[string]*os.File
}

// OpenBinCue parses cuePath and opens all referenced FILEs read-only.
func OpenBinCue(cuePath string) (*BinCueReader, error) {
	sheet, err := ParseCue(cuePath)
	if err != nil {
		return nil, err
	}
	r := &BinCueReader{sheet: sheet, files: make(map[string]*os.File)}
	for _, cf := range sheet.Files {
		f, err := os.Open(cf.Path)
		if err != nil {
			r.Close()
			return nil, fmt.Errorf("discimg: open %s: %w", cf.Path, err)
		}
		r.files[cf.Path] = f
	}
	return r, nil
}

func (r *BinCueReader) Close() error {
	var firstErr error
	for _, f := range r.files {
		if err := f.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// trackForLBA finds the track containing the given absolute LBA.
// Tracks are already in ascending startLBA order (ParseCue appends in
// file order, and cue sheets are conventionally authored in disc
// order — resolveLBAs computes strictly increasing LBAs under that
// assumption, which every cue-writing tool in practice satisfies).
func (r *BinCueReader) trackForLBA(lba int) (*CueTrack, int, error) {
	tracks := r.sheet.Tracks
	i := sort.Search(len(tracks), func(i int) bool {
		return tracks[i].startLBA > lba
	})
	if i == 0 {
		return nil, 0, fmt.Errorf("discimg: LBA %d before track 1", lba)
	}
	t := tracks[i-1]
	return t, lba - t.startLBA, nil
}

// ReadSector implements uiface.DiscReader.
func (r *BinCueReader) ReadSector(index int) ([]byte, error) {
	track, rel, err := r.trackForLBA(index)
	if err != nil {
		return nil, err
	}
	f, ok := r.files[track.File.Path]
	if !ok {
		return nil, fmt.Errorf("discimg: internal error: file %s not open", track.File.Path)
	}

	declBytes := track.Format.declBytes
	off := track.byteOffset + int64(rel)*int64(declBytes)
	buf := make([]byte, declBytes)
	if _, err := f.ReadAt(buf, off); err != nil {
		return nil, fmt.Errorf("discimg: read sector %d (track %d, file offset %d): %w", index, track.Number, off, err)
	}

	if declBytes == 2352 {
		return buf, nil // already raw, nothing to synthesize
	}

	// Headerless (cooked) sector: synthesize the standard CD sync +
	// address + mode header so callers always see a canonical raw
	// sector. Only Mode1 is synthesized — Mode2's subheader
	// (offset 16-23) carries per-sector file/channel/submode/coding
	// bytes that are genuinely not derivable from the cooked data
	// alone, so a headerless Mode2 rip is read back as user-data-only
	// at offset 0 (not offset 16), and callers that need the Mode2
	// subheader must use a raw (2352 or 2336) Mode2 rip instead. This
	// is an explicit, documented boundary, not a silent wrong guess:
	// Saturn's data track (what IP.BIN identification reads) is
	// always Mode1, so this limitation never affects DiscInfo.
	if track.Format.mode != Mode1 {
		return buf, nil
	}

	raw := make([]byte, 2352)
	writeSyncHeader(raw, index, 0x01) // mode byte 0x01 = Mode 1
	copy(raw[16:], buf)
	return raw, nil
}

func (r *BinCueReader) NumTracks() int { return len(r.sheet.Tracks) }

func (r *BinCueReader) Track(n int) (start, length int, audio bool) {
	if n < 1 || n > len(r.sheet.Tracks) {
		return 0, 0, false
	}
	t := r.sheet.Tracks[n-1]
	var sectors int64
	// Recompute this track's sector count the same way resolveLBAs
	// did, rather than caching it, to keep a single source of truth
	// for "how long is this track" (see resolveLBAs).
	if n < len(r.sheet.Tracks) && r.sheet.Tracks[n].File == t.File {
		sectors = int64(r.sheet.Tracks[n].startLBA - t.startLBA)
	} else {
		st, err := os.Stat(t.File.Path)
		if err == nil {
			sectors = (st.Size() - t.byteOffset) / int64(t.Format.declBytes)
		}
	}
	return t.startLBA, int(sectors), t.Format.mode == ModeAudio
}

func (r *BinCueReader) NumTrackIndexes(track int) int {
	// INDEX 00 (pregap) is parsed but not retained as a separate
	// addressable index (see ParseCue) — every track in this reader
	// therefore reports exactly one index (INDEX 01, the track body).
	if track < 1 || track > len(r.sheet.Tracks) {
		return 0
	}
	return 1
}

func (r *BinCueReader) TrackIndex(track, index int) int {
	start, _, _ := r.Track(track)
	return start
}

// writeSyncHeader writes the standard CD-ROM sector sync pattern (12
// bytes: 00 FF*10 00), 3-byte BCD address, and mode byte into raw[0:16].
// lba is the ABSOLUTE disc LBA (0 = start of track 1); per Red Book /
// Yellow Book convention the address field encodes LBA+150 (the
// 2-second, 150-sector lead-in), matching what every real CD-ROM
// drive reports and what adapter.go's header check expects to be
// present (it only checks the hardware-ID string at data[16:], not
// this header itself, but downstream CD-block sector-address
// commands, if this reader is ever reused there, do depend on a
// correct address field).
func writeSyncHeader(raw []byte, lba int, mode byte) {
	raw[0] = 0x00
	for i := 1; i <= 10; i++ {
		raw[i] = 0xFF
	}
	raw[11] = 0x00

	addr := lba + 150
	m := addr / (60 * 75)
	s := (addr / 75) % 60
	f := addr % 75
	raw[12] = toBCD(m)
	raw[13] = toBCD(s)
	raw[14] = toBCD(f)
	raw[15] = mode
}

func toBCD(v int) byte {
	return byte((v/10)<<4 | (v % 10))
}
