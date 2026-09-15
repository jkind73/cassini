// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package ebiten

import (
	"archive/zip"
	"fmt"
	"io"
)

// readZipEntry reads one named entry's full contents from a zip
// archive. Used by browser.go's readBIOSBytes for BIOS files
// biosdb.Scan found packed inside an archive (e.g. stvbios.zip) rather
// than as a loose file.
func readZipEntry(zipPath, entryName string) ([]byte, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("open zip %s: %w", zipPath, err)
	}
	defer zr.Close()

	for _, f := range zr.File {
		if f.Name != entryName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open zip entry %s in %s: %w", entryName, zipPath, err)
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	return nil, fmt.Errorf("entry %s not found in %s", entryName, zipPath)
}
