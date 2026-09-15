package m68k

// checkInterrupt tests whether a pending interrupt should be serviced
// and processes it if so. Called at the start of each Step.
func (c *CPU) checkInterrupt() {
	if c.pendingIPL == 0 {
		return
	}

	mask := uint8((c.reg.SR >> 8) & 7)

	// Level 7 is non-maskable; all others must exceed the current mask
	if c.pendingIPL > mask || c.pendingIPL == 7 {
		c.processInterrupt()
	}
}

// processInterrupt services the pending interrupt: saves context, reads
// the vector, and jumps to the handler.
func (c *CPU) processInterrupt() {
	level := c.pendingIPL
	vec := c.pendingVec
	c.pendingIPL = 0
	c.pendingVec = nil

	oldSR := c.reg.SR

	// Enter supervisor mode, clear trace, set interrupt mask to this level
	if c.reg.SR&flagS == 0 {
		c.reg.USP = c.reg.A[7]
		c.reg.A[7] = c.reg.SSP
	}
	c.reg.SR = (c.reg.SR | flagS) & ^flagT
	c.reg.SR = (c.reg.SR & 0xF8FF) | uint16(level)<<8

	// Push return frame
	c.pushLong(c.reg.PC)
	c.pushWord(oldSR)

	// Determine vector number
	var vectorNum uint8
	if vec != nil {
		vectorNum = *vec
	} else {
		vectorNum = 24 + level // auto-vector
	}

	// Read handler address
	addr := c.readBus(sizeLong, uint32(vectorNum)*4)
	if addr == 0 {
		addr = c.readBus(sizeLong, vecSpuriousInterrupt*4)
	}

	c.reg.PC = addr

	c.stopped = false
	c.cycles += 44
}
