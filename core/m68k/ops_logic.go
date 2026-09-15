package m68k

func init() {
	registerAND()
	registerANDI()
	registerOR()
	registerORI()
	registerEOR()
	registerEORI()
	registerNOT()
	registerTST()
	registerTAS()
	registerShifts()
}

// --- AND ---

func registerAND() {
	for dn := uint16(0); dn < 8; dn++ {
		for szBits := uint16(0); szBits < 3; szBits++ {
			// <ea> AND Dn -> Dn
			for mode := uint16(0); mode < 8; mode++ {
				if mode == 1 {
					continue
				}
				for reg := uint16(0); reg < 8; reg++ {
					if mode == 7 && reg > 4 {
						continue
					}
					opcode := 0xC000 | dn<<9 | szBits<<6 | mode<<3 | reg
					opcodeTable[opcode] = makeANDtoReg(dn, mode, reg)
				}
			}
			// Dn AND <ea> -> <ea>
			for mode := uint16(2); mode < 8; mode++ {
				for reg := uint16(0); reg < 8; reg++ {
					if mode == 7 && reg > 1 {
						continue
					}
					opcode := 0xC000 | dn<<9 | (szBits+4)<<6 | mode<<3 | reg
					opcodeTable[opcode] = makeANDtoEA(dn, mode, reg)
				}
			}
		}
	}
}

func makeANDtoReg(dn, mode, reg uint16) opFunc {
	read := makeEARead(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	isMem := mode >= 2 && !(mode == 7 && reg == 4)
	return func(c *CPU) {
		sz := sizeEncoding((c.ir >> 6) & 3)
		result := read(c, sz) & (c.reg.D[dn] & sz.Mask())
		c.setFlagsLogical(result, sz)
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

func makeANDtoEA(dn, mode, reg uint16) opFunc {
	addr := makeEAMemAddr(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	return func(c *CPU) {
		sz := sizeEncoding(((c.ir >> 6) & 7) - 4)
		a := addr(c, sz)
		result := c.readBus(sz, a) & (c.reg.D[dn] & sz.Mask())
		c.setFlagsLogical(result, sz)
		c.writeBus(sz, a, result)
		if sz == sizeLong {
			c.cycles += 12 + eaBase + eaLong
		} else {
			c.cycles += 8 + eaBase
		}
	}
}

// --- ANDI ---

func registerANDI() {
	for szBits := uint16(0); szBits < 3; szBits++ {
		for mode := uint16(0); mode < 8; mode++ {
			if mode == 1 {
				continue
			}
			for reg := uint16(0); reg < 8; reg++ {
				if mode == 7 && reg > 1 {
					continue
				}
				opcode := 0x0200 | szBits<<6 | mode<<3 | reg
				opcodeTable[opcode] = makeANDI(mode, reg)
			}
		}
	}
}

func makeANDI(mode, reg uint16) opFunc {
	if mode == 0 {
		return func(c *CPU) {
			sz := sizeEncoding((c.ir >> 6) & 3)
			var imm uint32
			if sz == sizeLong {
				imm = c.fetchPCLong()
			} else {
				imm = uint32(c.fetchPC()) & sz.Mask()
			}
			result := (c.reg.D[reg] & sz.Mask()) & imm
			c.setFlagsLogical(result, sz)
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
		result := c.readBus(sz, a) & imm
		c.setFlagsLogical(result, sz)
		c.writeBus(sz, a, result)
		if sz == sizeLong {
			c.cycles += 20 + eaBase + eaLong
		} else {
			c.cycles += 12 + eaBase
		}
	}
}

// --- OR ---

func registerOR() {
	for dn := uint16(0); dn < 8; dn++ {
		for szBits := uint16(0); szBits < 3; szBits++ {
			for mode := uint16(0); mode < 8; mode++ {
				if mode == 1 {
					continue
				}
				for reg := uint16(0); reg < 8; reg++ {
					if mode == 7 && reg > 4 {
						continue
					}
					opcode := 0x8000 | dn<<9 | szBits<<6 | mode<<3 | reg
					opcodeTable[opcode] = makeORtoReg(dn, mode, reg)
				}
			}
			for mode := uint16(2); mode < 8; mode++ {
				for reg := uint16(0); reg < 8; reg++ {
					if mode == 7 && reg > 1 {
						continue
					}
					opcode := 0x8000 | dn<<9 | (szBits+4)<<6 | mode<<3 | reg
					opcodeTable[opcode] = makeORtoEA(dn, mode, reg)
				}
			}
		}
	}
}

func makeORtoReg(dn, mode, reg uint16) opFunc {
	read := makeEARead(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	isMem := mode >= 2 && !(mode == 7 && reg == 4)
	return func(c *CPU) {
		sz := sizeEncoding((c.ir >> 6) & 3)
		result := read(c, sz) | (c.reg.D[dn] & sz.Mask())
		c.setFlagsLogical(result, sz)
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

func makeORtoEA(dn, mode, reg uint16) opFunc {
	addr := makeEAMemAddr(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	return func(c *CPU) {
		sz := sizeEncoding(((c.ir >> 6) & 7) - 4)
		a := addr(c, sz)
		result := c.readBus(sz, a) | (c.reg.D[dn] & sz.Mask())
		c.setFlagsLogical(result, sz)
		c.writeBus(sz, a, result)
		if sz == sizeLong {
			c.cycles += 12 + eaBase + eaLong
		} else {
			c.cycles += 8 + eaBase
		}
	}
}

// --- ORI ---

func registerORI() {
	for szBits := uint16(0); szBits < 3; szBits++ {
		for mode := uint16(0); mode < 8; mode++ {
			if mode == 1 {
				continue
			}
			for reg := uint16(0); reg < 8; reg++ {
				if mode == 7 && reg > 1 {
					continue
				}
				opcode := 0x0000 | szBits<<6 | mode<<3 | reg
				opcodeTable[opcode] = makeORI(mode, reg)
			}
		}
	}
}

func makeORI(mode, reg uint16) opFunc {
	if mode == 0 {
		return func(c *CPU) {
			sz := sizeEncoding((c.ir >> 6) & 3)
			var imm uint32
			if sz == sizeLong {
				imm = c.fetchPCLong()
			} else {
				imm = uint32(c.fetchPC()) & sz.Mask()
			}
			result := (c.reg.D[reg] & sz.Mask()) | imm
			c.setFlagsLogical(result, sz)
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
		result := c.readBus(sz, a) | imm
		c.setFlagsLogical(result, sz)
		c.writeBus(sz, a, result)
		if sz == sizeLong {
			c.cycles += 20 + eaBase + eaLong
		} else {
			c.cycles += 12 + eaBase
		}
	}
}

// --- EOR ---

func registerEOR() {
	for dn := uint16(0); dn < 8; dn++ {
		for szBits := uint16(0); szBits < 3; szBits++ {
			for mode := uint16(0); mode < 8; mode++ {
				if mode == 1 {
					continue
				}
				for reg := uint16(0); reg < 8; reg++ {
					if mode == 7 && reg > 1 {
						continue
					}
					opcode := 0xB000 | dn<<9 | (szBits+4)<<6 | mode<<3 | reg
					opcodeTable[opcode] = makeEOR(dn, mode, reg)
				}
			}
		}
	}
}

func makeEOR(dn, mode, reg uint16) opFunc {
	if mode == 0 {
		return func(c *CPU) {
			sz := sizeEncoding(((c.ir >> 6) & 7) - 4)
			result := (c.reg.D[reg] & sz.Mask()) ^ (c.reg.D[dn] & sz.Mask())
			c.setFlagsLogical(result, sz)
			mask := sz.Mask()
			c.reg.D[reg] = (c.reg.D[reg] & ^mask) | (result & mask)
			if sz == sizeLong {
				c.cycles += 8
			} else {
				c.cycles += 4
			}
		}
	}
	addr := makeEAMemAddr(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	return func(c *CPU) {
		sz := sizeEncoding(((c.ir >> 6) & 7) - 4)
		a := addr(c, sz)
		result := c.readBus(sz, a) ^ (c.reg.D[dn] & sz.Mask())
		c.setFlagsLogical(result, sz)
		c.writeBus(sz, a, result)
		if sz == sizeLong {
			c.cycles += 12 + eaBase + eaLong
		} else {
			c.cycles += 8 + eaBase
		}
	}
}

// --- EORI ---

func registerEORI() {
	for szBits := uint16(0); szBits < 3; szBits++ {
		for mode := uint16(0); mode < 8; mode++ {
			if mode == 1 {
				continue
			}
			for reg := uint16(0); reg < 8; reg++ {
				if mode == 7 && reg > 1 {
					continue
				}
				opcode := 0x0A00 | szBits<<6 | mode<<3 | reg
				opcodeTable[opcode] = makeEORI(mode, reg)
			}
		}
	}
}

func makeEORI(mode, reg uint16) opFunc {
	if mode == 0 {
		return func(c *CPU) {
			sz := sizeEncoding((c.ir >> 6) & 3)
			var imm uint32
			if sz == sizeLong {
				imm = c.fetchPCLong()
			} else {
				imm = uint32(c.fetchPC()) & sz.Mask()
			}
			result := (c.reg.D[reg] & sz.Mask()) ^ imm
			c.setFlagsLogical(result, sz)
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
		result := c.readBus(sz, a) ^ imm
		c.setFlagsLogical(result, sz)
		c.writeBus(sz, a, result)
		if sz == sizeLong {
			c.cycles += 20 + eaBase + eaLong
		} else {
			c.cycles += 12 + eaBase
		}
	}
}

// --- NOT ---

func registerNOT() {
	for szBits := uint16(0); szBits < 3; szBits++ {
		for mode := uint16(0); mode < 8; mode++ {
			if mode == 1 {
				continue
			}
			for reg := uint16(0); reg < 8; reg++ {
				if mode == 7 && reg > 1 {
					continue
				}
				opcode := 0x4600 | szBits<<6 | mode<<3 | reg
				opcodeTable[opcode] = makeNOT(mode, reg)
			}
		}
	}
}

func makeNOT(mode, reg uint16) opFunc {
	if mode == 0 {
		return func(c *CPU) {
			sz := sizeEncoding((c.ir >> 6) & 3)
			result := ^c.reg.D[reg] & sz.Mask()
			c.setFlagsLogical(result, sz)
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
		result := ^c.readBus(sz, a) & sz.Mask()
		c.setFlagsLogical(result, sz)
		c.writeBus(sz, a, result)
		if sz == sizeLong {
			c.cycles += 12 + eaBase + eaLong
		} else {
			c.cycles += 8 + eaBase
		}
	}
}

// --- TST ---

func registerTST() {
	for szBits := uint16(0); szBits < 3; szBits++ {
		for mode := uint16(0); mode < 8; mode++ {
			if mode == 1 {
				continue
			}
			for reg := uint16(0); reg < 8; reg++ {
				if mode == 7 && reg > 1 {
					continue
				}
				opcode := 0x4A00 | szBits<<6 | mode<<3 | reg
				opcodeTable[opcode] = makeTST(mode, reg)
			}
		}
	}
}

func makeTST(mode, reg uint16) opFunc {
	if mode == 0 {
		return func(c *CPU) {
			sz := sizeEncoding((c.ir >> 6) & 3)
			c.setFlagsLogical(c.reg.D[reg]&sz.Mask(), sz)
			c.cycles += 4
		}
	}
	read := makeEARead(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	return func(c *CPU) {
		sz := sizeEncoding((c.ir >> 6) & 3)
		val := read(c, sz)
		c.setFlagsLogical(val, sz)
		c.cycles += 4 + eaBase
		if sz == sizeLong {
			c.cycles += eaLong
		}
	}
}

// --- TAS ---

// registerTAS registers TAS <ea>.
// Encoding: 0100 1010 11 MMM RRR
func registerTAS() {
	for mode := uint16(0); mode < 8; mode++ {
		if mode == 1 {
			continue
		}
		for reg := uint16(0); reg < 8; reg++ {
			if mode == 7 && reg > 1 {
				continue
			}
			opcode := 0x4AC0 | mode<<3 | reg
			opcodeTable[opcode] = makeTAS(mode, reg)
		}
	}
}

func makeTAS(mode, reg uint16) opFunc {
	if mode == 0 {
		return func(c *CPU) {
			val := c.reg.D[reg] & 0xFF
			c.setFlagsLogical(val, sizeByte)
			c.reg.D[reg] = (c.reg.D[reg] & 0xFFFFFF00) | (val | 0x80)
			c.cycles += 4
		}
	}
	addr := makeEAMemAddr(mode, reg)
	eaBase, _ := eaFetchConst(mode, reg)
	return func(c *CPU) {
		a := addr(c, sizeByte)
		val := c.readBus(sizeByte, a)
		c.setFlagsLogical(val, sizeByte)
		c.writeBus(sizeByte, a, val|0x80)
		c.cycles += 10 + eaBase
	}
}

// --- Shifts and Rotates ---
// ASL, ASR, LSL, LSR, ROL, ROR, ROXL, ROXR
// Register form: 1110 CCC D SS i TT RRR
//   CCC = count/register, D = direction (0=right, 1=left)
//   SS = size, i = 0:immediate count 1:register count
//   TT = type (00=AS, 01=LS, 10=ROX, 11=RO)
//   RRR = data register
// Memory form: 1110 0TT D 11 eee eee (always word, count=1)

func registerShifts() {
	// Register/immediate forms
	for cnt := uint16(0); cnt < 8; cnt++ {
		for dir := uint16(0); dir < 2; dir++ {
			for szBits := uint16(0); szBits < 3; szBits++ {
				for ir := uint16(0); ir < 2; ir++ { // 0=immediate count, 1=register count
					for typ := uint16(0); typ < 4; typ++ {
						for dreg := uint16(0); dreg < 8; dreg++ {
							opcode := 0xE000 | cnt<<9 | dir<<8 | szBits<<6 | ir<<5 | typ<<3 | dreg
							opcodeTable[opcode] = opShiftReg
						}
					}
				}
			}
		}
	}

	// Memory forms (word only, count=1)
	for dir := uint16(0); dir < 2; dir++ {
		for typ := uint16(0); typ < 4; typ++ {
			for mode := uint16(2); mode < 8; mode++ {
				for reg := uint16(0); reg < 8; reg++ {
					if mode == 7 && reg > 1 {
						continue
					}
					opcode := 0xE0C0 | typ<<9 | dir<<8 | mode<<3 | reg
					opcodeTable[opcode] = makeShiftMem(mode, reg)
				}
			}
		}
	}
}

func opShiftReg(c *CPU) {
	cnt := (c.ir >> 9) & 7
	dir := (c.ir >> 8) & 1 // 0=right, 1=left
	sz := sizeEncoding((c.ir >> 6) & 3)
	ir := (c.ir >> 5) & 1
	typ := (c.ir >> 3) & 3
	dreg := c.ir & 7

	var count uint32
	if ir != 0 {
		count = c.reg.D[cnt] & 63
	} else {
		count = uint32(cnt)
		if count == 0 {
			count = 8
		}
	}

	val := c.reg.D[dreg] & sz.Mask()
	result := doShift(c, val, count, dir, typ, sz)

	mask := sz.Mask()
	c.reg.D[dreg] = (c.reg.D[dreg] & ^mask) | (result & mask)

	c.cycles += 6 + 2*uint64(count)
	if sz == sizeLong {
		c.cycles += 2
	}
}

func makeShiftMem(mode, reg uint16) opFunc {
	addr := makeEAMemAddr(mode, reg)
	eaBase, _ := eaFetchConst(mode, reg)
	return func(c *CPU) {
		dir := (c.ir >> 8) & 1
		typ := (c.ir >> 9) & 3
		a := addr(c, sizeWord)
		val := c.readBus(sizeWord, a)
		result := doShift(c, val, 1, dir, typ, sizeWord)
		c.writeBus(sizeWord, a, result)
		c.cycles += 8 + eaBase
	}
}

// doShift performs the actual shift/rotate operation.
func doShift(c *CPU, val, count uint32, dir, typ uint16, sz size) uint32 {
	msb := sz.MSB()
	mask := sz.Mask()

	if count == 0 {
		c.setFlagsLogical(val, sz)
		if typ == 2 {
			// ROXL/ROXR: C = X when count is 0
			if c.reg.SR&flagX != 0 {
				c.reg.SR |= flagC
			}
		}
		return val
	}

	var result uint32

	switch typ {
	case 0: // Arithmetic shift (AS)
		if dir == 1 { // ASL
			result = val
			c.reg.SR &^= flagV
			for i := uint32(0); i < count; i++ {
				msbit := result & msb
				result = (result << 1) & mask
				if result&msb != msbit {
					c.reg.SR |= flagV
				}
			}
			lastOut := (val >> (sz.Bits() - count)) & 1
			if lastOut != 0 {
				c.reg.SR |= flagC | flagX
			} else {
				c.reg.SR &^= flagC | flagX
			}
		} else { // ASR
			sign := val & msb
			result = val
			for i := uint32(0); i < count; i++ {
				result = (result >> 1) | sign
			}
			result &= mask
			var lastOut uint32
			if count >= sz.Bits() {
				lastOut = (val >> (sz.Bits() - 1)) & 1 // sign bit
			} else {
				lastOut = (val >> (count - 1)) & 1
			}
			if lastOut != 0 {
				c.reg.SR |= flagC | flagX
			} else {
				c.reg.SR &^= flagC | flagX
			}
			c.reg.SR &^= flagV
		}

	case 1: // Logical shift (LS)
		if dir == 1 { // LSL
			result = (val << count) & mask
			lastOut := (val >> (sz.Bits() - count)) & 1
			if lastOut != 0 {
				c.reg.SR |= flagC | flagX
			} else {
				c.reg.SR &^= flagC | flagX
			}
		} else { // LSR
			result = (val & mask) >> count
			lastOut := (val >> (count - 1)) & 1
			if lastOut != 0 {
				c.reg.SR |= flagC | flagX
			} else {
				c.reg.SR &^= flagC | flagX
			}
		}
		c.reg.SR &^= flagV

	case 2: // Rotate through extend (ROX)
		bits := sz.Bits()
		if dir == 1 { // ROXL
			result = val
			for i := uint32(0); i < count; i++ {
				x := uint32(0)
				if c.reg.SR&flagX != 0 {
					x = 1
				}
				if result&msb != 0 {
					c.reg.SR |= flagX | flagC
				} else {
					c.reg.SR &^= flagX | flagC
				}
				result = ((result << 1) | x) & mask
			}
		} else { // ROXR
			result = val
			for i := uint32(0); i < count; i++ {
				x := uint32(0)
				if c.reg.SR&flagX != 0 {
					x = 1
				}
				if result&1 != 0 {
					c.reg.SR |= flagX | flagC
				} else {
					c.reg.SR &^= flagX | flagC
				}
				result = (result >> 1) | (x << (bits - 1))
			}
			result &= mask
		}
		c.reg.SR &^= flagV

	case 3: // Rotate (RO)
		bits := sz.Bits()
		if dir == 1 { // ROL
			shift := count % bits
			result = ((val << shift) | (val >> (bits - shift))) & mask
		} else { // ROR
			shift := count % bits
			result = ((val >> shift) | (val << (bits - shift))) & mask
		}
		if dir == 1 {
			if result&1 != 0 {
				c.reg.SR |= flagC
			} else {
				c.reg.SR &^= flagC
			}
		} else {
			if result&msb != 0 {
				c.reg.SR |= flagC
			} else {
				c.reg.SR &^= flagC
			}
		}
		c.reg.SR &^= flagV
	}

	// Set N and Z
	c.reg.SR &^= flagN | flagZ
	if result&msb != 0 {
		c.reg.SR |= flagN
	}
	if result&mask == 0 {
		c.reg.SR |= flagZ
	}

	return result
}
