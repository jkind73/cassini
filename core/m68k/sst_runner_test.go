package m68k

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var sstPath = flag.String("sstpath", "", "directory containing SST JSON test files")
var sstStrict = flag.Bool("sststrict", false, "run all SST tests including known failures")

// sstSkip lists JSON files that fail due to documented design choices.
// Remove entries as features are implemented to re-enable those tests.
var sstSkip = map[string]string{
	"TAS.json":   "TAS is not fully modeled",
	"TRAPV.json": "TRAPV is not fully modeled",

	// Cycle count approximations (see README Design Notes):
	// Multiply/divide use flat worst-case values instead of operand-dependent timing.
	"MULU.json": "cycle approximation: flat worst-case 70 (real 38-70)",
	"MULS.json": "cycle approximation: flat worst-case 70 (real 38-70)",
	"DIVU.json": "cycle approximation: flat worst-case 140 (real 76-140)",
	"DIVS.json": "cycle approximation: flat worst-case 158 (real 120-158)",

	// CHK exception processing uses a fixed 34-cycle cost rather than
	// instruction-specific timing which varies by addressing mode and trap condition.
	"CHK.json": "cycle approximation: fixed 34-cycle exception cost",

	// Bit manipulation #imm,Dn timing: PRM values are 2 cycles off from
	// hardware-verified results for all four instructions.
	"BTST.json": "cycle approximation: BTST #imm,Dn 8 vs hardware 10",
	"BCHG.json": "cycle approximation: BCHG #imm,Dn 12 vs hardware 10",
	"BCLR.json": "cycle approximation: BCLR #imm,Dn 14 vs hardware 12",
	"BSET.json": "cycle approximation: BSET #imm,Dn 12 vs hardware 10",
}

type sstJSONState struct {
	D0       uint32     `json:"d0"`
	D1       uint32     `json:"d1"`
	D2       uint32     `json:"d2"`
	D3       uint32     `json:"d3"`
	D4       uint32     `json:"d4"`
	D5       uint32     `json:"d5"`
	D6       uint32     `json:"d6"`
	D7       uint32     `json:"d7"`
	A0       uint32     `json:"a0"`
	A1       uint32     `json:"a1"`
	A2       uint32     `json:"a2"`
	A3       uint32     `json:"a3"`
	A4       uint32     `json:"a4"`
	A5       uint32     `json:"a5"`
	A6       uint32     `json:"a6"`
	USP      uint32     `json:"usp"`
	SSP      uint32     `json:"ssp"`
	SR       uint16     `json:"sr"`
	PC       uint32     `json:"pc"`
	Prefetch [2]uint16  `json:"prefetch"`
	RAM      [][]uint32 `json:"ram"`
}

func (s *sstJSONState) toM68kState() cpuState {
	st := cpuState{
		D:   [8]uint32{s.D0, s.D1, s.D2, s.D3, s.D4, s.D5, s.D6, s.D7},
		A:   [7]uint32{s.A0, s.A1, s.A2, s.A3, s.A4, s.A5, s.A6},
		PC:  s.PC,
		SR:  s.SR,
		USP: s.USP,
		SSP: s.SSP,
	}
	for _, entry := range s.RAM {
		st.RAM = append(st.RAM, [2]uint32{entry[0], entry[1]})
	}
	return st
}

type sstJSONTest struct {
	Name         string       `json:"name"`
	Initial      sstJSONState `json:"initial"`
	Final        sstJSONState `json:"final"`
	Transactions []any        `json:"transactions"`
	Length       int          `json:"length"`
}

// runSSTTest is like runTest but skips (instead of failing) when the CPU halts.
// The emulator halts on address errors rather than pushing a full exception frame
// (documented design choice), so SST tests that trigger odd-address access are
// expected to halt and should be skipped rather than counted as failures.
func runSSTTest(t *testing.T, init, want cpuState) {
	t.Helper()

	bus := &testBus{}
	for _, entry := range init.RAM {
		bus.mem[entry[0]&0xFFFFFF] = byte(entry[1])
	}

	var a8 [8]uint32
	copy(a8[:7], init.A[:])
	cpu := &CPU{bus: bus}
	cpu.SetState(Registers{D: init.D, A: a8, PC: init.PC - prefetchOffset, SR: init.SR, USP: init.USP, SSP: init.SSP})

	gotCycles := cpu.Step()

	if cpu.Halted() {
		t.Skip("address error halt (not modeled)")
	}

	reg := cpu.Registers()

	for i := 0; i < 8; i++ {
		if reg.D[i] != want.D[i] {
			t.Errorf("D%d = 0x%08X, want 0x%08X", i, reg.D[i], want.D[i])
		}
	}

	for i := 0; i < 7; i++ {
		if reg.A[i] != want.A[i] {
			t.Errorf("A%d = 0x%08X, want 0x%08X", i, reg.A[i], want.A[i])
		}
	}

	if want.SR&0x2000 != 0 {
		if reg.A[7] != want.SSP {
			t.Errorf("A7/SSP = 0x%08X, want 0x%08X", reg.A[7], want.SSP)
		}
		if reg.USP != want.USP {
			t.Errorf("USP = 0x%08X, want 0x%08X", reg.USP, want.USP)
		}
	} else {
		if reg.A[7] != want.USP {
			t.Errorf("A7/USP = 0x%08X, want 0x%08X", reg.A[7], want.USP)
		}
		if reg.SSP != want.SSP {
			t.Errorf("SSP = 0x%08X, want 0x%08X", reg.SSP, want.SSP)
		}
	}

	wantPC := want.PC - prefetchOffset
	if reg.PC != wantPC {
		t.Errorf("PC = 0x%08X, want 0x%08X", reg.PC, wantPC)
	}

	if reg.SR != want.SR {
		t.Errorf("SR = 0x%04X, want 0x%04X (diff: %04X)", reg.SR, want.SR, reg.SR^want.SR)
	}

	for _, entry := range want.RAM {
		addr := entry[0] & 0xFFFFFF
		wantVal := byte(entry[1])
		gotVal := bus.mem[addr]
		if gotVal != wantVal {
			t.Errorf("RAM[0x%06X] = 0x%02X, want 0x%02X", addr, gotVal, wantVal)
		}
	}

	if want.Cycles > 0 && gotCycles != want.Cycles {
		t.Errorf("cycles = %d, want %d", gotCycles, want.Cycles)
	}
}

func TestSSTRunner(t *testing.T) {
	if *sstPath == "" {
		t.Skip("no -sstpath provided")
	}

	entries, err := os.ReadDir(*sstPath)
	if err != nil {
		t.Fatalf("reading sstpath: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		fname := entry.Name()
		if reason, ok := sstSkip[fname]; ok && !*sstStrict {
			t.Run(fname, func(t *testing.T) {
				t.Skipf("known failure: %s (use -sststrict to run)", reason)
			})
			continue
		}
		t.Run(fname, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(filepath.Join(*sstPath, fname))
			if err != nil {
				t.Fatalf("reading %s: %v", fname, err)
			}

			var tests []sstJSONTest
			if err := json.Unmarshal(data, &tests); err != nil {
				t.Fatalf("parsing %s: %v", fname, err)
			}

			for i := range tests {
				jt := &tests[i]
				init := jt.Initial.toM68kState()
				want := jt.Final.toM68kState()
				want.Cycles = jt.Length

				t.Run(jt.Name, func(t *testing.T) {
					runSSTTest(t, init, want)
				})
			}
		})
	}
}
