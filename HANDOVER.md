# Project Overview & Handover Context for Cassini (Windows Native)

## Repository Target
- **Path**: `C:\Users\jkind\cassini\`
- **Project**: `cassini` (100% hardware cycle-accurate Sega Saturn emulator)
- **UI Framework**: `eblitui` desktop UI (`github.com/user-none/eblitui`)

---

## Single Source of Truth & Build Instructions
1. **Source Location**: `C:\Users\jkind\cassini\` is the single source of truth.
2. **Windows Compilation Command**:
   ```cmd
   set PATH=C:\msys64\ucrt64\bin;%PATH%
   set CGO_ENABLED=1
   go build -v -o cassini.exe ./cmd/desktop/
   ```
3. **No Twin Directories**: Build directly in `C:\Users\jkind\cassini`.

---

## Status & Architecture Target
- **Core Engine & Hardware Modules**: SH-2, VDP1, VDP2, SCU, SCSP, SMPC, CD-BLOCK.
- **Master Clock Scheduler (`Orbiter`)**: Target package `/pkg/orbiter` for master clock tick arbitration.
- **Frontend & Options (`Huygens`)**: Target package `/pkg/huygens` for UI controls and input mapping.
- **Timing Regression Suite (`Grand Tour`)**: Target package `/test/grandtour` for continuous timing verification.
