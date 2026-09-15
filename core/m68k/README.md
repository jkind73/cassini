# Cassini MC68EC000 core

This directory is Cassini's locally maintained Motorola 68000-family CPU core.
Cassini uses it to emulate the Saturn SCSP's MC68EC000 sound CPU. It is part of
the main Cassini module so CPU execution, bus transactions, interrupt timing,
and save-state changes can evolve together with the SCSP and master timeline.

## Provenance

The initial source and tests were imported without behavioral changes from
[`github.com/user-none/go-chip-m68k`](https://github.com/user-none/go-chip-m68k)
version `v0.1.2`, commit `780201073a735f28d19e0c4c804071b9b30be3a1`
(2026-07-05). The upstream MIT license is retained in [LICENSE](LICENSE).
Cassini no longer resolves that module at build time.

## Accuracy baseline

The imported core is instruction-oriented and cycle-approximate. In particular:

- `StepCycles` executes an instruction atomically, then spreads an over-budget
  cycle deficit over later calls. It does not expose individual MC68000 bus
  cycles or the instruction prefetch pipeline.
- multiply and divide use fixed worst-case timings instead of operand-dependent
  hardware timings;
- CHK uses a fixed exception cost;
- immediate bit-operation timings differ from hardware-verified test data;
- address errors halt instead of constructing the MC68000 exception frame;
- trace exceptions are not implemented, and TAS/TRAPV are not fully modeled.

These are recorded limitations, not claims of cycle accuracy. The local core's
long-term contract is to replace them with behavior supported by Motorola
documentation and focused hardware/reference tests, without game-specific
timing hacks.

Run its focused tests with:

```sh
go test ./core/m68k
```
