# Project-Specific Rules for erings (Windows Native)

## Build & Environment Synchronization Rules
1. **Windows Native Source Single Source of Truth**: `C:\Users\jkind\erings\` is the authoritative source repository.
2. **Build Execution Rule**: Always build directly in `C:\Users\jkind\erings\`:
   ```cmd
   set PATH=C:\msys64\ucrt64\bin;%PATH%
   set CGO_ENABLED=1
   go build -v -o erings.exe ./cmd/desktop/
   ```
3. **No Unverified Declarations**: Never report that a feature or UI element is completed/visible until verifying that the exact source code changes were compiled into `C:\Users\jkind\erings\erings.exe`.
4. **Listen to Environment Warnings**: If the user points out a potential environment, path, sync, or build discrepancy, pause and verify the build pipeline before taking any other action.
