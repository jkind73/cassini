package m68k

func init() {
	registerBTST()
	registerBCHG()
	registerBCLR()
	registerBSET()
}

// Bit operations have two forms:
// Dynamic: 0000 DDD1 00tt teee (Dn specifies bit number)
// Static:  0000 1000 00tt teee + immediate word (bit number in extension)
// tt = 00:BTST, 01:BCHG, 10:BCLR, 11:BSET
// For Dn destination: operates on long (bit mod 32)
// For memory: operates on byte (bit mod 8)

// --- BTST ---

func registerBTST() {
	// Dynamic form: BTST Dn,<ea> (includes immediate as source)
	for dn := uint16(0); dn < 8; dn++ {
		for mode := uint16(0); mode < 8; mode++ {
			if mode == 1 {
				continue
			}
			for reg := uint16(0); reg < 8; reg++ {
				if mode == 7 && reg > 4 {
					continue
				}
				opcode := 0x0100 | dn<<9 | mode<<3 | reg
				opcodeTable[opcode] = makeBTSTdyn(dn, mode, reg)
			}
		}
	}
	// Static form: BTST #imm,<ea>
	for mode := uint16(0); mode < 8; mode++ {
		if mode == 1 {
			continue
		}
		for reg := uint16(0); reg < 8; reg++ {
			if mode == 7 && reg > 3 {
				continue
			}
			opcode := 0x0800 | mode<<3 | reg
			opcodeTable[opcode] = makeBTSTstatic(mode, reg)
		}
	}
}

func makeBTSTdyn(dn, mode, reg uint16) opFunc {
	if mode == 0 {
		return func(c *CPU) {
			bitNum := c.reg.D[dn] & 31
			if c.reg.D[reg]&(1<<bitNum) == 0 {
				c.reg.SR |= flagZ
			} else {
				c.reg.SR &^= flagZ
			}
			c.cycles += 6
		}
	}
	read := makeEARead(mode, reg)
	eaBase, _ := eaFetchConst(mode, reg)
	return func(c *CPU) {
		bitNum := c.reg.D[dn] & 7
		val := read(c, sizeByte)
		if val&(1<<bitNum) == 0 {
			c.reg.SR |= flagZ
		} else {
			c.reg.SR &^= flagZ
		}
		c.cycles += 4 + eaBase
	}
}

func makeBTSTstatic(mode, reg uint16) opFunc {
	if mode == 0 {
		return func(c *CPU) {
			bitNum := uint32(c.fetchPC()&0xFF) & 31
			if c.reg.D[reg]&(1<<bitNum) == 0 {
				c.reg.SR |= flagZ
			} else {
				c.reg.SR &^= flagZ
			}
			c.cycles += 10
		}
	}
	read := makeEARead(mode, reg)
	eaBase, _ := eaFetchConst(mode, reg)
	return func(c *CPU) {
		bitNum := uint32(c.fetchPC()&0xFF) & 7
		val := read(c, sizeByte)
		if val&(1<<bitNum) == 0 {
			c.reg.SR |= flagZ
		} else {
			c.reg.SR &^= flagZ
		}
		c.cycles += 8 + eaBase
	}
}

// --- BCHG ---

func registerBCHG() {
	for dn := uint16(0); dn < 8; dn++ {
		for mode := uint16(0); mode < 8; mode++ {
			if mode == 1 {
				continue
			}
			for reg := uint16(0); reg < 8; reg++ {
				if mode == 7 && reg > 1 {
					continue
				}
				opcode := 0x0140 | dn<<9 | mode<<3 | reg
				opcodeTable[opcode] = makeBCHGdyn(dn, mode, reg)
			}
		}
	}
	for mode := uint16(0); mode < 8; mode++ {
		if mode == 1 {
			continue
		}
		for reg := uint16(0); reg < 8; reg++ {
			if mode == 7 && reg > 1 {
				continue
			}
			opcode := 0x0840 | mode<<3 | reg
			opcodeTable[opcode] = makeBCHGstatic(mode, reg)
		}
	}
}

func makeBCHGdyn(dn, mode, reg uint16) opFunc {
	if mode == 0 {
		return func(c *CPU) {
			bitNum := c.reg.D[dn] & 31
			mask := uint32(1) << bitNum
			if c.reg.D[reg]&mask == 0 {
				c.reg.SR |= flagZ
			} else {
				c.reg.SR &^= flagZ
			}
			c.reg.D[reg] ^= mask
			if bitNum < 16 {
				c.cycles += 6
			} else {
				c.cycles += 8
			}
		}
	}
	addr := makeEAMemAddr(mode, reg)
	eaBase, _ := eaFetchConst(mode, reg)
	return func(c *CPU) {
		bitNum := c.reg.D[dn] & 7
		a := addr(c, sizeByte)
		val := c.readBus(sizeByte, a)
		mask := uint32(1) << bitNum
		if val&mask == 0 {
			c.reg.SR |= flagZ
		} else {
			c.reg.SR &^= flagZ
		}
		c.writeBus(sizeByte, a, val^mask)
		c.cycles += 8 + eaBase
	}
}

func makeBCHGstatic(mode, reg uint16) opFunc {
	if mode == 0 {
		return func(c *CPU) {
			bitNum := uint32(c.fetchPC()&0xFF) & 31
			mask := uint32(1) << bitNum
			if c.reg.D[reg]&mask == 0 {
				c.reg.SR |= flagZ
			} else {
				c.reg.SR &^= flagZ
			}
			c.reg.D[reg] ^= mask
			c.cycles += 12
		}
	}
	addr := makeEAMemAddr(mode, reg)
	eaBase, _ := eaFetchConst(mode, reg)
	return func(c *CPU) {
		bitNum := uint32(c.fetchPC()&0xFF) & 7
		a := addr(c, sizeByte)
		val := c.readBus(sizeByte, a)
		mask := uint32(1) << bitNum
		if val&mask == 0 {
			c.reg.SR |= flagZ
		} else {
			c.reg.SR &^= flagZ
		}
		c.writeBus(sizeByte, a, val^mask)
		c.cycles += 12 + eaBase
	}
}

// --- BCLR ---

func registerBCLR() {
	for dn := uint16(0); dn < 8; dn++ {
		for mode := uint16(0); mode < 8; mode++ {
			if mode == 1 {
				continue
			}
			for reg := uint16(0); reg < 8; reg++ {
				if mode == 7 && reg > 1 {
					continue
				}
				opcode := 0x0180 | dn<<9 | mode<<3 | reg
				opcodeTable[opcode] = makeBCLRdyn(dn, mode, reg)
			}
		}
	}
	for mode := uint16(0); mode < 8; mode++ {
		if mode == 1 {
			continue
		}
		for reg := uint16(0); reg < 8; reg++ {
			if mode == 7 && reg > 1 {
				continue
			}
			opcode := 0x0880 | mode<<3 | reg
			opcodeTable[opcode] = makeBCLRstatic(mode, reg)
		}
	}
}

func makeBCLRdyn(dn, mode, reg uint16) opFunc {
	if mode == 0 {
		return func(c *CPU) {
			bitNum := c.reg.D[dn] & 31
			mask := uint32(1) << bitNum
			if c.reg.D[reg]&mask == 0 {
				c.reg.SR |= flagZ
			} else {
				c.reg.SR &^= flagZ
			}
			c.reg.D[reg] &^= mask
			if bitNum < 16 {
				c.cycles += 8
			} else {
				c.cycles += 10
			}
		}
	}
	addr := makeEAMemAddr(mode, reg)
	eaBase, _ := eaFetchConst(mode, reg)
	return func(c *CPU) {
		bitNum := c.reg.D[dn] & 7
		a := addr(c, sizeByte)
		val := c.readBus(sizeByte, a)
		mask := uint32(1) << bitNum
		if val&mask == 0 {
			c.reg.SR |= flagZ
		} else {
			c.reg.SR &^= flagZ
		}
		c.writeBus(sizeByte, a, val&^mask)
		c.cycles += 8 + eaBase
	}
}

func makeBCLRstatic(mode, reg uint16) opFunc {
	if mode == 0 {
		return func(c *CPU) {
			bitNum := uint32(c.fetchPC()&0xFF) & 31
			mask := uint32(1) << bitNum
			if c.reg.D[reg]&mask == 0 {
				c.reg.SR |= flagZ
			} else {
				c.reg.SR &^= flagZ
			}
			c.reg.D[reg] &^= mask
			c.cycles += 14
		}
	}
	addr := makeEAMemAddr(mode, reg)
	eaBase, _ := eaFetchConst(mode, reg)
	return func(c *CPU) {
		bitNum := uint32(c.fetchPC()&0xFF) & 7
		a := addr(c, sizeByte)
		val := c.readBus(sizeByte, a)
		mask := uint32(1) << bitNum
		if val&mask == 0 {
			c.reg.SR |= flagZ
		} else {
			c.reg.SR &^= flagZ
		}
		c.writeBus(sizeByte, a, val&^mask)
		c.cycles += 12 + eaBase
	}
}

// --- BSET ---

func registerBSET() {
	for dn := uint16(0); dn < 8; dn++ {
		for mode := uint16(0); mode < 8; mode++ {
			if mode == 1 {
				continue
			}
			for reg := uint16(0); reg < 8; reg++ {
				if mode == 7 && reg > 1 {
					continue
				}
				opcode := 0x01C0 | dn<<9 | mode<<3 | reg
				opcodeTable[opcode] = makeBSETdyn(dn, mode, reg)
			}
		}
	}
	for mode := uint16(0); mode < 8; mode++ {
		if mode == 1 {
			continue
		}
		for reg := uint16(0); reg < 8; reg++ {
			if mode == 7 && reg > 1 {
				continue
			}
			opcode := 0x08C0 | mode<<3 | reg
			opcodeTable[opcode] = makeBSETstatic(mode, reg)
		}
	}
}

func makeBSETdyn(dn, mode, reg uint16) opFunc {
	if mode == 0 {
		return func(c *CPU) {
			bitNum := c.reg.D[dn] & 31
			mask := uint32(1) << bitNum
			if c.reg.D[reg]&mask == 0 {
				c.reg.SR |= flagZ
			} else {
				c.reg.SR &^= flagZ
			}
			c.reg.D[reg] |= mask
			if bitNum < 16 {
				c.cycles += 6
			} else {
				c.cycles += 8
			}
		}
	}
	addr := makeEAMemAddr(mode, reg)
	eaBase, _ := eaFetchConst(mode, reg)
	return func(c *CPU) {
		bitNum := c.reg.D[dn] & 7
		a := addr(c, sizeByte)
		val := c.readBus(sizeByte, a)
		mask := uint32(1) << bitNum
		if val&mask == 0 {
			c.reg.SR |= flagZ
		} else {
			c.reg.SR &^= flagZ
		}
		c.writeBus(sizeByte, a, val|mask)
		c.cycles += 8 + eaBase
	}
}

func makeBSETstatic(mode, reg uint16) opFunc {
	if mode == 0 {
		return func(c *CPU) {
			bitNum := uint32(c.fetchPC()&0xFF) & 31
			mask := uint32(1) << bitNum
			if c.reg.D[reg]&mask == 0 {
				c.reg.SR |= flagZ
			} else {
				c.reg.SR &^= flagZ
			}
			c.reg.D[reg] |= mask
			c.cycles += 12
		}
	}
	addr := makeEAMemAddr(mode, reg)
	eaBase, _ := eaFetchConst(mode, reg)
	return func(c *CPU) {
		bitNum := uint32(c.fetchPC()&0xFF) & 7
		a := addr(c, sizeByte)
		val := c.readBus(sizeByte, a)
		mask := uint32(1) << bitNum
		if val&mask == 0 {
			c.reg.SR |= flagZ
		} else {
			c.reg.SR &^= flagZ
		}
		c.writeBus(sizeByte, a, val|mask)
		c.cycles += 12 + eaBase
	}
}
