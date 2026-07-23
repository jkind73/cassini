# Project Rules for erings (Windows Native)

## 1. Transparency & Honest Technical Communication
- **No Fluff & No Sycophancy**: Never use hyperbolic claims like "100% complete", "perfect", or "flawless". State progress in plain, brutal technical terms.
- **Upfront Feasibility & Gaps**: If a request is impossible, partial, or requires a workaround, state it **immediately in sentence one** before writing any code. Never cover up gaps or promise native hardware features when only software/mouse mappings are implemented.

## 2. Environment & Single Source of Truth
- **Authoritative Repository**: `C:\Users\jkind\erings\` is the single source of truth.
- **Direct Windows Build**: Always build directly in `C:\Users\jkind\erings\`:
  ```cmd
  set PATH=C:\msys64\ucrt64\bin;%PATH%
  set CGO_ENABLED=1
  go build -v -o erings.exe ./cmd/desktop/
  ```
- **No Twin Directories**: Never use `/home/jkind/erings` or `erings_build`. Build directly in `C:\Users\jkind\erings\`.

## 3. Mandatory Self-Verification & Proof of Work
- **No Unverified Declarations**: Never claim a feature, fix, or UI control is completed or visible without verifying that the exact source code changes compiled into `C:\Users\jkind\erings\erings.exe` and checking the executable timestamp.
- **Proof-of-Work Evidence**: Every completion response must include:
  1. Files & line numbers modified.
  2. Executable path & timestamp verified.
  3. Terminal build/test output.

## 4. Immediate Pause on Discrepancies
- If the user points out a potential environment, path, sync, UI visibility, or build discrepancy, execution MUST immediately pause to verify the build pipeline before taking any other action.
