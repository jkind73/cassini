package m68k

func init() {
	registerNOP()
	registerSTOP()
	registerRESET()
	registerTRAP()
	registerTRAPV()
	registerLINK()
	registerUNLK()
	registerMoveToFromSR()
	registerAndiOriEoriSRCCR()
}

// --- NOP ---

func registerNOP() {
	opcodeTable[0x4E71] = opNOP
}

func opNOP(c *CPU) {
	c.cycles += 4
}

// --- STOP ---

func registerSTOP() {
	opcodeTable[0x4E72] = opSTOP
}

func opSTOP(c *CPU) {
	if !c.supervisor() {
		c.exception(vecPrivilegeViolation)
		return
	}

	imm := c.fetchPC()
	c.setSR(imm)
	c.stopped = true
	// The 68000 halts after STOP, and the prefetch pipeline does not
	// advance. To match the hardware PC state, rewind PC to the
	// instruction start so that resuming via interrupt sees the
	// correct next-instruction address in the exception frame.
	c.reg.PC = c.prevPC
	c.cycles += 4
}

// --- RESET ---

func registerRESET() {
	opcodeTable[0x4E70] = opRESET
}

func opRESET(c *CPU) {
	if !c.supervisor() {
		c.exception(vecPrivilegeViolation)
		return
	}

	c.bus.Reset()
	c.cycles += 132
}

// --- TRAP ---

func registerTRAP() {
	// Encoding: 0100 1110 0100 VVVV (vector 0-15 -> exception vectors 32-47)
	for v := uint16(0); v < 16; v++ {
		opcode := 0x4E40 | v
		opcodeTable[opcode] = opTRAP
	}
}

func opTRAP(c *CPU) {
	vector := int(c.ir&0xF) + vecTrap0
	c.exception(vector)
}

// --- TRAPV ---

func registerTRAPV() {
	opcodeTable[0x4E76] = opTRAPV
}

func opTRAPV(c *CPU) {
	if c.reg.SR&flagV != 0 {
		c.exception(vecTRAPV)
	} else {
		c.cycles += 4
	}
}

// --- LINK ---

func registerLINK() {
	// Encoding: 0100 1110 0101 0AAA
	for an := uint16(0); an < 8; an++ {
		opcodeTable[0x4E50|an] = opLINK
	}
}

func opLINK(c *CPU) {
	an := c.ir & 7
	disp := int16(c.fetchPC())

	c.pushLong(c.reg.A[an])
	c.reg.A[an] = c.reg.A[7]
	c.reg.A[7] = uint32(int32(c.reg.A[7]) + int32(disp))

	c.cycles += 16
}

// --- UNLK ---

func registerUNLK() {
	// Encoding: 0100 1110 0101 1AAA
	for an := uint16(0); an < 8; an++ {
		opcodeTable[0x4E58|an] = opUNLK
	}
}

func opUNLK(c *CPU) {
	an := c.ir & 7
	c.reg.A[7] = c.reg.A[an]
	c.reg.A[an] = c.popLong()

	c.cycles += 12
}

// --- MOVE to/from SR, MOVE to/from CCR ---

func registerMoveToFromSR() {
	// MOVE SR,<ea> (read SR - privileged on 010+, unprivileged on 000)
	// Encoding: 0100 0000 11ss ssss
	for mode := uint16(0); mode < 8; mode++ {
		if mode == 1 {
			continue
		}
		for reg := uint16(0); reg < 8; reg++ {
			if mode == 7 && reg > 1 {
				continue
			}
			opcodeTable[0x40C0|mode<<3|reg] = makeMOVEfromSR(mode, reg)
		}
	}

	// MOVE <ea>,CCR
	// Encoding: 0100 0100 11ss ssss
	for mode := uint16(0); mode < 8; mode++ {
		if mode == 1 {
			continue
		}
		for reg := uint16(0); reg < 8; reg++ {
			if mode == 7 && reg > 4 {
				continue
			}
			opcodeTable[0x44C0|mode<<3|reg] = makeMOVEtoCCR(mode, reg)
		}
	}

	// MOVE <ea>,SR (privileged)
	// Encoding: 0100 0110 11ss ssss
	for mode := uint16(0); mode < 8; mode++ {
		if mode == 1 {
			continue
		}
		for reg := uint16(0); reg < 8; reg++ {
			if mode == 7 && reg > 4 {
				continue
			}
			opcodeTable[0x46C0|mode<<3|reg] = makeMOVEtoSR(mode, reg)
		}
	}

	// MOVE USP,An and MOVE An,USP (privileged)
	// Encoding: 0100 1110 0110 DAAA (D=0: An->USP, D=1: USP->An)
	for an := uint16(0); an < 8; an++ {
		opcodeTable[0x4E60|an] = opMOVEtoUSP
		opcodeTable[0x4E68|an] = opMOVEfromUSP
	}
}

func makeMOVEfromSR(mode, reg uint16) opFunc {
	if mode == 0 {
		return func(c *CPU) {
			c.reg.D[reg] = (c.reg.D[reg] & 0xFFFF0000) | uint32(c.reg.SR)
			c.cycles += 6
		}
	}
	addr := makeEAMemAddr(mode, reg)
	eaBase, _ := eaFetchConst(mode, reg)
	return func(c *CPU) {
		a := addr(c, sizeWord)
		c.writeBus(sizeWord, a, uint32(c.reg.SR))
		c.cycles += 8 + eaBase
	}
}

func makeMOVEtoCCR(mode, reg uint16) opFunc {
	read := makeEARead(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	return func(c *CPU) {
		val := read(c, sizeWord)
		c.setCCR(uint8(val))
		c.cycles += 12 + eaBase
		if sizeWord == sizeLong {
			c.cycles += eaLong
		}
	}
}

func makeMOVEtoSR(mode, reg uint16) opFunc {
	read := makeEARead(mode, reg)
	eaBase, eaLong := eaFetchConst(mode, reg)
	return func(c *CPU) {
		if !c.supervisor() {
			c.exception(vecPrivilegeViolation)
			return
		}
		val := read(c, sizeWord)
		c.setSR(uint16(val))
		c.cycles += 12 + eaBase
		if sizeWord == sizeLong {
			c.cycles += eaLong
		}
	}
}

func opMOVEtoUSP(c *CPU) {
	if !c.supervisor() {
		c.exception(vecPrivilegeViolation)
		return
	}
	an := c.ir & 7
	c.reg.USP = c.reg.A[an]
	c.cycles += 4
}

func opMOVEfromUSP(c *CPU) {
	if !c.supervisor() {
		c.exception(vecPrivilegeViolation)
		return
	}
	an := c.ir & 7
	c.reg.A[an] = c.reg.USP
	c.cycles += 4
}

// --- ANDI/ORI/EORI to CCR and SR ---

func registerAndiOriEoriSRCCR() {
	// ANDI to CCR: 0000 0010 0011 1100
	opcodeTable[0x023C] = opANDItoCCR
	// ANDI to SR:  0000 0010 0111 1100
	opcodeTable[0x027C] = opANDItoSR
	// ORI to CCR:  0000 0000 0011 1100
	opcodeTable[0x003C] = opORItoCCR
	// ORI to SR:   0000 0000 0111 1100
	opcodeTable[0x007C] = opORItoSR
	// EORI to CCR: 0000 1010 0011 1100
	opcodeTable[0x0A3C] = opEORItoCCR
	// EORI to SR:  0000 1010 0111 1100
	opcodeTable[0x0A7C] = opEORItoSR
}

func opANDItoCCR(c *CPU) {
	imm := c.fetchPC()
	c.setCCR(uint8(c.reg.SR) & uint8(imm))
	c.cycles += 20
}

func opANDItoSR(c *CPU) {
	if !c.supervisor() {
		c.exception(vecPrivilegeViolation)
		return
	}
	imm := c.fetchPC()
	c.setSR(c.reg.SR & imm)
	c.cycles += 20
}

func opORItoCCR(c *CPU) {
	imm := c.fetchPC()
	c.setCCR(uint8(c.reg.SR) | uint8(imm))
	c.cycles += 20
}

func opORItoSR(c *CPU) {
	if !c.supervisor() {
		c.exception(vecPrivilegeViolation)
		return
	}
	imm := c.fetchPC()
	c.setSR(c.reg.SR | imm)
	c.cycles += 20
}

func opEORItoCCR(c *CPU) {
	imm := c.fetchPC()
	c.setCCR(uint8(c.reg.SR) ^ uint8(imm))
	c.cycles += 20
}

func opEORItoSR(c *CPU) {
	if !c.supervisor() {
		c.exception(vecPrivilegeViolation)
		return
	}
	imm := c.fetchPC()
	c.setSR(c.reg.SR ^ imm)
	c.cycles += 20
}
