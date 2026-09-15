// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package chdlib

import "fmt"

// CDReader implements uiface.DiscReader over a CHD CD-ROM image,
// reading real decompressed sector data via libchdr (chd_read) --
// no synthesized headers, no guessed layout: CHDR_WANT_RAW_DATA_SECTOR
// means libchdr itself reconstructs the full raw 2352-byte sector
// during decompression, for every track type.
type CDReader struct {
	f      *File
	tracks []TrackInfo

	framesPerHunk int
	hunkCache     []byte
	hunkCacheIdx  int
	hunkCacheOK   bool
}

// OpenCD opens path as a CD-ROM CHD and reads its track table.
func OpenCD(path string) (*CDReader, error) {
	f, err := Open(path)
	if err != nil {
		return nil, err
	}
	tracks, err := readTrackTable(f)
	if err != nil {
		f.Close()
		return nil, err
	}
	hb := f.Header().HunkBytes
	if hb == 0 || hb%cdFrameSize != 0 {
		f.Close()
		return nil, fmt.Errorf("chdlib: %s: hunk size %d is not a whole multiple of the CD frame size %d -- not laid out the way a CD CHD is expected to be", path, hb, cdFrameSize)
	}
	return &CDReader{f: f, tracks: tracks, framesPerHunk: int(hb) / cdFrameSize, hunkCacheIdx: -1}, nil
}

func (r *CDReader) Close() error { return r.f.Close() }

// Header returns the underlying CHD's header (SHA1/version/etc).
func (r *CDReader) Header() Header { return r.f.Header() }

// trackForFrame finds the track containing the given absolute
// CHD-frame index.
func (r *CDReader) trackForFrame(frame int) (*TrackInfo, error) {
	for i := range r.tracks {
		t := &r.tracks[i]
		if frame >= t.startFrame && frame < t.startFrame+t.Frames {
			return t, nil
		}
	}
	return nil, fmt.Errorf("chdlib: frame %d is past the end of the last track", frame)
}

// ReadSector implements uiface.DiscReader. index is treated as an
// absolute CHD-frame index -- for track 1 (the only track
// adapter.go's DiscInfo ever reads), this is identical to LBA, since
// track.startFrame for the first track is always 0. See TrackInfo's
// doc comment for the one documented simplification this carries for
// later tracks.
func (r *CDReader) ReadSector(index int) ([]byte, error) {
	if _, err := r.trackForFrame(index); err != nil {
		return nil, err
	}

	hunkIdx := index / r.framesPerHunk
	frameInHunk := index % r.framesPerHunk

	if !r.hunkCacheOK || r.hunkCacheIdx != hunkIdx {
		buf, err := r.f.ReadHunk(uint32(hunkIdx))
		if err != nil {
			return nil, fmt.Errorf("chdlib: read sector %d: %w", index, err)
		}
		r.hunkCache, r.hunkCacheIdx, r.hunkCacheOK = buf, hunkIdx, true
	}

	// Data block comes first in the decompressed hunk (all frames'
	// 2352-byte sectors concatenated), followed by the subcode block
	// (all frames' 96-byte subcode concatenated) -- verified directly
	// in libchdr's cdlz_codec_decompress (src/libchdr_codec_cdlz.c):
	// it writes the base (data) decompressor's output starting at
	// buffer offset 0 for frames*CD_MAX_SECTOR_DATA bytes, then the
	// subcode decompressor's output starting at
	// buffer[frames*CD_MAX_SECTOR_DATA]. This is NOT the naive
	// per-frame-interleaved [data][sub][data][sub]... layout one might
	// otherwise assume from cdFrameSize being a per-frame constant.
	const rawSectorSize = 2352
	off := frameInHunk * rawSectorSize
	if off+rawSectorSize > len(r.hunkCache) {
		return nil, fmt.Errorf("chdlib: sector %d: computed offset %d exceeds hunk size %d", index, off, len(r.hunkCache))
	}
	sector := make([]byte, rawSectorSize)
	copy(sector, r.hunkCache[off:off+rawSectorSize])
	return sector, nil
}

func (r *CDReader) NumTracks() int { return len(r.tracks) }

func (r *CDReader) Track(n int) (start, length int, audio bool) {
	if n < 1 || n > len(r.tracks) {
		return 0, 0, false
	}
	t := r.tracks[n-1]
	return t.startFrame, t.Frames, t.IsAudio
}

func (r *CDReader) NumTrackIndexes(track int) int {
	if track < 1 || track > len(r.tracks) {
		return 0
	}
	return 1 // CHD's track metadata (as parsed here) doesn't expose sub-track INDEX points beyond the track start
}

func (r *CDReader) TrackIndex(track, index int) int {
	start, _, _ := r.Track(track)
	return start
}
