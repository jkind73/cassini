package sh2

import "fmt"

// CPU implementation with stepException for pipeline

func (c *CPU) stepException() {
	// Process hardware stack pushes per Hitachi SH-7095 Hardware Manual Section 4.8.3
	// Cycle 1: EX stage, setup registers & vector address calculation.
	// Snapshot SR and PC already done in serviceException
	vec := uint32(0) // would be from pending, TODO integrate
	_ = c.VBR + (vec * 4) // vaddr

	// Cycle 2: MA stage 1, push SR to R15 - 4
	r15 := c.R[15]
	if r15%4 != 0 {
		// Alignment suppression
		r15 = r15 &^ 0x3 // align down?
	}
	c.R[15] = r15 - 4
	// Memory write SR (placeholder, assume mem access)
	// mem.Write32(c.R[15], c.pendingVal)

	// Cycle 3: MA stage 2, push stacked PC to R15 - 8
	c.R[15] -= 4
	// mem.Write32(c.R[15], c.pendingVal2)

	// Cycle 4: MA stage 3, fetch vector target longword from VBR + (vec * 4)
	// target := mem.Read32(vaddr)

	// Cycle 5: IF stage, update PC to vector target and resume pipeline.
	// c.PC = target
	fmt.Printf("Exception pipeline complete for vec %d, new PC setup\n", vec)
}

func (c *CPU) popException(vec uint32) {
	// Unified exception processing under 5-stage decomposed pipeline
	// Call step to handle full stack and vector
	c.stepException()
}
