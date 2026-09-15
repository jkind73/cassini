// Package m68k implements a Motorola 68000 CPU emulator.
//
// The MC68000 is a 32-bit internal / 16-bit external CISC processor with:
//   - Eight 32-bit data registers (D0-D7)
//   - Eight 32-bit address registers (A0-A7), where A7 is the stack pointer
//   - A 32-bit program counter (24-bit external address bus)
//   - A 16-bit status register (system byte + condition code register)
//   - Dual stack pointers (USP for user mode, SSP for supervisor mode)
package m68k

import "log"

// Bus provides word-aligned memory access for the CPU.
// All addresses are 24-bit (masked by the CPU before calling).
// Word and long accesses to odd addresses are detected by the CPU
// and cause an address error before reaching the bus.
type Bus interface {
	Read8(addr uint32) uint8
	Read16(addr uint32) uint16
	Read32(addr uint32) uint32
	Write8(addr uint32, val uint8)
	Write16(addr uint32, val uint16)
	Write32(addr uint32, val uint32)
	Reset()
}

// Registers holds the programmer-visible state of the MC68000.
type Registers struct {
	D   [8]uint32 // Data registers
	A   [8]uint32 // Address registers (A7 is active stack pointer)
	PC  uint32    // Program counter
	SR  uint16    // Status register
	USP uint32    // User stack pointer (shadowed)
	SSP uint32    // Supervisor stack pointer (shadowed)
	IR  uint16    // Instruction register (first word of executing instruction)
}

// CPU is the MC68000 processor.
type CPU struct {
	reg    Registers
	bus    Bus
	cycles uint64

	// The instruction register holds the first word of the currently
	// executing instruction, latched at fetch time.
	ir uint16

	stopped bool   // Set by STOP, cleared by interrupt
	halted  bool   // Set by double bus fault
	prevPC  uint32 // PC of the previous instruction (for diagnostics)

	// Interrupt state
	pendingIPL uint8  // Pending interrupt priority level (1-7, 0=none)
	pendingVec *uint8 // Pending interrupt vector (nil = auto-vector)

	// Cycle deficit from StepCycles when an instruction's cost exceeded the budget.
	deficit int
}

// New creates a CPU wired to the given bus and performs a hardware reset.
// The reset reads the initial SSP from address 0 and PC from address 4.
func New(bus Bus) *CPU {
	c := &CPU{bus: bus}
	c.Reset()
	return c
}

// Reset performs a hardware reset: loads SSP from address 0x000000 and
// PC from address 0x000004, enters supervisor mode with interrupts masked.
func (c *CPU) Reset() {
	c.reg = Registers{SR: 0x2700}
	c.stopped = false
	c.halted = false
	c.cycles = 0
	c.deficit = 0
	c.pendingIPL = 0
	c.pendingVec = nil

	ssp := c.bus.Read32(0)
	c.reg.A[7] = ssp
	c.reg.SSP = ssp
	c.reg.PC = c.bus.Read32(4)
}

// Halted returns true if the CPU is halted due to a double bus fault.
func (c *CPU) Halted() bool {
	return c.halted
}

// Step executes a single instruction and returns the number of cycles consumed.
// Returns 0 if the CPU is halted (double bus fault).
func (c *CPU) Step() int {
	if c.halted {
		return 0
	}

	before := c.cycles

	if c.stopped {
		c.cycles += 4
		c.checkInterrupt()
		return int(c.cycles - before)
	}

	c.checkInterrupt()

	// Address error: instruction fetch from odd PC
	if c.reg.PC&1 != 0 {
		log.Printf("[m68k] address error: odd PC=%06x prevPC=%06x prevIR=%04x",
			c.reg.PC, c.prevPC, c.ir)
		c.halted = true
		return 0
	}

	c.prevPC = c.reg.PC
	c.ir = c.fetchPC()
	c.reg.IR = c.ir

	handler := opcodeTable[c.ir]
	if handler == nil {
		switch c.ir >> 12 {
		case 0xA:
			c.exception(vecLineA)
		case 0xF:
			c.exception(vecLineF)
		default:
			c.exception(vecIllegalInstruction)
		}
	} else {
		handler(c)
	}

	// Post-instruction odd-PC check: catch branches/jumps to odd addresses.
	// On real hardware the prefetch pipeline would trigger this during the
	// instruction; we don't model prefetch so check here instead.
	if !c.halted && c.reg.PC&1 != 0 {
		log.Printf("[m68k] address error: odd PC=%06x prevPC=%06x IR=%04x",
			c.reg.PC, c.prevPC, c.ir)
		c.halted = true
	}

	return int(c.cycles - before)
}

// StepCycles executes a single instruction within the given cycle budget.
// If a previous instruction's cost exceeded its budget, the deficit is paid
// down first without executing a new instruction. When a new instruction
// executes and its cost exceeds the budget, the excess is stored as a
// deficit to be charged on subsequent calls. Returns the number of cycles
// consumed from this call's budget.
func (c *CPU) StepCycles(budget int) int {
	if c.halted {
		return 0
	}

	// Pay down deficit from a previous instruction that exceeded its budget.
	if c.deficit > 0 {
		if budget >= c.deficit {
			n := c.deficit
			c.deficit = 0
			return n
		}
		c.deficit -= budget
		return budget
	}

	cost := c.Step()

	if cost <= budget {
		return cost
	}

	c.deficit = cost - budget
	return budget
}

// Deficit returns the remaining cycle deficit from a previous StepCycles
// call where the instruction cost exceeded the budget.
func (c *CPU) Deficit() int {
	return c.deficit
}

// Cycles returns the total cycle count since the last reset.
func (c *CPU) Cycles() uint64 {
	return c.cycles
}

// AddCycles advances the cycle counter by n without executing any
// instruction. Used to account for external bus-hold periods such as
// DMA seizing the 68K bus.
func (c *CPU) AddCycles(n uint64) {
	c.cycles += n
}

// Registers returns a snapshot of the current register state.
func (c *CPU) Registers() Registers {
	return c.reg
}

// RequestInterrupt queues an interrupt at the given priority level (1-7).
// Pass nil for vector to use auto-vectoring.
// A higher level replaces a lower pending level.
func (c *CPU) RequestInterrupt(level uint8, vector *uint8) {
	if level > c.pendingIPL {
		c.pendingIPL = level
		c.pendingVec = vector
	}
}

// SetIPL drives the encoded interrupt level present on the IPL2-IPL0
// inputs (M68000 User's Manual Sec 3.5). The input is level-sensitive: it
// reflects the level the interrupting device currently asserts, so it may
// raise or lower the pending level (level 0 means no interrupt requested).
// A device that withdraws its request before the processor acknowledges it
// must lower the line through this method; a request still asserted at an
// instruction boundary is compared against the status register mask
// (Sec 6.3.2). Pass nil for vector to use auto-vectoring.
func (c *CPU) SetIPL(level uint8, vector *uint8) {
	c.pendingIPL = level
	c.pendingVec = vector
}

// readBus reads from the bus with 24-bit address masking.
// Word and long accesses to odd addresses halt the CPU (address error).
func (c *CPU) readBus(sz size, addr uint32) uint32 {
	if c.halted {
		return 0
	}
	if sz != sizeByte && addr&1 != 0 {
		log.Printf("[m68k] address error: read %s from odd addr=%06x PC=%06x prevPC=%06x IR=%04x",
			sz, addr&0xFFFFFF, c.reg.PC, c.prevPC, c.ir)
		c.halted = true
		return 0
	}
	addr &= 0xFFFFFF
	switch sz {
	case sizeByte:
		return uint32(c.bus.Read8(addr))
	case sizeWord:
		return uint32(c.bus.Read16(addr))
	case sizeLong:
		return uint32(c.bus.Read32(addr))
	}
	return 0
}

// writeBus writes to the bus with 24-bit address masking.
// Word and long accesses to odd addresses halt the CPU (address error).
func (c *CPU) writeBus(sz size, addr uint32, val uint32) {
	if c.halted {
		return
	}
	if sz != sizeByte && addr&1 != 0 {
		log.Printf("[m68k] address error: write %s to odd addr=%06x val=%08x PC=%06x prevPC=%06x IR=%04x",
			sz, addr&0xFFFFFF, val&sz.Mask(), c.reg.PC, c.prevPC, c.ir)
		c.halted = true
		return
	}
	addr &= 0xFFFFFF
	val &= sz.Mask()
	switch sz {
	case sizeByte:
		c.bus.Write8(addr, uint8(val))
	case sizeWord:
		c.bus.Write16(addr, uint16(val))
	case sizeLong:
		c.bus.Write32(addr, val)
	}
}

// fetchPC reads a 16-bit word at the current PC and advances PC by 2.
func (c *CPU) fetchPC() uint16 {
	val := c.readBus(sizeWord, c.reg.PC)
	c.reg.PC += 2
	return uint16(val)
}

// fetchPCLong reads a 32-bit long at the current PC and advances PC by 4.
func (c *CPU) fetchPCLong() uint32 {
	hi := c.fetchPC()
	lo := c.fetchPC()
	return uint32(hi)<<16 | uint32(lo)
}

// pushWord pushes a 16-bit word onto the active stack (A7).
func (c *CPU) pushWord(val uint16) {
	c.reg.A[7] -= 2
	c.writeBus(sizeWord, c.reg.A[7], uint32(val))
}

// pushLong pushes a 32-bit long onto the active stack (A7).
func (c *CPU) pushLong(val uint32) {
	c.reg.A[7] -= 4
	c.writeBus(sizeLong, c.reg.A[7], val)
}

// popWord pops a 16-bit word from the active stack (A7).
func (c *CPU) popWord() uint16 {
	val := c.readBus(sizeWord, c.reg.A[7])
	c.reg.A[7] += 2
	return uint16(val)
}

// popLong pops a 32-bit long from the active stack (A7).
func (c *CPU) popLong() uint32 {
	val := c.readBus(sizeLong, c.reg.A[7])
	c.reg.A[7] += 4
	return val
}

// supervisor returns true if the CPU is in supervisor mode.
func (c *CPU) supervisor() bool {
	return c.reg.SR&flagS != 0
}

// setSR sets the status register, handling stack pointer swaps
// when transitioning between supervisor and user mode.
func (c *CPU) setSR(sr uint16) {
	oldS := c.reg.SR & flagS
	newS := sr & flagS

	if oldS != 0 && newS == 0 {
		// Leaving supervisor mode: save SSP, restore USP
		c.reg.SSP = c.reg.A[7]
		c.reg.A[7] = c.reg.USP
	} else if oldS == 0 && newS != 0 {
		// Entering supervisor mode: save USP, restore SSP
		c.reg.USP = c.reg.A[7]
		c.reg.A[7] = c.reg.SSP
	}

	// Mask to valid 68000 SR bits: T__S__III___XNZVC (0xA71F)
	c.reg.SR = sr & 0xA71F
}

// setCCR sets only the condition code register (low byte of SR).
// Only bits 0-4 (XNZVC) are valid on the 68000; bits 5-7 are always 0.
func (c *CPU) setCCR(ccr uint8) {
	c.reg.SR = (c.reg.SR & 0xFF00) | uint16(ccr&0x1F)
}

// SetState sets all programmer-visible registers directly without
// performing a hardware reset. This is intended for testing, where
// exact CPU state must be established before executing an instruction.
func (c *CPU) SetState(regs Registers) {
	c.reg.D = regs.D
	c.reg.SR = regs.SR
	c.reg.USP = regs.USP
	c.reg.SSP = regs.SSP
	c.reg.PC = regs.PC
	c.stopped = false
	c.halted = false
	c.cycles = 0
	c.deficit = 0
	c.pendingIPL = 0
	c.pendingVec = nil

	// A7 is the active stack pointer: SSP in supervisor mode, USP in user mode
	for i := 0; i < 7; i++ {
		c.reg.A[i] = regs.A[i]
	}
	if regs.SR&flagS != 0 {
		c.reg.A[7] = regs.SSP
	} else {
		c.reg.A[7] = regs.USP
	}
}
