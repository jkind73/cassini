// Copyright 2026 The erings Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package core

import "testing"

func TestSTVEEPROM_ReadWrite(t *testing.T) {
	eeprom := NewSTVEEPROM()

	// Initial header read check: word 0 = 0x5354 ('S','T')
	// Send Start Bit (1), READ opcode (10), Address 000000 -> 1 10 000000
	// Bit sequence: 1, 1, 0, 0, 0, 0, 0, 0, 0
	clockBit := func(bit bool) {
		eeprom.SetPins(true, false, bit)
		eeprom.SetPins(true, true, bit)
	}

	// Start bit
	clockBit(true)
	// Opcode 10 (READ)
	clockBit(true)
	clockBit(false)
	// Address 0x00
	for i := 0; i < 6; i++ {
		clockBit(false)
	}

	// Read 16 bits out
	var readVal uint16
	for i := 0; i < 16; i++ {
		clockBit(false)
		if eeprom.DataOut() {
			readVal |= (1 << (15 - i))
		}
	}

	if readVal != 0x5354 {
		t.Errorf("ST-V EEPROM read word 0 = 0x%04X, want 0x5354", readVal)
	}
}
