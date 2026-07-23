# Project-Specific Rules for erings

## Build & Environment Synchronization Rules
1. **Source Tree Single Source of Truth**: `/home/jkind/erings/` is the authoritative source repository.
2. **Build Execution Rule**: Whenever building the Windows executable via CMD/MSYS2:
   - Always copy/sync the workspace source tree directly to `C:\Users\jkind\erings_build\` **BEFORE** running `go build`.
   - Never build in `C:\Users\jkind\erings_build\` without syncing files first.
3. **No Unverified Declarations**: Never report that a feature or UI element is completed/visible until verifying that the exact source code changes were compiled into the exact binary path being tested.
4. **Listen to Environment Warnings**: If the user points out a potential environment, path, sync, or build discrepancy, pause and verify the build pipeline before taking any other action.
