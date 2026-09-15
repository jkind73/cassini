# Full Motion Cartridge (Video CD Card / MPEG Card) status

## What actually exists already

Substantial, real infrastructure already exists in cassini, and I was
wrong in an earlier summary to say I'd "never found any interface for
it" -- I hadn't looked carefully enough. `cdblock_mpeg_decode.go`
(1305 lines) and `cdblock_cmd_mpeg.go` (560 lines) implement MPEG-1
video/audio decode logic and the CD block's command dispatch for it,
and `saturn_cdblock_mpeg.md` documents the architecture precisely.

Crucially, this is modeled correctly against real hardware: the
Video CD Card / MPEG Card connects through the CD block's own AUX bus
(two decoder LSIs at $0A100000/$0A180000 on the SH-1 side), not
through the SH-2 cartridge slot the way RAM/ROM carts do. cassini's
choice to implement this on `*CDBlock` rather than as a `Bus`-level
cartridge (like `romcart.go`) is architecturally the right call, not
an inconsistency.

## The real gap

Per the doc's own words: "On a stock unit this subsystem is
initialized and probed at boot, then never runs: task 8 waits forever,
and the DMA/capture interrupts never fire." This is correct,
intentional behavior *without* a cartridge present -- exactly mirroring
how the ROM cart address window correctly returns open bus (0xFF) with
no cart inserted.

The actual missing piece, confirmed by grep against the real project
files: the extension-image load path itself isn't implemented in Go
code at all yet. The doc describes it precisely -- at boot, the CD
block is supposed to read a length + firmware image from a window at
$0E000000, copy it to buffer DRAM at $0907B000, call the image's entry
point at $0907B004, and that entry point populates a RAM dispatch
table at $0907B008 that every MPEG host command ($90-$AF) dispatches
through. I grepped the entire codebase for 0x0E000000 and
0907B000/0907B008: zero hits outside comments. The mechanism is fully
specified in the documentation but not yet wired into cdblock.go.

This means two things are needed before any cartridge could work, not
one:

1. The loader mechanism itself -- decode the $0E000000 window, copy
   into buffer DRAM, invoke the entry point, all timed correctly
   against the CD block's own interrupt/task model (task 8, DMAC2/3,
   vectors 76/78/88/89 -- the same interrupt-timing-sensitive
   territory as the SMPC continuation work, just on the CD block's
   SH-1 side instead of SMPC).
2. A host-side hook to actually supply the cartridge's firmware image
   bytes -- the mechanical, low-risk part, directly analogous to
   SetBIOS/SetROMCartridge (host supplies bytes, core has no
   filesystem access of its own).

## Why this isn't patched in this pass

Same reasoning as multitap, for the same underlying reason: item 1
above is genuine, undocumented-in-code, interrupt-timing-sensitive
core work I can't test against real hardware timing or a real firmware
image in this sandbox. Unlike the RAM/ROM cart work, I also have zero
verified hash or content data for any actual Video CD Card / MPEG Card
firmware dump -- I didn't find anything resembling the Mednafen/Ymir
cross-references that made the KOF95/Ultraman ROM cart work possible.
Wiring a loader with no real image to validate it against would be
building and shipping untested code in the most literal sense.

## What would be safe to do now, if wanted

Item 2 alone -- the host-supplies-bytes hook -- follows the exact same
established pattern as SetBIOS/SetROMCartridge and carries the same
low risk, once item 1 exists to receive it. Not useful in isolation
though: a SetMPEGCartridge(data []byte) method with nothing on the
other end to consume it wouldn't do anything.

## Recommendation

Treat this as its own dedicated pass, same footing as multitap:
implement the $0E000000 loader mechanism against the documented spec
(the .md file is detailed enough to work from), then the host hook,
then test against whatever real or homebrew MPEG cartridge firmware
image becomes available. Not started here.
