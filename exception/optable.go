package sh2

// Opcode table

func (c *CPU) opIllegal() {
	if c.inDelay {
		c.serviceException(uint32(vecSlotIllegal))
	} else {
		c.serviceException(uint32(vecIllegalInstr))
	}
}
