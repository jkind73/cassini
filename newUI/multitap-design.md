# Multi Terminal 6 (6-Player Adaptor) design

Not implemented as a patch in this pass -- see the risk note at the
end for why. This is a complete, precise specification, verified
against the official Sega SMPC User's Manual (translated scan,
www.infochunk.com/saturn/segahtml_en/hard/smpc/hon/, pages p03_12
through p03_17), sufficient to implement directly once someone can
test it against real timing/compatibility test ROMs.

## Sourcing

Every byte-level detail below is transcribed from Sega's own 1997
document, not reconstructed from memory or a third party's
reimplementation. Cross-checked against cassini's own existing code:
the port-status byte convention (`0xF1` = multitap ID `F` + connector
count `1`) and the peripheral-ID byte convention (`0x02` = type `0`,
size `2`) both already match the manual exactly, which is a strong
consistency signal that cassini's existing SMPC model is
protocol-accurate and this design builds on solid ground.

## Wire format (all confirmed from the manual)

**Port status byte** (1 per port): `[MultitapID:4][Connectors:4]`
- `MultitapID = 0xF`, `Connectors = 1`: direct single-peripheral
  connection (cassini's current, only, behavior)
- `MultitapID = 0x0`, `Connectors = 4`: Sega Tap (4-port adaptor)
- `MultitapID = 0x1`, `Connectors = 6`: Saturn 6-Player Adaptor
- `MultitapID = 0x0-0xE`, `Connectors = 0`: nothing connected / unknown
  device

**Peripheral ID byte** (1 per connected device, follows immediately
after the port status byte, or after the previous peripheral's data
for the 2nd+ device on a tap): `[Type:4][DataSize:4]`. Self-describing
-- a reader never needs a separate lookup table for byte count, only
for what the data *means*.

**Overall stream, per INTBACK peripheral collection**: `[Port1
status][Port1 peripheral 1: ID+data][Port1 peripheral 2: ID+data]...
[Port2 status][Port2 peripheral 1: ID+data]...`. A port not in 0-byte
mode always contributes at least its status byte; a port with
`Connectors=1` contributes exactly one peripheral block after it, a
port with `Connectors=6` contributes six.

## The actual new work: resumable streaming across OREG (32 bytes)

`OREG` is a fixed 32-byte array (confirmed: `oreg [32]uint8` already
in cassini's `SMPC` struct). A full 6-Player Adaptor port alone can
produce far more than that (1 status + 6 × up to 7 bytes for a 3D
Control Pad = 43 bytes for ONE port), so the logical stream above must
be split across multiple OREG batches, with the game issuing a
`continue` request (the existing `CONT`/bit-7 mechanism cassini's
`processINTBACKStep` already implements correctly, timing included)
between batches.

Design: build the full logical stream once, as a plain `[]byte`, when
peripheral collection starts; each `collectPeripheralData` call
(initial dispatch and every subsequent continue) just copies the next
up-to-32-byte window and advances a stream position.

```go
// New SMPC fields
intbackStream    []byte // full serialized peripheral-data stream for this session
intbackStreamPos int    // bytes already emitted across previous batches
```

```go
// buildIntbackStream replaces collectPeripheralData's direct oreg
// writes with a two-phase design: serialize once, then chunk.
func (s *SMPC) buildIntbackStream() []byte {
    var buf []byte
    if s.intbackP1MD != 3 {
        buf = s.appendPortStream(buf, 0)
    }
    if s.intbackP2MD != 3 {
        buf = s.appendPortStream(buf, 1)
    }
    return buf
}

// appendPortStream appends one port's status byte and every connected
// peripheral's ID+data block. tapConnectors(port) returns 1 (direct,
// current-only behavior) or 4/6 once a Sega Tap / 6-Player Adaptor is
// configured for that port (new per-port state needed: multitap type
// + connector count, analogous to gunEnabled/mouseEnabled).
func (s *SMPC) appendPortStream(buf []byte, port int) []byte {
    tapID, connectors := s.multitapConfig(port) // 0xF,1 today; 0x0,4 or 0x1,6 with a tap
    buf = append(buf, tapID<<4|connectors)
    for sub := 0; sub < int(connectors); sub++ {
        buf = s.appendPeripheralBlock(buf, port, sub)
    }
    return buf
}
```

`collectPeripheralData` becomes:

```go
func (s *SMPC) collectPeripheralData() {
    if s.intbackStream == nil {
        s.intbackStream = s.buildIntbackStream()
        s.intbackStreamPos = 0
    }
    n := copy(s.oreg[:], s.intbackStream[s.intbackStreamPos:])
    s.intbackStreamPos += n
    remaining := len(s.intbackStream) - s.intbackStreamPos

    var sr uint8 = 0x40 // bit7=1 fixed (already how cassini encodes this), PDE equivalent
    if s.intbackStreamPos == n {
        sr |= 0x40 // PDL: this batch IS the 1st peripheral data
    }
    if remaining > 0 {
        sr |= 0x20 // NPE: more data remains, game must issue continue
    } else {
        s.intbackStream = nil // session done; next collection starts fresh
    }
    s.sr = sr
}
```

**This is the part I'm not confident enough to ship blind**: the exact
PDL semantics ("Peripheral data after 0: 2nd / 1: 1st peripheral
data") is oddly worded even in the primary source (likely a rough
translation), and cassini's *existing* fixed `0x60` for the
single-pad case sets both PDL and NPE unconditionally -- which happens
to work today only because no non-multitap game actually checks NPE.
The design above computes both bits for real, but "PDL means batch
one vs. batch two+" is my best reading of an ambiguous translated
sentence, not something I can independently verify without a real
Saturn multitap compatibility test ROM (e.g. a homebrew SMPC
conformance test) to check the computed SR sequence against. Getting
this wrong would desync peripheral polling for real 6-player games in
a way that's hard to diagnose (input would appear to work sometimes
and glitch unpredictably depending on exact continue-request timing).

## New per-port state needed (mirrors gunEnabled/mouseEnabled/analogEnabled from smpc.go.patch.txt)

```go
multitapType [2]uint8 // 0xF=none, 0x0=Sega Tap, 0x1=6-Player Adaptor
// padState already sized [12]uint16 (2 ports x 6) -- already
// correctly dimensioned for this, confirmed unused capacity from the
// original SMPC struct.
```

`SetPadData`/`PadData` already accept ports 0-11 (`port/6` = physical
port, `port%6` = sub-connector index within that port's tap) -- no
change needed there, this dead capacity becomes live once
`appendPeripheralBlock` actually reads from it for sub-ports beyond 0.

## UI-side work not covered by this design

Even once the above is implemented and verified, `input.go`'s
`inputPoller` (maxPlayers, currently mis-set to 8 -- see the earlier
conversation) needs updating to actually route players 3-12 to
`port*6+sub` addressing, and the browser/settings UI needs a way to
say "6-Player Adaptor connected on port 1" per port (this IS a
legitimate case for exposing to the UI, unlike the RAM cart size,
since real hardware requires the player to physically own and plug in
the adaptor -- cassini can't auto-detect it from the disc the way BIOS
region or RAM cart size can be inferred).

## Recommendation

Implement `smpc.go.patch.txt` (Mouse, 3D Control Pad, Mission Stick,
Racing Controller) now -- verified, low-risk, self-contained. Treat
this document as the starting point for a dedicated multitap pass once
there's a way to validate the SR bit sequence against real hardware
behavior or a trusted reference emulator's test suite, rather than
risk shipping a cycle-accurate emulator's interrupt-timing-sensitive
code on my best reading of one ambiguous sentence in a
machine-translated manual.

## Update: the byte budget is more forgiving than first assessed

Re-reading the manual's full peripheral tables (fetched in full after
this document's first draft) narrows the actual risk considerably. The
manual states outright: "The maximum data size for each tap of
Multi-Terminal 6 is 15 bytes. Use the port mode in 15-byte mode" --
Sega's own spec constrains what a compliant 6P tap can present, and
the failure mode I was worried about only appears in a specific
combination:

- One port with a 6-Player Adaptor of plain digital pads: 1 status +
  6*(1 ID + 2 data) = 19 bytes. Comfortably under 32 with the other
  port even fully populated (a lone standard pad is 4 bytes: 19+4=23).
- Sega Tap (4-port) of digital pads: 1 + 4*3 = 13 bytes -- fits under
  15-byte mode on its own, no continuation needed at all regardless of
  the other port.
- The overflow case I originally sized against was the worst
  case -- BOTH ports simultaneously running a 6P tap of the largest
  peripheral (3D Control Pad, 7 bytes/device): 2*(1+6*7) = 86 bytes,
  which does need real continuation.

So: a **scoped** version -- 6-Player Adaptor or Sega Tap on one port
at a time, standard digital pads only (no analog controllers behind
the tap) -- fits inside a single 32-byte OREG batch with NPE
legitimately 0 (no continuation ever triggered), meaning it would NOT
exercise the ambiguous PDL semantics or the multi-batch resume logic
at all. That's a meaningfully smaller, safer piece of work than full
general multitap, and is probably worth doing as a first step whenever
this is picked back up -- but it's still being held per instruction,
not implemented in this pass.
