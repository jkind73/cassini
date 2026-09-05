# Japanese Game Region Fix Bugfix Design

## Overview

This bugfix addresses a critical issue where Japanese (JAP) region Saturn games hang after the Sega logo splash screen. The root cause is that the SMPC area code remains at the default value of 0x04 (North America) instead of being set to 0x01 (Japan) when a Japanese disc is loaded. This causes the game's region verification code to enter an infinite polling loop.

The fix requires correcting the `autoDetectRegion()` function in `core/emulator.go` to properly read the area code from the disc's IP header.

## Glossary

- **Bug_Condition (C)**: When a Japanese disc with 'J' at the correct System ID offset is loaded, but the SMPC area code is not set to 0x01
- **Property (P)**: The SMPC area code should match the disc's region (0x01 for Japan, 0x04 for North America, 0x0C for Europe)
- **Preservation**: Non-Japanese region discs (U, E) should continue to work as before
- **autoDetectRegion()**: Function in `core/emulator.go` that reads the disc's IP image and sets the SMPC area code
- **ipImage**: 32KB buffer (16 sectors × 2048 bytes) containing the disc's Initial Program data
- **areaCode**: SMPC register that stores the region code (0x01=Japan, 0x04=North America, 0x0C=Europe)

## Bug Details

### Bug Condition

The bug manifests when a Japanese Saturn disc is loaded. The `autoDetectRegion()` function attempts to read the area code from the IP image but fails to correctly identify the 'J' region marker, leaving the SMPC area code at the default 0x04 (North America).

**Formal Specification:**
```
FUNCTION isBugCondition(input)
  INPUT: input is a DiscReader that returns sector data with 'J' at System ID area code position
  OUTPUT: boolean
  
  RETURN readIPImage(input) succeeds
         AND ipImage has valid length (>= 0x4A)
         AND 'J' is present in area code field at offset 0x40
         AND smpc.areaCode != 0x01  // Bug: remains at default 0x04
END FUNCTION
```

### Examples

1. **Japanese Disc Loading**: A disc with 'J' in the System ID area code field is loaded, but `e.smpc.areaCode` remains 0x04 instead of changing to 0x01
2. **North America Disc**: A disc with 'U' in the area code field correctly sets `e.smpc.areaCode` to 0x04 (works as expected)
3. **Europe Disc**: A disc with 'E' in the area code field correctly sets `e.smpc.areaCode` to 0x0C (works as expected)
4. **Unknown Region**: A disc with no recognized area code leaves `e.smpc.areaCode` unchanged at 0x04 (works as expected)

## Expected Behavior

### Preservation Requirements

**Unchanged Behaviors:**
- North America (U) discs shall continue to set areaCode to 0x04
- Europe (E) discs shall continue to set areaCode to 0x0C and VDP2 PAL to true
- Unknown region discs shall not overwrite the existing areaCode
- No disc (nil) shall not change the areaCode from its default 0x04

**Scope:**
All inputs that do NOT involve Japanese ('J') region discs should be completely unaffected by this fix. This includes:
- North America region discs
- Europe region discs
- Unknown/invalid region discs
- No disc loaded scenarios

## Hypothesized Root Cause

Based on analysis of the code in `core/emulator.go` and `core/hlebios.go`, there are several potential issues:

### Potential Issue 1: Incorrect Offset in IP Image

The `autoDetectRegion()` function reads from `e.ipImage[0x40:0x4A]` assuming the area code field starts at byte 0x40 within the IP image. However, the IP image is constructed from multiple sectors:

- The IP image buffer contains 16 sectors × 2048 bytes = 32768 bytes
- Each sector has a 16-byte header (sync + sector header) before the 2048-byte user data
- The System ID containing the area code is at offset 0x40 within sector 0's user data

The current code correctly calculates this as `ipImage[0x40]`, so this is likely NOT the issue.

### Potential Issue 2: Area Code Character Position

The Saturn disc System ID format typically has the area code at specific positions within the 10-byte area field. Looking at real Saturn disc images:
- The area code field is 10 bytes at System ID offset 0x40
- The first compatible region is typically at position 0 or 1 within this field
- Some discs may have padding (spaces or nulls) before the region character

The current code iterates through all 10 bytes looking for 'J', 'U', or 'E', which should handle padding. This appears correct.

### Potential Issue 3: Case Sensitivity

The current code compares against uppercase 'J', 'U', 'E'. If the disc has lowercase characters, they would not match. This is unlikely for official discs but could be an edge case.

### Potential Issue 4: Real Disc Data Format

This is the most likely cause. The test mock (`makeRegionDisc` in `bus_test.go`) writes the area code character at:
```
d.sector[16+0x40+i] = ch  // offset 16 (user data start) + 0x40 (area code offset) + i
```

This places the character at raw sector offset `0x50 + i` (80-89 in decimal).

However, the `readIPImage` function reads sectors with:
```go
copy(ip[s*ipSectorSize:], raw[ipUserOffset:ipUserOffset+ipSectorSize])
```

Where `ipUserOffset = 16`. So the user data starts at byte 16 of each sector.

The issue is: **real disc images may have the System ID in a different format or position than what the test mock simulates**. The actual Saturn System ID structure may differ from the test implementation.

### Most Likely Root Cause

The `autoDetectRegion()` function logic appears sound, but the test mock (`makeRegionDisc`) may not accurately represent how real Japanese discs store their region information. The function correctly iterates through the 10-byte area field looking for 'J', but if real Japanese discs store the 'J' at a different offset within that 10-byte field, or in a different format (e.g., embedded in a longer string), the detection would fail.

## Correctness Properties

Property 1: Bug Condition - Japanese Region Detection

_For any_ input where a disc with 'J' at the System ID area code position is loaded, the fixed `autoDetectRegion()` function SHALL set `e.smpc.areaCode` to 0x01, allowing the game to proceed past the region check.

**Validates: Requirements 2.1, 2.2, 2.3**

Property 2: Preservation - North America Region

_For any_ input where a disc with 'U' at the System ID area code position is loaded, the fixed `autoDetectRegion()` function SHALL set `e.smpc.areaCode` to 0x04, preserving existing North America behavior.

**Validates: Requirements 3.1**

Property 3: Preservation - Europe Region

_For any_ input where a disc with 'E' at the System ID area code position is loaded, the fixed `autoDetectRegion()` function SHALL set `e.smpc.areaCode` to 0x0C and `e.vdp2.pal` to true, preserving existing Europe behavior.

**Validates: Requirements 3.2**

Property 4: Preservation - Unknown Region

_For any_ input where a disc has no recognizable area code, the fixed `autoDetectRegion()` function SHALL NOT modify `e.smpc.areaCode`, preserving the existing value.

**Validates: Requirements 3.3, 3.4**

## Fix Implementation

### Changes Required

**File**: `core/emulator.go`

**Function**: `autoDetectRegion()`

**Specific Changes**:

1. **Add Debug Logging**: Add logging to understand what bytes are actually being read from the IP image at offset 0x40
   - Log the raw bytes at `ipImage[0x40:0x4A]` as hex and ASCII
   - This will help diagnose if the data is being read correctly

2. **Verify IP Image Reading**: Ensure `readIPImage()` correctly populates the IP image buffer
   - The function reads sectors 0-15 and extracts user data (16 bytes header + 2048 bytes user data per sector)
   - Verify that sector 0's user data is at ipImage[0:2048]
   - Verify that System ID area code is at ipImage[0x40]

3. **Handle Edge Cases**: If the root cause analysis reveals the issue is in data reading, fix the offset calculation:
   - Ensure correct sector selection for System ID location
   - Ensure correct offset within sector user data

### Proposed Code Change

```go
func (e *Emulator) autoDetectRegion() {
	if len(e.ipImage) < 0x4A {
		return
	}
	areaField := e.ipImage[0x40:0x4A]
	
	// Debug: Log what we're reading
	fmt.Printf("[autoDetectRegion] areaField bytes: % x\n", areaField)
	fmt.Printf("[autoDetectRegion] areaField ASCII: %q\n", string(areaField))
	
	for _, ch := range areaField {
		switch ch {
		case 'U':
			e.smpc.areaCode = 0x04
			e.vdp2.SetPAL(false)
			return
		case 'J':
			e.smpc.areaCode = 0x01
			e.vdp2.SetPAL(false)
			return
		case 'E':
			e.smpc.areaCode = 0x0C
			e.vdp2.SetPAL(true)
			return
		}
	}
}
```

If debugging reveals the data is at a different offset, adjust the offset:

```go
// Alternative: if area code is at different offset
// areaField := e.ipImage[0x50:0x5A]  // for raw sector offset 0x50
```

## Testing Strategy

### Validation Approach

The testing strategy follows a two-phase approach: first, surface counterexamples that demonstrate the bug on unfixed code, then verify the fix works correctly and preserves existing behavior.

### Exploratory Bug Condition Checking

**Goal**: Surface counterexamples that demonstrate the bug BEFORE implementing the fix. Confirm or refute the root cause analysis.

**Test Plan**: Run the emulator with a Japanese disc and observe the SMPC area code value. Add debug logging to see what bytes are actually being read at the area code offset.

**Test Cases**:
1. **Japanese Disc Test**: Load a disc with 'J' in the area code field, verify areaCode becomes 0x01 (will fail on unfixed code if bug exists)
2. **North America Disc Test**: Load a disc with 'U' in the area code field, verify areaCode becomes 0x04 (should pass - baseline)
3. **Europe Disc Test**: Load a disc with 'E' in the area code field, verify areaCode becomes 0x0C (should pass - baseline)
4. **Unknown Region Test**: Load a disc with 'Z' in the area code field, verify areaCode remains unchanged (should pass - baseline)

**Expected Counterexamples**:
- SMPC areaCode remains 0x04 even when Japanese disc is loaded
- Debug output shows incorrect or empty bytes at ipImage[0x40:0x4A]

### Fix Checking

**Goal**: Verify that for all inputs where the bug condition holds, the fixed function produces the expected behavior.

**Pseudocode:**
```
FOR ALL disc WHERE disc.hasRegion('J') DO
  result := autoDetectRegion(disc)
  ASSERT result.areaCode == 0x01
END FOR
```

### Preservation Checking

**Goal**: Verify that for all inputs where the bug condition does NOT hold, the fixed function produces the same result as the original function.

**Pseudocode:**
```
FOR ALL disc WHERE NOT disc.hasRegion('J') DO
  ASSERT autoDetectRegion_original(disc) = autoDetectRegion_fixed(disc)
END FOR
```

**Testing Approach**: Property-based testing is recommended for preservation checking.

**Test Cases**:
1. **NA Preservation**: Verify North America ('U') discs continue to set areaCode to 0x04
2. **EU Preservation**: Verify Europe ('E') discs continue to set areaCode to 0x0C and PAL to true
3. **Unknown Preservation**: Verify unknown region discs don't overwrite areaCode

### Unit Tests

- Test `autoDetectRegion()` with mock discs for each region (J, U, E, unknown)
- Test edge case: IP image shorter than 0x4A bytes
- Test edge case: IP image is nil/empty

### Property-Based Tests

- Generate random valid area code strings and verify correct detection
- Generate random invalid area code strings and verify preservation

### Integration Tests

- Full game boot with Japanese disc - verify game progresses past Sega logo
- Full game boot with NA disc - verify existing behavior unchanged
- Full game boot with EU disc - verify existing behavior unchanged