package m68k

// EA addressing mode categories.
const (
	eaDataReg   = iota // Data register direct (Dn)
	eaAddrReg          // Address register direct (An)
	eaMemory           // All memory addressing modes
	eaImmediate        // Immediate (#imm)
)

// ea represents a resolved effective address operand.
type ea struct {
	mode uint8  // eaDataReg, eaAddrReg, eaMemory, eaImmediate
	reg  uint8  // register number (for register modes)
	addr uint32 // memory address (for memory modes)
	imm  uint32 // immediate value (for immediate mode)
}

// read returns the value at this effective address.
func (e ea) read(c *CPU, sz size) uint32 {
	switch e.mode {
	case eaDataReg:
		return c.reg.D[e.reg] & sz.Mask()
	case eaAddrReg:
		return c.reg.A[e.reg] & sz.Mask()
	case eaMemory:
		return c.readBus(sz, e.addr)
	case eaImmediate:
		return e.imm & sz.Mask()
	}
	return 0
}

// write stores a value at this effective address.
// Data register writes preserve upper bits for byte/word operations.
// Address register writes always store the full 32-bit value.
func (e ea) write(c *CPU, sz size, val uint32) {
	switch e.mode {
	case eaDataReg:
		mask := sz.Mask()
		c.reg.D[e.reg] = (c.reg.D[e.reg] & ^mask) | (val & mask)
	case eaAddrReg:
		c.reg.A[e.reg] = val
	case eaMemory:
		c.writeBus(sz, e.addr, val)
	}
}

// address returns the memory address (only valid for memory EAs).
func (e ea) address() uint32 {
	return e.addr
}

// resolveEA decodes and resolves an effective address from a mode/register pair.
// The mode is bits 5-3 and reg is bits 2-0 of the standard EA field.
// Extension words are fetched from the instruction stream as needed.
func (c *CPU) resolveEA(mode, reg uint8, sz size) ea {
	switch mode {
	case 0: // Dn - Data register direct
		return ea{mode: eaDataReg, reg: reg}

	case 1: // An - Address register direct
		return ea{mode: eaAddrReg, reg: reg}

	case 2: // (An) - Address register indirect
		return ea{mode: eaMemory, addr: c.reg.A[reg]}

	case 3: // (An)+ - Address register indirect with postincrement
		addr := c.reg.A[reg]
		inc := uint32(sz)
		if reg == 7 && sz == sizeByte {
			inc = 2 // SP always stays word-aligned
		}
		c.reg.A[reg] += inc
		return ea{mode: eaMemory, addr: addr}

	case 4: // -(An) - Address register indirect with predecrement
		dec := uint32(sz)
		if reg == 7 && sz == sizeByte {
			dec = 2 // SP always stays word-aligned
		}
		c.reg.A[reg] -= dec
		return ea{mode: eaMemory, addr: c.reg.A[reg]}

	case 5: // d16(An) - Address register indirect with displacement
		disp := int16(c.fetchPC())
		return ea{mode: eaMemory, addr: uint32(int32(c.reg.A[reg]) + int32(disp))}

	case 6: // d8(An,Xn) - Address register indirect with index
		ext := c.fetchPC()
		return ea{mode: eaMemory, addr: c.calcIndex(c.reg.A[reg], ext)}

	case 7:
		switch reg {
		case 0: // abs.W - Absolute short (sign-extended to 32 bits)
			addr := int16(c.fetchPC())
			return ea{mode: eaMemory, addr: uint32(int32(addr))}

		case 1: // abs.L - Absolute long
			addr := c.fetchPCLong()
			return ea{mode: eaMemory, addr: addr}

		case 2: // d16(PC) - PC relative with displacement
			pc := c.reg.PC // PC points to the extension word
			disp := int16(c.fetchPC())
			return ea{mode: eaMemory, addr: uint32(int32(pc) + int32(disp))}

		case 3: // d8(PC,Xn) - PC relative with index
			pc := c.reg.PC // PC points to the extension word
			ext := c.fetchPC()
			return ea{mode: eaMemory, addr: c.calcIndex(pc, ext)}

		case 4: // #imm - Immediate
			switch sz {
			case sizeByte:
				val := c.fetchPC()
				return ea{mode: eaImmediate, imm: uint32(val & 0xFF)}
			case sizeWord:
				val := c.fetchPC()
				return ea{mode: eaImmediate, imm: uint32(val)}
			case sizeLong:
				val := c.fetchPCLong()
				return ea{mode: eaImmediate, imm: val}
			}
		}
	}

	// Invalid EA - treat as illegal instruction
	c.exception(vecIllegalInstruction)
	return ea{}
}

// calcIndex computes a base + d8(Xn) indexed address from an extension word.
// Extension word format: D/A | Reg(3) | W/L | 0(3) | Disp(8)
func (c *CPU) calcIndex(base uint32, ext uint16) uint32 {
	disp := int8(ext & 0xFF)
	xn := (ext >> 12) & 7

	var idx int32
	if ext&0x8000 != 0 {
		idx = int32(c.reg.A[xn])
	} else {
		idx = int32(c.reg.D[xn])
	}

	// Bit 11: 0 = sign-extend word index, 1 = full long index
	if ext&0x0800 == 0 {
		idx = int32(int16(idx))
	}

	return uint32(int32(base) + idx + int32(disp))
}
