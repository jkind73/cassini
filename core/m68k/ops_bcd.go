package m68k

func init() {
	registerABCD()
	registerSBCD()
	registerNBCD()
}

// --- ABCD ---

func registerABCD() {
	// Encoding: 1100 XXX1 0000 RYYY  R=0: Dy,Dx  R=1: -(Ay),-(Ax)
	for rx := uint16(0); rx < 8; rx++ {
		for ry := uint16(0); ry < 8; ry++ {
			opcodeTable[0xC100|rx<<9|ry] = opABCDreg
			opcodeTable[0xC108|rx<<9|ry] = opABCDmem
		}
	}
}

func opABCDreg(c *CPU) {
	rx := (c.ir >> 9) & 7
	ry := c.ir & 7

	s := c.reg.D[ry] & 0xFF
	d := c.reg.D[rx] & 0xFF
	result := bcdAdd(c, s, d)
	c.reg.D[rx] = (c.reg.D[rx] & 0xFFFFFF00) | (result & 0xFF)

	c.cycles += 6
}

func opABCDmem(c *CPU) {
	rx := (c.ir >> 9) & 7
	ry := c.ir & 7

	src := c.resolveEA(4, uint8(ry), sizeByte) // -(Ay)
	s := src.read(c, sizeByte)
	dst := c.resolveEA(4, uint8(rx), sizeByte) // -(Ax)
	d := dst.read(c, sizeByte)
	result := bcdAdd(c, s, d)
	dst.write(c, sizeByte, result)

	c.cycles += 18
}

func bcdAdd(c *CPU, s, d uint32) uint32 {
	x := uint32(0)
	if c.reg.SR&flagX != 0 {
		x = 1
	}

	binary := s + d + x

	lo := (s & 0x0F) + (d & 0x0F) + x
	hi := ((s >> 4) & 0x0F) + ((d >> 4) & 0x0F)

	if lo > 9 {
		lo += 6
	}
	hi += lo >> 4 // carry from low nibble correction
	lo &= 0x0F

	carry := false
	if hi > 9 {
		hi += 6
		carry = true
	}

	r8 := ((hi << 4) | lo) & 0xFF
	c.reg.SR &^= flagC | flagX | flagN | flagV
	if carry {
		c.reg.SR |= flagC | flagX
	}
	if r8&0x80 != 0 {
		c.reg.SR |= flagN
	}
	// V: bit 7 went from 0 to 1 during BCD correction
	if binary&0x80 == 0 && r8&0x80 != 0 {
		c.reg.SR |= flagV
	}
	if r8 != 0 {
		c.reg.SR &^= flagZ
	}

	return r8
}

// --- SBCD ---

func registerSBCD() {
	for rx := uint16(0); rx < 8; rx++ {
		for ry := uint16(0); ry < 8; ry++ {
			opcodeTable[0x8100|rx<<9|ry] = opSBCDreg
			opcodeTable[0x8108|rx<<9|ry] = opSBCDmem
		}
	}
}

func opSBCDreg(c *CPU) {
	rx := (c.ir >> 9) & 7
	ry := c.ir & 7

	s := c.reg.D[ry] & 0xFF
	d := c.reg.D[rx] & 0xFF
	result := bcdSub(c, s, d)
	c.reg.D[rx] = (c.reg.D[rx] & 0xFFFFFF00) | (result & 0xFF)

	c.cycles += 6
}

func opSBCDmem(c *CPU) {
	rx := (c.ir >> 9) & 7
	ry := c.ir & 7

	src := c.resolveEA(4, uint8(ry), sizeByte)
	s := src.read(c, sizeByte)
	dst := c.resolveEA(4, uint8(rx), sizeByte)
	d := dst.read(c, sizeByte)
	result := bcdSub(c, s, d)
	dst.write(c, sizeByte, result)

	c.cycles += 18
}

func bcdSub(c *CPU, s, d uint32) uint32 {
	x := uint32(0)
	if c.reg.SR&flagX != 0 {
		x = 1
	}

	binary := d - s - x

	lo := (d & 0x0F) - (s & 0x0F) - x
	res := binary
	if lo&0x10 != 0 {
		res -= 6
	}

	// The hi correction (0x60) is applied when the binary subtraction
	// underflowed. Borrow also includes cases where the lo-nibble correction
	// alone pushed the result past the byte boundary (invalid BCD inputs).
	binBorrow := d < s+x
	borrow := binBorrow || res&0x100 != 0

	if binBorrow {
		res -= 0x60
	}

	r8 := res & 0xFF

	c.reg.SR &^= flagC | flagX | flagN | flagV
	if borrow {
		c.reg.SR |= flagC | flagX
	}
	if r8&0x80 != 0 {
		c.reg.SR |= flagN
	}
	// V: bit 7 went from 1 to 0 during BCD correction (sign change)
	if binary&0x80 != 0 && r8&0x80 == 0 {
		c.reg.SR |= flagV
	}
	if r8 != 0 {
		c.reg.SR &^= flagZ
	}

	return r8
}

// --- NBCD ---

func registerNBCD() {
	// Encoding: 0100 1000 00ss ssss
	for mode := uint16(0); mode < 8; mode++ {
		if mode == 1 {
			continue
		}
		for reg := uint16(0); reg < 8; reg++ {
			if mode == 7 && reg > 1 {
				continue
			}
			opcodeTable[0x4800|mode<<3|reg] = makeNBCD(mode, reg)
		}
	}
}

func makeNBCD(mode, reg uint16) opFunc {
	if mode == 0 {
		return func(c *CPU) {
			d := c.reg.D[reg] & 0xFF
			result := bcdSub(c, d, 0)
			c.reg.D[reg] = (c.reg.D[reg] & 0xFFFFFF00) | (result & 0xFF)
			c.cycles += 6
		}
	}
	addr := makeEAMemAddr(mode, reg)
	eaBase, _ := eaFetchConst(mode, reg)
	return func(c *CPU) {
		a := addr(c, sizeByte)
		d := c.readBus(sizeByte, a)
		result := bcdSub(c, d, 0)
		c.writeBus(sizeByte, a, result)
		c.cycles += 8 + eaBase
	}
}
