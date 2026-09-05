package sh2

// SH-2 Interrupt and Exception handling

type CPU struct {
	PC         uint32
	SR         uint32
	R          [16]uint32
	VBR        uint32
	pending    []func()
	pendingVal uint32 // for SR snapshot
	pendingVal2 uint32 // for PC
	inDelay    bool
	// more fields...
}

const (
	vecIllegalInstr = 4
	vecSlotIllegal  = 6
)

func (c *CPU) setPending(handler func(), delayCycles int) {
	// Schedule for later
	c.pending = append(c.pending, handler)
	// In real, would use timer or cycle count
}

func (c *CPU) serviceException(vec uint32) {
	// Refactored to schedule synchronous exceptions into decomposed 5-cycle pipeline
	// Snapshot pendingVal (SR) and pendingVal2 (stacked return PC)
	c.pendingVal = c.SR
	c.pendingVal2 = c.PC // per Table 4.11
	c.setPending(func() { c.popException(vec) }, 4)
}

// popException defined in cpu.go for 5-cycle pipeline
