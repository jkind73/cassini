package m68k

import "log"

// MC68000 exception vector numbers.
const (
	vecResetSSP           = 0
	vecResetPC            = 1
	vecBusError           = 2
	vecAddressError       = 3
	vecIllegalInstruction = 4
	vecDivideByZero       = 5
	vecCHK                = 6
	vecTRAPV              = 7
	vecPrivilegeViolation = 8
	vecTrace              = 9
	vecLineA              = 10
	vecLineF              = 11
	vecUninitialized      = 15
	vecSpuriousInterrupt  = 24
	vecAutoVector1        = 25
	vecTrap0              = 32 // TRAP #0 through TRAP #15 = vectors 32-47
)

// exception processes an exception: enters supervisor mode, pushes the
// return frame (PC + SR), reads the vector, and jumps to the handler.
func (c *CPU) exception(vector int) {
	// Log error exceptions (vectors 2-11) for diagnostics
	if vector >= vecBusError && vector <= vecLineF {
		log.Printf("[m68k] exception %d at PC=%06x SR=%04x", vector, c.reg.PC, c.reg.SR)
	}

	// Determine the PC to push. For group 1 fault exceptions (illegal
	// instruction, privilege violation, Line-A, Line-F), the 68000 pushes
	// the address of the faulting instruction. For all other exceptions
	// (group 2: TRAP, TRAPV, CHK, divide-by-zero; and interrupts/trace),
	// the 68000 pushes the next instruction address (current PC).
	pushPC := c.reg.PC
	switch vector {
	case vecIllegalInstruction, vecPrivilegeViolation, vecLineA, vecLineF:
		pushPC = c.prevPC
	}

	oldSR := c.reg.SR

	// Enter supervisor mode, clear trace
	if c.reg.SR&flagS == 0 {
		c.reg.USP = c.reg.A[7]
		c.reg.A[7] = c.reg.SSP
	}
	c.reg.SR = (c.reg.SR | flagS) & ^flagT

	// Push PC and old SR onto supervisor stack
	c.pushLong(pushPC)
	c.pushWord(oldSR)

	// Read handler address from vector table
	addr := c.readBus(sizeLong, uint32(vector)*4)
	if addr == 0 {
		// Uninitialized vector: try the uninitialized-interrupt vector
		addr = c.readBus(sizeLong, vecUninitialized*4)
		if addr == 0 {
			// Double fault on uninitialized vectors: halt
			c.halted = true
			return
		}
	}
	c.reg.PC = addr

	c.cycles += 34
}
