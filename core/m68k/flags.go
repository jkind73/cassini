package m68k

// Status register flag bits.
const (
	flagC uint16 = 1 << iota // Carry
	flagV                    // Overflow
	flagZ                    // Zero
	flagN                    // Negative
	flagX                    // Extend

	flagS uint16 = 1 << 13 // Supervisor
	flagT uint16 = 1 << 15 // Trace
)

// setFlagsAdd sets XNZVC after an addition: result = dst + src.
func (c *CPU) setFlagsAdd(src, dst, result uint32, sz size) {
	msb := sz.MSB()
	mask := sz.Mask()
	r := result & mask
	s := src & mask
	d := dst & mask

	c.reg.SR &^= flagX | flagN | flagZ | flagV | flagC

	if r == 0 {
		c.reg.SR |= flagZ
	}
	if r&msb != 0 {
		c.reg.SR |= flagN
	}
	// Overflow: both operands same sign, result different sign
	if (s^r)&(d^r)&msb != 0 {
		c.reg.SR |= flagV
	}
	// Carry: unsigned overflow
	if result&(msb<<1) != 0 || (sz == sizeLong && ((s&d|(s|d)&^r)&msb != 0)) {
		c.reg.SR |= flagC | flagX
	}
}

// setFlagsSub sets XNZVC after a subtraction: result = dst - src.
func (c *CPU) setFlagsSub(src, dst, result uint32, sz size) {
	msb := sz.MSB()
	mask := sz.Mask()
	r := result & mask
	s := src & mask
	d := dst & mask

	c.reg.SR &^= flagX | flagN | flagZ | flagV | flagC

	if r == 0 {
		c.reg.SR |= flagZ
	}
	if r&msb != 0 {
		c.reg.SR |= flagN
	}
	// Overflow: operands different sign, result sign differs from dst
	if (s^d)&(r^d)&msb != 0 {
		c.reg.SR |= flagV
	}
	// Borrow
	if (s&^d|r&^d|s&r)&msb != 0 {
		c.reg.SR |= flagC | flagX
	}
}

// setFlagsCmp sets NZVC after a comparison (subtraction without storing).
// Does not modify the X flag.
func (c *CPU) setFlagsCmp(src, dst, result uint32, sz size) {
	msb := sz.MSB()
	mask := sz.Mask()
	r := result & mask
	s := src & mask
	d := dst & mask

	c.reg.SR &^= flagN | flagZ | flagV | flagC

	if r == 0 {
		c.reg.SR |= flagZ
	}
	if r&msb != 0 {
		c.reg.SR |= flagN
	}
	if (s^d)&(r^d)&msb != 0 {
		c.reg.SR |= flagV
	}
	if (s&^d|r&^d|s&r)&msb != 0 {
		c.reg.SR |= flagC
	}
}

// setFlagsLogical sets NZ, clears VC after a logical operation.
func (c *CPU) setFlagsLogical(result uint32, sz size) {
	c.reg.SR &^= flagN | flagZ | flagV | flagC

	if result&sz.Mask() == 0 {
		c.reg.SR |= flagZ
	}
	if result&sz.MSB() != 0 {
		c.reg.SR |= flagN
	}
}

// testCondition evaluates an MC68000 condition code (0-15).
func (c *CPU) testCondition(cc uint16) bool {
	sr := c.reg.SR
	switch cc {
	case 0: // T - True
		return true
	case 1: // F - False
		return false
	case 2: // HI - !C & !Z
		return sr&(flagC|flagZ) == 0
	case 3: // LS - C | Z
		return sr&(flagC|flagZ) != 0
	case 4: // CC - !C
		return sr&flagC == 0
	case 5: // CS - C
		return sr&flagC != 0
	case 6: // NE - !Z
		return sr&flagZ == 0
	case 7: // EQ - Z
		return sr&flagZ != 0
	case 8: // VC - !V
		return sr&flagV == 0
	case 9: // VS - V
		return sr&flagV != 0
	case 10: // PL - !N
		return sr&flagN == 0
	case 11: // MI - N
		return sr&flagN != 0
	case 12: // GE - (N & V) | (!N & !V)
		n := sr&flagN != 0
		v := sr&flagV != 0
		return n == v
	case 13: // LT - (N & !V) | (!N & V)
		n := sr&flagN != 0
		v := sr&flagV != 0
		return n != v
	case 14: // GT - (N & V & !Z) | (!N & !V & !Z)
		n := sr&flagN != 0
		v := sr&flagV != 0
		z := sr&flagZ != 0
		return n == v && !z
	case 15: // LE - Z | (N & !V) | (!N & V)
		n := sr&flagN != 0
		v := sr&flagV != 0
		z := sr&flagZ != 0
		return z || n != v
	}
	return false
}
