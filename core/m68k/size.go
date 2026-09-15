package m68k

// size represents the operand width of a memory access or ALU operation.
type size int

const (
	sizeByte size = 1
	sizeWord size = 2
	sizeLong size = 4
)

// Mask returns a bitmask covering the valid bits for this size.
func (s size) Mask() uint32 {
	switch s {
	case sizeByte:
		return 0xFF
	case sizeWord:
		return 0xFFFF
	case sizeLong:
		return 0xFFFFFFFF
	default:
		return 0
	}
}

// MSB returns the most-significant bit position for this size.
func (s size) MSB() uint32 {
	switch s {
	case sizeByte:
		return 0x80
	case sizeWord:
		return 0x8000
	case sizeLong:
		return 0x80000000
	default:
		return 0
	}
}

// Bits returns the number of bits for this size.
func (s size) Bits() uint32 {
	return uint32(s) * 8
}

// String returns a human-readable name for this size.
func (s size) String() string {
	switch s {
	case sizeByte:
		return "byte"
	case sizeWord:
		return "word"
	case sizeLong:
		return "long"
	default:
		return "unknown"
	}
}
