package m68k

import "fmt"

// Disassemble decodes the single instruction at pc, reading instruction
// words through fetch. It returns the instruction text in Motorola syntax
// and the instruction length in bytes (always even, 2-10). Opcode words
// the execution core treats as illegal (opcodeTable has no handler)
// decode as "DC.W $XXXX" with length 2 so a linear sweep can continue.
// PC-relative operands and branch targets are printed as the resolved
// absolute address.
func Disassemble(pc uint32, fetch func(addr uint32) uint16) (string, uint32) {
	d := &dasm{pc: pc, at: pc, fetch: fetch}
	op := fetch(pc)
	if op == 0x4AFC {
		// The official illegal-instruction opcode; the execution core
		// (correctly) has no handler for it, but it has a mnemonic.
		d.at = pc + 2
		return "ILLEGAL", 2
	}
	if opcodeTable[op] == nil {
		return d.illegal(op), 2
	}
	text := d.instruction()
	if d.bad {
		return d.illegal(op), 2
	}
	return text, d.at - pc
}

// dasm is a cursor over the instruction words of one instruction.
type dasm struct {
	pc    uint32 // address of the instruction
	at    uint32 // address of the next unread word
	fetch func(addr uint32) uint16
	bad   bool // an operand field had no valid decoding
}

func (d *dasm) word() uint16 {
	v := d.fetch(d.at)
	d.at += 2
	return v
}

func (d *dasm) long() uint32 {
	hi := uint32(d.word())
	return hi<<16 | uint32(d.word())
}

// dispWord returns the next instruction word interpreted as a signed
// 16-bit displacement, sign-extended for 32-bit address arithmetic.
func (d *dasm) dispWord() uint32 {
	return uint32(int16(d.word()))
}

// dispByte interprets the low 8 bits of an instruction or extension
// word as a signed displacement, sign-extended for 32-bit address
// arithmetic.
func dispByte(w uint16) uint32 {
	return uint32(int8(w))
}

// cond is indexed by the 4-bit condition field.
var cond = [16]string{
	"T", "F", "HI", "LS", "CC", "CS", "NE", "EQ",
	"VC", "VS", "PL", "MI", "GE", "LT", "GT", "LE",
}

var sizeSuffix = map[size]string{sizeByte: ".B", sizeWord: ".W", sizeLong: ".L"}

// moveSize maps the MOVE size field (bits 13-12) to an operand size.
var moveSize = [4]size{0, sizeByte, sizeLong, sizeWord}

// stdSize maps the common 2-bit size field (bits 7-6) to an operand size.
var stdSize = [4]size{sizeByte, sizeWord, sizeLong, 0}

func (d *dasm) illegal(op uint16) string {
	// Rewind so length counts only the opcode word.
	d.at = d.pc + 2
	return fmt.Sprintf("DC.W $%04X", op)
}

// ea formats one effective-address operand, consuming extension words.
// sz selects the width of an immediate operand.
func (d *dasm) ea(mode, reg int, sz size) string {
	switch mode {
	case 0:
		return fmt.Sprintf("D%d", reg)
	case 1:
		return fmt.Sprintf("A%d", reg)
	case 2:
		return fmt.Sprintf("(A%d)", reg)
	case 3:
		return fmt.Sprintf("(A%d)+", reg)
	case 4:
		return fmt.Sprintf("-(A%d)", reg)
	case 5:
		return fmt.Sprintf("$%04X(A%d)", d.word(), reg)
	case 6:
		return d.indexOperand(fmt.Sprintf("A%d", reg), 0)
	case 7:
		switch reg {
		case 0:
			// Absolute short is sign-extended by the hardware; print the
			// effective 24-bit address.
			return fmt.Sprintf("($%06X).W", d.dispWord()&0xFFFFFF)
		case 1:
			return fmt.Sprintf("($%08X).L", d.long())
		case 2:
			base := d.at
			return fmt.Sprintf("($%06X)(PC)", (base+d.dispWord())&0xFFFFFF)
		case 3:
			return d.indexOperand("PC", d.at)
		case 4:
			switch sz {
			case sizeLong:
				return fmt.Sprintf("#$%08X", d.long())
			case sizeByte:
				return fmt.Sprintf("#$%02X", d.word()&0xFF)
			default:
				return fmt.Sprintf("#$%04X", d.word())
			}
		}
	}
	d.bad = true
	return "?"
}

// indexOperand formats a brief-extension-word operand: d8(base,Xn.size).
// pcBase is nonzero for the PC-relative form, where the displacement is
// resolved against the extension word address.
func (d *dasm) indexOperand(base string, pcBase uint32) string {
	ext := d.word()
	xn := fmt.Sprintf("D%d", (ext>>12)&7)
	if ext&0x8000 != 0 {
		xn = fmt.Sprintf("A%d", (ext>>12)&7)
	}
	xs := ".W"
	if ext&0x0800 != 0 {
		xs = ".L"
	}
	if pcBase != 0 {
		return fmt.Sprintf("($%06X)(PC,%s%s)", (pcBase+dispByte(ext))&0xFFFFFF, xn, xs)
	}
	return fmt.Sprintf("$%02X(%s,%s%s)", uint8(ext), base, xn, xs)
}

// regList formats a MOVEM register mask. When reversed (predecrement
// mode), bit 15 is D0 and bit 0 is A7; otherwise bit 0 is D0.
func regList(mask uint16, reversed bool) string {
	bit := func(i int) bool {
		if reversed {
			return mask&(1<<(15-i)) != 0
		}
		return mask&(1<<i) != 0
	}
	name := func(i int) string {
		if i < 8 {
			return fmt.Sprintf("D%d", i)
		}
		return fmt.Sprintf("A%d", i-8)
	}
	out := ""
	// Walk D0-D7 then A0-A7, collapsing runs within each bank.
	for bank := 0; bank < 2; bank++ {
		i := bank * 8
		end := i + 8
		for i < end {
			if !bit(i) {
				i++
				continue
			}
			j := i
			for j+1 < end && bit(j+1) {
				j++
			}
			if out != "" {
				out += "/"
			}
			if j > i {
				out += name(i) + "-" + name(j)
			} else {
				out += name(i)
			}
			i = j + 1
		}
	}
	if out == "" {
		return "(none)"
	}
	return out
}

func (d *dasm) instruction() string {
	op := d.word()
	switch op >> 12 {
	case 0x0:
		return d.group0(op)
	case 0x1, 0x2, 0x3:
		return d.move(op)
	case 0x4:
		return d.group4(op)
	case 0x5:
		return d.group5(op)
	case 0x6:
		return d.branch(op)
	case 0x7:
		if op&0x0100 != 0 {
			return d.illegal(op)
		}
		return fmt.Sprintf("MOVEQ #$%02X,D%d", uint8(op), (op>>9)&7)
	case 0x8:
		return d.group8(op)
	case 0x9, 0xD:
		return d.addSub(op)
	case 0xB:
		return d.groupB(op)
	case 0xC:
		return d.groupC(op)
	case 0xE:
		return d.shifts(op)
	}
	return d.illegal(op)
}

// group0 covers immediate ALU ops, static/dynamic bit ops, and MOVEP.
func (d *dasm) group0(op uint16) string {
	mode := int(op>>3) & 7
	reg := int(op) & 7

	if op&0x0100 != 0 {
		if mode == 1 {
			// MOVEP: bit 6 selects long size, bit 7 the direction.
			disp := d.word()
			dn := (op >> 9) & 7
			sz := ".W"
			if op&0x0040 != 0 {
				sz = ".L"
			}
			if op&0x0080 != 0 {
				return fmt.Sprintf("MOVEP%s D%d,$%04X(A%d)", sz, dn, disp, reg)
			}
			return fmt.Sprintf("MOVEP%s $%04X(A%d),D%d", sz, disp, reg, dn)
		}
		// Dynamic bit op: BTST/BCHG/BCLR/BSET Dn,ea.
		name := [4]string{"BTST", "BCHG", "BCLR", "BSET"}[(op>>6)&3]
		return fmt.Sprintf("%s D%d,%s", name, (op>>9)&7, d.ea(mode, reg, sizeByte))
	}

	switch (op >> 9) & 7 {
	case 0, 1, 2, 3, 5, 6: // ORI, ANDI, SUBI, ADDI, EORI, CMPI
		name := [8]string{"ORI", "ANDI", "SUBI", "ADDI", "", "EORI", "CMPI", ""}[(op>>9)&7]
		sz := stdSize[(op>>6)&3]
		if sz == 0 {
			return d.illegal(op)
		}
		// ORI/ANDI/EORI to CCR or SR use the immediate mode with size
		// byte (CCR) or word (SR).
		if mode == 7 && reg == 4 {
			if name != "ORI" && name != "ANDI" && name != "EORI" {
				return d.illegal(op)
			}
			switch sz {
			case sizeByte:
				return fmt.Sprintf("%s #$%02X,CCR", name, d.word()&0xFF)
			case sizeWord:
				return fmt.Sprintf("%s #$%04X,SR", name, d.word())
			default:
				return d.illegal(op)
			}
		}
		var imm string
		switch sz {
		case sizeLong:
			imm = fmt.Sprintf("#$%08X", d.long())
		case sizeByte:
			imm = fmt.Sprintf("#$%02X", d.word()&0xFF)
		default:
			imm = fmt.Sprintf("#$%04X", d.word())
		}
		return fmt.Sprintf("%s%s %s,%s", name, sizeSuffix[sz], imm, d.ea(mode, reg, sz))
	case 4: // Static bit op: BTST/BCHG/BCLR/BSET #imm,ea.
		name := [4]string{"BTST", "BCHG", "BCLR", "BSET"}[(op>>6)&3]
		bit := d.word() & 0xFF
		return fmt.Sprintf("%s #%d,%s", name, bit, d.ea(mode, reg, sizeByte))
	}
	return d.illegal(op)
}

func (d *dasm) move(op uint16) string {
	sz := moveSize[(op>>12)&3]
	src := d.ea(int(op>>3)&7, int(op)&7, sz)
	dstMode := int(op>>6) & 7
	dstReg := int(op>>9) & 7
	if dstMode == 1 {
		if sz == sizeByte {
			return d.illegal(op)
		}
		return fmt.Sprintf("MOVEA%s %s,A%d", sizeSuffix[sz], src, dstReg)
	}
	dst := d.ea(dstMode, dstReg, sz)
	return fmt.Sprintf("MOVE%s %s,%s", sizeSuffix[sz], src, dst)
}

// group4 covers the miscellaneous 0100 family.
func (d *dasm) group4(op uint16) string {
	mode := int(op>>3) & 7
	reg := int(op) & 7

	switch {
	case op == 0x4E70:
		return "RESET"
	case op == 0x4E71:
		return "NOP"
	case op == 0x4E72:
		return fmt.Sprintf("STOP #$%04X", d.word())
	case op == 0x4E73:
		return "RTE"
	case op == 0x4E75:
		return "RTS"
	case op == 0x4E76:
		return "TRAPV"
	case op == 0x4E77:
		return "RTR"
	case op&0xFFF0 == 0x4E40:
		return fmt.Sprintf("TRAP #%d", op&0xF)
	case op&0xFFF8 == 0x4E50:
		return fmt.Sprintf("LINK A%d,#$%04X", reg, d.word())
	case op&0xFFF8 == 0x4E58:
		return fmt.Sprintf("UNLK A%d", reg)
	case op&0xFFF8 == 0x4E60:
		return fmt.Sprintf("MOVE A%d,USP", reg)
	case op&0xFFF8 == 0x4E68:
		return fmt.Sprintf("MOVE USP,A%d", reg)
	case op&0xFFC0 == 0x4E80:
		return fmt.Sprintf("JSR %s", d.ea(mode, reg, sizeWord))
	case op&0xFFC0 == 0x4EC0:
		return fmt.Sprintf("JMP %s", d.ea(mode, reg, sizeWord))
	case op&0xFFF8 == 0x4840:
		return fmt.Sprintf("SWAP D%d", reg)
	case op&0xFFC0 == 0x4840: // mode != 0 given SWAP above
		return fmt.Sprintf("PEA %s", d.ea(mode, reg, sizeLong))
	case op&0xFFB8 == 0x4880 && mode == 0:
		if op&0x0040 != 0 {
			return fmt.Sprintf("EXT.L D%d", reg)
		}
		return fmt.Sprintf("EXT.W D%d", reg)
	case op&0xFB80 == 0x4880: // MOVEM
		sz, suffix := sizeWord, ".W"
		if op&0x0040 != 0 {
			sz, suffix = sizeLong, ".L"
		}
		mask := d.word()
		list := regList(mask, mode == 4)
		target := d.ea(mode, reg, sz)
		if op&0x0400 != 0 {
			return fmt.Sprintf("MOVEM%s %s,%s", suffix, target, list)
		}
		return fmt.Sprintf("MOVEM%s %s,%s", suffix, list, target)
	case op&0xFFC0 == 0x40C0:
		return fmt.Sprintf("MOVE SR,%s", d.ea(mode, reg, sizeWord))
	case op&0xFFC0 == 0x44C0:
		return fmt.Sprintf("MOVE %s,CCR", d.ea(mode, reg, sizeWord))
	case op&0xFFC0 == 0x46C0:
		return fmt.Sprintf("MOVE %s,SR", d.ea(mode, reg, sizeWord))
	case op&0xFFC0 == 0x4800:
		return fmt.Sprintf("NBCD %s", d.ea(mode, reg, sizeByte))
	case op&0xFFC0 == 0x4AC0:
		return fmt.Sprintf("TAS %s", d.ea(mode, reg, sizeByte))
	case op&0xFF00 == 0x4A00:
		sz := stdSize[(op>>6)&3]
		if sz == 0 {
			return d.illegal(op)
		}
		return fmt.Sprintf("TST%s %s", sizeSuffix[sz], d.ea(mode, reg, sz))
	case op&0xF1C0 == 0x41C0:
		return fmt.Sprintf("LEA %s,A%d", d.ea(mode, reg, sizeLong), (op>>9)&7)
	case op&0xF1C0 == 0x4180:
		return fmt.Sprintf("CHK %s,D%d", d.ea(mode, reg, sizeWord), (op>>9)&7)
	case op&0xFF00 == 0x4000, op&0xFF00 == 0x4200, op&0xFF00 == 0x4400, op&0xFF00 == 0x4600:
		name := [4]string{"NEGX", "CLR", "NEG", "NOT"}[(op>>9)&3]
		sz := stdSize[(op>>6)&3]
		if sz == 0 {
			return d.illegal(op)
		}
		return fmt.Sprintf("%s%s %s", name, sizeSuffix[sz], d.ea(mode, reg, sz))
	}
	return d.illegal(op)
}

// group5 covers ADDQ/SUBQ, Scc, and DBcc.
func (d *dasm) group5(op uint16) string {
	mode := int(op>>3) & 7
	reg := int(op) & 7

	if op&0x00C0 == 0x00C0 {
		cc := cond[(op>>8)&0xF]
		if mode == 1 {
			// DBcc: DBT is written DBRA-style as DBF is; keep raw names.
			ext := d.at
			target := ext + d.dispWord()
			return fmt.Sprintf("DB%s D%d,$%06X", cc, reg, target&0xFFFFFF)
		}
		return fmt.Sprintf("S%s %s", cc, d.ea(mode, reg, sizeByte))
	}

	sz := stdSize[(op>>6)&3]
	data := (op >> 9) & 7
	if data == 0 {
		data = 8
	}
	name := "ADDQ"
	if op&0x0100 != 0 {
		name = "SUBQ"
	}
	return fmt.Sprintf("%s%s #%d,%s", name, sizeSuffix[sz], data, d.ea(mode, reg, sz))
}

func (d *dasm) branch(op uint16) string {
	cc := cond[(op>>8)&0xF]
	name := "B" + cc
	if cc == "T" {
		name = "BRA"
	} else if cc == "F" {
		name = "BSR"
	}
	if op&0xFF == 0 {
		ext := d.at
		target := ext + d.dispWord()
		return fmt.Sprintf("%s.W $%06X", name, target&0xFFFFFF)
	}
	return fmt.Sprintf("%s.S $%06X", name, (d.at+dispByte(op))&0xFFFFFF)
}

// group8 covers OR, DIVU/DIVS, and SBCD.
func (d *dasm) group8(op uint16) string {
	mode := int(op>>3) & 7
	reg := int(op) & 7
	dn := (op >> 9) & 7

	switch {
	case op&0x01C0 == 0x00C0:
		return fmt.Sprintf("DIVU %s,D%d", d.ea(mode, reg, sizeWord), dn)
	case op&0x01C0 == 0x01C0:
		return fmt.Sprintf("DIVS %s,D%d", d.ea(mode, reg, sizeWord), dn)
	case op&0x01F0 == 0x0100:
		if mode&1 != 0 {
			return fmt.Sprintf("SBCD -(A%d),-(A%d)", reg, dn)
		}
		return fmt.Sprintf("SBCD D%d,D%d", reg, dn)
	}
	return d.dnEA("OR", op)
}

// addSub covers ADD/ADDA/ADDX (0xD) and SUB/SUBA/SUBX (0x9).
func (d *dasm) addSub(op uint16) string {
	name := "ADD"
	if op>>12 == 0x9 {
		name = "SUB"
	}
	mode := int(op>>3) & 7
	reg := int(op) & 7
	dn := (op >> 9) & 7

	if op&0x00C0 == 0x00C0 {
		sz, suffix := sizeWord, ".W"
		if op&0x0100 != 0 {
			sz, suffix = sizeLong, ".L"
		}
		return fmt.Sprintf("%sA%s %s,A%d", name, suffix, d.ea(mode, reg, sz), dn)
	}
	if op&0x0100 != 0 && mode <= 1 {
		sz := stdSize[(op>>6)&3]
		if mode == 1 {
			return fmt.Sprintf("%sX%s -(A%d),-(A%d)", name, sizeSuffix[sz], reg, dn)
		}
		return fmt.Sprintf("%sX%s D%d,D%d", name, sizeSuffix[sz], reg, dn)
	}
	return d.dnEA(name, op)
}

// groupB covers CMP, CMPA, CMPM, and EOR.
func (d *dasm) groupB(op uint16) string {
	mode := int(op>>3) & 7
	reg := int(op) & 7
	dn := (op >> 9) & 7

	if op&0x00C0 == 0x00C0 {
		sz, suffix := sizeWord, ".W"
		if op&0x0100 != 0 {
			sz, suffix = sizeLong, ".L"
		}
		return fmt.Sprintf("CMPA%s %s,A%d", suffix, d.ea(mode, reg, sz), dn)
	}
	if op&0x0100 != 0 {
		sz := stdSize[(op>>6)&3]
		if mode == 1 {
			return fmt.Sprintf("CMPM%s (A%d)+,(A%d)+", sizeSuffix[sz], reg, dn)
		}
		return fmt.Sprintf("EOR%s D%d,%s", sizeSuffix[sz], dn, d.ea(mode, reg, sz))
	}
	sz := stdSize[(op>>6)&3]
	return fmt.Sprintf("CMP%s %s,D%d", sizeSuffix[sz], d.ea(mode, reg, sz), dn)
}

// groupC covers AND, MULU/MULS, ABCD, and EXG.
func (d *dasm) groupC(op uint16) string {
	mode := int(op>>3) & 7
	reg := int(op) & 7
	dn := (op >> 9) & 7

	switch {
	case op&0x01C0 == 0x00C0:
		return fmt.Sprintf("MULU %s,D%d", d.ea(mode, reg, sizeWord), dn)
	case op&0x01C0 == 0x01C0:
		return fmt.Sprintf("MULS %s,D%d", d.ea(mode, reg, sizeWord), dn)
	case op&0x01F0 == 0x0100:
		if mode&1 != 0 {
			return fmt.Sprintf("ABCD -(A%d),-(A%d)", reg, dn)
		}
		return fmt.Sprintf("ABCD D%d,D%d", reg, dn)
	case op&0x01F8 == 0x0140:
		return fmt.Sprintf("EXG D%d,D%d", dn, reg)
	case op&0x01F8 == 0x0148:
		return fmt.Sprintf("EXG A%d,A%d", dn, reg)
	case op&0x01F8 == 0x0188:
		return fmt.Sprintf("EXG D%d,A%d", dn, reg)
	}
	return d.dnEA("AND", op)
}

// dnEA formats the shared Dn-with-EA ALU form: bit 8 selects direction.
func (d *dasm) dnEA(name string, op uint16) string {
	mode := int(op>>3) & 7
	reg := int(op) & 7
	dn := (op >> 9) & 7
	sz := stdSize[(op>>6)&3]
	if sz == 0 {
		return d.illegal(op)
	}
	if op&0x0100 != 0 {
		return fmt.Sprintf("%s%s D%d,%s", name, sizeSuffix[sz], dn, d.ea(mode, reg, sz))
	}
	return fmt.Sprintf("%s%s %s,D%d", name, sizeSuffix[sz], d.ea(mode, reg, sz), dn)
}

// shifts covers the 0xE family: ASx, LSx, ROXx, ROx in register and
// memory forms.
func (d *dasm) shifts(op uint16) string {
	dir := "R"
	if op&0x0100 != 0 {
		dir = "L"
	}
	if op&0x00C0 == 0x00C0 {
		// Memory form: one-bit shift on a word EA. Bit 11 must be clear;
		// encodings with it set are 68020 bit-field instructions.
		if op&0x0800 != 0 {
			return d.illegal(op)
		}
		name := [4]string{"AS", "LS", "ROX", "RO"}[(op>>9)&3]
		return fmt.Sprintf("%s%s %s", name, dir, d.ea(int(op>>3)&7, int(op)&7, sizeWord))
	}
	name := [4]string{"AS", "LS", "ROX", "RO"}[(op>>3)&3]
	sz := stdSize[(op>>6)&3]
	reg := int(op) & 7
	if op&0x0020 != 0 {
		return fmt.Sprintf("%s%s%s D%d,D%d", name, dir, sizeSuffix[sz], (op>>9)&7, reg)
	}
	count := (op >> 9) & 7
	if count == 0 {
		count = 8
	}
	return fmt.Sprintf("%s%s%s #%d,D%d", name, dir, sizeSuffix[sz], count, reg)
}
