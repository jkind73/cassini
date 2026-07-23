// Copyright 2026 The erings Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package core

// STVEEPROM implements the Microchip/National 93C45/93C46 16-bit serial EEPROM
// used on the Sega Titan Video (ST-V) arcade motherboard for system configuration,
// coin counter settings, and high score backup.
type STVEEPROM struct {
	data [64]uint16 // 64 x 16-bit words (128 bytes total)

	// Serial interface state
	cs    bool // Chip Select (active high)
	sk    bool // Serial Clock
	di    bool // Data In
	doPin bool // Data Out

	state     int    // State machine (0=idle, 1=command, 2=address, 3=data)
	bitBuffer uint16 // Shift register for incoming bits
	bitCount  int    // Number of bits shifted in
	curCmd    uint8  // Current opcode
	curAddr   uint8  // Target word address (0-63)
	writeEn   bool   // Write enable state (EWEN/EWDS)
}

// NewSTVEEPROM initializes an ST-V serial EEPROM with default factory settings.
func NewSTVEEPROM() *STVEEPROM {
	e := &STVEEPROM{
		writeEn: true,
	}
	// Default ST-V EEPROM header signature
	e.data[0] = 0x5354 // 'S','T'
	e.data[1] = 0x2D56 // '-','V'
	return e
}

// SetPins sets the state of the CS, SK, and DI control pins and ticks the serial interface.
func (e *STVEEPROM) SetPins(cs, sk, di bool) {
	prevSK := e.sk
	e.cs = cs
	e.sk = sk
	e.di = di

	if !e.cs {
		e.state = 0
		e.bitBuffer = 0
		e.bitCount = 0
		e.doPin = true
		return
	}

	// Rising edge of Serial Clock (SK)
	if !prevSK && e.sk {
		e.clockInBit()
	}
}

// DataOut returns the state of the serial Data Out (DO) pin.
func (e *STVEEPROM) DataOut() bool {
	return e.doPin
}

func (e *STVEEPROM) clockInBit() {
	bit := uint16(0)
	if e.di {
		bit = 1
	}

	switch e.state {
	case 0: // Idle - waiting for Start bit (1)
		if bit == 1 {
			e.state = 1
			e.bitBuffer = 0
			e.bitCount = 0
		}

	case 1: // Command & Address shift (8 bits: 2-bit opcode + 6-bit address)
		e.bitBuffer = (e.bitBuffer << 1) | bit
		e.bitCount++
		if e.bitCount == 8 {
			e.curCmd = uint8((e.bitBuffer >> 6) & 0x03)
			e.curAddr = uint8(e.bitBuffer & 0x3F)
			e.executeCommand()
		}

	case 2: // Read Data Out shift (16 bits out)
		if e.bitCount < 16 {
			val := e.data[e.curAddr&0x3F]
			e.doPin = (val & (1 << (15 - e.bitCount))) != 0
			e.bitCount++
		} else {
			e.state = 0
		}

	case 3: // Write Data In shift (16 bits in)
		e.bitBuffer = (e.bitBuffer << 1) | bit
		e.bitCount++
		if e.bitCount == 16 {
			if e.writeEn {
				e.data[e.curAddr&0x3F] = e.bitBuffer
			}
			e.state = 0
			e.doPin = true
		}
	}
}

func (e *STVEEPROM) executeCommand() {
	switch e.curCmd {
	case 0x02: // READ
		e.state = 2
		e.bitCount = 0
		val := e.data[e.curAddr&0x3F]
		e.doPin = (val & 0x8000) != 0

	case 0x01: // WRITE
		if e.writeEn {
			e.state = 3
			e.bitBuffer = 0
			e.bitCount = 0
		} else {
			e.state = 0
		}

	case 0x00: // Control sub-commands (EWDS, ERAL, EWEN)
		sub := (e.curAddr >> 4) & 0x03
		switch sub {
		case 0x00: // EWDS (Write Disable)
			e.writeEn = false
		case 0x03: // EWEN (Write Enable)
			e.writeEn = true
		}
		e.state = 0

	default:
		e.state = 0
	}
}
