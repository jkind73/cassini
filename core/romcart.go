// Copyright 2026 The erings Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package core

// ROMCartridge represents an A-Bus ROM cartridge (mapped to 0x02000000 - 0x04FFFFFF).
type ROMCartridge struct {
	data []byte
}

// NewROMCartridge creates a new ROM cartridge with the given binary data.
func NewROMCartridge(data []byte) *ROMCartridge {
	return &ROMCartridge{
		data: data,
	}
}

// Read8 reads a byte from the ROM cartridge.
func (c *ROMCartridge) Read8(off uint32) uint8 {
	if int(off) < len(c.data) {
		return c.data[off]
	}
	return 0xFF
}

// Read16 reads a big-endian 16-bit word from the ROM cartridge.
func (c *ROMCartridge) Read16(off uint32) uint16 {
	if int(off+1) < len(c.data) {
		return uint16(c.data[off])<<8 | uint16(c.data[off+1])
	}
	return 0xFFFF
}

// Read32 reads a big-endian 32-bit word from the ROM cartridge.
func (c *ROMCartridge) Read32(off uint32) uint32 {
	if int(off+3) < len(c.data) {
		return uint32(c.data[off])<<24 | uint32(c.data[off+1])<<16 | uint32(c.data[off+2])<<8 | uint32(c.data[off+3])
	}
	return 0xFFFFFFFF
}
