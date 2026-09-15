package m68k

func init() {
	registerBcc()
	registerBRA()
	registerBSR()
	registerDBcc()
	registerJMP()
	registerJSR()
	registerRTS()
	registerRTE()
	registerRTR()
	registerScc()
}

// --- Bcc ---

func registerBcc() {
	// Encoding: 0110 CCCC DDDDDDDD
	// CC = condition (2-15; 0=BRA, 1=BSR handled separately)
	// DD = 8-bit displacement (0 = 16-bit extension, FF = 32-bit extension on 020+)
	for cc := uint16(2); cc < 16; cc++ {
		for disp := uint16(0); disp < 256; disp++ {
			opcode := 0x6000 | cc<<8 | disp
			opcodeTable[opcode] = opBcc
		}
	}
}

func opBcc(c *CPU) {
	cc := (c.ir >> 8) & 0xF
	disp := int32(int8(c.ir & 0xFF))
	base := c.reg.PC // PC after opcode fetch = instruction address + 2

	if disp == 0 {
		disp = int32(int16(c.fetchPC()))
	}

	if c.testCondition(cc) {
		// Displacement is relative to instruction address + 2
		c.reg.PC = uint32(int32(base) + disp)
		c.cycles += 10
	} else {
		c.cycles += 8
		if int8(c.ir&0xFF) == 0 {
			c.cycles += 4
		}
	}
}

// --- BRA ---

func registerBRA() {
	for disp := uint16(0); disp < 256; disp++ {
		opcode := 0x6000 | disp
		opcodeTable[opcode] = opBRA
	}
}

func opBRA(c *CPU) {
	disp := int32(int8(c.ir & 0xFF))
	base := c.reg.PC // PC after fetching opcode word

	if disp == 0 {
		disp = int32(int16(c.fetchPC()))
	}

	c.reg.PC = uint32(int32(base) + disp)
	c.cycles += 10
}

// --- BSR ---

func registerBSR() {
	for disp := uint16(0); disp < 256; disp++ {
		opcode := 0x6100 | disp
		opcodeTable[opcode] = opBSR
	}
}

func opBSR(c *CPU) {
	disp := int32(int8(c.ir & 0xFF))
	base := c.reg.PC

	if disp == 0 {
		disp = int32(int16(c.fetchPC()))
	}

	c.pushLong(c.reg.PC)
	c.reg.PC = uint32(int32(base) + disp)
	c.cycles += 18
}

// --- DBcc ---

func registerDBcc() {
	// Encoding: 0101 CCCC 1100 1DDD
	for cc := uint16(0); cc < 16; cc++ {
		for dn := uint16(0); dn < 8; dn++ {
			opcode := 0x50C8 | cc<<8 | dn
			opcodeTable[opcode] = opDBcc
		}
	}
}

func opDBcc(c *CPU) {
	cc := (c.ir >> 8) & 0xF
	dn := c.ir & 7

	disp := int16(c.fetchPC())

	if c.testCondition(cc) {
		// Condition true: no branch, no decrement
		c.cycles += 12
		return
	}

	// Decrement low word of Dn
	val := int16(c.reg.D[dn]&0xFFFF) - 1
	c.reg.D[dn] = (c.reg.D[dn] & 0xFFFF0000) | uint32(uint16(val))

	if val == -1 {
		// Counter expired: fall through
		c.cycles += 14
	} else {
		// Branch
		c.reg.PC = uint32(int32(c.reg.PC) - 2 + int32(disp))
		c.cycles += 10
	}
}

// --- JMP ---

func registerJMP() {
	// Encoding: 0100 1110 11ss ssss (control addressing modes)
	for mode := uint16(2); mode < 8; mode++ {
		if mode == 3 || mode == 4 {
			continue
		}
		for reg := uint16(0); reg < 8; reg++ {
			if mode == 7 && reg > 3 {
				continue
			}
			opcode := 0x4EC0 | mode<<3 | reg
			opcodeTable[opcode] = makeJMP(mode, reg)
		}
	}
}

func makeJMP(mode, reg uint16) opFunc {
	addr := makeEAMemAddr(mode, reg)
	var cycles uint64
	switch mode {
	case 2:
		cycles = 8
	case 5:
		cycles = 10
	case 6:
		cycles = 14
	case 7:
		switch reg {
		case 0, 2:
			cycles = 10
		case 1:
			cycles = 12
		case 3:
			cycles = 14
		}
	}
	return func(c *CPU) {
		c.reg.PC = addr(c, sizeWord)
		c.cycles += cycles
	}
}

// --- JSR ---

func registerJSR() {
	for mode := uint16(2); mode < 8; mode++ {
		if mode == 3 || mode == 4 {
			continue
		}
		for reg := uint16(0); reg < 8; reg++ {
			if mode == 7 && reg > 3 {
				continue
			}
			opcode := 0x4E80 | mode<<3 | reg
			opcodeTable[opcode] = makeJSR(mode, reg)
		}
	}
}

func makeJSR(mode, reg uint16) opFunc {
	addr := makeEAMemAddr(mode, reg)
	var cycles uint64
	switch mode {
	case 2:
		cycles = 16
	case 5:
		cycles = 18
	case 6:
		cycles = 22
	case 7:
		switch reg {
		case 0, 2:
			cycles = 18
		case 1:
			cycles = 20
		case 3:
			cycles = 22
		}
	}
	return func(c *CPU) {
		target := addr(c, sizeWord)
		c.pushLong(c.reg.PC)
		c.reg.PC = target
		c.cycles += cycles
	}
}

// --- RTS ---

func registerRTS() {
	opcodeTable[0x4E75] = opRTS
}

func opRTS(c *CPU) {
	c.reg.PC = c.popLong()
	c.cycles += 16
}

// --- RTE ---

func registerRTE() {
	opcodeTable[0x4E73] = opRTE
}

func opRTE(c *CPU) {
	if !c.supervisor() {
		c.exception(vecPrivilegeViolation)
		return
	}

	sr := c.popWord()
	pc := c.popLong()
	c.setSR(sr)
	c.reg.PC = pc

	c.cycles += 20
}

// --- RTR ---

func registerRTR() {
	opcodeTable[0x4E77] = opRTR
}

func opRTR(c *CPU) {
	ccr := c.popWord()
	c.setCCR(uint8(ccr))
	c.reg.PC = c.popLong()

	c.cycles += 20
}

// --- Scc ---

func registerScc() {
	// Encoding: 0101 CCCC 11ss ssss
	for cc := uint16(0); cc < 16; cc++ {
		for mode := uint16(0); mode < 8; mode++ {
			if mode == 1 {
				continue
			}
			for reg := uint16(0); reg < 8; reg++ {
				if mode == 7 && reg > 1 {
					continue
				}
				opcode := 0x50C0 | cc<<8 | mode<<3 | reg
				opcodeTable[opcode] = makeScc(mode, reg)
			}
		}
	}
}

func makeScc(mode, reg uint16) opFunc {
	if mode == 0 {
		return func(c *CPU) {
			cc := (c.ir >> 8) & 0xF
			if c.testCondition(cc) {
				c.reg.D[reg] = (c.reg.D[reg] & 0xFFFFFF00) | 0xFF
				c.cycles += 6
			} else {
				c.reg.D[reg] = c.reg.D[reg] & 0xFFFFFF00
				c.cycles += 4
			}
		}
	}
	addr := makeEAMemAddr(mode, reg)
	eaBase, _ := eaFetchConst(mode, reg)
	return func(c *CPU) {
		cc := (c.ir >> 8) & 0xF
		a := addr(c, sizeByte)
		if c.testCondition(cc) {
			c.writeBus(sizeByte, a, 0xFF)
		} else {
			c.writeBus(sizeByte, a, 0x00)
		}
		c.cycles += 8 + eaBase
	}
}
