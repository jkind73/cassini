package m68k

// opFunc is the handler signature for a single MC68000 instruction.
// The first word of the instruction is already in c.ir when called.
type opFunc func(*CPU)

// opcodeTable is a 64K-entry lookup table indexed by the first instruction word.
// nil entries are treated as illegal instructions.
var opcodeTable [65536]opFunc
