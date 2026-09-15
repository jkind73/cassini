// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

// Package hashid computes file identity the way MAME's romcmp/hash.xml
// tooling does: CRC32 (IEEE, the same polynomial zip/MAME both use)
// plus SHA1, not MD5 and not SHA256. This is deliberate — it's what
// lets cassini's identification match directly against MAME's own
// driver source (see biosdb) and every MAME-derived hash list (No-
// Intro, TOSEC, Redump) without a translation layer.
package hashid

import (
	"archive/zip"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"io"
	"os"
)

// Digest is a computed CRC32+SHA1 pair.
type Digest struct {
	CRC32 uint32
	SHA1  string // hex, lowercase, 40 chars
	Size  int64
}

func (d Digest) String() string {
	return fmt.Sprintf("crc32=%08x sha1=%s size=%d", d.CRC32, d.SHA1, d.Size)
}

// HashReader computes both hashes in a single pass over r.
func HashReader(r io.Reader) (Digest, error) {
	crcH := crc32.NewIEEE()
	shaH := sha1.New()
	n, err := io.Copy(io.MultiWriter(crcH, shaH), r)
	if err != nil {
		return Digest{}, fmt.Errorf("hashid: read: %w", err)
	}
	return Digest{
		CRC32: crcH.Sum32(),
		SHA1:  hex.EncodeToString(shaH.Sum(nil)),
		Size:  n,
	}, nil
}

// HashFile hashes a plain file on disk (e.g. a loose sega_101.bin).
// Uses a buffered reader sized for spinning-disk-friendly sequential
// reads; BIOS files are small (512KB) so this is a non-issue in
// practice, but the same code path is reused for multi-hundred-MB
// disc images in the ROM scanner (item 4), where it matters.
func HashFile(path string) (Digest, error) {
	f, err := os.Open(path)
	if err != nil {
		return Digest{}, fmt.Errorf("hashid: open %s: %w", path, err)
	}
	defer f.Close()

	d, err := HashReader(f)
	if err != nil {
		return Digest{}, fmt.Errorf("hashid: %s: %w", path, err)
	}
	return d, nil
}

// ZipEntry is one hashed member of a zip archive (e.g. one IC chip
// dump inside stvbios.zip).
type ZipEntry struct {
	Name   string // as stored in the archive, e.g. "epr-20091.ic8"
	Digest Digest
}

// HashZipEntries hashes every regular-file entry inside a zip archive.
// It always computes CRC32+SHA1 fresh by decompressing and streaming
// through both hashers, rather than trusting the CRC32 the zip format
// stores in its own local/central-directory headers — the stored
// value covers only the compressed-stream integrity check zip itself
// performs, and using our own computation keeps this function's
// behavior identical regardless of what container format wraps the
// data, which matters once the same Digest type is compared against
// both zip-derived and loose-file-derived entries in biosdb.
func HashZipEntries(path string) ([]ZipEntry, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("hashid: open zip %s: %w", path, err)
	}
	defer zr.Close()

	entries := make([]ZipEntry, 0, len(zr.File))
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("hashid: open zip entry %s in %s: %w", f.Name, path, err)
		}
		d, err := HashReader(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("hashid: hash zip entry %s in %s: %w", f.Name, path, err)
		}
		entries = append(entries, ZipEntry{Name: f.Name, Digest: d})
	}
	return entries, nil
}
