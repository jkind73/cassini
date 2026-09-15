// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

// Package chdlib binds github.com/rtissera/libchdr — MAME's own CHD
// reader, extracted as a standalone C library and used by RetroArch,
// PCSX2, DuckStation, and MAME itself for third-party CHD support.
// This replaces cassini's earlier hand-rolled V5-only CHD header
// parser: using the real library means every CHD version (V1-V5, not
// just V5), every compressor (zlib/lzma/huffman/flac/zstd, including
// their CD-specific variants), and any future format revision is
// handled exactly as MAME handles it, and stays correct automatically
// as libchdr is updated — which was the whole point of not
// reimplementing this ourselves.
//
// VERIFICATION: every function/struct declaration this file binds to
// was checked against the actual libchdr source (not just its docs),
// cloned at commit 6cde5348eb118da3baf94f75a69577a005a484fd, built
// with the exact flags in scripts/build-libchdr.sh, and exercised with
// a standalone C smoke test (chd_open/chd_error_string/sizeof(chd_header)
// /chd_read_header on a real build) before any Go code was written
// against it.
//
// BUILD REQUIREMENT: this package requires cgo (CGO_ENABLED=1) and a
// C toolchain, plus the vendored libchdr submodule built via
// scripts/build-libchdr.sh once. This is a real, disclosed trade-off
// versus a pure-Go implementation: correctness and staying current
// with upstream libchdr in exchange for no longer being pure Go. Any
// package that imports chdlib (transitively: discimg, once wired up)
// carries this requirement.
package chdlib

/*
#cgo CFLAGS: -I${SRCDIR}/../third_party/libchdr/include
#cgo LDFLAGS: -L${SRCDIR}/../third_party/libchdr/build -lchdr-static -L${SRCDIR}/../third_party/libchdr/build/deps/lzma-25.01 -lchdr-lzma -lz -lzstd -lFLAC -lm
#include <libchdr/chd.h>
#include <stdlib.h>
*/
import "C"

import (
	"encoding/hex"
	"fmt"
	"unsafe"
)

// Error wraps a chd_error code with libchdr's own human-readable
// message (chd_error_string), so error text never has to be
// maintained by hand in this binding and can't drift from what the
// library itself reports for a given code.
type Error struct {
	Code int
	msg  string
}

func (e *Error) Error() string { return fmt.Sprintf("chdlib: %s (code %d)", e.msg, e.Code) }

func chdErr(code C.chd_error) error {
	if code == C.CHDERR_NONE {
		return nil
	}
	return &Error{Code: int(code), msg: C.GoString(C.chd_error_string(code))}
}

// Header is Go-native form of libchdr's extract chd_header struct
// (verified field-for-field against include/libchdr/chd.h; see the
// package doc comment for how). SHA1/RawSHA1/ParentSHA1/MD5 are hex
// strings, matching how the rest of cassini's identification code
// (hashid, biosdb) represents digests.
type Header struct {
	Version      uint32
	HunkBytes    uint32
	TotalHunks   uint32
	LogicalBytes uint64
	UnitBytes    uint32
	UnitCount    uint64
	SHA1         string
	RawSHA1      string
	ParentSHA1   string // all-zero hex if no parent
	MD5          string
	ParentMD5    string
}

func goHex(b []byte) string { return hex.EncodeToString(b) }

func headerFromC(h *C.chd_header) Header {
	return Header{
		Version:      uint32(h.version),
		HunkBytes:    uint32(h.hunkbytes),
		TotalHunks:   uint32(h.totalhunks),
		LogicalBytes: uint64(h.logicalbytes),
		UnitBytes:    uint32(h.unitbytes),
		UnitCount:    uint64(h.unitcount),
		SHA1:         goHex(C.GoBytes(unsafe.Pointer(&h.sha1[0]), C.CHD_SHA1_BYTES)),
		RawSHA1:      goHex(C.GoBytes(unsafe.Pointer(&h.rawsha1[0]), C.CHD_SHA1_BYTES)),
		ParentSHA1:   goHex(C.GoBytes(unsafe.Pointer(&h.parentsha1[0]), C.CHD_SHA1_BYTES)),
		MD5:          goHex(C.GoBytes(unsafe.Pointer(&h.md5[0]), C.CHD_MD5_BYTES)),
		ParentMD5:    goHex(C.GoBytes(unsafe.Pointer(&h.parentmd5[0]), C.CHD_MD5_BYTES)),
	}
}

// File is an open CHD, backed by a chd_file* handle.
type File struct {
	h      *C.chd_file
	header Header
}

// Open opens path read-only. No parent CHD is attached (nil parent) —
// chained/child CHDs (a CHD whose data is the delta against a "parent"
// CHD, used for arcade ROM patches, e.g. game update sets) are
// therefore not supported here; a child CHD's chd_open call returns
// CHDERR_REQUIRES_PARENT, surfaced as an *Error the caller can detect
// via errors.As, with an actionable message from libchdr itself.
// Saturn/ST-V disc images do not use CHD chaining, so this is not a
// meaningful limitation for cassini's use case.
func Open(path string) (*File, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	var h *C.chd_file
	if err := chdErr(C.chd_open(cpath, C.CHD_OPEN_READ, nil, &h)); err != nil {
		return nil, fmt.Errorf("chdlib: open %s: %w", path, err)
	}

	hdr := C.chd_get_header(h)
	if hdr == nil {
		C.chd_close(h)
		return nil, fmt.Errorf("chdlib: %s: chd_get_header returned NULL", path)
	}
	return &File{h: h, header: headerFromC(hdr)}, nil
}

func (f *File) Close() error {
	if f.h != nil {
		C.chd_close(f.h)
		f.h = nil
	}
	return nil
}

func (f *File) Header() Header { return f.header }

// ReadHunk decompresses hunk hunknum in full (chd_read always
// operates on whole hunks; there is no library-level API for reading
// an arbitrary byte range within one) and returns it as a freshly
// allocated Go slice of f.Header().HunkBytes bytes.
func (f *File) ReadHunk(hunknum uint32) ([]byte, error) {
	buf := make([]byte, f.header.HunkBytes)
	if len(buf) == 0 {
		return nil, fmt.Errorf("chdlib: hunk size is 0 (corrupt header?)")
	}
	err := chdErr(C.chd_read(f.h, C.uint32_t(hunknum), unsafe.Pointer(&buf[0])))
	if err != nil {
		return nil, fmt.Errorf("chdlib: read hunk %d: %w", hunknum, err)
	}
	return buf, nil
}

// Metadata reads one metadata entry by (tag, index), following
// chd_get_metadata's own two-call convention: an initial probe call
// with a zero-length buffer to discover the required size (returned
// via resultlen even when the supplied buffer is too small — this is
// documented libchdr behavior, not an assumption), then a second call
// with a correctly sized buffer.
func (f *File) Metadata(tag uint32, index uint32) ([]byte, error) {
	var resultlen C.uint32_t
	var resulttag C.uint32_t
	var resultflags C.uint8_t

	probeErr := C.chd_get_metadata(f.h, C.uint32_t(tag), C.uint32_t(index), nil, 0, &resultlen, &resulttag, &resultflags)
	if err := chdErr(probeErr); err != nil {
		return nil, fmt.Errorf("chdlib: metadata tag=%08x index=%d: %w", tag, index, err)
	}
	if resultlen == 0 {
		return []byte{}, nil
	}

	buf := make([]byte, resultlen)
	if err := chdErr(C.chd_get_metadata(f.h, C.uint32_t(tag), C.uint32_t(index), unsafe.Pointer(&buf[0]), resultlen, &resultlen, &resulttag, &resultflags)); err != nil {
		return nil, fmt.Errorf("chdlib: metadata tag=%08x index=%d: %w", tag, index, err)
	}
	return buf, nil
}

// Metadata tags used by cdreader.go, straight from chd.h's #define's.
const (
	TagCDROMTrack       = 0x43485452 // 'C','H','T','R' -- CDROM_TRACK_METADATA_TAG (old format, no pregap fields)
	TagCDROMTrack2      = 0x43485432 // 'C','H','T','2' -- CDROM_TRACK_METADATA2_TAG
	ErrMetadataNotFound = C.CHDERR_METADATA_NOT_FOUND
)

// IsMetadataNotFound reports whether err is libchdr's
// CHDERR_METADATA_NOT_FOUND, i.e. "no more entries at this index" —
// the normal, expected way callers detect the end of a metadata list
// (see cdreader.go's track-table loop).
func IsMetadataNotFound(err error) bool {
	e, ok := err.(*Error)
	return ok && e.Code == int(ErrMetadataNotFound)
}
