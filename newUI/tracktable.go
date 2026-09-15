// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package chdlib

import "fmt"

// cdFrameSize is libchdr's fixed per-frame I/O granularity: 2352-byte
// raw sector + 96-byte subcode, verified directly in
// include/libchdr/cdrom.h (CD_MAX_SECTOR_DATA=2352,
// CD_MAX_SUBCODE_DATA=96, CD_FRAME_SIZE=their sum). With
// CHDR_WANT_RAW_DATA_SECTOR+CHDR_WANT_SUBCODE both ON (which
// scripts/build-libchdr.sh pins explicitly), every CD codec
// reconstructs full raw sectors regardless of the track's original
// stored type, so this is the only size that matters for reading --
// per-track data size (what MAME calls "datasize") is only needed to
// report track metadata (audio vs. data, for uiface.DiscReader.Track),
// never for addressing.
const cdFrameSize = 2352 + 96

// TrackInfo is one parsed CD track metadata entry.
type TrackInfo struct {
	Number     int
	TypeString string // e.g. "MODE1_RAW", "AUDIO" -- see trackIsAudio's doc comment for the verified table
	Frames     int
	IsAudio    bool

	// startFrame is this track's first CHD-frame index, computed as
	// the running sum of preceding tracks' Frames. This assumes each
	// track's stored data begins immediately after the previous
	// track's (i.e. any pregap is already counted within Frames) --
	// true for the overwhelming majority of chdman-created CD CHDs.
	// It does not model a pregap stored as a separate, unlisted gap
	// between tracks (MAME's own "pgdatasize" distinction, which the
	// CDROM_TRACK_METADATA2 string doesn't expose here). Since
	// cassini only ever reads IP.BIN from track 1's early sectors,
	// this simplification cannot affect volume-header identification
	// -- it would only matter for precise seeking into later tracks
	// (e.g. CD-DA), which this reader does not attempt to support.
	startFrame int
}

// trackIsAudio classifies a track TYPE string, mirroring MAME's
// cdrom_file::get_type_string table (src/lib/util/cdrom.cpp) exactly.
// Not used for CHD hunk addressing (see cdFrameSize's doc comment),
// only for Track()/NumTracks() reporting fidelity.
func trackIsAudio(typeString string) (bool, error) {
	switch typeString {
	case "MODE1", "MODE1_RAW", "MODE2", "MODE2_FORM1", "MODE2_FORM2", "MODE2_FORM_MIX", "MODE2_RAW":
		return false, nil
	case "AUDIO":
		return true, nil
	default:
		return false, fmt.Errorf("chdlib: unrecognized CD track type %q", typeString)
	}
}

// readTrackTable reads every CDROM_TRACK_METADATA2 entry (falling
// back to the older CDROM_TRACK_METADATA format, which lacks the
// pregap fields, for CHDs written by an older chdman) until
// CHDERR_METADATA_NOT_FOUND signals the end of the list -- the normal
// termination condition per libchdr's own convention, not an error.
func readTrackTable(f *File) ([]TrackInfo, error) {
	var tracks []TrackInfo
	frame := 0

	tag := uint32(TagCDROMTrack2)
	idx := 0
	triedLegacy := false
	for {
		data, err := f.Metadata(tag, uint32(idx))
		if err != nil {
			if IsMetadataNotFound(err) {
				if idx == 0 && !triedLegacy && tag == TagCDROMTrack2 {
					// No CHT2 entries at all -- retry the whole scan
					// against the legacy CHTR tag before giving up.
					tag = TagCDROMTrack
					triedLegacy = true
					continue
				}
				break
			}
			return nil, err
		}

		var num, frames, pregap, postgap int
		var typeStr, subTypeStr, pgType, pgSub string
		var n int
		text := string(data)
		if tag == TagCDROMTrack2 {
			n, err = fmt.Sscanf(text, "TRACK:%d TYPE:%s SUBTYPE:%s FRAMES:%d PREGAP:%d PGTYPE:%s PGSUB:%s POSTGAP:%d",
				&num, &typeStr, &subTypeStr, &frames, &pregap, &pgType, &pgSub, &postgap)
		} else {
			n, err = fmt.Sscanf(text, "TRACK:%d TYPE:%s SUBTYPE:%s FRAMES:%d", &num, &typeStr, &subTypeStr, &frames)
		}
		wantFields := 4
		if tag == TagCDROMTrack2 {
			wantFields = 8
		}
		if err != nil || n != wantFields {
			return nil, fmt.Errorf("chdlib: malformed track metadata entry %d: %q", idx, text)
		}

		isAudio, terr := trackIsAudio(typeStr)
		if terr != nil {
			return nil, terr
		}

		tracks = append(tracks, TrackInfo{
			Number:     num,
			TypeString: typeStr,
			Frames:     frames,
			IsAudio:    isAudio,
			startFrame: frame,
		})
		frame += frames
		idx++
	}

	if len(tracks) == 0 {
		return nil, fmt.Errorf("chdlib: no CD track metadata found -- this CHD may not be a CD-ROM image (e.g. a hard disk CHD)")
	}
	return tracks, nil
}
