// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package core

// STVBoard implements the Sega Titan Video (ST-V) arcade system board I/O.
// It interfaces the ST-V arcade cabinet controls (Coin 1/2, Test, Service,
// 4-Player Joystick inputs) and DIP switches into the Saturn SMPC/A-Bus I/O.
type STVBoard struct {
	eeprom  *STVEEPROM
	dipSw   uint8  // 8-bit arcade DIP switches
	inputs1 uint16 // Coin 1, Coin 2, Test, Service, P1 Start/Btns
	inputs2 uint16 // P2 Start/Btns, etc.
}

// NewSTVBoard creates an ST-V arcade board interface.
func NewSTVBoard() *STVBoard {
	return &STVBoard{
		eeprom:  NewSTVEEPROM(),
		dipSw:   0xFF,   // active-low, all OFF by default
		inputs1: 0xFFFF, // active-low, all unpressed
		inputs2: 0xFFFF,
	}
}

// SetArcadeInputs updates active-low arcade buttons (Coin 1/2, Test, Service, P1/P2).
func (s *STVBoard) SetArcadeInputs(inputs1, inputs2 uint16) {
	s.inputs1 = inputs1
	s.inputs2 = inputs2
}

// SetDIPSwitches configures the 8-bit arcade DIP switch positions.
func (s *STVBoard) SetDIPSwitches(sw uint8) {
	s.dipSw = sw
}

// EEPROM returns the attached 93C45/93C46 serial EEPROM.
func (s *STVBoard) EEPROM() *STVEEPROM {
	return s.eeprom
}
