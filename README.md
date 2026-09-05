# Cassini

Cassini is a 100% hardware cycle-accurate Sega Saturn emulator written in Go. 

Unlike traditional emulators that rely on High-Level Emulation (HLE) or loose synchronization loops, Cassini advances the entire virtual machine on a single master clock timeline. Every chip—including the dual Hitachi SH-2 CPUs, the VDP1/VDP2 graphics processors, and the SCU—is synchronized cycle by cycle.

## Philosophy

Cassini is built on the principle that true compatibility and software preservation come from replicating the exact physical behavior of the silicon, not from hacking game-specific patches.

*   **Bare-Metal Precision:** No high-level guesswork. We target the absolute lowest level of the hardware bus.
*   **Deterministic by Design:** Every run with the same inputs and media produces the exact same byte-identical state. This unlocks flawless save states and deep debugging tools.
*   **Go-Powered Concurrency:** Utilizing Go's native channels and goroutines to model the Saturn's complex, independent hardware buses while maintaining strict, deterministic cycle synchronization.

---

## Architecture & Module Codenames

The Sega Saturn is notoriously complex, utilizing a multi-processor layout. Cassini mirrors this architecture through distinct, isolated Go packages named after the system hardware and historic space exploration terms:

### The Core Engine
*   **`/internal/scu` (Saturn Control Unit):** The central hub managing internal data flow, DMA transfers, and interrupt-recognition latency.
*   **`/internal/sh2` (DualSH2):** The twin Hitachi SH-2 processors running in perfect synchronization.
*   **`/internal/vdp1` & `/internal/vdp2`:** Dedicated background and sprite rendering engines, advancing along a unified beam-position timeline.
*   **`/internal/smpc`:** System Manager and Peripheral Control, managing system deployment and low-level timings.

### Infrastructure & Tooling
*   **`Orbiter` (`/pkg/orbiter`):** The master clock synchronization loop. It arbitrates the system bus per clock tick across all processors.
*   **`Huygens` (`/pkg/huygens`):** The external interface layer housing the native desktop UI, peripheral input mappings, and frontend controls.
*   **`Grand Tour` (`/test/grandtour`):** Cassini’s continuous integration test suite. It validates cycle counts and instruction timings against automated regression benchmarks to guarantee absolute accuracy.

---

## Features (Target Roadmap)

*   **Cycle-Driven Chip Bus:** Exact replication of bus contention and priorities between the CPUs, SCU, and graphics processors.
*   **Deterministic Replay:** Capture entire gameplay sessions into lightweight scripts that replay byte-for-byte identically every time.
*   **Scriptable & Headless:** Completely decoupled from the host GUI. Run Cassini windowless via the command line to dump frames, run automated hardware tests, or interface with external tools.
*   **Developer-First Tooling:** A built-in console and interactive memory inspector capable of tracing exactly which instruction or bus master last modified a specific word in memory.

---

## Development & Building

### Prerequisites
*   Go 1.24 or higher

### Building from Source
Because Cassini prioritizes strict hardware accuracy, always compile with optimizations enabled for real-time performance:

```bash
git clone https://github.com
cd cassini
go build -o cassini ./cmd/cassini
```

### Running the Emulator
Launch directly from the command line by pointing to a configuration file or a raw binary:

```bash
./cassini --config cassini.toml
./cassini --bios path/to/saturn.bin --cdrom path/to/game.cue
```

---

## License

Cassini is open-source software licensed under the GNU General Public License v3 (GPL-3.0). 

*Disclaimer: Sega Saturn is a trademark of its respective owners. Cassini is an independent, unofficial project and is not affiliated with, sponsored by, or endorsed by any trademark holder.*
