package m68k

import (
	"encoding/binary"
	"errors"
)

// cpuSerializeVersion is incremented whenever the binary layout changes.
const cpuSerializeVersion = 1

// SerializeSize is the number of bytes produced by CPU.Serialize.
// Update this constant whenever the binary layout changes.
const SerializeSize = 104

// Serialize writes the full CPU state into buf, which must be at least
// SerializeSize bytes. Returns an error if the buffer is too small.
// Bus references are not included.
func (c *CPU) Serialize(buf []byte) error {
	if len(buf) < SerializeSize {
		return errors.New("m68k: serialize buffer too small")
	}

	buf[0] = cpuSerializeVersion
	be := binary.BigEndian
	off := 1

	for i := 0; i < 8; i++ {
		be.PutUint32(buf[off:], c.reg.D[i])
		off += 4
	}
	for i := 0; i < 8; i++ {
		be.PutUint32(buf[off:], c.reg.A[i])
		off += 4
	}

	be.PutUint32(buf[off:], c.reg.PC)
	off += 4
	be.PutUint16(buf[off:], c.reg.SR)
	off += 2
	be.PutUint32(buf[off:], c.reg.USP)
	off += 4
	be.PutUint32(buf[off:], c.reg.SSP)
	off += 4
	be.PutUint16(buf[off:], c.reg.IR)
	off += 2

	be.PutUint64(buf[off:], c.cycles)
	off += 8
	be.PutUint16(buf[off:], c.ir)
	off += 2

	buf[off] = boolByte(c.stopped)
	off++
	buf[off] = boolByte(c.halted)
	off++

	be.PutUint32(buf[off:], c.prevPC)
	off += 4

	buf[off] = c.pendingIPL
	off++

	if c.pendingVec != nil {
		buf[off] = 1
		buf[off+1] = *c.pendingVec
	} else {
		buf[off] = 0
		buf[off+1] = 0
	}
	off += 2

	be.PutUint32(buf[off:], uint32(int32(c.deficit)))
	return nil
}

func boolByte(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

// Deserialize restores CPU state from buf, which must be at least
// SerializeSize bytes. Returns an error if the buffer is too small or
// the version does not match. The bus and cycleBus fields are left unchanged.
func (c *CPU) Deserialize(buf []byte) error {
	if len(buf) < SerializeSize {
		return errors.New("m68k: deserialize buffer too small")
	}
	if buf[0] != cpuSerializeVersion {
		return errors.New("m68k: unsupported serialize version")
	}

	be := binary.BigEndian
	off := 1

	for i := 0; i < 8; i++ {
		c.reg.D[i] = be.Uint32(buf[off:])
		off += 4
	}
	for i := 0; i < 8; i++ {
		c.reg.A[i] = be.Uint32(buf[off:])
		off += 4
	}

	c.reg.PC = be.Uint32(buf[off:])
	off += 4
	c.reg.SR = be.Uint16(buf[off:])
	off += 2
	c.reg.USP = be.Uint32(buf[off:])
	off += 4
	c.reg.SSP = be.Uint32(buf[off:])
	off += 4
	c.reg.IR = be.Uint16(buf[off:])
	off += 2

	c.cycles = be.Uint64(buf[off:])
	off += 8
	c.ir = be.Uint16(buf[off:])
	off += 2

	c.stopped = buf[off] != 0
	off++
	c.halted = buf[off] != 0
	off++

	c.prevPC = be.Uint32(buf[off:])
	off += 4

	c.pendingIPL = buf[off]
	off++

	if buf[off] != 0 {
		v := buf[off+1]
		c.pendingVec = &v
	} else {
		c.pendingVec = nil
	}
	off += 2

	c.deficit = int(int32(be.Uint32(buf[off:])))
	return nil
}
