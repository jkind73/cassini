package m68k

import "testing"

// testBus is a flat 16MB byte-array bus for testing.
// Supports Read/Write at any address in the 24-bit space.
type testBus struct {
	mem [16 * 1024 * 1024]byte
}

func (b *testBus) Read8(addr uint32) uint8 {
	addr &= 0xFFFFFF
	return b.mem[addr]
}

func (b *testBus) Read16(addr uint32) uint16 {
	addr &= 0xFFFFFF
	return uint16(b.mem[addr])<<8 | uint16(b.mem[addr+1])
}

func (b *testBus) Read32(addr uint32) uint32 {
	addr &= 0xFFFFFF
	return uint32(b.mem[addr])<<24 | uint32(b.mem[addr+1])<<16 |
		uint32(b.mem[addr+2])<<8 | uint32(b.mem[addr+3])
}

func (b *testBus) Write8(addr uint32, val uint8) {
	addr &= 0xFFFFFF
	b.mem[addr] = val
}

func (b *testBus) Write16(addr uint32, val uint16) {
	addr &= 0xFFFFFF
	b.mem[addr] = byte(val >> 8)
	b.mem[addr+1] = byte(val)
}

func (b *testBus) Write32(addr uint32, val uint32) {
	addr &= 0xFFFFFF
	b.mem[addr] = byte(val >> 24)
	b.mem[addr+1] = byte(val >> 16)
	b.mem[addr+2] = byte(val >> 8)
	b.mem[addr+3] = byte(val)
}

func (b *testBus) Reset() {}

// cpuState captures the full programmer-visible state for a test case.
// RAM entries are [address, byte_value] pairs.
// A[7] is unused; the active stack pointer is derived from USP/SSP/SR.
type cpuState struct {
	D      [8]uint32
	A      [7]uint32
	PC     uint32
	SR     uint16
	USP    uint32
	SSP    uint32
	RAM    [][2]uint32
	Halted bool
	Cycles int // Expected cycle count (0 = don't check)
}

// prefetchOffset is the 68000 prefetch pipeline offset.
// The SingleStepTests JSON data models the 68000's 2-word prefetch queue,
// where the PC register is 4 bytes ahead of the instruction being executed.
// Our emulator does not model the prefetch pipeline, so we adjust PC by -4
// when loading initial state and comparing final state.
const prefetchOffset uint32 = 4

// runTest loads initial state, executes one Step, and compares against expected state.
// PC values from the test data are adjusted by -prefetchOffset to account for the
// 68000's prefetch pipeline (instruction is at PC-4 in the hardware model).
func runTest(t *testing.T, init, want cpuState) {
	t.Helper()

	bus := &testBus{}

	// Load initial RAM (byte-level entries)
	for _, entry := range init.RAM {
		bus.mem[entry[0]&0xFFFFFF] = byte(entry[1])
	}

	// Bridge [7]uint32 to [8]uint32 for SetState (A7 is set from USP/SSP)
	var a8 [8]uint32
	copy(a8[:7], init.A[:])
	cpu := &CPU{bus: bus}
	cpu.SetState(Registers{D: init.D, A: a8, PC: init.PC - prefetchOffset, SR: init.SR, USP: init.USP, SSP: init.SSP})

	gotCycles := cpu.Step()

	if want.Halted {
		if !cpu.Halted() {
			t.Errorf("expected CPU to be halted, but it is not")
		}
		return // Register/memory state is undefined after halt
	}
	if cpu.Halted() {
		t.Errorf("CPU unexpectedly halted")
		return
	}

	reg := cpu.Registers()

	// Compare data registers
	for i := 0; i < 8; i++ {
		if reg.D[i] != want.D[i] {
			t.Errorf("D%d = 0x%08X, want 0x%08X", i, reg.D[i], want.D[i])
		}
	}

	// Compare address registers (A0-A6)
	for i := 0; i < 7; i++ {
		if reg.A[i] != want.A[i] {
			t.Errorf("A%d = 0x%08X, want 0x%08X", i, reg.A[i], want.A[i])
		}
	}

	// Compare stack pointers and A7.
	// In supervisor mode, A[7] is the live SSP and reg.USP is the shadow USP.
	// In user mode, A[7] is the live USP and reg.SSP is the shadow SSP.
	// The JSON always provides the "real" USP/SSP values regardless of mode.
	if want.SR&0x2000 != 0 {
		// Supervisor mode: A7 = SSP, USP is shadow
		if reg.A[7] != want.SSP {
			t.Errorf("A7/SSP = 0x%08X, want 0x%08X", reg.A[7], want.SSP)
		}
		if reg.USP != want.USP {
			t.Errorf("USP = 0x%08X, want 0x%08X", reg.USP, want.USP)
		}
	} else {
		// User mode: A7 = USP, SSP is shadow
		if reg.A[7] != want.USP {
			t.Errorf("A7/USP = 0x%08X, want 0x%08X", reg.A[7], want.USP)
		}
		if reg.SSP != want.SSP {
			t.Errorf("SSP = 0x%08X, want 0x%08X", reg.SSP, want.SSP)
		}
	}

	// Compare PC (adjusted for prefetch offset)
	wantPC := want.PC - prefetchOffset
	if reg.PC != wantPC {
		t.Errorf("PC = 0x%08X, want 0x%08X", reg.PC, wantPC)
	}

	// Compare SR
	if reg.SR != want.SR {
		t.Errorf("SR = 0x%04X, want 0x%04X (diff: %04X)", reg.SR, want.SR, reg.SR^want.SR)
	}

	// Compare RAM
	for _, entry := range want.RAM {
		addr := entry[0] & 0xFFFFFF
		wantVal := byte(entry[1])
		gotVal := bus.mem[addr]
		if gotVal != wantVal {
			t.Errorf("RAM[0x%06X] = 0x%02X, want 0x%02X", addr, gotVal, wantVal)
		}
	}

	// Compare cycles (when expected value is provided)
	if want.Cycles > 0 && gotCycles != want.Cycles {
		t.Errorf("cycles = %d, want %d", gotCycles, want.Cycles)
	}
}

// writeWord stores a big-endian 16-bit word into the test bus memory.
func writeWord(bus *testBus, addr uint32, val uint16) {
	bus.mem[addr] = byte(val >> 8)
	bus.mem[addr+1] = byte(val)
}

// fillNOPs writes NOP instructions (0x4E71, 4 cycles each) starting at addr.
func fillNOPs(bus *testBus, addr uint32, count int) {
	for i := 0; i < count; i++ {
		writeWord(bus, addr+uint32(i*2), 0x4E71)
	}
}

// newNOPCPU creates a CPU with NOPs at the given PC and returns it ready to run.
func newNOPCPU(nopCount int) (*CPU, *testBus) {
	bus := &testBus{}
	pc := uint32(0x1000)
	fillNOPs(bus, pc, nopCount)
	cpu := &CPU{bus: bus}
	cpu.SetState(Registers{PC: pc, SR: 0x2700, SSP: 0x10000})
	return cpu, bus
}
