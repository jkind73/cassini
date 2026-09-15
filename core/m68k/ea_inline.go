package m68k

// eaReadFunc reads a value from a pre-resolved effective address.
type eaReadFunc func(c *CPU, sz size) uint32

// eaAddrFunc computes a memory effective address.
type eaAddrFunc func(c *CPU, sz size) uint32

// makeEARead returns a closure that reads from the given EA mode/reg.
// The mode/reg are baked into the closure, eliminating the resolveEA
// and ea.read switch dispatches at execution time.
func makeEARead(mode, reg uint16) eaReadFunc {
	switch mode {
	case 0:
		return func(c *CPU, sz size) uint32 { return c.reg.D[reg] & sz.Mask() }
	case 1:
		return func(c *CPU, sz size) uint32 { return c.reg.A[reg] & sz.Mask() }
	case 2:
		return func(c *CPU, sz size) uint32 { return c.readBus(sz, c.reg.A[reg]) }
	case 3:
		if reg == 7 {
			return func(c *CPU, sz size) uint32 {
				addr := c.reg.A[7]
				inc := uint32(sz)
				if sz == sizeByte {
					inc = 2
				}
				c.reg.A[7] += inc
				return c.readBus(sz, addr)
			}
		}
		return func(c *CPU, sz size) uint32 {
			addr := c.reg.A[reg]
			c.reg.A[reg] += uint32(sz)
			return c.readBus(sz, addr)
		}
	case 4:
		if reg == 7 {
			return func(c *CPU, sz size) uint32 {
				dec := uint32(sz)
				if sz == sizeByte {
					dec = 2
				}
				c.reg.A[7] -= dec
				return c.readBus(sz, c.reg.A[7])
			}
		}
		return func(c *CPU, sz size) uint32 {
			c.reg.A[reg] -= uint32(sz)
			return c.readBus(sz, c.reg.A[reg])
		}
	case 5:
		return func(c *CPU, sz size) uint32 {
			disp := int16(c.fetchPC())
			return c.readBus(sz, uint32(int32(c.reg.A[reg])+int32(disp)))
		}
	case 6:
		return func(c *CPU, sz size) uint32 {
			ext := c.fetchPC()
			return c.readBus(sz, c.calcIndex(c.reg.A[reg], ext))
		}
	case 7:
		switch reg {
		case 0:
			return func(c *CPU, sz size) uint32 {
				addr := int16(c.fetchPC())
				return c.readBus(sz, uint32(int32(addr)))
			}
		case 1:
			return func(c *CPU, sz size) uint32 {
				return c.readBus(sz, c.fetchPCLong())
			}
		case 2:
			return func(c *CPU, sz size) uint32 {
				pc := c.reg.PC
				disp := int16(c.fetchPC())
				return c.readBus(sz, uint32(int32(pc)+int32(disp)))
			}
		case 3:
			return func(c *CPU, sz size) uint32 {
				pc := c.reg.PC
				ext := c.fetchPC()
				return c.readBus(sz, c.calcIndex(pc, ext))
			}
		case 4:
			return func(c *CPU, sz size) uint32 {
				if sz == sizeLong {
					return c.fetchPCLong()
				}
				return uint32(c.fetchPC()) & sz.Mask()
			}
		}
	}
	return nil
}

// makeEAMemAddr returns a closure that computes a memory effective address.
// Valid for modes 2-7 (memory addressing modes). Side effects
// (postincrement/predecrement) are applied during address computation.
func makeEAMemAddr(mode, reg uint16) eaAddrFunc {
	switch mode {
	case 2:
		return func(c *CPU, _ size) uint32 { return c.reg.A[reg] }
	case 3:
		if reg == 7 {
			return func(c *CPU, sz size) uint32 {
				addr := c.reg.A[7]
				inc := uint32(sz)
				if sz == sizeByte {
					inc = 2
				}
				c.reg.A[7] += inc
				return addr
			}
		}
		return func(c *CPU, sz size) uint32 {
			addr := c.reg.A[reg]
			c.reg.A[reg] += uint32(sz)
			return addr
		}
	case 4:
		if reg == 7 {
			return func(c *CPU, sz size) uint32 {
				dec := uint32(sz)
				if sz == sizeByte {
					dec = 2
				}
				c.reg.A[7] -= dec
				return c.reg.A[7]
			}
		}
		return func(c *CPU, sz size) uint32 {
			c.reg.A[reg] -= uint32(sz)
			return c.reg.A[reg]
		}
	case 5:
		return func(c *CPU, _ size) uint32 {
			disp := int16(c.fetchPC())
			return uint32(int32(c.reg.A[reg]) + int32(disp))
		}
	case 6:
		return func(c *CPU, _ size) uint32 {
			ext := c.fetchPC()
			return c.calcIndex(c.reg.A[reg], ext)
		}
	case 7:
		switch reg {
		case 0:
			return func(c *CPU, _ size) uint32 {
				addr := int16(c.fetchPC())
				return uint32(int32(addr))
			}
		case 1:
			return func(c *CPU, _ size) uint32 { return c.fetchPCLong() }
		case 2:
			return func(c *CPU, _ size) uint32 {
				pc := c.reg.PC
				disp := int16(c.fetchPC())
				return uint32(int32(pc) + int32(disp))
			}
		case 3:
			return func(c *CPU, _ size) uint32 {
				pc := c.reg.PC
				ext := c.fetchPC()
				return c.calcIndex(pc, ext)
			}
		}
	}
	return nil
}

// eaFetchConst returns precomputed EA source fetch cycle costs.
// base is the cost for byte/word sizes. longExtra is added for long size.
func eaFetchConst(mode, reg uint16) (base uint64, longExtra uint64) {
	switch mode {
	case 0, 1:
		return 0, 0
	case 2, 3:
		return 4, 4
	case 4:
		return 6, 4
	case 5:
		return 8, 4
	case 6:
		return 10, 4
	case 7:
		switch reg {
		case 0:
			return 8, 4
		case 1:
			return 12, 4
		case 2:
			return 8, 4
		case 3:
			return 10, 4
		case 4:
			return 4, 4
		}
	}
	return 0, 0
}

// eaWriteConst returns precomputed EA destination write cycle costs.
// Same as eaFetchConst except -(An) costs 4 instead of 6.
func eaWriteConst(mode, reg uint16) (base uint64, longExtra uint64) {
	switch mode {
	case 0, 1:
		return 0, 0
	case 2, 3, 4:
		return 4, 4
	case 5:
		return 8, 4
	case 6:
		return 10, 4
	case 7:
		switch reg {
		case 0:
			return 8, 4
		case 1:
			return 12, 4
		}
	}
	return 0, 0
}
