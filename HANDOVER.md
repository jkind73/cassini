# Project Overview & Handover Context for erings (Windows Native)

## Repository Target
- **Path**: `C:\Users\jkind\erings\`
- **Project**: `erings` (Sega Saturn Emulator & Desktop UI)
- **UI Framework**: `eblitui` desktop UI (`github.com/user-none/eblitui`)

---

## Single Source of Truth & Build Instructions
1. **Source Location**: `C:\Users\jkind\erings\` is the single source of truth.
2. **Windows Compilation Command**:
   ```cmd
   set PATH=C:\msys64\ucrt64\bin;%PATH%
   set CGO_ENABLED=1
   go build -v -o erings.exe ./cmd/desktop/
   ```
3. **No Twin Directories**: Never use `/home/jkind/erings` or `erings_build`. Build directly in `C:\Users\jkind\erings`.

---

## Active & Implemented Features Status

### 1. Sinden Lightgun & Multi-Controller System
- **Core Options Integration** (`adapter/adapter.go`):
  - `lightgun_mode` ("Sinden Lightgun / Virtua Gun", Bool, Default: "true", Category: `CategoryInput`)
  - `lightgun_port` ("Lightgun Controller Port", Select: "Port 1", "Port 2", Category: `CategoryInput`)
  - `active_players` ("Active Controller Ports", Select: "2 Players", "4 Players", "6 Players", "12 Players", Category: `CategoryInput`)
- **UI Exposure**: Options are registered under `CoreOptionCategoryInput` so they appear at the top of the **Settings -> Input** screen.

### 2. Configuration System
- **Default Auto-Creation**: If `config.json` is missing from `%APPDATA%\erings\`, the emulator automatically populates default values for all core options upon startup (`fast_boot`, `lightgun_mode`, `lightgun_port`, `active_players`, `scale_factor`, `texture_filtering`, `aspect_ratio`, `display_shader`, `upscaler_filter`, `crt_bloom`, `crt_curvature`).

### 3. Graphics Enhancements Engine
- **VDP1 3D Resolution Scaling**: Custom scale factors (1x, 2x, 3x, 4x internal upscaling).
- **Texture Filtering**: Bilinear sub-texel filtering for VDP1 sprites & quad polygons.
- **Display Shaders & Post-Processing**: CRT shader (scanlines, curvature, aperture grille, bloom) and presentation upscaler filters (Nearest, Bilinear, Bicubic, HQ2x, HQ4x, xBRZ).

---

## Project Rules (AGENTS.md)
1. **Native Source Single Source of Truth**: All edits and builds MUST happen inside `C:\Users\jkind\erings\`.
2. **Empirical Verification**: Never report a feature or UI element as complete without compiling `erings.exe` directly in the target folder and verifying build timestamps.
3. **Strict Compliance**: All features must be immediately visible and accessible in the UI layout without manual documentation or hidden flags.
