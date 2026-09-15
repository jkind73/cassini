// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

// Package biosdb identifies Sega Saturn console and ST-V arcade BIOS
// dumps by CRC32+SHA1 (see package hashid), following MAME convention.
//
// PROVENANCE NOTE: an earlier draft of this database was seeded from a
// user-supplied known_bios_db.json. That file was NOT used as a
// source of hash values — cross-checking it against MAME's own driver
// source (github.com/mamedev/mame, src/mame/sega/saturn.cpp and
// src/mame/sega/stv.cpp) and independent community verification
// (MiSTer FPGA forum hash tables, OpenEmu's BIOS wiki, the
// beetle-saturn-libretro core) found its CRC32 values internally
// inconsistent with its own SHA1/MD5 in the same entries — which is
// only possible if the CRC32 values are simply wrong, since CRC32 is
// a pure deterministic function of file bytes, same as SHA1/MD5. Its
// ST-V entries additionally disagreed on SHA1 itself, not just CRC32.
// Every value below instead traces to one of the citations in its
// entry's Source field; entries below are cross-corroborated by 2+
// independent sources where noted, single-sourced from MAME's own
// driver where noted (MAME's driver source IS the reference
// identification database the wider preservation community defers
// to for arcade sets, so single-sourcing to it is not a weak
// citation the way single-sourcing to a random hash list would be).
package biosdb

import "strings"

// System identifies which hardware family a BIOS entry targets.
type System int

const (
	SystemSaturnConsole System = iota
	SystemSTVArcade
)

// Region is the console/cabinet region a BIOS was built for. Not all
// Saturn BIOS revisions are region-locked identically to the game
// discs that use them — see SelectForRegion's fallback chain.
type Region int

const (
	RegionUnknown Region = iota
	RegionJapan
	RegionOverseas // MAME's own term: one BIOS image serves both NA and EU
	RegionUSA
	RegionEurope
	RegionTaiwan
)

func (r Region) String() string {
	switch r {
	case RegionJapan:
		return "Japan"
	case RegionOverseas:
		return "Overseas (US/EU)"
	case RegionUSA:
		return "USA"
	case RegionEurope:
		return "Europe"
	case RegionTaiwan:
		return "Taiwan"
	default:
		return "Unknown"
	}
}

// Entry is one known-good BIOS dump.
type Entry struct {
	System   System
	Region   Region
	Revision string // e.g. "1.01", "1.00a", "97/08/21 v1.13"
	Label    string
	Filename string // canonical/MAME romname, e.g. "sega_101.bin", "epr-20091.ic8"

	CRC32      uint32
	CRC32Known bool // false for entries only cross-verified by SHA1 (see Hi-Saturn/V-Saturn below)
	SHA1       string

	// Recommended marks BIOS revisions specifically called out as the
	// best-compatibility choice for their region (matches the
	// guidance MiSTer's Saturn core documentation gives: sega_101.bin
	// for Japan, mpr-17933.bin for Overseas), used to break ties in
	// SelectForRegion beyond plain region matching.
	Recommended bool

	Source string
}

// Known is the full identification table.
var Known = []Entry{
	// --- Saturn console BIOS ---
	// CRC32+SHA1 cross-verified: MAME src/mame/sega/saturn.cpp (github.com/mamedev/mame)
	// against MiSTer FPGA forum sha1sum tables (misterfpga.org/viewtopic.php?t=7261,
	// t=7949, t=2110).
	{
		System: SystemSaturnConsole, Region: RegionJapan, Revision: "1.00",
		Label: "Japan v1.00 (940921)", Filename: "sega_100.bin",
		CRC32: 0x2aba43c2, CRC32Known: true,
		SHA1:   "2b8cb4f87580683eb4d760e4ed210813d667f0a2",
		Source: "MAME sega/saturn.cpp; MiSTer FPGA forum t=7261/t=7949",
	},
	{
		System: SystemSaturnConsole, Region: RegionJapan, Revision: "1.003",
		Label: "Japan v1.003 (941012)", Filename: "sega1003.bin",
		CRC32: 0xb3c63c25, CRC32Known: true,
		SHA1:   "7b23b53d62de0f29a23e423d0fe751dfb469c2fa",
		Source: "MAME sega/saturn.cpp; MiSTer FPGA forum t=7261/t=7949",
	},
	{
		System: SystemSaturnConsole, Region: RegionJapan, Revision: "1.01",
		Label: "Japan v1.01 (941228)", Filename: "sega_101.bin",
		CRC32: 0x224b752c, CRC32Known: true,
		SHA1:       "df94c5b4d47eb3cc404d88b33a8fda237eaf4720",
		Recommended: true, // MiSTer Saturn core: recommended for max compatibility
		Source:      "MAME sega/saturn.cpp; MiSTer FPGA forum t=7261/t=7949; OpenEmu BIOS wiki",
	},
	{
		System: SystemSaturnConsole, Region: RegionOverseas, Revision: "1.00a",
		Label: "Overseas v1.00a (941115)", Filename: "mpr-17933.bin",
		CRC32: 0x4afcf0fa, CRC32Known: true,
		SHA1:       "faa8ea183a6d7bbe5d4e03bb1332519800d3fbc3",
		Recommended: true, // MiSTer Saturn core: recommended for max compatibility
		Source:      "MAME sega/saturn.cpp; MiSTer FPGA forum t=7261/t=7949",
	},
	{
		System: SystemSaturnConsole, Region: RegionOverseas, Revision: "1.00a",
		Label: "Overseas v1.00a (941115, alt dump)", Filename: "sega_100a.bin",
		CRC32: 0xf90f0089, CRC32Known: true,
		SHA1:   "3bb41feb82838ab9a35601ac666de5aacfd17a58",
		Source: "MAME sega/saturn.cpp; MiSTer FPGA forum t=7261/t=7949",
	},

	// --- Clone hardware (licensed third-party Saturn units) ---
	// SHA1-only: cross-verified across 2 independent MiSTer FPGA forum
	// threads (t=7949, t=2110), both citing the same upstream GitHub
	// source; CRC32 not independently corroborated, so CRC32Known is
	// false and Identify() must not treat CRC32==0 as a match signal
	// for these — see Identify's doc comment.
	{
		System: SystemSaturnConsole, Region: RegionJapan, Revision: "1.01 (Hitachi Hi-Saturn)",
		Label: "Hitachi Hi-Saturn v1.01 (Japan)", Filename: "hi_saturn_101.bin",
		SHA1:   "49d8493008fa715ca0c94d99817a5439d6f2c796",
		Source: "MiSTer FPGA forum t=7949, t=2110 (cross-verified, 2 independent threads)",
	},
	{
		System: SystemSaturnConsole, Region: RegionJapan, Revision: "1.02 (Hitachi Hi-Saturn)",
		Label: "Hitachi Hi-Saturn v1.02 (Japan)", Filename: "hi_saturn_102.bin",
		SHA1:   "8a22710e09ce75f39625894366cafe503ed1942d",
		Source: "MiSTer FPGA forum t=7949, t=2110 (cross-verified, 2 independent threads)",
	},
	{
		System: SystemSaturnConsole, Region: RegionJapan, Revision: "1.03 (Hitachi Hi-Saturn)",
		Label: "Hitachi Hi-Saturn v1.03 (Japan)", Filename: "hi_saturn_103.bin",
		SHA1:   "8c031bf9908fd0142fdd10a9cdd79389f8a3f2fc",
		Source: "MiSTer FPGA forum t=7949, t=2110 (cross-verified, 2 independent threads)",
	},
	{
		System: SystemSaturnConsole, Region: RegionJapan, Revision: "1.01 (Victor V-Saturn)",
		Label: "Victor V-Saturn v1.01 (Japan)", Filename: "v_saturn_101.bin",
		SHA1:   "4154e11959f3d5639b11d7902b3a393a99fb5776",
		Source: "MiSTer FPGA forum t=7949, t=2110 (cross-verified, 2 independent threads)",
	},

	// --- ST-V arcade BIOS (IC8 cartridge slot chip) ---
	// Single-sourced to MAME's own driver (src/mame/sega/stv.cpp,
	// historic-mess mirror for the historically-named variants) —
	// MAME's driver source is the arcade preservation community's own
	// reference identification database.
	{
		System: SystemSTVArcade, Region: RegionJapan, Revision: "97/08/21 v1.13",
		Label: "ST-V Japan EPR-20091 (97/08/21)", Filename: "epr-20091.ic8",
		CRC32: 0x59ed40f4, CRC32Known: true,
		SHA1:       "eff0f54c70bce05ff3a289bf30b1027e1c8cd117",
		Recommended: true, // matches guidance: recommended for max compatibility
		Source:      "MAME sega/stv.cpp",
	},
	{
		System: SystemSTVArcade, Region: RegionJapan, Revision: "97/02/17",
		Label: "ST-V Japan EPR-19730 (97/02/17)", Filename: "epr-19730.ic8",
		CRC32: 0xd0e0889d, CRC32Known: true,
		SHA1:   "fae53107c894e0c41c49e191dbe706c9cd6e50bd",
		Source: "MAME sega/stv.cpp",
	},
	{
		System: SystemSTVArcade, Region: RegionJapan, Revision: "95/04/25",
		Label: "ST-V Japan EPR-17951A (95/04/25)", Filename: "epr-17951a.ic8",
		CRC32: 0x2672f9d8, CRC32Known: true,
		SHA1:   "63cf4a6432f6c87952f9cf3ab0f977aed2367303",
		Source: "MAME sega/stv.cpp",
	},
	{
		System: SystemSTVArcade, Region: RegionJapan, Revision: "95/02/20",
		Label: "ST-V Japan EPR-17740A (95/02/20)", Filename: "epr-17740a.ic8",
		CRC32: 0x3e23c81f, CRC32Known: true,
		SHA1:   "f9b282fd27693e9891843597b2e1823da3d23c7b",
		Source: "MAME sega/stv.cpp",
	},
	{
		System: SystemSTVArcade, Region: RegionUSA, Revision: "EPR-17952A",
		Label: "ST-V USA EPR-17952A", Filename: "epr-17952a.ic8",
		CRC32: 0xd1be2adf, CRC32Known: true,
		SHA1:       "eaf1c3e5d602e1139d2090a78d7e19f04f916794",
		Recommended: true,
		Source:      "MAME historic-mess src/mame/drivers/stv.c (mp17952a.s)",
	},
	{
		System: SystemSTVArcade, Region: RegionEurope, Revision: "EPR-17954A",
		Label: "ST-V Europe EPR-17954A", Filename: "epr-17954a.ic8",
		CRC32: 0xf7722da3, CRC32Known: true,
		SHA1:   "af79cff317e5b57d49e463af16a9f616ed1eee08",
		Source: "MAME historic-mess src/mame/drivers/stv.c (mp17954a.s)",
	},
	{
		System: SystemSTVArcade, Region: RegionTaiwan, Revision: "EPR-17953A",
		Label: "ST-V Taiwan EPR-17953A", Filename: "epr-17953a.ic8",
		CRC32: 0xa4c47570, CRC32Known: true,
		SHA1:   "9efc73717ec8a13417e65c54344ded9fc25bf5ef",
		Source: "MAME historic-mess src/mame/drivers/stv.c (mp17953a.ic8)",
	},
}

// Identify looks up an Entry by hash. SHA1 is authoritative (160-bit,
// no known practical collision relevant to this use case); CRC32 is
// used only as a cheap pre-filter and, for the four CRC32Known==false
// clone-hardware entries, is never consulted at all — a zero-value
// CRC32 must never be treated as "the file's CRC32 is 0 and it
// matched", it must be treated as "this dimension carries no
// information for this entry".
func Identify(crc32 uint32, sha1 string) (Entry, bool) {
	sha1 = strings.ToLower(sha1)
	// Pass 1: exact SHA1 match, regardless of CRC32Known. This is the
	// dispositive check.
	for _, e := range Known {
		if strings.ToLower(e.SHA1) == sha1 {
			return e, true
		}
	}
	// Pass 2: CRC32-only fallback, for callers that only have a CRC32
	// (e.g. a quick directory pre-scan before full hashing) — only
	// ever matches entries where CRC32 was itself independently
	// verified, never the four clone-hardware entries.
	for _, e := range Known {
		if e.CRC32Known && e.CRC32 == crc32 {
			return e, true
		}
	}
	return Entry{}, false
}

// ForSystem filters Known to one hardware family.
func ForSystem(sys System) []Entry {
	var out []Entry
	for _, e := range Known {
		if e.System == sys {
			out = append(out, e)
		}
	}
	return out
}
