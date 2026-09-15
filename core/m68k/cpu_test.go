package m68k

import "testing"

func TestInstructionCycles(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(bus *testBus, pc uint32) // write opcode + extensions at pc
		d      [8]uint32
		a      [8]uint32
		ssp    uint32
		cycles int
	}{
		// --- MOVE ---
		{
			name: "MOVE.W D0,D1 = 4",
			setup: func(bus *testBus, pc uint32) {
				// 0x3200: sz=W, dstReg=1, dstMode=0, srcMode=0, srcReg=0
				writeWord(bus, pc, 0x3200)
			},
			cycles: 4,
		},
		{
			name: "MOVE.W D0,(A1) = 8",
			setup: func(bus *testBus, pc uint32) {
				// 0x3280: sz=W, dstReg=1, dstMode=2, srcMode=0, srcReg=0
				writeWord(bus, pc, 0x3280)
			},
			a:      [8]uint32{0, 0x2000},
			cycles: 8, // 4 + 0(Dn) + 4((An) write)
		},
		{
			name: "MOVE.L (A0),D1 = 12",
			setup: func(bus *testBus, pc uint32) {
				// 0x2210: sz=L, dstReg=1, dstMode=0, srcMode=2, srcReg=0
				writeWord(bus, pc, 0x2210)
			},
			a:      [8]uint32{0x2000},
			cycles: 12, // 4 + 8((An) fetch Long) + 0(Dn)
		},
		{
			name: "MOVE.W #imm,D0 = 8",
			setup: func(bus *testBus, pc uint32) {
				// 0x303C: sz=W, dstReg=0, dstMode=0, srcMode=7, srcReg=4
				writeWord(bus, pc, 0x303C)
				writeWord(bus, pc+2, 0x0042) // immediate value
			},
			cycles: 8, // 4 + 4(#imm) + 0(Dn)
		},
		// --- MOVEA ---
		{
			name: "MOVEA.W D0,A1 = 4",
			setup: func(bus *testBus, pc uint32) {
				// 0x3240: sz=W, dstReg=1, dstMode=1(An), srcMode=0, srcReg=0
				writeWord(bus, pc, 0x3240)
			},
			cycles: 4, // 4 + 0(Dn)
		},
		{
			name: "MOVEA.L (A0),A1 = 12",
			setup: func(bus *testBus, pc uint32) {
				// 0x2250: sz=L, dstReg=1, dstMode=1(An), srcMode=2, srcReg=0
				writeWord(bus, pc, 0x2250)
			},
			a:      [8]uint32{0x2000},
			cycles: 12, // 4 + 8((An) fetch Long)
		},
		// --- LEA ---
		{
			name: "LEA (A0),A1 = 4",
			setup: func(bus *testBus, pc uint32) {
				// 0x43D0: An=1, srcMode=2, srcReg=0
				writeWord(bus, pc, 0x43D0)
			},
			a:      [8]uint32{0x2000},
			cycles: 4,
		},
		{
			name: "LEA d16(A0),A1 = 8",
			setup: func(bus *testBus, pc uint32) {
				// 0x43E8: An=1, srcMode=5, srcReg=0
				writeWord(bus, pc, 0x43E8)
				writeWord(bus, pc+2, 0x0010) // displacement
			},
			a:      [8]uint32{0x2000},
			cycles: 8,
		},
		{
			name: "LEA abs.W,A1 = 8",
			setup: func(bus *testBus, pc uint32) {
				// 0x43F8: An=1, srcMode=7, srcReg=0
				writeWord(bus, pc, 0x43F8)
				writeWord(bus, pc+2, 0x2000) // abs.W address
			},
			cycles: 8,
		},
		{
			name: "LEA abs.L,A1 = 12",
			setup: func(bus *testBus, pc uint32) {
				// 0x43F9: An=1, srcMode=7, srcReg=1
				writeWord(bus, pc, 0x43F9)
				writeWord(bus, pc+2, 0x0000) // abs.L high
				writeWord(bus, pc+4, 0x2000) // abs.L low
			},
			cycles: 12,
		},
		// --- PEA ---
		{
			name: "PEA (A0) = 12",
			setup: func(bus *testBus, pc uint32) {
				// 0x4850: srcMode=2, srcReg=0
				writeWord(bus, pc, 0x4850)
			},
			a:      [8]uint32{0x2000},
			ssp:    0x10000,
			cycles: 12,
		},
		{
			name: "PEA d16(A0) = 16",
			setup: func(bus *testBus, pc uint32) {
				// 0x4868: srcMode=5, srcReg=0
				writeWord(bus, pc, 0x4868)
				writeWord(bus, pc+2, 0x0010) // displacement
			},
			a:      [8]uint32{0x2000},
			ssp:    0x10000,
			cycles: 16,
		},
		{
			name: "PEA abs.L = 20",
			setup: func(bus *testBus, pc uint32) {
				// 0x4879: srcMode=7, srcReg=1
				writeWord(bus, pc, 0x4879)
				writeWord(bus, pc+2, 0x0000)
				writeWord(bus, pc+4, 0x2000)
			},
			ssp:    0x10000,
			cycles: 20,
		},
		// --- JMP ---
		{
			name: "JMP (A0) = 8",
			setup: func(bus *testBus, pc uint32) {
				// 0x4ED0: mode=2, reg=0
				writeWord(bus, pc, 0x4ED0)
			},
			a:      [8]uint32{0x2000},
			cycles: 8,
		},
		{
			name: "JMP abs.W = 10",
			setup: func(bus *testBus, pc uint32) {
				// 0x4EF8: mode=7, reg=0
				writeWord(bus, pc, 0x4EF8)
				writeWord(bus, pc+2, 0x2000)
			},
			cycles: 10,
		},
		{
			name: "JMP abs.L = 12",
			setup: func(bus *testBus, pc uint32) {
				// 0x4EF9: mode=7, reg=1
				writeWord(bus, pc, 0x4EF9)
				writeWord(bus, pc+2, 0x0000)
				writeWord(bus, pc+4, 0x2000)
			},
			cycles: 12,
		},
		// --- JSR ---
		{
			name: "JSR (A0) = 16",
			setup: func(bus *testBus, pc uint32) {
				// 0x4E90: mode=2, reg=0
				writeWord(bus, pc, 0x4E90)
			},
			a:      [8]uint32{0x2000},
			ssp:    0x10000,
			cycles: 16,
		},
		{
			name: "JSR abs.W = 18",
			setup: func(bus *testBus, pc uint32) {
				// 0x4EB8: mode=7, reg=0
				writeWord(bus, pc, 0x4EB8)
				writeWord(bus, pc+2, 0x2000)
			},
			ssp:    0x10000,
			cycles: 18,
		},
		{
			name: "JSR abs.L = 20",
			setup: func(bus *testBus, pc uint32) {
				// 0x4EB9: mode=7, reg=1
				writeWord(bus, pc, 0x4EB9)
				writeWord(bus, pc+2, 0x0000)
				writeWord(bus, pc+4, 0x2000)
			},
			ssp:    0x10000,
			cycles: 20,
		},
		// --- MOVEM ---
		{
			name: "MOVEM.W D0-D3,(A0) reg-to-mem = 24",
			setup: func(bus *testBus, pc uint32) {
				// 0x4890: dir=0, sz=W, mode=2, reg=0
				writeWord(bus, pc, 0x4890)
				writeWord(bus, pc+2, 0x000F) // mask: D0-D3
			},
			a:      [8]uint32{0x2000},
			cycles: 24, // base=8 + 4 regs × 4 = 24
		},
		{
			name: "MOVEM.L (A0)+,D0-D3 mem-to-reg = 44",
			setup: func(bus *testBus, pc uint32) {
				// 0x4CD8: dir=1, sz=L, mode=3(An+), reg=0
				writeWord(bus, pc, 0x4CD8)
				writeWord(bus, pc+2, 0x000F) // mask: D0-D3
			},
			a:      [8]uint32{0x2000},
			cycles: 44, // base=12 + 4 regs × 8 = 44
		},
		{
			name: "MOVEM.W D0,-(A0) reg-to-mem 1 reg = 12",
			setup: func(bus *testBus, pc uint32) {
				// 0x48A0: dir=0, sz=W, mode=4(-(An)), reg=0
				writeWord(bus, pc, 0x48A0)
				writeWord(bus, pc+2, 0x8000) // -(An) reversed: bit 15 = D0
			},
			a:      [8]uint32{0x2010},
			cycles: 12, // base=8 + 1 reg × 4 = 12
		},
		// --- ADD ---
		{
			name: "ADD.W D0,D1 = 4",
			setup: func(bus *testBus, pc uint32) {
				// ADD.W D0,D1: 0xD240 = 1101 001 001 000 000
				writeWord(bus, pc, 0xD240)
			},
			cycles: 4,
		},
		{
			name: "ADD.L D0,D1 = 8",
			setup: func(bus *testBus, pc uint32) {
				// ADD.L D0,D1: 0xD280
				writeWord(bus, pc, 0xD280)
			},
			cycles: 8,
		},
		{
			name: "ADD.W (A0),D1 = 8",
			setup: func(bus *testBus, pc uint32) {
				// ADD.W (A0),D1: 0xD250
				writeWord(bus, pc, 0xD250)
			},
			a:      [8]uint32{0x2000},
			cycles: 8, // 4 + 4((An))
		},
		{
			name: "ADD.L (A0),D1 = 14",
			setup: func(bus *testBus, pc uint32) {
				// ADD.L (A0),D1: 0xD290
				writeWord(bus, pc, 0xD290)
			},
			a:      [8]uint32{0x2000},
			cycles: 14, // 6 + 8((An) Long)
		},
		{
			name: "ADD.L #imm,D1 = 16",
			setup: func(bus *testBus, pc uint32) {
				// ADD.L #imm,D1: opcode=0xD2BC (mode=7,reg=4)... wait
				// Actually ADD <ea>,Dn with immediate: D000 | 1<<9 | 2<<6 | 7<<3 | 4
				// = 0xD000 | 0x0200 | 0x0080 | 0x003C = 0xD2BC
				writeWord(bus, pc, 0xD2BC)
				writeWord(bus, pc+2, 0x0000)
				writeWord(bus, pc+4, 0x0001) // #1
			},
			cycles: 16, // 8 + 8(#imm Long)
		},
		{
			name: "ADD.W D0,(A1) = 12",
			setup: func(bus *testBus, pc uint32) {
				// ADD.W D0,(A1): 0xD151 = 1101 000 101 010 001
				writeWord(bus, pc, 0xD151)
			},
			a:      [8]uint32{0, 0x2000},
			cycles: 12, // 8 + 4((An))
		},
		{
			name: "ADD.L D0,d16(A1) = 24",
			setup: func(bus *testBus, pc uint32) {
				// ADD.L D0,d16(A1): 0xD1A9 = 1101 000 110 101 001
				writeWord(bus, pc, 0xD1A9)
				writeWord(bus, pc+2, 0x0010) // displacement
			},
			a:      [8]uint32{0, 0x2000},
			cycles: 24, // 12 + 12(d16 Long)
		},
		// --- ADDA ---
		{
			name: "ADDA.W D0,A1 = 8",
			setup: func(bus *testBus, pc uint32) {
				// ADDA.W D0,A1: 0xD2C0
				writeWord(bus, pc, 0xD2C0)
			},
			cycles: 8,
		},
		{
			name: "ADDA.L (A0),A1 = 14",
			setup: func(bus *testBus, pc uint32) {
				// ADDA.L (A0),A1: 0xD3D0
				writeWord(bus, pc, 0xD3D0)
			},
			a:      [8]uint32{0x2000},
			cycles: 14, // 6 + 8((An) Long)
		},
		// --- ADDI ---
		{
			name: "ADDI.W #imm,D0 = 8",
			setup: func(bus *testBus, pc uint32) {
				// ADDI.W #imm,D0: 0x0640
				writeWord(bus, pc, 0x0640)
				writeWord(bus, pc+2, 0x0001)
			},
			cycles: 8,
		},
		{
			name: "ADDI.L #imm,D0 = 16",
			setup: func(bus *testBus, pc uint32) {
				// ADDI.L #imm,D0: 0x0680
				writeWord(bus, pc, 0x0680)
				writeWord(bus, pc+2, 0x0000)
				writeWord(bus, pc+4, 0x0001)
			},
			cycles: 16,
		},
		{
			name: "ADDI.W #imm,(A0) = 16",
			setup: func(bus *testBus, pc uint32) {
				// ADDI.W #imm,(A0): 0x0650
				writeWord(bus, pc, 0x0650)
				writeWord(bus, pc+2, 0x0001) // immediate
			},
			a:      [8]uint32{0x2000},
			cycles: 16, // 12 + 4((An))
		},
		// --- ADDQ ---
		{
			name: "ADDQ.W #1,D0 = 4",
			setup: func(bus *testBus, pc uint32) {
				// ADDQ.W #1,D0: 0x5240
				writeWord(bus, pc, 0x5240)
			},
			cycles: 4,
		},
		{
			name: "ADDQ.L #1,D0 = 8",
			setup: func(bus *testBus, pc uint32) {
				// ADDQ.L #1,D0: 0x5280
				writeWord(bus, pc, 0x5280)
			},
			cycles: 8,
		},
		{
			name: "ADDQ.W #1,(A0) = 12",
			setup: func(bus *testBus, pc uint32) {
				// ADDQ.W #1,(A0): 0x5250
				writeWord(bus, pc, 0x5250)
			},
			a:      [8]uint32{0x2000},
			cycles: 12, // 8 + 4((An))
		},
		// --- CMP ---
		{
			name: "CMP.W D0,D1 = 4",
			setup: func(bus *testBus, pc uint32) {
				// CMP.W D0,D1: 0xB240
				writeWord(bus, pc, 0xB240)
			},
			cycles: 4,
		},
		{
			name: "CMP.L D0,D1 = 6",
			setup: func(bus *testBus, pc uint32) {
				// CMP.L D0,D1: 0xB280
				writeWord(bus, pc, 0xB280)
			},
			cycles: 6,
		},
		{
			name: "CMP.L (A0),D1 = 14",
			setup: func(bus *testBus, pc uint32) {
				// CMP.L (A0),D1: 0xB290
				writeWord(bus, pc, 0xB290)
			},
			a:      [8]uint32{0x2000},
			cycles: 14, // 6 + 8((An) Long)
		},
		// --- CMPA ---
		{
			name: "CMPA.W D0,A1 = 6",
			setup: func(bus *testBus, pc uint32) {
				// CMPA.W D0,A1: 0xB2C0
				writeWord(bus, pc, 0xB2C0)
			},
			cycles: 6,
		},
		{
			name: "CMPA.L (A0),A1 = 14",
			setup: func(bus *testBus, pc uint32) {
				// CMPA.L (A0),A1: 0xB3D0
				writeWord(bus, pc, 0xB3D0)
			},
			a:      [8]uint32{0x2000},
			cycles: 14, // 6 + 8((An) Long)
		},
		// --- CMPI ---
		{
			name: "CMPI.W #imm,D0 = 8",
			setup: func(bus *testBus, pc uint32) {
				// CMPI.W #imm,D0: 0x0C40
				writeWord(bus, pc, 0x0C40)
				writeWord(bus, pc+2, 0x0000)
			},
			cycles: 8,
		},
		{
			name: "CMPI.L #imm,D0 = 14",
			setup: func(bus *testBus, pc uint32) {
				// CMPI.L #imm,D0: 0x0C80
				writeWord(bus, pc, 0x0C80)
				writeWord(bus, pc+2, 0x0000)
				writeWord(bus, pc+4, 0x0000)
			},
			cycles: 14,
		},
		// --- CMPM ---
		{
			name: "CMPM.W (A0)+,(A1)+ = 12",
			setup: func(bus *testBus, pc uint32) {
				// CMPM.W (A0)+,(A1)+: 0xB348
				writeWord(bus, pc, 0xB348)
			},
			a:      [8]uint32{0x2000, 0x3000},
			cycles: 12,
		},
		{
			name: "CMPM.L (A0)+,(A1)+ = 20",
			setup: func(bus *testBus, pc uint32) {
				// CMPM.L (A0)+,(A1)+: 0xB388
				writeWord(bus, pc, 0xB388)
			},
			a:      [8]uint32{0x2000, 0x3000},
			cycles: 20,
		},
		// --- NEG ---
		{
			name: "NEG.W D0 = 4",
			setup: func(bus *testBus, pc uint32) {
				// NEG.W D0: 0x4440
				writeWord(bus, pc, 0x4440)
			},
			cycles: 4,
		},
		{
			name: "NEG.L D0 = 6",
			setup: func(bus *testBus, pc uint32) {
				// NEG.L D0: 0x4480
				writeWord(bus, pc, 0x4480)
			},
			cycles: 6,
		},
		{
			name: "NEG.W (A0) = 12",
			setup: func(bus *testBus, pc uint32) {
				// NEG.W (A0): 0x4450
				writeWord(bus, pc, 0x4450)
			},
			a:      [8]uint32{0x2000},
			cycles: 12, // 8 + 4((An))
		},
		{
			name: "NEG.L (A0) = 20",
			setup: func(bus *testBus, pc uint32) {
				// NEG.L (A0): 0x4490
				writeWord(bus, pc, 0x4490)
			},
			a:      [8]uint32{0x2000},
			cycles: 20, // 12 + 8((An) Long)
		},
		// --- CLR ---
		{
			name: "CLR.L D0 = 6",
			setup: func(bus *testBus, pc uint32) {
				// CLR.L D0: 0x4280
				writeWord(bus, pc, 0x4280)
			},
			cycles: 6,
		},
		// --- NOT ---
		{
			name: "NOT.W (A0) = 12",
			setup: func(bus *testBus, pc uint32) {
				// NOT.W (A0): 0x4650
				writeWord(bus, pc, 0x4650)
			},
			a:      [8]uint32{0x2000},
			cycles: 12, // 8 + 4((An))
		},
		// --- AND ---
		{
			name: "AND.W (A0),D1 = 8",
			setup: func(bus *testBus, pc uint32) {
				// AND.W (A0),D1: 0xC250
				writeWord(bus, pc, 0xC250)
			},
			a:      [8]uint32{0x2000},
			cycles: 8, // 4 + 4((An))
		},
		{
			name: "AND.L (A0),D1 = 14",
			setup: func(bus *testBus, pc uint32) {
				// AND.L (A0),D1: 0xC290
				writeWord(bus, pc, 0xC290)
			},
			a:      [8]uint32{0x2000},
			cycles: 14, // 6 + 8((An) Long)
		},
		// --- EOR ---
		{
			name: "EOR.W D0,D1 = 4",
			setup: func(bus *testBus, pc uint32) {
				// EOR.W D0,D1: 0xB141
				writeWord(bus, pc, 0xB141)
			},
			cycles: 4,
		},
		{
			name: "EOR.L D0,D1 = 8",
			setup: func(bus *testBus, pc uint32) {
				// EOR.L D0,D1: 0xB181
				writeWord(bus, pc, 0xB181)
			},
			cycles: 8,
		},
		{
			name: "EOR.W D0,(A1) = 12",
			setup: func(bus *testBus, pc uint32) {
				// EOR.W D0,(A1): 0xB151
				writeWord(bus, pc, 0xB151)
			},
			a:      [8]uint32{0, 0x2000},
			cycles: 12, // 8 + 4((An))
		},
		// --- TST ---
		{
			name: "TST.W D0 = 4",
			setup: func(bus *testBus, pc uint32) {
				// TST.W D0: 0x4A40
				writeWord(bus, pc, 0x4A40)
			},
			cycles: 4,
		},
		{
			name: "TST.L (A0) = 12",
			setup: func(bus *testBus, pc uint32) {
				// TST.L (A0): 0x4A90
				writeWord(bus, pc, 0x4A90)
			},
			a:      [8]uint32{0x2000},
			cycles: 12, // 4 + 8((An) Long)
		},
		// --- SUB (spot check) ---
		{
			name: "SUB.L -(A0),D1 = 16",
			setup: func(bus *testBus, pc uint32) {
				// SUB.L -(A0),D1: 0x92A0
				writeWord(bus, pc, 0x92A0)
			},
			a:      [8]uint32{0x2004},
			cycles: 16, // 6 + 10(-(An) Long)
		},
		// --- SUBI (spot check) ---
		{
			name: "SUBI.W #imm,(A0) = 16",
			setup: func(bus *testBus, pc uint32) {
				// SUBI.W #imm,(A0): 0x0450
				writeWord(bus, pc, 0x0450)
				writeWord(bus, pc+2, 0x0001)
			},
			a:      [8]uint32{0x2000},
			cycles: 16, // 12 + 4((An))
		},
		// --- Shift memory (spot check) ---
		{
			name: "ASL.W (A0) = 12",
			setup: func(bus *testBus, pc uint32) {
				// ASL.W (A0): 0xE1D0 = 1110 000 1 11 010 000
				writeWord(bus, pc, 0xE1D0)
			},
			a:      [8]uint32{0x2000},
			cycles: 12, // 8 + 4((An))
		},
		// --- BTST ---
		{
			name: "BTST D0,D1 = 6",
			setup: func(bus *testBus, pc uint32) {
				// BTST D0,D1: 0x0101
				writeWord(bus, pc, 0x0101)
			},
			cycles: 6,
		},
		{
			name: "BTST D0,(A0) = 8",
			setup: func(bus *testBus, pc uint32) {
				// BTST D0,(A0): 0x0110
				writeWord(bus, pc, 0x0110)
			},
			a:      [8]uint32{0x2000},
			cycles: 8, // 4 + 4((An))
		},
		{
			name: "BTST D0,d16(A0) = 12",
			setup: func(bus *testBus, pc uint32) {
				// BTST D0,d16(A0): 0x0128
				writeWord(bus, pc, 0x0128)
				writeWord(bus, pc+2, 0x0010)
			},
			a:      [8]uint32{0x2000},
			cycles: 12, // 4 + 8(d16)
		},
		{
			name: "BTST #n,D0 = 10",
			setup: func(bus *testBus, pc uint32) {
				// BTST #n,D0: 0x0800
				writeWord(bus, pc, 0x0800)
				writeWord(bus, pc+2, 0x0000) // bit number
			},
			cycles: 10,
		},
		{
			name: "BTST #n,(A0) = 12",
			setup: func(bus *testBus, pc uint32) {
				// BTST #n,(A0): 0x0810
				writeWord(bus, pc, 0x0810)
				writeWord(bus, pc+2, 0x0000) // bit number
			},
			a:      [8]uint32{0x2000},
			cycles: 12, // 8 + 4((An))
		},
		// --- BSET ---
		{
			name: "BSET D0,(A0) = 12",
			setup: func(bus *testBus, pc uint32) {
				// BSET D0,(A0): 0x01D0
				writeWord(bus, pc, 0x01D0)
			},
			a:      [8]uint32{0x2000},
			cycles: 12, // 8 + 4((An))
		},
		{
			name: "BCLR #n,(A0) = 16",
			setup: func(bus *testBus, pc uint32) {
				// BCLR #n,(A0): 0x0890
				writeWord(bus, pc, 0x0890)
				writeWord(bus, pc+2, 0x0000) // bit number
			},
			a:      [8]uint32{0x2000},
			cycles: 16, // 12 + 4((An))
		},
		// --- Scc ---
		{
			name: "ST D0 (true) = 6",
			setup: func(bus *testBus, pc uint32) {
				// ST D0: 0x50C0 (cc=0, always true)
				writeWord(bus, pc, 0x50C0)
			},
			cycles: 6,
		},
		{
			name: "SF D0 (false) = 4",
			setup: func(bus *testBus, pc uint32) {
				// SF D0: 0x51C0 (cc=1, always false)
				writeWord(bus, pc, 0x51C0)
			},
			cycles: 4,
		},
		{
			name: "ST (A0) mem = 12",
			setup: func(bus *testBus, pc uint32) {
				// ST (A0): 0x50D0 (cc=0, always true, mode=2, reg=0)
				writeWord(bus, pc, 0x50D0)
			},
			a:      [8]uint32{0x2000},
			cycles: 12, // 8 + 4((An))
		},
		{
			name: "SF (A0) mem = 12",
			setup: func(bus *testBus, pc uint32) {
				// SF (A0): 0x51D0 (cc=1, always false, mode=2, reg=0)
				writeWord(bus, pc, 0x51D0)
			},
			a:      [8]uint32{0x2000},
			cycles: 12, // 8 + 4((An)) — same for true and false
		},
		// --- NBCD ---
		{
			name: "NBCD D0 = 6",
			setup: func(bus *testBus, pc uint32) {
				// NBCD D0: 0x4800
				writeWord(bus, pc, 0x4800)
			},
			cycles: 6,
		},
		{
			name: "NBCD (A0) = 12",
			setup: func(bus *testBus, pc uint32) {
				// NBCD (A0): 0x4810
				writeWord(bus, pc, 0x4810)
			},
			a:      [8]uint32{0x2000},
			cycles: 12, // 8 + 4((An))
		},
		// --- MOVE from SR ---
		{
			name: "MOVE SR,D0 = 6",
			setup: func(bus *testBus, pc uint32) {
				// MOVE SR,D0: 0x40C0
				writeWord(bus, pc, 0x40C0)
			},
			cycles: 6,
		},
		{
			name: "MOVE SR,(A0) = 12",
			setup: func(bus *testBus, pc uint32) {
				// MOVE SR,(A0): 0x40D0
				writeWord(bus, pc, 0x40D0)
			},
			a:      [8]uint32{0x2000},
			cycles: 12, // 8 + 4((An))
		},
		// --- MOVE to CCR ---
		{
			name: "MOVE D0,CCR = 12",
			setup: func(bus *testBus, pc uint32) {
				// MOVE D0,CCR: 0x44C0
				writeWord(bus, pc, 0x44C0)
			},
			cycles: 12,
		},
		{
			name: "MOVE (A0),CCR = 16",
			setup: func(bus *testBus, pc uint32) {
				// MOVE (A0),CCR: 0x44D0
				writeWord(bus, pc, 0x44D0)
			},
			a:      [8]uint32{0x2000},
			cycles: 16, // 12 + 4((An))
		},
		// --- MOVE to SR ---
		{
			name: "MOVE D0,SR = 12",
			setup: func(bus *testBus, pc uint32) {
				// MOVE D0,SR: 0x46C0
				writeWord(bus, pc, 0x46C0)
			},
			cycles: 12,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bus := &testBus{}
			cpu := &CPU{bus: bus}

			pc := uint32(0x1000)
			tt.setup(bus, pc)

			ssp := tt.ssp
			if ssp == 0 {
				ssp = 0x10000
			}
			cpu.SetState(Registers{D: tt.d, A: tt.a, PC: pc, SR: 0x2700, SSP: ssp})

			got := cpu.Step()
			if got != tt.cycles {
				t.Errorf("got %d cycles, want %d", got, tt.cycles)
			}
		})
	}
}

func TestAddressError(t *testing.T) {
	t.Run("word read from odd address halts", func(t *testing.T) {
		bus := &testBus{}
		cpu := &CPU{bus: bus}

		// MOVE.W (A0), D0 — opcode 0x3010
		pc := uint32(0x1000)
		writeWord(bus, pc, 0x3010)

		cpu.SetState(Registers{A: [8]uint32{0x2001}, PC: pc, SR: 0x2700, SSP: 0x10000})
		cpu.Step()

		if !cpu.Halted() {
			t.Errorf("expected CPU to be halted after word read from odd address")
		}
	})

	t.Run("long read from odd address halts", func(t *testing.T) {
		bus := &testBus{}
		cpu := &CPU{bus: bus}

		// MOVE.L (A0), D0 — opcode 0x2010
		pc := uint32(0x1000)
		writeWord(bus, pc, 0x2010)

		cpu.SetState(Registers{A: [8]uint32{0x2001}, PC: pc, SR: 0x2700, SSP: 0x10000})
		cpu.Step()

		if !cpu.Halted() {
			t.Errorf("expected CPU to be halted after long read from odd address")
		}
	})

	t.Run("word write to odd address halts", func(t *testing.T) {
		bus := &testBus{}
		cpu := &CPU{bus: bus}

		// MOVE.W D0, (A0) — opcode 0x3080
		pc := uint32(0x1000)
		writeWord(bus, pc, 0x3080)

		cpu.SetState(Registers{D: [8]uint32{0x1234}, A: [8]uint32{0x2001}, PC: pc, SR: 0x2700, SSP: 0x10000})
		cpu.Step()

		if !cpu.Halted() {
			t.Errorf("expected CPU to be halted after word write to odd address")
		}
	})

	t.Run("long write to odd address halts", func(t *testing.T) {
		bus := &testBus{}
		cpu := &CPU{bus: bus}

		// MOVE.L D0, (A0) — opcode 0x2080
		pc := uint32(0x1000)
		writeWord(bus, pc, 0x2080)

		cpu.SetState(Registers{D: [8]uint32{0x12345678}, A: [8]uint32{0x2001}, PC: pc, SR: 0x2700, SSP: 0x10000})
		cpu.Step()

		if !cpu.Halted() {
			t.Errorf("expected CPU to be halted after long write to odd address")
		}
	})

	t.Run("byte read from odd address works", func(t *testing.T) {
		bus := &testBus{}
		cpu := &CPU{bus: bus}

		// MOVE.B (A0), D0 — opcode 0x1010
		pc := uint32(0x1000)
		writeWord(bus, pc, 0x1010)

		bus.mem[0x2001] = 0xAB
		cpu.SetState(Registers{A: [8]uint32{0x2001}, PC: pc, SR: 0x2700, SSP: 0x10000})
		cpu.Step()

		if cpu.Halted() {
			t.Errorf("CPU should not halt on byte read from odd address")
		}
		reg := cpu.Registers()
		if reg.D[0]&0xFF != 0xAB {
			t.Errorf("D0 low byte = 0x%02X, want 0xAB", reg.D[0]&0xFF)
		}
	})

	t.Run("byte write to odd address works", func(t *testing.T) {
		bus := &testBus{}
		cpu := &CPU{bus: bus}

		// MOVE.B D0, (A0) — opcode 0x1080
		pc := uint32(0x1000)
		writeWord(bus, pc, 0x1080)

		cpu.SetState(Registers{D: [8]uint32{0xCD}, A: [8]uint32{0x2001}, PC: pc, SR: 0x2700, SSP: 0x10000})
		cpu.Step()

		if cpu.Halted() {
			t.Errorf("CPU should not halt on byte write to odd address")
		}
		if bus.mem[0x2001] != 0xCD {
			t.Errorf("RAM[0x2001] = 0x%02X, want 0xCD", bus.mem[0x2001])
		}
	})

	t.Run("odd PC halts", func(t *testing.T) {
		bus := &testBus{}
		cpu := &CPU{bus: bus}

		// Put a NOP at address 0x1000 in case fetch reaches there
		writeWord(bus, 0x1000, 0x4E71)

		// Set PC to an odd address
		cpu.SetState(Registers{PC: 0x1001, SR: 0x2700, SSP: 0x10000})
		cycles := cpu.Step()

		if !cpu.Halted() {
			t.Errorf("expected CPU to be halted with odd PC")
		}
		if cycles != 0 {
			t.Errorf("Step() returned %d cycles, want 0 for halted CPU", cycles)
		}
	})

	t.Run("odd SSP during exception halts", func(t *testing.T) {
		bus := &testBus{}
		cpu := &CPU{bus: bus}

		// Use an unimplemented opcode to trigger illegal instruction exception.
		// Opcode 0x4AFC is the explicit ILLEGAL instruction on 68000.
		// The illegal instruction vector (4) address = 4*4 = 16.
		// Put a handler address at vector 4 (address 0x10).
		bus.mem[0x10] = 0x00
		bus.mem[0x11] = 0x00
		bus.mem[0x12] = 0x20
		bus.mem[0x13] = 0x00 // handler at 0x2000

		pc := uint32(0x1000)
		writeWord(bus, pc, 0x4AFC)

		// SSP is odd — the exception push (pushLong/pushWord) will try
		// to write to an odd address, triggering the alignment check.
		cpu.SetState(Registers{PC: pc, SR: 0x2700, SSP: 0x10001})
		cpu.Step()

		if !cpu.Halted() {
			t.Errorf("expected CPU to be halted when exception pushes to odd SSP")
		}
	})
}

func TestStepCycles(t *testing.T) {
	t.Run("budget larger than cost", func(t *testing.T) {
		cpu, _ := newNOPCPU(1)

		cycles := cpu.StepCycles(100)
		if cycles != 4 {
			t.Errorf("StepCycles(100) = %d, want 4", cycles)
		}
		if cpu.Deficit() != 0 {
			t.Errorf("Deficit() = %d, want 0", cpu.Deficit())
		}
	})

	t.Run("budget equal to cost", func(t *testing.T) {
		cpu, _ := newNOPCPU(1)

		cycles := cpu.StepCycles(4)
		if cycles != 4 {
			t.Errorf("StepCycles(4) = %d, want 4", cycles)
		}
		if cpu.Deficit() != 0 {
			t.Errorf("Deficit() = %d, want 0", cpu.Deficit())
		}
	})

	t.Run("budget smaller than cost creates deficit", func(t *testing.T) {
		cpu, _ := newNOPCPU(1)

		cycles := cpu.StepCycles(1)
		if cycles != 1 {
			t.Errorf("StepCycles(1) = %d, want 1", cycles)
		}
		if cpu.Deficit() != 3 {
			t.Errorf("Deficit() = %d, want 3", cpu.Deficit())
		}
	})

	t.Run("deficit paid off in one call", func(t *testing.T) {
		cpu, _ := newNOPCPU(2)

		// First call: NOP costs 4, budget is 1 → deficit = 3
		cpu.StepCycles(1)

		// Second call: pay off deficit of 3 with budget of 100
		cycles := cpu.StepCycles(100)
		if cycles != 3 {
			t.Errorf("StepCycles(100) = %d, want 3", cycles)
		}
		if cpu.Deficit() != 0 {
			t.Errorf("Deficit() = %d, want 0", cpu.Deficit())
		}
	})

	t.Run("deficit paid off across multiple calls", func(t *testing.T) {
		cpu, _ := newNOPCPU(2)

		// NOP costs 4, budget is 1 → deficit = 3
		cpu.StepCycles(1)

		// Pay 1 of 3 → deficit = 2
		cycles := cpu.StepCycles(1)
		if cycles != 1 {
			t.Errorf("StepCycles(1) = %d, want 1", cycles)
		}
		if cpu.Deficit() != 2 {
			t.Errorf("Deficit() = %d, want 2", cpu.Deficit())
		}

		// Pay 1 of 2 → deficit = 1
		cycles = cpu.StepCycles(1)
		if cycles != 1 {
			t.Errorf("StepCycles(1) = %d, want 1", cycles)
		}
		if cpu.Deficit() != 1 {
			t.Errorf("Deficit() = %d, want 1", cpu.Deficit())
		}

		// Pay 1 of 1 → deficit = 0
		cycles = cpu.StepCycles(1)
		if cycles != 1 {
			t.Errorf("StepCycles(1) = %d, want 1", cycles)
		}
		if cpu.Deficit() != 0 {
			t.Errorf("Deficit() = %d, want 0", cpu.Deficit())
		}
	})

	t.Run("multiple instructions within budget", func(t *testing.T) {
		cpu, _ := newNOPCPU(10)

		// Run 3 NOPs using StepCycles in a budget loop
		budget := 12
		count := 0
		for budget > 0 {
			cycles := cpu.StepCycles(budget)
			budget -= cycles
			count++
		}
		if count != 3 {
			t.Errorf("executed %d steps, want 3", count)
		}
		if budget != 0 {
			t.Errorf("remaining budget = %d, want 0", budget)
		}
	})

	t.Run("scanline boundary simulation", func(t *testing.T) {
		cpu, _ := newNOPCPU(20)

		// Scanline 1: budget of 10 cycles. NOPs cost 4 each.
		// Should fit 2 NOPs (8 cycles), third NOP overflows (4 > 2 remaining).
		budget := 10
		total := 0
		for budget > 0 {
			cycles := cpu.StepCycles(budget)
			budget -= cycles
			total += cycles
		}
		if total != 10 {
			t.Errorf("scanline 1 total = %d, want 10", total)
		}
		deficit := cpu.Deficit()
		if deficit != 2 {
			t.Errorf("deficit after scanline 1 = %d, want 2", deficit)
		}

		// Scanline 2: budget of 10. First call pays off deficit of 2.
		budget = 10
		total = 0
		first := cpu.StepCycles(budget)
		budget -= first
		total += first
		if first != 2 {
			t.Errorf("first call of scanline 2 = %d, want 2 (deficit payoff)", first)
		}

		// Continue running the rest of the budget
		for budget > 0 {
			cycles := cpu.StepCycles(budget)
			budget -= cycles
			total += cycles
		}
		if total != 10 {
			t.Errorf("scanline 2 total = %d, want 10", total)
		}
	})

	t.Run("halted CPU returns zero", func(t *testing.T) {
		cpu, _ := newNOPCPU(1)

		// Set PC to odd address to trigger halt
		cpu.SetState(Registers{PC: 0x1001, SR: 0x2700, SSP: 0x10000})
		cpu.Step()

		cycles := cpu.StepCycles(100)
		if cycles != 0 {
			t.Errorf("StepCycles(100) on halted CPU = %d, want 0", cycles)
		}
	})

	t.Run("reset clears deficit", func(t *testing.T) {
		cpu, bus := newNOPCPU(1)

		// Create a deficit
		cpu.StepCycles(1)
		if cpu.Deficit() == 0 {
			t.Fatal("expected non-zero deficit before reset")
		}

		// Set up reset vectors so Reset() works
		bus.Write32(0, 0x10000) // SSP
		bus.Write32(4, 0x1000)  // PC
		fillNOPs(bus, 0x1000, 10)

		cpu.Reset()
		if cpu.Deficit() != 0 {
			t.Errorf("Deficit() after Reset = %d, want 0", cpu.Deficit())
		}
	})
}

func TestAddCycles(t *testing.T) {
	cpu, _ := newNOPCPU(1)

	before := cpu.Cycles()
	cpu.AddCycles(100)
	after := cpu.Cycles()

	if after-before != 100 {
		t.Errorf("AddCycles(100): got delta %d, want 100", after-before)
	}
}
