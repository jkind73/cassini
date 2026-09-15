// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package discimg

import (
	"fmt"
	"os"
)

// IsoReader implements uiface.DiscReader over a single-track .iso.
// Sector size (2048 cooked vs 2352 raw) is auto-detected from file
// size: real Saturn/CD data discs have sector counts in the low tens
// of thousands, so testing which of the two candidate sizes divides
// the file size evenly is reliable in practice (a coincidental exact
// match at both sizes would require the file size to be divisible by
// lcm(2048,2352)=401,408 bytes exactly while also being a plausible
// disc size — not seen in real dumps). If neither divides evenly, the
// file is rejected rather than guessed at.
type IsoReader struct {
	f       *os.File
	secSize int
}

func OpenIso(path string) (*IsoReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("discimg: open %s: %w", path, err)
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("discimg: stat %s: %w", path, err)
	}
	size := st.Size()

	secSize := 0
	switch {
	case size%2352 == 0:
		secSize = 2352
	case size%2048 == 0:
		secSize = 2048
	default:
		f.Close()
		return nil, fmt.Errorf("discimg: %s: size %d is not a whole multiple of 2048 or 2352 bytes; not a recognized ISO sector layout", path, size)
	}
	return &IsoReader{f: f, secSize: secSize}, nil
}

func (r *IsoReader) Close() error { return r.f.Close() }

func (r *IsoReader) ReadSector(index int) ([]byte, error) {
	buf := make([]byte, r.secSize)
	off := int64(index) * int64(r.secSize)
	if _, err := r.f.ReadAt(buf, off); err != nil {
		return nil, fmt.Errorf("discimg: read sector %d: %w", index, err)
	}
	if r.secSize == 2352 {
		return buf, nil
	}
	raw := make([]byte, 2352)
	writeSyncHeader(raw, index, 0x01) // ISOs are always Mode1 data
	copy(raw[16:], buf)
	return raw, nil
}

func (r *IsoReader) NumTracks() int { return 1 }

func (r *IsoReader) Track(n int) (start, length int, audio bool) {
	if n != 1 {
		return 0, 0, false
	}
	st, err := r.f.Stat()
	if err != nil {
		return 0, 0, false
	}
	return 0, int(st.Size() / int64(r.secSize)), false
}

func (r *IsoReader) NumTrackIndexes(track int) int {
	if track != 1 {
		return 0
	}
	return 1
}

func (r *IsoReader) TrackIndex(track, index int) int { return 0 }
