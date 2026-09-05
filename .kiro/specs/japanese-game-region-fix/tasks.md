# Implementation Plan

## Bugfix: Japanese Game Region Fix

- [ ] 1. Write bug condition exploration test
  - **Property 1: Bug Condition** - Japanese Region Detection
  - **CRITICAL**: This test MUST FAIL on unfixed code - failure confirms the bug exists
  - **DO NOT attempt to fix the test or the code when it fails**
  - **NOTE**: This test encodes the expected behavior - it will validate the fix when it passes after implementation
  - **GOAL**: Surface counterexamples that demonstrate the bug exists
  - **Scoped PBT Approach**: For deterministic bugs, scope the property to the concrete failing case(s) to ensure reproducibility
  - Test implementation details from Bug Condition in design
  - The test assertions should match the Expected Behavior Properties from design
  - Run test on UNFIXED code
  - **EXPECTED OUTCOME**: Test FAILS (this is correct - it proves the bug exists)
  - Document counterexamples found: SMPC areaCode remains 0x04 even when Japanese disc is loaded with 'J' in the area code field
  - Mark task complete when test is written, run, and failure is documented
  - _Requirements: 2.1, 2.2, 2.3_

- [ ] 2. Write preservation property tests (BEFORE implementing fix)
  - **Property 2: Preservation** - North America Region Detection
  - **IMPORTANT**: Follow observation-first methodology
  - Observe behavior on UNFIXED code for non-buggy inputs (U region discs)
  - Write property-based tests capturing observed behavior patterns from Preservation Requirements
  - Property-based testing generates many test cases for stronger guarantees
  - Run tests on UNFIXED code
  - **EXPECTED OUTCOME**: Tests PASS (this confirms baseline behavior to preserve)
  - Verify: North America ('U') discs correctly set areaCode to 0x04
  - Mark task complete when tests are written, run, and passing on unfixed code
  - _Requirements: 3.1_

  - **Property 2: Preservation** - Europe Region Detection
  - **IMPORTANT**: Follow observation-first methodology  
  - Observe behavior on UNFIXED code for non-buggy inputs (E region discs)
  - Verify: Europe ('E') discs correctly set areaCode to 0x0C and PAL to true
  - Run tests on UNFIXED code
  - **EXPECTED OUTCOME**: Tests PASS (this confirms baseline behavior to preserve)
  - Mark task complete when tests are written, run, and passing on unfixed code
  - _Requirements: 3.2_

  - **Property 2: Preservation** - Unknown Region Handling
  - **IMPORTANT**: Follow observation-first methodology
  - Observe behavior on UNFIXED code for unknown region discs
  - Verify: Unknown region discs don't overwrite areaCode
  - Run tests on UNFIXED code
  - **EXPECTED OUTCOME**: Tests PASS (this confirms baseline behavior to preserve)
  - Mark task complete when tests are written, run, and passing on unfixed code
  - _Requirements: 3.3, 3.4_

- [ ] 3. Implement fix for Japanese region detection bug

  - [ ] 3.1 Add debug logging to autoDetectRegion
    - Log the raw bytes at ipImage[0x40:0x4A] as hex and ASCII
    - This will help diagnose if the data is being read correctly
    - Run the test and capture debug output to see what bytes are actually read
    - _Bug_Condition: isBugCondition(input) where Japanese disc with 'J' at System ID area code position results in areaCode != 0x01_
    - _Requirements: 2.1, 2.2, 2.3_

  - [ ] 3.2 Investigate IP image offset correctness
    - Verify readIPImage() correctly populates the IP image buffer
    - Check if the offset should be 0x40 or 0x50 based on debug output
    - Compare test mock behavior vs real disc format
    - _Bug_Condition: isBugCondition(input) from design_
    - _Expected_Behavior: expectedBehavior(result) from design_
    - _Requirements: 2.1, 2.2, 2.3_

  - [ ] 3.3 Implement the correction based on findings
    - Adjust offset if debug reveals data is at different position (e.g., 0x50 vs 0x40)
    - Or fix case sensitivity if that's the issue
    - Or fix test mock to match real disc format if needed
    - _Bug_Condition: isBugCondition(input) where input.quantity = 0_
    - _Expected_Behavior: expectedBehavior(result) from design_
    - _Preservation: Preservation Requirements from design_
    - _Requirements: 2.1, 2.2, 2.3, 3.1, 3.2, 3.3, 3.4_

  - [ ] 3.4 Verify bug condition exploration test now passes
    - **Property 1: Expected Behavior** - Japanese Region Detection
    - **IMPORTANT**: Re-run the SAME test from task 1 - do NOT write a new test
    - The test from task 1 encodes the expected behavior
    - When this test passes, it confirms the expected behavior is satisfied
    - Run bug condition exploration test from step 1
    - **EXPECTED OUTCOME**: Test PASSES (confirms bug is fixed)
    - Verify: SMPC areaCode is now set to 0x01 for Japanese discs
    - _Requirements: 2.1, 2.2, 2.3_

  - [ ] 3.5 Verify preservation tests still pass
    - **Property 2: Preservation** - All Region Handling
    - **IMPORTANT**: Re-run the SAME tests from task 2 - do NOT write new tests
    - Run preservation property tests from step 2
    - **EXPECTED OUTCOME**: Tests PASS (confirms no regressions)
    - Verify North America discs still set areaCode to 0x04
    - Verify Europe discs still set areaCode to 0x0C and PAL to true
    - Verify unknown region discs don't overwrite areaCode
    - _Requirements: 3.1, 3.2, 3.3, 3.4_

- [ ] 4. Checkpoint - Ensure all tests pass
  - Ensure all tests pass, ask the user if questions arise.
  - Run full test suite to verify no regressions in other areas
  - Document final fix applied and verification results