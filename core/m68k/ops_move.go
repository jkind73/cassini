package m68k

import "math/bits"

func init() {
	registerMOVE()
	registerMOVEA()
	registerMOVEQ()
	registerMOVEP()
	registerLEA()
	registerPEA()
	registerMOVEM()
	registerEXG()
	registerSWAP()
}

// registerMOVE registers all MOVE.B/W/L opcodes.
// Encoding: 00SS DDDd ddss ssss
//
//	SS = size (01=B, 11=W, 10=L)
//	DDD/ddd = destination reg/mode (note: reversed from source)
//	sss/ssssss = source mode/reg
func registerMOVE() {
	for _, szBits := range []uint16{0x1000, 0x2000, 0x3000} {
		for dstMode := uint16(0); dstMode < 8; dstMode++ {
			// Destination cannot be An direct (mode 1) or PC-relative/immediate
			if dstMode == 1 {
				continue
			}
			for dstReg := uint16(0); dstReg < 8; dstReg++ {
				if dstMode == 7 && dstReg > 1 {
					continue
				}
				for srcMode := uint16(0); srcMode < 8; srcMode++ {
					for srcReg := uint16(0); srcReg < 8; srcReg++ {
						if srcMode == 7 && srcReg > 4 {
							continue
						}
						opcode := szBits | dstReg<<9 | dstMode<<6 | srcMode<<3 | srcReg
						opcodeTable[opcode] = makeMOVE(srcMode, srcReg, dstMode, dstReg)
					}
				}
			}
		}
	}
}

func makeMOVE(srcMode, srcReg, dstMode, dstReg uint16) opFunc {
	read := makeEARead(srcMode, srcReg)
	srcBase, srcLong := eaFetchConst(srcMode, srcReg)
	dstBase, dstLong := eaWriteConst(dstMode, dstReg)

	if dstMode == 0 {
		return func(c *CPU) {
			sz := moveSizeMap[(c.ir>>12)&3]
			val := read(c, sz)
			mask := sz.Mask()
			c.reg.D[dstReg] = (c.reg.D[dstReg] & ^mask) | (val & mask)
			c.setFlagsLogical(val, sz)
			c.cycles += 4 + srcBase + dstBase
			if sz == sizeLong {
				c.cycles += srcLong + dstLong
			}
		}
	}
	dstAddr := makeEAMemAddr(dstMode, dstReg)
	return func(c *CPU) {
		sz := moveSizeMap[(c.ir>>12)&3]
		val := read(c, sz)
		a := dstAddr(c, sz)
		c.writeBus(sz, a, val)
		c.setFlagsLogical(val, sz)
		c.cycles += 4 + srcBase + dstBase
		if sz == sizeLong {
			c.cycles += srcLong + dstLong
		}
	}
}

// moveSizeMap maps the MOVE size encoding to Size.
// MOVE uses non-standard encoding: 01=Byte, 11=Word, 10=Long.
var moveSizeMap = [4]size{0, sizeByte, sizeLong, sizeWord}

// registerMOVEA registers MOVEA.W and MOVEA.L opcodes.
// Encoding: 00SS DDD0 01ss ssss (destination mode = 001 = An)
func registerMOVEA() {
	for _, szBits := range []uint16{0x2000, 0x3000} {
		for dstReg := uint16(0); dstReg < 8; dstReg++ {
			for srcMode := uint16(0); srcMode < 8; srcMode++ {
				for srcReg := uint16(0); srcReg < 8; srcReg++ {
					if srcMode == 7 && srcReg > 4 {
						continue
					}
					opcode := szBits | dstReg<<9 | 1<<6 | srcMode<<3 | srcReg
					opcodeTable[opcode] = makeMOVEA(dstReg, srcMode, srcReg)
				}
			}
		}
	}
}

func makeMOVEA(an, srcMode, srcReg uint16) opFunc {
	read := makeEARead(srcMode, srcReg)
	eaBase, eaLong := eaFetchConst(srcMode, srcReg)
	return func(c *CPU) {
		sz := moveSizeMap[(c.ir>>12)&3]
		val := read(c, sz)
		if sz == sizeWord {
			val = uint32(int32(int16(val)))
		}
		c.reg.A[an] = val
		// MOVEA does not affect condition codes
		c.cycles += 4 + eaBase
		if sz == sizeLong {
			c.cycles += eaLong
		}
	}
}

// registerMOVEQ registers MOVEQ #imm8,Dn.
// Encoding: 0111 DDD0 dddddddd
func registerMOVEQ() {
	for dn := uint16(0); dn < 8; dn++ {
		for data := uint16(0); data < 256; data++ {
			opcode := 0x7000 | dn<<9 | data
			opcodeTable[opcode] = opMOVEQ
		}
	}
}

func opMOVEQ(c *CPU) {
	dn := (c.ir >> 9) & 7
	data := int8(c.ir & 0xFF) // sign-extend to 8 bits
	c.reg.D[dn] = uint32(int32(data))
	c.setFlagsLogical(c.reg.D[dn], sizeLong)
	c.cycles += 4
}

// registerLEA registers LEA <ea>,An.
// Encoding: 0100 AAA1 11ss ssss (only control addressing modes)
func registerLEA() {
	for an := uint16(0); an < 8; an++ {
		for srcMode := uint16(2); srcMode < 8; srcMode++ {
			// Only control modes: (An), d16(An), d8(An,Xn), abs.W, abs.L, d16(PC), d8(PC,Xn)
			if srcMode == 3 || srcMode == 4 {
				continue // (An)+ and -(An) are not control modes
			}
			for srcReg := uint16(0); srcReg < 8; srcReg++ {
				if srcMode == 7 && srcReg > 3 {
					continue
				}
				opcode := 0x41C0 | an<<9 | srcMode<<3 | srcReg
				opcodeTable[opcode] = makeLEA(an, srcMode, srcReg)
			}
		}
	}
}

func makeLEA(an, srcMode, srcReg uint16) opFunc {
	addr := makeEAMemAddr(srcMode, srcReg)
	var cycles uint64
	switch srcMode {
	case 2:
		cycles = 4
	case 5:
		cycles = 8
	case 6:
		cycles = 12
	case 7:
		switch srcReg {
		case 0, 2:
			cycles = 8
		case 1, 3:
			cycles = 12
		}
	}
	return func(c *CPU) {
		c.reg.A[an] = addr(c, sizeLong)
		c.cycles += cycles
	}
}

// registerPEA registers PEA <ea>.
// Encoding: 0100 1000 01ss ssss (only control addressing modes)
func registerPEA() {
	for srcMode := uint16(2); srcMode < 8; srcMode++ {
		if srcMode == 3 || srcMode == 4 {
			continue
		}
		for srcReg := uint16(0); srcReg < 8; srcReg++ {
			if srcMode == 7 && srcReg > 3 {
				continue
			}
			opcode := 0x4840 | srcMode<<3 | srcReg
			opcodeTable[opcode] = makePEA(srcMode, srcReg)
		}
	}
}

func makePEA(srcMode, srcReg uint16) opFunc {
	addr := makeEAMemAddr(srcMode, srcReg)
	var cycles uint64
	switch srcMode {
	case 2:
		cycles = 12
	case 5:
		cycles = 16
	case 6:
		cycles = 20
	case 7:
		switch srcReg {
		case 0, 2:
			cycles = 16
		case 1, 3:
			cycles = 20
		}
	}
	return func(c *CPU) {
		c.pushLong(addr(c, sizeLong))
		c.cycles += cycles
	}
}

// registerMOVEM registers MOVEM.W and MOVEM.L (register to memory and memory to register).
// Encoding: 0100 1D00 1Sss ssss  D=direction(0=reg-to-mem,1=mem-to-reg), S=size(0=W,1=L)
func registerMOVEM() {
	for dir := uint16(0); dir < 2; dir++ {
		for szBit := uint16(0); szBit < 2; szBit++ {
			for mode := uint16(2); mode < 8; mode++ {
				// Reg-to-mem: (An), -(An), d16(An), d8(An,Xn), abs.W, abs.L
				// Mem-to-reg: (An), (An)+, d16(An), d8(An,Xn), abs.W, abs.L, d16(PC), d8(PC,Xn)
				if dir == 0 && mode == 3 {
					continue // (An)+ not valid for reg-to-mem
				}
				if dir == 1 && mode == 4 {
					continue // -(An) not valid for mem-to-reg
				}
				if mode == 1 {
					continue
				}
				for reg := uint16(0); reg < 8; reg++ {
					if mode == 7 {
						if dir == 0 && reg > 1 {
							continue
						}
						if dir == 1 && reg > 3 {
							continue
						}
					}
					opcode := 0x4880 | dir<<10 | szBit<<6 | mode<<3 | reg
					opcodeTable[opcode] = opMOVEM
				}
			}
		}
	}
}

func opMOVEM(c *CPU) {
	dir := (c.ir >> 10) & 1  // 0 = reg-to-mem, 1 = mem-to-reg
	szBit := (c.ir >> 6) & 1 // 0 = word, 1 = long
	mode := uint8((c.ir >> 3) & 7)
	reg := uint8(c.ir & 7)

	sz := sizeWord
	if szBit != 0 {
		sz = sizeLong
	}

	mask := c.fetchPC() // register list mask

	if dir == 0 {
		// Register to memory
		if mode == 4 {
			// -(An): mask is reversed -- bit 0=A7, bit 15=D0
			addr := c.reg.A[reg]
			for i := 0; i < 16; i++ {
				if mask&(1<<uint(i)) != 0 {
					addr -= uint32(sz)
					ri := 15 - i // reversed: bit 0->reg 15(A7), bit 15->reg 0(D0)
					if ri < 8 {
						c.writeBus(sz, addr, c.reg.D[ri])
					} else {
						c.writeBus(sz, addr, c.reg.A[ri-8])
					}
				}
			}
			c.reg.A[reg] = addr
		} else {
			// Other modes: normal order (D0 first, A7 last)
			src := c.resolveEA(mode, reg, sz)
			addr := src.address()
			for i := 0; i < 16; i++ {
				if mask&(1<<uint(i)) != 0 {
					if i < 8 {
						c.writeBus(sz, addr, c.reg.D[i])
					} else {
						c.writeBus(sz, addr, c.reg.A[i-8])
					}
					addr += uint32(sz)
				}
			}
		}
	} else {
		// Memory to registers
		if mode == 3 {
			// (An)+: load then update An
			addr := c.reg.A[reg]
			for i := 0; i < 16; i++ {
				if mask&(1<<uint(i)) != 0 {
					val := c.readBus(sz, addr)
					if sz == sizeWord {
						val = uint32(int32(int16(val)))
					}
					if i < 8 {
						c.reg.D[i] = val
					} else {
						c.reg.A[i-8] = val
					}
					addr += uint32(sz)
				}
			}
			c.reg.A[reg] = addr
		} else {
			src := c.resolveEA(mode, reg, sz)
			addr := src.address()
			for i := 0; i < 16; i++ {
				if mask&(1<<uint(i)) != 0 {
					val := c.readBus(sz, addr)
					if sz == sizeWord {
						val = uint32(int32(int16(val)))
					}
					if i < 8 {
						c.reg.D[i] = val
					} else {
						c.reg.A[i-8] = val
					}
					addr += uint32(sz)
				}
			}
		}
	}

	n := uint64(bits.OnesCount16(mask))

	perReg := uint64(4)
	if sz == sizeLong {
		perReg = 8
	}

	var base uint64
	if dir == 0 {
		// Register to memory (PRM Table 8-7)
		switch mode {
		case 2, 4: // (An), -(An)
			base = 8
		case 5: // d16(An)
			base = 12
		case 6: // d8(An,Xn)
			base = 14
		case 7:
			switch reg {
			case 0: // abs.W
				base = 12
			case 1: // abs.L
				base = 16
			}
		}
	} else {
		// Memory to register (PRM Table 8-7)
		switch mode {
		case 2, 3: // (An), (An)+
			base = 12
		case 5: // d16(An)
			base = 16
		case 6: // d8(An,Xn)
			base = 18
		case 7:
			switch reg {
			case 0: // abs.W
				base = 16
			case 1: // abs.L
				base = 20
			case 2: // d16(PC)
				base = 16
			case 3: // d8(PC,Xn)
				base = 18
			}
		}
	}

	c.cycles += base + n*perReg
}

// registerEXG registers EXG Dx,Dy / EXG Ax,Ay / EXG Dx,Ay.
// Encoding: 1100 XXX1 MMMM MYYY
func registerEXG() {
	for rx := uint16(0); rx < 8; rx++ {
		for ry := uint16(0); ry < 8; ry++ {
			// Data-Data: mode = 01000
			opcodeTable[0xC100|rx<<9|0x40|ry] = opEXG
			// Addr-Addr: mode = 01001
			opcodeTable[0xC100|rx<<9|0x48|ry] = opEXG
			// Data-Addr: mode = 10001
			opcodeTable[0xC100|rx<<9|0x88|ry] = opEXG
		}
	}
}

func opEXG(c *CPU) {
	rx := (c.ir >> 9) & 7
	ry := c.ir & 7
	opmode := (c.ir >> 3) & 0x1F

	switch opmode {
	case 0x08: // Data-Data
		c.reg.D[rx], c.reg.D[ry] = c.reg.D[ry], c.reg.D[rx]
	case 0x09: // Addr-Addr
		c.reg.A[rx], c.reg.A[ry] = c.reg.A[ry], c.reg.A[rx]
	case 0x11: // Data-Addr
		c.reg.D[rx], c.reg.A[ry] = c.reg.A[ry], c.reg.D[rx]
	}

	c.cycles += 6
}

// registerSWAP registers SWAP Dn.
// Encoding: 0100 1000 0100 0DDD
func registerSWAP() {
	for dn := uint16(0); dn < 8; dn++ {
		opcodeTable[0x4840|dn] = opSWAP
	}
}

func opSWAP(c *CPU) {
	dn := c.ir & 7
	val := c.reg.D[dn]
	c.reg.D[dn] = (val>>16)&0xFFFF | (val&0xFFFF)<<16
	c.setFlagsLogical(c.reg.D[dn], sizeLong)
	c.cycles += 4
}

// registerMOVEP registers MOVEP.W and MOVEP.L opcodes.
// Encoding: 0000 DDD OOO 001 AAA + 16-bit displacement
//
//	OOO=100: MOVEP.W (An),Dn   101: MOVEP.L (An),Dn
//	OOO=110: MOVEP.W Dn,(An)   111: MOVEP.L Dn,(An)
func registerMOVEP() {
	for dn := uint16(0); dn < 8; dn++ {
		for an := uint16(0); an < 8; an++ {
			opcodeTable[0x0108|dn<<9|an] = opMOVEP // W, mem->reg
			opcodeTable[0x0148|dn<<9|an] = opMOVEP // L, mem->reg
			opcodeTable[0x0188|dn<<9|an] = opMOVEP // W, reg->mem
			opcodeTable[0x01C8|dn<<9|an] = opMOVEP // L, reg->mem
		}
	}
}

func opMOVEP(c *CPU) {
	dn := (c.ir >> 9) & 7
	an := c.ir & 7
	opmode := (c.ir >> 6) & 7
	disp := int16(c.fetchPC())
	addr := uint32(int32(c.reg.A[an]) + int32(disp))

	switch opmode {
	case 4: // MOVEP.W mem->reg
		b0 := c.readBus(sizeByte, addr)
		b1 := c.readBus(sizeByte, addr+2)
		val := (b0 << 8) | b1
		c.reg.D[dn] = (c.reg.D[dn] & 0xFFFF0000) | (val & 0xFFFF)
		c.cycles += 16
	case 5: // MOVEP.L mem->reg
		b0 := c.readBus(sizeByte, addr)
		b1 := c.readBus(sizeByte, addr+2)
		b2 := c.readBus(sizeByte, addr+4)
		b3 := c.readBus(sizeByte, addr+6)
		c.reg.D[dn] = (b0 << 24) | (b1 << 16) | (b2 << 8) | b3
		c.cycles += 24
	case 6: // MOVEP.W reg->mem
		val := c.reg.D[dn]
		c.writeBus(sizeByte, addr, (val>>8)&0xFF)
		c.writeBus(sizeByte, addr+2, val&0xFF)
		c.cycles += 16
	case 7: // MOVEP.L reg->mem
		val := c.reg.D[dn]
		c.writeBus(sizeByte, addr, (val>>24)&0xFF)
		c.writeBus(sizeByte, addr+2, (val>>16)&0xFF)
		c.writeBus(sizeByte, addr+4, (val>>8)&0xFF)
		c.writeBus(sizeByte, addr+6, val&0xFF)
		c.cycles += 24
	}
	// MOVEP does not affect condition codes
}
