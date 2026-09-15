package m68k

// eaFetchCycles returns the source operand EA timing (PRM Table 8-1).
// For register-direct modes (Dn, An) returns 0.
// For memory/immediate modes returns the fetch cost.
// Long adds 4 to all non-zero values.
func eaFetchCycles(mode, reg uint8, sz size) uint64 {
	var base uint64
	switch mode {
	case 0, 1: // Dn, An
		base = 0
	case 2, 3: // (An), (An)+
		base = 4
	case 4: // -(An)
		base = 6
	case 5: // d16(An)
		base = 8
	case 6: // d8(An,Xn)
		base = 10
	case 7:
		switch reg {
		case 0: // abs.W
			base = 8
		case 1: // abs.L
			base = 12
		case 2: // d16(PC)
			base = 8
		case 3: // d8(PC,Xn)
			base = 10
		case 4: // #imm
			base = 4
		}
	}
	if sz == sizeLong && base > 0 {
		base += 4
	}
	return base
}

// eaWriteCycles returns the destination EA write timing.
// Same as eaFetchCycles except -(An) costs 4 (not 6).
func eaWriteCycles(mode, reg uint8, sz size) uint64 {
	var base uint64
	switch mode {
	case 0, 1: // Dn, An
		base = 0
	case 2, 3, 4: // (An), (An)+, -(An)
		base = 4
	case 5: // d16(An)
		base = 8
	case 6: // d8(An,Xn)
		base = 10
	case 7:
		switch reg {
		case 0: // abs.W
			base = 8
		case 1: // abs.L
			base = 12
		}
	}
	if sz == sizeLong && base > 0 {
		base += 4
	}
	return base
}
