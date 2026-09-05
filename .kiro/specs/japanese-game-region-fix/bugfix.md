# Bugfix Requirements Document

## Introduction

When running a Japanese (JAP) version of a Saturn game, the emulator gets stuck in an infinite loop after the Sega logo splash screen. The game completes the BIOS animation, loads the game, shows a splash screen and the Sega logo, then hangs. This bug prevents Japanese region games from progressing past the initial boot sequence.

## Bug Analysis

### Current Behavior (Defect)

1.1 WHEN a Japanese (JAP) region disc is loaded with 'J' at System ID offset 0x40 AND the emulator reads the disc's IP image THEN the SMPC area code remains at default 0x04 (North America) instead of being set to 0x01 (Japan)

1.2 WHEN the SMPC area code is 0x04 (North America) for a Japanese game THEN the game's region check code at PC=0x60023F0 enters a polling/timeout loop that spins forever waiting for hardware state that never becomes ready, with r1=0x00000000 indicating the comparison value is always zero

1.3 WHEN the BIOS region check passes (game boots past BIOS) but the SMPC area code is incorrect THEN the game's own region verification code at 0x60023F0 reads from memory or hardware expecting a non-zero region-dependent value and loops infinitely because the value remains zero

### Expected Behavior (Correct)

2.1 WHEN a Japanese disc with 'J' at System ID offset 0x40 is loaded THEN the SMPC area code SHALL be set to 0x01 (Japan) so that region-dependent hardware state is correctly configured

2.2 WHEN the SMPC area code is correctly set to 0x01 for a Japanese game THEN the polling loop at PC=0x60023F0 SHALL receive a non-zero comparison value and exit the loop, allowing the game to proceed to gameplay

2.3 WHEN region auto-detection successfully identifies Japan ('J') from the disc's IP header THEN the emulator SHALL propagate the correct area code before any game code executes, ensuring all region checks pass

### Unchanged Behavior (Regression Prevention)

3.1 WHEN a North America (U) disc is loaded THEN the SMPC area code SHALL CONTINUE TO be set to 0x04 as currently implemented

3.2 WHEN a Europe (E) disc is loaded THEN the SMPC area code SHALL CONTINUE TO be set to 0x0C and VDP2 PAL SHALL CONTINUE TO be set to true as currently implemented

3.3 WHEN an unknown region disc is loaded THEN the SMPC area code SHALL CONTINUE TO remain at its current value (not be overwritten) as currently implemented

3.4 WHEN no disc is loaded (nil DiscReader) THEN the SMPC area code SHALL CONTINUE TO remain at its default value of 0x04 as currently implemented