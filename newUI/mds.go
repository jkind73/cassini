// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package discimg

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrUnsupportedMDS is returned for .mds files describing more than
// one session or more than one normal (non-lead-in) track. This
// reader handles the common case for Saturn dumps -- a single Mode1
// data track in a same-named sibling .mdf -- and explicitly rejects
// anything else rather than guess at reading the wrong track as "the
// disc". Full multi-track MDS support would require decoding the
// per-track Data-Block table (mode, subchannel layout, per-track
// start offset within the .mdf), which this package does not
// implement; convert such an image to .cue/.bin instead.
var ErrUnsupportedMDS = fmt.Errorf("discimg: multi-session/multi-track .mds is not supported; convert to .cue/.bin or provide a .chd")

// MDS header layout, offsets/sizes/field meanings per the format
// writeup at forum.redump.org/topic/12357 (cross-referenced by
// multiple independent MDS-parsing tools' documentation):
//
//	00h 16  File ID ("MEDIA DESCRIPTOR")
//	10h  2  Version (unknown-meaning bytes)
//	12h  2  Media type
//	14h  2  Number of sessions
//	...
//	50h  4  Offset to first Session-Block (from start of file)
//
// Session-Block (18h bytes, at the offset read from 0x50):
//
//	08h  2  Session number (starting at 1)
//	0Eh  2  "Last Track" -- count of Data-Blocks with Point<A0h, i.e.
//	        the number of normal (non-lead-in/lead-out) tracks in
//	        this session -- this is the field that actually answers
//	        "how many playable tracks does this session have".
//
// All multi-byte fields are little-endian (Alcohol 120% is a native
// Windows/x86 application; every third-party MDS parser this format
// writeup was cross-checked against reads them as LE).
const (
	mdsOffSessionCount   = 0x14
	mdsOffSessionTblOff  = 0x50
	mdsSessionBlockSize  = 0x18
	mdsOffNormalTrackCnt = 0x0E // within a session block
)

// OpenMds opens a .mds/.mdf pair, verifying single-session,
// single-(normal-)track structure via the header fields above before
// handing off to OpenIso for the actual sector reading (the .mds
// header is consulted only to validate this assumption; sector data
// always comes from the .mdf, read the same way a plain .iso would
// be).
func OpenMds(mdsPath string) (*IsoReader, error) {
	dir := filepath.Dir(mdsPath)
	base := strings.TrimSuffix(filepath.Base(mdsPath), filepath.Ext(mdsPath))
	mdfPath := filepath.Join(dir, base+".mdf")

	if _, err := os.Stat(mdfPath); err != nil {
		return nil, fmt.Errorf("discimg: %s: expected sibling %s: %w", mdsPath, filepath.Base(mdfPath), err)
	}

	if err := verifySingleTrackMds(mdsPath); err != nil {
		return nil, err
	}

	return OpenIso(mdfPath)
}

func verifySingleTrackMds(mdsPath string) error {
	data, err := os.ReadFile(mdsPath)
	if err != nil {
		return fmt.Errorf("discimg: read %s: %w", mdsPath, err)
	}
	if len(data) < mdsOffSessionTblOff+4 {
		return fmt.Errorf("discimg: %s: too short to be a valid MDS header", mdsPath)
	}
	if string(data[0:16]) != "MEDIA DESCRIPTOR" {
		return fmt.Errorf("discimg: %s: bad MDS signature", mdsPath)
	}

	sessions := binary.LittleEndian.Uint16(data[mdsOffSessionCount:])
	if sessions != 1 {
		return ErrUnsupportedMDS
	}

	sessionOff := binary.LittleEndian.Uint32(data[mdsOffSessionTblOff:])
	if int(sessionOff)+mdsSessionBlockSize > len(data) {
		return fmt.Errorf("discimg: %s: session block offset out of range", mdsPath)
	}
	trackCount := binary.LittleEndian.Uint16(data[int(sessionOff)+mdsOffNormalTrackCnt:])
	if trackCount != 1 {
		return ErrUnsupportedMDS
	}
	return nil
}
