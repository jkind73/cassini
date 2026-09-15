package m68k

import "testing"

// setIPLCPU builds a CPU executing NOPs at 0x1000 with the level-2 autovector
// (vector 26, offset 0x68) pointing at a NOP handler at 0x2000, and the status
// register set to sr. Used to observe whether SetIPL causes the level-2
// interrupt to be taken on the next Step.
func setIPLCPU(sr uint16) (*CPU, *testBus) {
	bus := &testBus{}
	fillNOPs(bus, 0x1000, 8)
	fillNOPs(bus, 0x2000, 8)
	bus.Write32(0x68, 0x2000) // vector 26 = level 2 autovector -> handler
	cpu := &CPU{bus: bus}
	cpu.SetState(Registers{PC: 0x1000, SR: sr, SSP: 0x10000})
	return cpu, bus
}

// TestSetIPLLevelSensitive verifies that SetIPL drives the IPL2-IPL0 inputs as
// a level: a level above the mask is taken, and a level lowered back below the
// mask (including to 0) before the CPU services it is not taken. This mirrors
// the M68000 User's Manual Sec 3.5 (encoded level, level 0 = no request) and
// Sec 6.3.2 (pending level compared to the SR mask between instructions).
func TestSetIPLLevelSensitive(t *testing.T) {
	// Asserted level above the mask is taken: SetIPL(2) with SR mask 0 jumps
	// into the level-2 autovector handler at 0x2000.
	t.Run("asserted level is taken", func(t *testing.T) {
		cpu, _ := setIPLCPU(0x2000) // supervisor, mask 0
		cpu.SetIPL(2, nil)
		cpu.Step()
		if pc := cpu.Registers().PC; pc < 0x2000 || pc > 0x2010 {
			t.Errorf("PC = 0x%06X, want interrupt handler near 0x2000", pc)
		}
		if m := (cpu.Registers().SR >> 8) & 7; m != 2 {
			t.Errorf("SR mask = %d, want 2 (set to acknowledged level)", m)
		}
	})

	// Withdrawal: a request lowered back to 0 before the masked CPU services
	// it must not be taken (level 0 = no interrupt requested). The next Step
	// executes the NOP at 0x1000 instead of jumping to the handler.
	t.Run("withdrawn before service is not taken", func(t *testing.T) {
		cpu, _ := setIPLCPU(0x2000) // supervisor, mask 0
		cpu.SetIPL(2, nil)
		cpu.SetIPL(0, nil)
		cpu.Step()
		if pc := cpu.Registers().PC; pc != 0x1002 {
			t.Errorf("PC = 0x%06X, want 0x1002 (NOP executed, no interrupt)", pc)
		}
	})

	// Lowering to a level at or below the mask is honored: SetIPL(5) then
	// SetIPL(2) with mask 3 leaves level 2, which does not exceed the mask, so
	// nothing is taken. A raise-only latch would keep level 5 and wrongly fire.
	t.Run("lowered level at or below mask is not taken", func(t *testing.T) {
		cpu, _ := setIPLCPU(0x2300) // supervisor, mask 3
		cpu.SetIPL(5, nil)
		cpu.SetIPL(2, nil)
		cpu.Step()
		if pc := cpu.Registers().PC; pc != 0x1002 {
			t.Errorf("PC = 0x%06X, want 0x1002 (level 2 <= mask 3, no interrupt)", pc)
		}
	})
}
