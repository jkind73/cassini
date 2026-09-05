// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package core

import (
	"archive/zip"
	"bytes"
	"io"
)

// ProcessMAMEBIOS checks if the BIOS input is a ZIP archive (MAME format stv.zip / stv110.zip).
// If it is a ZIP archive, it extracts and stitches the ST-V ROM dump parts (e.g. stv110.bin /epr-17944.ic8)
// or concatenates byte-interleaved MAME split dumps into a 512KB system BIOS image.
func ProcessMAMEBIOS(data []byte) ([]byte, error) {
	if len(data) == biosSize {
		return data, nil // Plain 512KB raw binary
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return data, nil // Not a ZIP archive, pass through for validation
	}

	// ST-V MAME ROM dumps consist of 2 x 256KB or 4 x 128KB ROM files
	var romFiles []*zip.File
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		romFiles = append(romFiles, f)
	}

	if len(romFiles) == 0 {
		return data, nil
	}

	// Case 1: Single file inside ZIP (e.g. stv110.bin inside zip)
	if len(romFiles) == 1 && romFiles[0].UncompressedSize64 == uint64(biosSize) {
		rc, err := romFiles[0].Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}

	// Case 2: Multi-part MAME split BIOS (e.g. epr-17944.ic8, epr-17945.ic9)
	var concatenated []byte
	for _, f := range romFiles {
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		buf, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		concatenated = append(concatenated, buf...)
	}

	if len(concatenated) == biosSize {
		return concatenated, nil
	}

	return data, nil
}
