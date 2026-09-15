package m68k

import "testing"

func TestSerializeSize(t *testing.T) {
	if got := SerializeSize; got != 104 {
		t.Fatalf("SerializeSize = %d, want 104", got)
	}
}

func TestSerializeRoundTrip(t *testing.T) {
	bus := &testBus{}
	cpu := &CPU{bus: bus}

	// Fill with non-default values.
	for i := range cpu.reg.D {
		cpu.reg.D[i] = uint32(0x10 + i)
	}
	for i := range cpu.reg.A {
		cpu.reg.A[i] = uint32(0x20 + i)
	}
	cpu.reg.PC = 0x4000
	cpu.reg.SR = 0x2700
	cpu.reg.USP = 0x5000
	cpu.reg.SSP = 0x6000
	cpu.reg.IR = 0x4E71
	cpu.cycles = 9999
	cpu.ir = 0x1234
	cpu.stopped = true
	cpu.halted = true
	cpu.prevPC = 0x3FFE
	cpu.pendingIPL = 5
	vec := uint8(64)
	cpu.pendingVec = &vec
	cpu.deficit = 42

	buf := make([]byte, SerializeSize)
	if err := cpu.Serialize(buf); err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	// Deserialize into a fresh CPU with a different bus.
	bus2 := &testBus{}
	cpu2 := &CPU{bus: bus2}
	if err := cpu2.Deserialize(buf); err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	// Bus must not be overwritten.
	if cpu2.bus != bus2 {
		t.Fatal("Deserialize overwrote bus")
	}

	// Compare all fields.
	if cpu2.reg.D != cpu.reg.D {
		t.Errorf("D = %v, want %v", cpu2.reg.D, cpu.reg.D)
	}
	if cpu2.reg.A != cpu.reg.A {
		t.Errorf("A = %v, want %v", cpu2.reg.A, cpu.reg.A)
	}
	if cpu2.reg.PC != cpu.reg.PC {
		t.Errorf("PC = 0x%X, want 0x%X", cpu2.reg.PC, cpu.reg.PC)
	}
	if cpu2.reg.SR != cpu.reg.SR {
		t.Errorf("SR = 0x%X, want 0x%X", cpu2.reg.SR, cpu.reg.SR)
	}
	if cpu2.reg.USP != cpu.reg.USP {
		t.Errorf("USP = 0x%X, want 0x%X", cpu2.reg.USP, cpu.reg.USP)
	}
	if cpu2.reg.SSP != cpu.reg.SSP {
		t.Errorf("SSP = 0x%X, want 0x%X", cpu2.reg.SSP, cpu.reg.SSP)
	}
	if cpu2.reg.IR != cpu.reg.IR {
		t.Errorf("IR = 0x%X, want 0x%X", cpu2.reg.IR, cpu.reg.IR)
	}
	if cpu2.cycles != cpu.cycles {
		t.Errorf("cycles = %d, want %d", cpu2.cycles, cpu.cycles)
	}
	if cpu2.ir != cpu.ir {
		t.Errorf("ir = 0x%X, want 0x%X", cpu2.ir, cpu.ir)
	}
	if cpu2.stopped != cpu.stopped {
		t.Errorf("stopped = %v, want %v", cpu2.stopped, cpu.stopped)
	}
	if cpu2.halted != cpu.halted {
		t.Errorf("halted = %v, want %v", cpu2.halted, cpu.halted)
	}
	if cpu2.prevPC != cpu.prevPC {
		t.Errorf("prevPC = 0x%X, want 0x%X", cpu2.prevPC, cpu.prevPC)
	}
	if cpu2.pendingIPL != cpu.pendingIPL {
		t.Errorf("pendingIPL = %d, want %d", cpu2.pendingIPL, cpu.pendingIPL)
	}
	if cpu2.pendingVec == nil {
		t.Fatal("pendingVec = nil, want non-nil")
	}
	if *cpu2.pendingVec != *cpu.pendingVec {
		t.Errorf("*pendingVec = %d, want %d", *cpu2.pendingVec, *cpu.pendingVec)
	}
	if cpu2.deficit != cpu.deficit {
		t.Errorf("deficit = %d, want %d", cpu2.deficit, cpu.deficit)
	}
}

func TestSerializeRoundTripNilVector(t *testing.T) {
	bus := &testBus{}
	cpu := &CPU{bus: bus}
	cpu.reg.PC = 0x1000
	cpu.reg.SR = 0x2700
	cpu.pendingIPL = 3
	cpu.pendingVec = nil

	buf := make([]byte, SerializeSize)
	if err := cpu.Serialize(buf); err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	cpu2 := &CPU{bus: &testBus{}}
	if err := cpu2.Deserialize(buf); err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	if cpu2.pendingVec != nil {
		t.Errorf("pendingVec = %v, want nil", cpu2.pendingVec)
	}
	if cpu2.pendingIPL != 3 {
		t.Errorf("pendingIPL = %d, want 3", cpu2.pendingIPL)
	}
}

func TestSerializeRejectsTooSmall(t *testing.T) {
	cpu := &CPU{bus: &testBus{}}
	if err := cpu.Serialize(make([]byte, 10)); err == nil {
		t.Fatal("Serialize accepted a short buffer")
	}
}

func TestSerializeDeserializeRejectsTooSmall(t *testing.T) {
	cpu := &CPU{bus: &testBus{}}
	if err := cpu.Deserialize(make([]byte, 10)); err == nil {
		t.Fatal("Deserialize accepted a short buffer")
	}
}

func TestSerializeDeserializeRejectsBadVersion(t *testing.T) {
	cpu := &CPU{bus: &testBus{}}

	buf := make([]byte, SerializeSize)
	if err := cpu.Serialize(buf); err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	buf[0] = 99 // corrupt version
	cpu2 := &CPU{bus: &testBus{}}
	if err := cpu2.Deserialize(buf); err == nil {
		t.Fatal("Deserialize accepted wrong version")
	}
}

func TestSerializeResumeExecution(t *testing.T) {
	// Create a CPU with a small NOP program.
	bus := &testBus{}
	pc := uint32(0x1000)
	fillNOPs(bus, pc, 10)
	cpu1 := &CPU{bus: bus}
	cpu1.SetState(Registers{PC: pc, SR: 0x2700, SSP: 0x10000})

	// Run a few steps.
	cpu1.Step()
	cpu1.Step()

	// Serialize.
	buf := make([]byte, SerializeSize)
	if err := cpu1.Serialize(buf); err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	// Deserialize into a second CPU on the same bus.
	cpu2 := &CPU{bus: bus}
	if err := cpu2.Deserialize(buf); err != nil {
		t.Fatalf("Deserialize failed: %v", err)
	}

	// Run one more step on both.
	c1 := cpu1.Step()
	c2 := cpu2.Step()

	if c1 != c2 {
		t.Errorf("step cycles: cpu1=%d, cpu2=%d", c1, c2)
	}

	r1 := cpu1.Registers()
	r2 := cpu2.Registers()
	if r1 != r2 {
		t.Errorf("registers diverged:\n  cpu1=%+v\n  cpu2=%+v", r1, r2)
	}
	if cpu1.Cycles() != cpu2.Cycles() {
		t.Errorf("total cycles: cpu1=%d, cpu2=%d", cpu1.Cycles(), cpu2.Cycles())
	}
}
