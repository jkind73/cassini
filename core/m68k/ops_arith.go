package m68k

func init() {
	registerADD()
	registerADDA()
	registerADDI()
	registerADDQ()
	registerADDX()
	registerSUB()
	registerSUBA()
	registerSUBI()
	registerSUBQ()
	registerSUBX()
	registerCMP()
	registerCMPA()
	registerCMPI()
	registerCMPM()
	registerMULU()
	registerMULS()
	registerDIVU()
	registerDIVS()
	registerNEG()
	registerNEGX()
	registerCLR()
	registerEXT()
	registerCHK()
}

// sizeEncoding maps the standard 2-bit size field (bits 7-6) to Size.
func sizeEncoding(bits uint16) size {
	switch bits {
	case 0:
		return sizeByte
	case 1:
		return sizeWord
	case 2:
		return sizeLong
	}
	return 0
}

// --- ADD ---

// registerADD registers ADD <ea>,Dn and ADD Dn,<ea>.
// Encoding: 1101 DDD O SS eee eee
//
//	O=0: <ea>+Dn->Dn  O=1: Dn+<ea>-><ea>
func registerADD() {
	for dn := uint16(0); dn < 8; dn++ {
		for szBits := uint16(0); szBits < 3; szBits++ {
			// Direction 0: <ea>,Dn (all source EAs)
			for mode := uint16(0); mode < 8; mode++ {
				for reg := uint16(0); reg < 8; reg++ {
					if mode == 7 && reg > 4 {
						continue
					}
					// An direct only valid for Word/Long
					if mode == 1 && szBits == 0 {
						continue
					}
					opcode := 0xD000 | dn<<9 | szBits<<6 | mode<<3 | reg
					opcodeTable[opcode] = makeADDtoReg(dn, mode, reg)
				}
			}
			// Direction 1: Dn,<ea> (memory alterable only)
			for mode := uint16(2); mode < 8; mode++ {
				for reg := uint16(0); reg < 8; reg++ {
					if mode == 7 && reg > 1 {
						continue
					}
					opcode := 0xD000 | dn<<9 | (szBits+4)<<6 | mode<<3 | reg
					opcodeTable[opcode] = makeADDtoEA(dn, mode, reg)
				}
			}
		}
	}
}

func makeADDtoReg(dn, mode, reg uint16) opFunc {
	read := makeEARead(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	isMem := mode >= 2 && !(mode == 7 && reg == 4)
	return func(c *CPU) {
		sz := sizeEncoding((c.ir >> 6) & 3)
		s := read(c, sz)
		d := c.reg.D[dn] & sz.Mask()
		result := s + d
		c.setFlagsAdd(s, d, result, sz)
		mask := sz.Mask()
		c.reg.D[dn] = (c.reg.D[dn] & ^mask) | (result & mask)
		if sz != sizeLong {
			c.cycles += 4 + eaBase
		} else if isMem {
			c.cycles += 6 + eaBase + eaLong
		} else {
			c.cycles += 8 + eaBase + eaLong
		}
	}
}

func makeADDtoEA(dn, mode, reg uint16) opFunc {
	addr := makeEAMemAddr(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	return func(c *CPU) {
		sz := sizeEncoding(((c.ir >> 6) & 7) - 4)
		a := addr(c, sz)
		d := c.readBus(sz, a)
		s := c.reg.D[dn] & sz.Mask()
		result := s + d
		c.setFlagsAdd(s, d, result, sz)
		c.writeBus(sz, a, result)
		if sz == sizeLong {
			c.cycles += 12 + eaBase + eaLong
		} else {
			c.cycles += 8 + eaBase
		}
	}
}

// --- ADDA ---

func registerADDA() {
	for an := uint16(0); an < 8; an++ {
		for _, szBit := range []uint16{3, 7} { // 3=Word, 7=Long
			for mode := uint16(0); mode < 8; mode++ {
				for reg := uint16(0); reg < 8; reg++ {
					if mode == 7 && reg > 4 {
						continue
					}
					opcode := 0xD000 | an<<9 | szBit<<6 | mode<<3 | reg
					opcodeTable[opcode] = makeADDA(an, mode, reg)
				}
			}
		}
	}
}

func makeADDA(an, mode, reg uint16) opFunc {
	read := makeEARead(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	isMem := mode >= 2 && !(mode == 7 && reg == 4)
	return func(c *CPU) {
		sz := sizeWord
		if (c.ir>>6)&7 == 7 {
			sz = sizeLong
		}
		val := read(c, sz)
		if sz == sizeWord {
			val = uint32(int32(int16(val)))
		}
		c.reg.A[an] += val
		// ADDA does not affect condition codes
		if sz == sizeLong && isMem {
			c.cycles += 6 + eaBase + eaLong
		} else {
			c.cycles += 8 + eaBase
			if sz == sizeLong {
				c.cycles += eaLong
			}
		}
	}
}

// --- ADDI ---

func registerADDI() {
	for szBits := uint16(0); szBits < 3; szBits++ {
		for mode := uint16(0); mode < 8; mode++ {
			if mode == 1 {
				continue
			}
			for reg := uint16(0); reg < 8; reg++ {
				if mode == 7 && reg > 1 {
					continue
				}
				opcode := 0x0600 | szBits<<6 | mode<<3 | reg
				opcodeTable[opcode] = makeADDI(mode, reg)
			}
		}
	}
}

func makeADDI(mode, reg uint16) opFunc {
	if mode == 0 {
		return func(c *CPU) {
			sz := sizeEncoding((c.ir >> 6) & 3)
			var imm uint32
			if sz == sizeLong {
				imm = c.fetchPCLong()
			} else {
				imm = uint32(c.fetchPC()) & sz.Mask()
			}
			d := c.reg.D[reg] & sz.Mask()
			result := imm + d
			c.setFlagsAdd(imm, d, result, sz)
			mask := sz.Mask()
			c.reg.D[reg] = (c.reg.D[reg] & ^mask) | (result & mask)
			if sz == sizeLong {
				c.cycles += 16
			} else {
				c.cycles += 8
			}
		}
	}
	addr := makeEAMemAddr(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	return func(c *CPU) {
		sz := sizeEncoding((c.ir >> 6) & 3)
		var imm uint32
		if sz == sizeLong {
			imm = c.fetchPCLong()
		} else {
			imm = uint32(c.fetchPC()) & sz.Mask()
		}
		a := addr(c, sz)
		d := c.readBus(sz, a)
		result := imm + d
		c.setFlagsAdd(imm, d, result, sz)
		c.writeBus(sz, a, result)
		if sz == sizeLong {
			c.cycles += 20 + eaBase + eaLong
		} else {
			c.cycles += 12 + eaBase
		}
	}
}

// --- ADDQ ---

func registerADDQ() {
	for data := uint16(0); data < 8; data++ {
		for szBits := uint16(0); szBits < 3; szBits++ {
			for mode := uint16(0); mode < 8; mode++ {
				for reg := uint16(0); reg < 8; reg++ {
					if mode == 7 && reg > 1 {
						continue
					}
					// Byte size not valid for An
					if mode == 1 && szBits == 0 {
						continue
					}
					opcode := 0x5000 | data<<9 | szBits<<6 | mode<<3 | reg
					opcodeTable[opcode] = makeADDQ(data, mode, reg)
				}
			}
		}
	}
}

func makeADDQ(data, mode, reg uint16) opFunc {
	imm := uint32(data)
	if imm == 0 {
		imm = 8
	}
	if mode == 0 {
		return func(c *CPU) {
			sz := sizeEncoding((c.ir >> 6) & 3)
			d := c.reg.D[reg] & sz.Mask()
			result := imm + d
			c.setFlagsAdd(imm, d, result, sz)
			mask := sz.Mask()
			c.reg.D[reg] = (c.reg.D[reg] & ^mask) | (result & mask)
			if sz == sizeLong {
				c.cycles += 8
			} else {
				c.cycles += 4
			}
		}
	}
	if mode == 1 {
		return func(c *CPU) {
			// ADDQ to An: always 32-bit, no flags
			c.reg.A[reg] += imm
			c.cycles += 8
		}
	}
	addr := makeEAMemAddr(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	return func(c *CPU) {
		sz := sizeEncoding((c.ir >> 6) & 3)
		a := addr(c, sz)
		d := c.readBus(sz, a)
		result := imm + d
		c.setFlagsAdd(imm, d, result, sz)
		c.writeBus(sz, a, result)
		if sz == sizeLong {
			c.cycles += 12 + eaBase + eaLong
		} else {
			c.cycles += 8 + eaBase
		}
	}
}

// --- ADDX ---

func registerADDX() {
	for rx := uint16(0); rx < 8; rx++ {
		for ry := uint16(0); ry < 8; ry++ {
			for szBits := uint16(0); szBits < 3; szBits++ {
				// Dn,Dn
				opcodeTable[0xD100|rx<<9|szBits<<6|ry] = opADDXreg
				// -(Ax),-(Ay)
				opcodeTable[0xD108|rx<<9|szBits<<6|ry] = opADDXmem
			}
		}
	}
}

func opADDXreg(c *CPU) {
	rx := (c.ir >> 9) & 7
	sz := sizeEncoding((c.ir >> 6) & 3)
	ry := c.ir & 7

	s := c.reg.D[ry] & sz.Mask()
	d := c.reg.D[rx] & sz.Mask()
	x := uint32(0)
	if c.reg.SR&flagX != 0 {
		x = 1
	}
	result := d + s + x

	oldZ := c.reg.SR & flagZ
	c.setFlagsAdd(s, d, result, sz)
	// ADDX: Z flag only cleared, never set (preserves Z across multi-precision)
	if result&sz.Mask() == 0 {
		c.reg.SR = (c.reg.SR &^ flagZ) | oldZ
	}

	mask := sz.Mask()
	c.reg.D[rx] = (c.reg.D[rx] & ^mask) | (result & mask)

	c.cycles += 4
	if sz == sizeLong {
		c.cycles += 4
	}
}

func opADDXmem(c *CPU) {
	rx := (c.ir >> 9) & 7
	sz := sizeEncoding((c.ir >> 6) & 3)
	ry := c.ir & 7

	src := c.resolveEA(4, uint8(ry), sz) // -(Ay)
	s := src.read(c, sz)
	dst := c.resolveEA(4, uint8(rx), sz) // -(Ax)
	d := dst.read(c, sz)
	x := uint32(0)
	if c.reg.SR&flagX != 0 {
		x = 1
	}
	result := d + s + x

	oldZ := c.reg.SR & flagZ
	c.setFlagsAdd(s, d, result, sz)
	if result&sz.Mask() == 0 {
		c.reg.SR = (c.reg.SR &^ flagZ) | oldZ
	}

	dst.write(c, sz, result)
	if sz == sizeLong {
		c.cycles += 30
	} else {
		c.cycles += 18
	}
}

// --- SUB ---

func registerSUB() {
	for dn := uint16(0); dn < 8; dn++ {
		for szBits := uint16(0); szBits < 3; szBits++ {
			// <ea>,Dn
			for mode := uint16(0); mode < 8; mode++ {
				for reg := uint16(0); reg < 8; reg++ {
					if mode == 7 && reg > 4 {
						continue
					}
					if mode == 1 && szBits == 0 {
						continue
					}
					opcode := 0x9000 | dn<<9 | szBits<<6 | mode<<3 | reg
					opcodeTable[opcode] = makeSUBtoReg(dn, mode, reg)
				}
			}
			// Dn,<ea>
			for mode := uint16(2); mode < 8; mode++ {
				for reg := uint16(0); reg < 8; reg++ {
					if mode == 7 && reg > 1 {
						continue
					}
					opcode := 0x9000 | dn<<9 | (szBits+4)<<6 | mode<<3 | reg
					opcodeTable[opcode] = makeSUBtoEA(dn, mode, reg)
				}
			}
		}
	}
}

func makeSUBtoReg(dn, mode, reg uint16) opFunc {
	read := makeEARead(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	isMem := mode >= 2 && !(mode == 7 && reg == 4)
	return func(c *CPU) {
		sz := sizeEncoding((c.ir >> 6) & 3)
		s := read(c, sz)
		d := c.reg.D[dn] & sz.Mask()
		result := d - s
		c.setFlagsSub(s, d, result, sz)
		mask := sz.Mask()
		c.reg.D[dn] = (c.reg.D[dn] & ^mask) | (result & mask)
		if sz != sizeLong {
			c.cycles += 4 + eaBase
		} else if isMem {
			c.cycles += 6 + eaBase + eaLong
		} else {
			c.cycles += 8 + eaBase + eaLong
		}
	}
}

func makeSUBtoEA(dn, mode, reg uint16) opFunc {
	addr := makeEAMemAddr(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	return func(c *CPU) {
		sz := sizeEncoding(((c.ir >> 6) & 7) - 4)
		a := addr(c, sz)
		d := c.readBus(sz, a)
		s := c.reg.D[dn] & sz.Mask()
		result := d - s
		c.setFlagsSub(s, d, result, sz)
		c.writeBus(sz, a, result)
		if sz == sizeLong {
			c.cycles += 12 + eaBase + eaLong
		} else {
			c.cycles += 8 + eaBase
		}
	}
}

// --- SUBA ---

func registerSUBA() {
	for an := uint16(0); an < 8; an++ {
		for _, szBit := range []uint16{3, 7} {
			for mode := uint16(0); mode < 8; mode++ {
				for reg := uint16(0); reg < 8; reg++ {
					if mode == 7 && reg > 4 {
						continue
					}
					opcode := 0x9000 | an<<9 | szBit<<6 | mode<<3 | reg
					opcodeTable[opcode] = makeSUBA(an, mode, reg)
				}
			}
		}
	}
}

func makeSUBA(an, mode, reg uint16) opFunc {
	read := makeEARead(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	isMem := mode >= 2 && !(mode == 7 && reg == 4)
	return func(c *CPU) {
		sz := sizeWord
		if (c.ir>>6)&7 == 7 {
			sz = sizeLong
		}
		val := read(c, sz)
		if sz == sizeWord {
			val = uint32(int32(int16(val)))
		}
		c.reg.A[an] -= val
		if sz == sizeLong && isMem {
			c.cycles += 6 + eaBase + eaLong
		} else {
			c.cycles += 8 + eaBase
			if sz == sizeLong {
				c.cycles += eaLong
			}
		}
	}
}

// --- SUBI ---

func registerSUBI() {
	for szBits := uint16(0); szBits < 3; szBits++ {
		for mode := uint16(0); mode < 8; mode++ {
			if mode == 1 {
				continue
			}
			for reg := uint16(0); reg < 8; reg++ {
				if mode == 7 && reg > 1 {
					continue
				}
				opcode := 0x0400 | szBits<<6 | mode<<3 | reg
				opcodeTable[opcode] = makeSUBI(mode, reg)
			}
		}
	}
}

func makeSUBI(mode, reg uint16) opFunc {
	if mode == 0 {
		return func(c *CPU) {
			sz := sizeEncoding((c.ir >> 6) & 3)
			var imm uint32
			if sz == sizeLong {
				imm = c.fetchPCLong()
			} else {
				imm = uint32(c.fetchPC()) & sz.Mask()
			}
			d := c.reg.D[reg] & sz.Mask()
			result := d - imm
			c.setFlagsSub(imm, d, result, sz)
			mask := sz.Mask()
			c.reg.D[reg] = (c.reg.D[reg] & ^mask) | (result & mask)
			if sz == sizeLong {
				c.cycles += 16
			} else {
				c.cycles += 8
			}
		}
	}
	addr := makeEAMemAddr(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	return func(c *CPU) {
		sz := sizeEncoding((c.ir >> 6) & 3)
		var imm uint32
		if sz == sizeLong {
			imm = c.fetchPCLong()
		} else {
			imm = uint32(c.fetchPC()) & sz.Mask()
		}
		a := addr(c, sz)
		d := c.readBus(sz, a)
		result := d - imm
		c.setFlagsSub(imm, d, result, sz)
		c.writeBus(sz, a, result)
		if sz == sizeLong {
			c.cycles += 20 + eaBase + eaLong
		} else {
			c.cycles += 12 + eaBase
		}
	}
}

// --- SUBQ ---

func registerSUBQ() {
	for data := uint16(0); data < 8; data++ {
		for szBits := uint16(0); szBits < 3; szBits++ {
			for mode := uint16(0); mode < 8; mode++ {
				for reg := uint16(0); reg < 8; reg++ {
					if mode == 7 && reg > 1 {
						continue
					}
					if mode == 1 && szBits == 0 {
						continue
					}
					opcode := 0x5100 | data<<9 | szBits<<6 | mode<<3 | reg
					opcodeTable[opcode] = makeSUBQ(data, mode, reg)
				}
			}
		}
	}
}

func makeSUBQ(data, mode, reg uint16) opFunc {
	imm := uint32(data)
	if imm == 0 {
		imm = 8
	}
	if mode == 0 {
		return func(c *CPU) {
			sz := sizeEncoding((c.ir >> 6) & 3)
			d := c.reg.D[reg] & sz.Mask()
			result := d - imm
			c.setFlagsSub(imm, d, result, sz)
			mask := sz.Mask()
			c.reg.D[reg] = (c.reg.D[reg] & ^mask) | (result & mask)
			if sz == sizeLong {
				c.cycles += 8
			} else {
				c.cycles += 4
			}
		}
	}
	if mode == 1 {
		return func(c *CPU) {
			c.reg.A[reg] -= imm
			c.cycles += 8
		}
	}
	addr := makeEAMemAddr(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	return func(c *CPU) {
		sz := sizeEncoding((c.ir >> 6) & 3)
		a := addr(c, sz)
		d := c.readBus(sz, a)
		result := d - imm
		c.setFlagsSub(imm, d, result, sz)
		c.writeBus(sz, a, result)
		if sz == sizeLong {
			c.cycles += 12 + eaBase + eaLong
		} else {
			c.cycles += 8 + eaBase
		}
	}
}

// --- SUBX ---

func registerSUBX() {
	for rx := uint16(0); rx < 8; rx++ {
		for ry := uint16(0); ry < 8; ry++ {
			for szBits := uint16(0); szBits < 3; szBits++ {
				opcodeTable[0x9100|rx<<9|szBits<<6|ry] = opSUBXreg
				opcodeTable[0x9108|rx<<9|szBits<<6|ry] = opSUBXmem
			}
		}
	}
}

func opSUBXreg(c *CPU) {
	rx := (c.ir >> 9) & 7
	sz := sizeEncoding((c.ir >> 6) & 3)
	ry := c.ir & 7

	s := c.reg.D[ry] & sz.Mask()
	d := c.reg.D[rx] & sz.Mask()
	x := uint32(0)
	if c.reg.SR&flagX != 0 {
		x = 1
	}
	result := d - s - x

	oldZ := c.reg.SR & flagZ
	c.setFlagsSub(s, d, result, sz)
	// SUBX: Z flag only cleared, never set (preserves Z across multi-precision)
	if result&sz.Mask() == 0 {
		c.reg.SR = (c.reg.SR &^ flagZ) | oldZ
	}

	mask := sz.Mask()
	c.reg.D[rx] = (c.reg.D[rx] & ^mask) | (result & mask)

	c.cycles += 4
	if sz == sizeLong {
		c.cycles += 4
	}
}

func opSUBXmem(c *CPU) {
	rx := (c.ir >> 9) & 7
	sz := sizeEncoding((c.ir >> 6) & 3)
	ry := c.ir & 7

	src := c.resolveEA(4, uint8(ry), sz)
	s := src.read(c, sz)
	dst := c.resolveEA(4, uint8(rx), sz)
	d := dst.read(c, sz)
	x := uint32(0)
	if c.reg.SR&flagX != 0 {
		x = 1
	}
	result := d - s - x

	oldZ := c.reg.SR & flagZ
	c.setFlagsSub(s, d, result, sz)
	if result&sz.Mask() == 0 {
		c.reg.SR = (c.reg.SR &^ flagZ) | oldZ
	}

	dst.write(c, sz, result)
	if sz == sizeLong {
		c.cycles += 30
	} else {
		c.cycles += 18
	}
}

// --- CMP ---

func registerCMP() {
	for dn := uint16(0); dn < 8; dn++ {
		for szBits := uint16(0); szBits < 3; szBits++ {
			for mode := uint16(0); mode < 8; mode++ {
				for reg := uint16(0); reg < 8; reg++ {
					if mode == 7 && reg > 4 {
						continue
					}
					if mode == 1 && szBits == 0 {
						continue
					}
					opcode := 0xB000 | dn<<9 | szBits<<6 | mode<<3 | reg
					opcodeTable[opcode] = makeCMP(dn, mode, reg)
				}
			}
		}
	}
}

func makeCMP(dn, mode, reg uint16) opFunc {
	read := makeEARead(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	return func(c *CPU) {
		sz := sizeEncoding((c.ir >> 6) & 3)
		s := read(c, sz)
		d := c.reg.D[dn] & sz.Mask()
		result := d - s
		c.setFlagsCmp(s, d, result, sz)
		if sz == sizeLong {
			c.cycles += 6 + eaBase + eaLong
		} else {
			c.cycles += 4 + eaBase
		}
	}
}

// --- CMPA ---

func registerCMPA() {
	for an := uint16(0); an < 8; an++ {
		for _, szBit := range []uint16{3, 7} {
			for mode := uint16(0); mode < 8; mode++ {
				for reg := uint16(0); reg < 8; reg++ {
					if mode == 7 && reg > 4 {
						continue
					}
					opcode := 0xB000 | an<<9 | szBit<<6 | mode<<3 | reg
					opcodeTable[opcode] = makeCMPA(an, mode, reg)
				}
			}
		}
	}
}

func makeCMPA(an, mode, reg uint16) opFunc {
	read := makeEARead(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	return func(c *CPU) {
		sz := sizeWord
		if (c.ir>>6)&7 == 7 {
			sz = sizeLong
		}
		val := read(c, sz)
		if sz == sizeWord {
			val = uint32(int32(int16(val)))
		}
		d := c.reg.A[an]
		result := d - val
		c.setFlagsCmp(val, d, result, sizeLong)
		c.cycles += 6 + eaBase
		if sz == sizeLong {
			c.cycles += eaLong
		}
	}
}

// --- CMPI ---

func registerCMPI() {
	for szBits := uint16(0); szBits < 3; szBits++ {
		for mode := uint16(0); mode < 8; mode++ {
			if mode == 1 {
				continue
			}
			for reg := uint16(0); reg < 8; reg++ {
				if mode == 7 && reg > 1 {
					continue
				}
				opcode := 0x0C00 | szBits<<6 | mode<<3 | reg
				opcodeTable[opcode] = makeCMPI(mode, reg)
			}
		}
	}
}

func makeCMPI(mode, reg uint16) opFunc {
	if mode == 0 {
		return func(c *CPU) {
			sz := sizeEncoding((c.ir >> 6) & 3)
			var imm uint32
			if sz == sizeLong {
				imm = c.fetchPCLong()
			} else {
				imm = uint32(c.fetchPC()) & sz.Mask()
			}
			d := c.reg.D[reg] & sz.Mask()
			result := d - imm
			c.setFlagsCmp(imm, d, result, sz)
			if sz == sizeLong {
				c.cycles += 14
			} else {
				c.cycles += 8
			}
		}
	}
	addr := makeEAMemAddr(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	return func(c *CPU) {
		sz := sizeEncoding((c.ir >> 6) & 3)
		var imm uint32
		if sz == sizeLong {
			imm = c.fetchPCLong()
		} else {
			imm = uint32(c.fetchPC()) & sz.Mask()
		}
		a := addr(c, sz)
		d := c.readBus(sz, a)
		result := d - imm
		c.setFlagsCmp(imm, d, result, sz)
		if sz == sizeLong {
			c.cycles += 12 + eaBase + eaLong
		} else {
			c.cycles += 8 + eaBase
		}
	}
}

// --- CMPM ---

func registerCMPM() {
	for ax := uint16(0); ax < 8; ax++ {
		for ay := uint16(0); ay < 8; ay++ {
			for szBits := uint16(0); szBits < 3; szBits++ {
				opcode := 0xB108 | ax<<9 | szBits<<6 | ay
				opcodeTable[opcode] = opCMPM
			}
		}
	}
}

func opCMPM(c *CPU) {
	sz := sizeEncoding((c.ir >> 6) & 3)
	ay := c.ir & 7
	ax := (c.ir >> 9) & 7

	src := c.resolveEA(3, uint8(ay), sz) // (Ay)+
	s := src.read(c, sz)
	dst := c.resolveEA(3, uint8(ax), sz) // (Ax)+
	d := dst.read(c, sz)
	result := d - s
	c.setFlagsCmp(s, d, result, sz)

	if sz == sizeLong {
		c.cycles += 20
	} else {
		c.cycles += 12
	}
}

// --- MULU ---

func registerMULU() {
	for dn := uint16(0); dn < 8; dn++ {
		for mode := uint16(0); mode < 8; mode++ {
			if mode == 1 {
				continue
			}
			for reg := uint16(0); reg < 8; reg++ {
				if mode == 7 && reg > 4 {
					continue
				}
				opcode := 0xC0C0 | dn<<9 | mode<<3 | reg
				opcodeTable[opcode] = makeMULU(dn, mode, reg)
			}
		}
	}
}

func makeMULU(dn, mode, reg uint16) opFunc {
	read := makeEARead(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	return func(c *CPU) {
		s := read(c, sizeWord)
		d := c.reg.D[dn] & 0xFFFF
		result := s * d
		c.reg.D[dn] = result
		c.setFlagsLogical(result, sizeLong)
		c.cycles += 70 + eaBase
		if sizeWord == sizeLong {
			c.cycles += eaLong
		}
	}
}

// --- MULS ---

func registerMULS() {
	for dn := uint16(0); dn < 8; dn++ {
		for mode := uint16(0); mode < 8; mode++ {
			if mode == 1 {
				continue
			}
			for reg := uint16(0); reg < 8; reg++ {
				if mode == 7 && reg > 4 {
					continue
				}
				opcode := 0xC1C0 | dn<<9 | mode<<3 | reg
				opcodeTable[opcode] = makeMULS(dn, mode, reg)
			}
		}
	}
}

func makeMULS(dn, mode, reg uint16) opFunc {
	read := makeEARead(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	return func(c *CPU) {
		s := int32(int16(read(c, sizeWord)))
		d := int32(int16(c.reg.D[dn] & 0xFFFF))
		result := uint32(s * d)
		c.reg.D[dn] = result
		c.setFlagsLogical(result, sizeLong)
		c.cycles += 70 + eaBase
		if sizeWord == sizeLong {
			c.cycles += eaLong
		}
	}
}

// --- DIVU ---

func registerDIVU() {
	for dn := uint16(0); dn < 8; dn++ {
		for mode := uint16(0); mode < 8; mode++ {
			if mode == 1 {
				continue
			}
			for reg := uint16(0); reg < 8; reg++ {
				if mode == 7 && reg > 4 {
					continue
				}
				opcode := 0x80C0 | dn<<9 | mode<<3 | reg
				opcodeTable[opcode] = makeDIVU(dn, mode, reg)
			}
		}
	}
}

func makeDIVU(dn, mode, reg uint16) opFunc {
	read := makeEARead(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	return func(c *CPU) {
		divisor := read(c, sizeWord)
		if divisor == 0 {
			c.exception(vecDivideByZero)
			return
		}
		dividend := c.reg.D[dn]
		quotient := dividend / divisor
		remainder := dividend % divisor
		if quotient > 0xFFFF {
			c.reg.SR |= flagV | flagN
			c.reg.SR &^= flagC | flagZ
		} else {
			c.reg.D[dn] = (remainder&0xFFFF)<<16 | (quotient & 0xFFFF)
			c.setFlagsLogical(quotient, sizeWord)
		}
		c.cycles += 140 + eaBase
		if sizeWord == sizeLong {
			c.cycles += eaLong
		}
	}
}

// --- DIVS ---

func registerDIVS() {
	for dn := uint16(0); dn < 8; dn++ {
		for mode := uint16(0); mode < 8; mode++ {
			if mode == 1 {
				continue
			}
			for reg := uint16(0); reg < 8; reg++ {
				if mode == 7 && reg > 4 {
					continue
				}
				opcode := 0x81C0 | dn<<9 | mode<<3 | reg
				opcodeTable[opcode] = makeDIVS(dn, mode, reg)
			}
		}
	}
}

func makeDIVS(dn, mode, reg uint16) opFunc {
	read := makeEARead(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	return func(c *CPU) {
		divisor := int32(int16(read(c, sizeWord)))
		if divisor == 0 {
			c.exception(vecDivideByZero)
			return
		}
		dividend := int32(c.reg.D[dn])
		quotient := dividend / divisor
		remainder := dividend % divisor
		if quotient > 32767 || quotient < -32768 {
			c.reg.SR |= flagV | flagN
			c.reg.SR &^= flagC | flagZ
		} else {
			c.reg.D[dn] = uint32(remainder&0xFFFF)<<16 | uint32(quotient)&0xFFFF
			c.setFlagsLogical(uint32(quotient), sizeWord)
		}
		c.cycles += 158 + eaBase
		if sizeWord == sizeLong {
			c.cycles += eaLong
		}
	}
}

// --- NEG ---

func registerNEG() {
	for szBits := uint16(0); szBits < 3; szBits++ {
		for mode := uint16(0); mode < 8; mode++ {
			if mode == 1 {
				continue
			}
			for reg := uint16(0); reg < 8; reg++ {
				if mode == 7 && reg > 1 {
					continue
				}
				opcode := 0x4400 | szBits<<6 | mode<<3 | reg
				opcodeTable[opcode] = makeNEG(mode, reg)
			}
		}
	}
}

func makeNEG(mode, reg uint16) opFunc {
	if mode == 0 {
		return func(c *CPU) {
			sz := sizeEncoding((c.ir >> 6) & 3)
			d := c.reg.D[reg] & sz.Mask()
			result := uint32(0) - d
			c.setFlagsSub(d, 0, result, sz)
			mask := sz.Mask()
			c.reg.D[reg] = (c.reg.D[reg] & ^mask) | (result & mask)
			if sz == sizeLong {
				c.cycles += 6
			} else {
				c.cycles += 4
			}
		}
	}
	addr := makeEAMemAddr(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	return func(c *CPU) {
		sz := sizeEncoding((c.ir >> 6) & 3)
		a := addr(c, sz)
		d := c.readBus(sz, a)
		result := uint32(0) - d
		c.setFlagsSub(d, 0, result, sz)
		c.writeBus(sz, a, result)
		if sz == sizeLong {
			c.cycles += 12 + eaBase + eaLong
		} else {
			c.cycles += 8 + eaBase
		}
	}
}

// --- NEGX ---

func registerNEGX() {
	for szBits := uint16(0); szBits < 3; szBits++ {
		for mode := uint16(0); mode < 8; mode++ {
			if mode == 1 {
				continue
			}
			for reg := uint16(0); reg < 8; reg++ {
				if mode == 7 && reg > 1 {
					continue
				}
				opcode := 0x4000 | szBits<<6 | mode<<3 | reg
				opcodeTable[opcode] = makeNEGX(mode, reg)
			}
		}
	}
}

func makeNEGX(mode, reg uint16) opFunc {
	if mode == 0 {
		return func(c *CPU) {
			sz := sizeEncoding((c.ir >> 6) & 3)
			d := c.reg.D[reg] & sz.Mask()
			x := uint32(0)
			if c.reg.SR&flagX != 0 {
				x = 1
			}
			result := uint32(0) - d - x
			oldZ := c.reg.SR & flagZ
			c.setFlagsSub(d, 0, result, sz)
			if result&sz.Mask() == 0 {
				c.reg.SR = (c.reg.SR &^ flagZ) | oldZ
			}
			mask := sz.Mask()
			c.reg.D[reg] = (c.reg.D[reg] & ^mask) | (result & mask)
			if sz == sizeLong {
				c.cycles += 6
			} else {
				c.cycles += 4
			}
		}
	}
	addr := makeEAMemAddr(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	return func(c *CPU) {
		sz := sizeEncoding((c.ir >> 6) & 3)
		a := addr(c, sz)
		d := c.readBus(sz, a)
		x := uint32(0)
		if c.reg.SR&flagX != 0 {
			x = 1
		}
		result := uint32(0) - d - x
		oldZ := c.reg.SR & flagZ
		c.setFlagsSub(d, 0, result, sz)
		if result&sz.Mask() == 0 {
			c.reg.SR = (c.reg.SR &^ flagZ) | oldZ
		}
		c.writeBus(sz, a, result)
		if sz == sizeLong {
			c.cycles += 12 + eaBase + eaLong
		} else {
			c.cycles += 8 + eaBase
		}
	}
}

// --- CLR ---

func registerCLR() {
	for szBits := uint16(0); szBits < 3; szBits++ {
		for mode := uint16(0); mode < 8; mode++ {
			if mode == 1 {
				continue
			}
			for reg := uint16(0); reg < 8; reg++ {
				if mode == 7 && reg > 1 {
					continue
				}
				opcode := 0x4200 | szBits<<6 | mode<<3 | reg
				opcodeTable[opcode] = makeCLR(mode, reg)
			}
		}
	}
}

func makeCLR(mode, reg uint16) opFunc {
	if mode == 0 {
		return func(c *CPU) {
			sz := sizeEncoding((c.ir >> 6) & 3)
			mask := sz.Mask()
			c.reg.D[reg] = c.reg.D[reg] & ^mask
			c.reg.SR &^= flagN | flagV | flagC
			c.reg.SR |= flagZ
			if sz == sizeLong {
				c.cycles += 6
			} else {
				c.cycles += 4
			}
		}
	}
	addr := makeEAMemAddr(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	return func(c *CPU) {
		sz := sizeEncoding((c.ir >> 6) & 3)
		a := addr(c, sz)
		c.writeBus(sz, a, 0)
		c.reg.SR &^= flagN | flagV | flagC
		c.reg.SR |= flagZ
		if sz == sizeLong {
			c.cycles += 12 + eaBase + eaLong
		} else {
			c.cycles += 8 + eaBase
		}
	}
}

// --- EXT ---

func registerEXT() {
	for dn := uint16(0); dn < 8; dn++ {
		// EXT.W (byte->word): opmode 010
		opcodeTable[0x4880|dn] = opEXTW
		// EXT.L (word->long): opmode 011
		opcodeTable[0x48C0|dn] = opEXTL
	}
}

func opEXTW(c *CPU) {
	dn := c.ir & 7
	val := uint32(int16(int8(c.reg.D[dn])))
	c.reg.D[dn] = (c.reg.D[dn] & 0xFFFF0000) | (val & 0xFFFF)
	c.setFlagsLogical(val, sizeWord)
	c.cycles += 4
}

func opEXTL(c *CPU) {
	dn := c.ir & 7
	val := uint32(int32(int16(c.reg.D[dn])))
	c.reg.D[dn] = val
	c.setFlagsLogical(val, sizeLong)
	c.cycles += 4
}

// --- CHK ---

// registerCHK registers CHK <ea>,Dn (word only on 68000).
// Encoding: 0100 DDD 110 MMM RRR
func registerCHK() {
	for dn := uint16(0); dn < 8; dn++ {
		for mode := uint16(0); mode < 8; mode++ {
			if mode == 1 {
				continue
			}
			for reg := uint16(0); reg < 8; reg++ {
				if mode == 7 && reg > 4 {
					continue
				}
				opcode := 0x4180 | dn<<9 | mode<<3 | reg
				opcodeTable[opcode] = makeCHK(dn, mode, reg)
			}
		}
	}
}

func makeCHK(dn, mode, reg uint16) opFunc {
	read := makeEARead(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	return func(c *CPU) {
		bound := int16(read(c, sizeWord))
		val := int16(c.reg.D[dn] & 0xFFFF)
		if val < 0 {
			c.reg.SR &^= flagN | flagZ | flagV | flagC
			c.reg.SR |= flagN
			c.exception(vecCHK)
			return
		}
		if val > bound {
			c.reg.SR &^= flagN | flagZ | flagV | flagC
			c.exception(vecCHK)
			return
		}
		c.setFlagsCmp(uint32(val), uint32(bound), uint32(bound-val), sizeWord)
		c.cycles += 10 + eaBase
		if sizeWord == sizeLong {
			c.cycles += eaLong
		}
	}
}
