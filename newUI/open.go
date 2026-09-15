// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package discimg

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jkind73/cassini/chdlib"
	"github.com/jkind73/cassini/uiface"
)

// DiscReadCloser is a uiface.DiscReader that also owns file handles
// needing an explicit close. Every concrete reader in this package
// (and chdlib.CDReader) already satisfies this.
type DiscReadCloser interface {
	uiface.DiscReader
	Close() error
}

// Open dispatches to the right reader by file extension:
// .cue -> OpenBinCue, .iso -> OpenIso, .mds -> OpenMds, .chd ->
// chdlib.OpenCD. This is the single entry point both ui/ebiten's
// direct-run path and its browser launch path use, so "how does
// cassini open a disc" has exactly one implementation regardless of
// which UI flow triggered it.
func Open(path string) (DiscReadCloser, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".cue":
		return OpenBinCue(path)
	case ".iso":
		return OpenIso(path)
	case ".mds":
		return OpenMds(path)
	case ".chd":
		return chdlib.OpenCD(path)
	default:
		return nil, fmt.Errorf("discimg: %s: unrecognized disc image extension (expected .cue, .iso, .mds, or .chd)", path)
	}
}
