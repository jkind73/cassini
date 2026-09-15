package m68k

import "testing"

// disasmAt runs Disassemble over the given instruction words placed at pc.
func disasmAt(t *testing.T, pc uint32, words []uint16) (string, uint32) {
	t.Helper()
	fetch := func(addr uint32) uint16 {
		i := (addr - pc) / 2
		if int(i) >= len(words) {
			t.Fatalf("fetch beyond provided words: addr $%06X", addr)
		}
		return words[i]
	}
	return Disassemble(pc, fetch)
}

func TestDisassemble(t *testing.T) {
	cases := []struct {
		name  string
		pc    uint32
		words []uint16
		want  string
		size  uint32
	}{
		{"NOP", 0x1000, []uint16{0x4E71}, "NOP", 2},
		{"RTS", 0x1000, []uint16{0x4E75}, "RTS", 2},
		{"RTE", 0x1000, []uint16{0x4E73}, "RTE", 2},
		{"RESET", 0x1000, []uint16{0x4E70}, "RESET", 2},
		{"STOP", 0x1000, []uint16{0x4E72, 0x2000}, "STOP #$2000", 4},
		{"TRAP", 0x1000, []uint16{0x4E45}, "TRAP #5", 2},
		{"LINK", 0x1000, []uint16{0x4E56, 0xFFF8}, "LINK A6,#$FFF8", 4},
		{"UNLK", 0x1000, []uint16{0x4E5E}, "UNLK A6", 2},
		{"MOVE to USP", 0x1000, []uint16{0x4E61}, "MOVE A1,USP", 2},
		{"MOVE from USP", 0x1000, []uint16{0x4E69}, "MOVE USP,A1", 2},

		{"MOVE.W Dn,Dn", 0x1000, []uint16{0x3200}, "MOVE.W D0,D1", 2},
		{"MOVE.B (An)+,Dn", 0x1000, []uint16{0x1618}, "MOVE.B (A0)+,D3", 2},
		{"MOVE.L imm,(An)", 0x1000, []uint16{0x22BC, 0x1234, 0x5678}, "MOVE.L #$12345678,(A1)", 6},
		{"MOVEA.L (A7)+,A0", 0x1000, []uint16{0x205F}, "MOVEA.L (A7)+,A0", 2},
		{"MOVE.W absW,Dn", 0x1000, []uint16{0x3038, 0x0700}, "MOVE.W ($000700).W,D0", 4},
		{"MOVE.W absW high", 0x1000, []uint16{0x3038, 0x8000}, "MOVE.W ($FF8000).W,D0", 4},
		{"MOVE.W absL,Dn", 0x1000, []uint16{0x3039, 0x0002, 0x5A00}, "MOVE.W ($00025A00).L,D0", 6},
		{"MOVE.W d16(An),Dn", 0x1000, []uint16{0x3028, 0x0010}, "MOVE.W $0010(A0),D0", 4},
		{"MOVE.W pcrel,Dn", 0x1000, []uint16{0x303A, 0x0010}, "MOVE.W ($001012)(PC),D0", 4},
		{"MOVE.B d8(An,Dn),Dn", 0x1000, []uint16{0x1030, 0x3004}, "MOVE.B $04(A0,D3.W),D0", 4},
		{"MOVEQ", 0x1000, []uint16{0x74FF}, "MOVEQ #$FF,D2", 2},

		{"LEA absL", 0x1000, []uint16{0x43F9, 0x0000, 0x0700}, "LEA ($00000700).L,A1", 6},
		{"PEA absL", 0x1000, []uint16{0x4879, 0x0000, 0x1000}, "PEA ($00001000).L", 6},
		{"JSR absL", 0x1000, []uint16{0x4EB9, 0x0000, 0x1234}, "JSR ($00001234).L", 6},
		{"JMP (An)", 0x1000, []uint16{0x4ED0}, "JMP (A0)", 2},
		{"SWAP", 0x1000, []uint16{0x4841}, "SWAP D1", 2},
		{"EXT.W", 0x1000, []uint16{0x4884}, "EXT.W D4", 2},
		{"EXT.L", 0x1000, []uint16{0x48C4}, "EXT.L D4", 2},
		{"CLR.L", 0x1000, []uint16{0x4283}, "CLR.L D3", 2},
		{"TST.W absW", 0x1000, []uint16{0x4A78, 0x0700}, "TST.W ($000700).W", 4},
		{"TAS", 0x1000, []uint16{0x4AD0}, "TAS (A0)", 2},
		{"NOT.B Dn", 0x1000, []uint16{0x4600}, "NOT.B D0", 2},
		{"NEG.W Dn", 0x1000, []uint16{0x4441}, "NEG.W D1", 2},
		{"MOVE from SR", 0x1000, []uint16{0x40C0}, "MOVE SR,D0", 2},
		{"MOVE to SR", 0x1000, []uint16{0x46FC, 0x2700}, "MOVE #$2700,SR", 4},
		{"MOVE to CCR", 0x1000, []uint16{0x44C0}, "MOVE D0,CCR", 2},
		{"ILLEGAL", 0x1000, []uint16{0x4AFC}, "ILLEGAL", 2},

		{"MOVEM.L push", 0x1000, []uint16{0x48E7, 0xFFFE}, "MOVEM.L D0-D7/A0-A6,-(A7)", 4},
		{"MOVEM.L pop", 0x1000, []uint16{0x4CDF, 0x7FFF}, "MOVEM.L (A7)+,D0-D7/A0-A6", 4},
		{"MOVEM.W sparse", 0x1000, []uint16{0x48A7, 0x8420}, "MOVEM.W D0/D5/A2,-(A7)", 4},

		{"ADDQ.W", 0x1000, []uint16{0x5240}, "ADDQ.W #1,D0", 2},
		{"SUBQ.L An", 0x1000, []uint16{0x518A}, "SUBQ.L #8,A2", 2},
		{"SNE", 0x1000, []uint16{0x56C0}, "SNE D0", 2},
		{"DBF", 0x1000, []uint16{0x51C8, 0xFFFC}, "DBF D0,$000FFE", 4},

		{"BRA.S", 0x1000, []uint16{0x6010}, "BRA.S $001012", 2},
		{"BSR.W", 0x1000, []uint16{0x6100, 0x0100}, "BSR.W $001102", 4},
		{"BNE.S back", 0x1000, []uint16{0x66FE}, "BNE.S $001000", 2},
		{"BEQ.S", 0x1000, []uint16{0x6708}, "BEQ.S $00100A", 2},

		{"ORI to CCR", 0x1000, []uint16{0x003C, 0x0001}, "ORI #$01,CCR", 4},
		{"ANDI to SR", 0x1000, []uint16{0x027C, 0xF8FF}, "ANDI #$F8FF,SR", 4},
		{"ANDI.W", 0x1000, []uint16{0x0240, 0x0FFF}, "ANDI.W #$0FFF,D0", 4},
		{"CMPI.W d16(An)", 0x1000, []uint16{0x0C68, 0x0034, 0x0012}, "CMPI.W #$0034,$0012(A0)", 6},
		{"ADDI.B", 0x1000, []uint16{0x0600, 0x0005}, "ADDI.B #$05,D0", 4},
		{"BTST imm", 0x1000, []uint16{0x0800, 0x0007}, "BTST #7,D0", 4},
		{"BSET Dn,(An)", 0x1000, []uint16{0x03D0}, "BSET D1,(A0)", 2},
		{"MOVEP.W mem to reg", 0x1000, []uint16{0x0308, 0x0004}, "MOVEP.W $0004(A0),D1", 4},

		{"OR.W", 0x1000, []uint16{0x8041}, "OR.W D1,D0", 2},
		{"DIVU", 0x1000, []uint16{0x80C1}, "DIVU D1,D0", 2},
		{"DIVS", 0x1000, []uint16{0x81C1}, "DIVS D1,D0", 2},
		{"SBCD", 0x1000, []uint16{0x8101}, "SBCD D1,D0", 2},

		{"SUB.W", 0x1000, []uint16{0x9041}, "SUB.W D1,D0", 2},
		{"SUBA.L", 0x1000, []uint16{0x93C8}, "SUBA.L A0,A1", 2},
		{"ADD.L to ea", 0x1000, []uint16{0xD191}, "ADD.L D0,(A1)", 2},
		{"ADD.L from ea", 0x1000, []uint16{0xD091}, "ADD.L (A1),D0", 2},
		{"ADDX.L", 0x1000, []uint16{0xD581}, "ADDX.L D1,D2", 2},

		{"CMP.B (An)+", 0x1000, []uint16{0xB018}, "CMP.B (A0)+,D0", 2},
		{"CMPA.W", 0x1000, []uint16{0xB0C9}, "CMPA.W A1,A0", 2},
		{"CMPM.B", 0x1000, []uint16{0xB308}, "CMPM.B (A0)+,(A1)+", 2},
		{"EOR.W", 0x1000, []uint16{0xB141}, "EOR.W D0,D1", 2},

		{"AND.W imm ea", 0x1000, []uint16{0xC47C, 0x00F0}, "AND.W #$00F0,D2", 4},
		{"MULU imm", 0x1000, []uint16{0xC0FC, 0x0002}, "MULU #$0002,D0", 4},
		{"MULS", 0x1000, []uint16{0xC1C1}, "MULS D1,D0", 2},
		{"EXG Dn,Dn", 0x1000, []uint16{0xC141}, "EXG D0,D1", 2},
		{"EXG Dn,An", 0x1000, []uint16{0xC188}, "EXG D0,A0", 2},
		{"ABCD", 0x1000, []uint16{0xC101}, "ABCD D1,D0", 2},

		{"LSR.W imm", 0x1000, []uint16{0xE248}, "LSR.W #1,D0", 2},
		{"ASL.L count8", 0x1000, []uint16{0xE183}, "ASL.L #8,D3", 2},
		{"ROL.W Dn", 0x1000, []uint16{0xE37A}, "ROL.W D1,D2", 2},
		{"LSR mem", 0x1000, []uint16{0xE2F8, 0x0700}, "LSR ($000700).W", 4},
		{"MOVE.W pcrel index", 0x1000, []uint16{0x303B, 0x3010}, "MOVE.W ($001012)(PC,D3.W),D0", 4},

		{"line-A illegal", 0x1000, []uint16{0xA000}, "DC.W $A000", 2},
		{"line-F illegal", 0x1000, []uint16{0xF123}, "DC.W $F123", 2},
		{"68020 bitfield illegal", 0x1000, []uint16{0xE8C0}, "DC.W $E8C0", 2},
		{"ea mode 7 reg 5 illegal", 0x1000, []uint16{0x4A7D}, "DC.W $4A7D", 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, size := disasmAt(t, tc.pc, tc.words)
			if got != tc.want {
				t.Errorf("text: got %q, want %q", got, tc.want)
			}
			if size != tc.size {
				t.Errorf("size: got %d, want %d", size, tc.size)
			}
		})
	}
}

// TestDisassembleTableAgreement sweeps every opcode word and checks the
// disassembler against the execution core's opcode table: implemented
// opcodes must decode to text, unimplemented ones must decode to DC.W.
// Extension words read as zero.
func TestDisassembleTableAgreement(t *testing.T) {
	for op := 0; op < 0x10000; op++ {
		if op == 0x4AFC {
			continue // prints its mnemonic ILLEGAL, not DC.W
		}
		fetch := func(addr uint32) uint16 {
			if addr == 0 {
				return uint16(op)
			}
			return 0
		}
		text, _ := Disassemble(0, fetch)
		isDCW := len(text) >= 4 && text[:4] == "DC.W"
		if opcodeTable[op] == nil && !isDCW {
			t.Errorf("$%04X: illegal opcode decoded as %q", op, text)
		}
		if opcodeTable[op] != nil && isDCW {
			t.Errorf("$%04X: implemented opcode decoded as DC.W", op)
		}
	}
}
