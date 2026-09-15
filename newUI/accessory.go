// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package ebiten

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/jkind73/cassini/config"
	"github.com/jkind73/cassini/uiface"
)

// accessoryState owns per-port accessory selection. Lives on game
// (alongside poller/present), constructed once at session-independent
// startup and reused across sessions -- the accessory *configuration*
// is process-wide (set in the settings screen, item 8), while the
// underlying SMPC accessory *instances* are per-session (created
// fresh in startSession via the Set*Mode calls, same lifecycle as
// everything else that needs a live m.emu).
type accessoryState struct {
	port1, port2 config.PortAccessory
}

func newAccessoryState(p1, p2 config.PortAccessory) *accessoryState {
	return &accessoryState{port1: p1, port2: p2}
}

// applyToSession configures SMPC-level accessory selection once at
// session start (called from startSession, after m.emu is created and
// before Start(), same ordering as BIOS/disc/ROM-cart setup).
func (a *accessoryState) applyToSession(emu uiface.Emulator) {
	applyAccessoryMode(emu, 0, a.port1)
	applyAccessoryMode(emu, 1, a.port2)
}

func applyAccessoryMode(emu uiface.Emulator, port int, acc config.PortAccessory) {
	switch acc.Type {
	case "3dpad", "missionstick":
		// Both use SetAnalogPadMode to enable the port -- the SMPC
		// side (smpc.go.patch.txt) distinguishes 3D Control Pad vs
		// Mission Stick framing by which Set*Data call last wrote to
		// analogData[port].missionMode, not by a separate mode flag,
		// matching how the manual itself treats these as variants of
		// the same "Multi Controller"-family port state rather than
		// wholly distinct device classes.
		emu.SetAnalogPadMode(port, true)
	case "racing":
		emu.SetRacingMode(port, true)
	case "mouse":
		emu.SetMouseMode(port, true)
	case "keyboard":
		emu.SetKeyboardMode(port, true)
	case "md3":
		emu.SetMDPadMode(port, true, false)
	case "md6":
		emu.SetMDPadMode(port, true, true)
	default: // "pad": nothing to enable, it's the always-available fallback
	}
}

// poll feeds live input for the current frame. Call once per Update()
// tick during modePlaying, same cadence as inputPoller.poll -- entirely
// separate from inputPoller (which only ever drives the standard
// digital pad's uiface.SystemInfo.Buttons mapping); accessory input has
// its own shape per device type and talks to the emulator through the
// dedicated Set*Data methods instead of SetInput.
// poll feeds live input for the current frame. Call once per Update()
// tick during modePlaying, same cadence as inputPoller.poll -- and
// like inputPoller.poll, routes everything through m.sendInput's
// core-thread queue rather than calling m.emu directly: SMPC's new
// per-port state fields (mouseData/analogData/racingData/mdPadData)
// are read during collectPeripheralData mid-RunFrame, so writing them
// straight from the UI thread would be exactly the same
// SetOption-class data race the rest of this codebase is careful to
// avoid (see optionEvent's doc comment). Only the one-time mode
// *selection* in applyToSession is safe to call directly -- that
// happens in startSession, before the core thread goroutine exists.
func (a *accessoryState) poll(m *manager) {
	pollAccessory(m, 0, a.port1)
	pollAccessory(m, 1, a.port2)
}

func pollAccessory(m *manager, port int, acc config.PortAccessory) {
	switch acc.Type {
	case "3dpad":
		buttons := gamepadDigitalButtons()
		ax := calibratedChannel(m, "ax")
		ay := calibratedChannel(m, "ay")
		ar := calibratedChannel(m, "ar")
		al := calibratedChannel(m, "al")
		m.sendInput(inputEvent{kind: inputAnalogPad, player: port, padButtons: buttons, ax: ax, ay: ay, ar: ar, al: al})
	case "missionstick":
		buttons := gamepadDigitalButtons()
		ax := calibratedChannel(m, "ax")
		ay := calibratedChannel(m, "ay")
		az := calibratedChannel(m, "az")
		m.sendInput(inputEvent{kind: inputMissionStick, player: port, padButtons: buttons, ax: ax, ay: ay, az: az})
	case "racing":
		buttons := gamepadDigitalButtons()
		ax := calibratedChannel(m, "ax") // steering
		m.sendInput(inputEvent{kind: inputRacing, player: port, padButtons: buttons, ax: ax})
	case "mouse":
		pollMouse(m, port)
	case "keyboard":
		pollKeyboard(m, port)
	case "md3":
		pollMDPad(m, port, false)
	case "md6":
		pollMDPad(m, port, true)
	}
}

// --- gamepad-analog-backed devices (3D Control Pad, Mission Stick,
// Racing Controller) ---
//
// Digital button assignments below have no default host-input mapping
// in the manual (it only specifies the Saturn-side wire format, see
// smpc.go.patch.txt) -- they're a reasonable, documented UX choice,
// not something verified against any hardware spec, and that's fine:
// there is no "hardware-accurate" answer to "which Xbox button is
// Saturn's A button" since that mapping never existed on real
// hardware.
//
// Analog channels are different: the manual DOES specify an exact
// 8-bit wire value per channel (0x00/0x80/0xFF at rest/center/full
// throw), and that value must be produced by explicit calibration
// (calibration.go), never approximated -- see calibratedChannel below.
// Only the first connected standard-layout gamepad is used; no
// keyboard fallback is attempted for analog axes, since a keyboard
// has no analog input to offer.

func firstGamepadID() (ebiten.GamepadID, bool) {
	var ids []ebiten.GamepadID
	ids = ebiten.AppendGamepadIDs(ids)
	if len(ids) == 0 || !ebiten.IsStandardGamepadLayoutAvailable(ids[0]) {
		return 0, false
	}
	return ids[0], true
}

func gamepadDigitalButtons() uint16 {
	id, ok := firstGamepadID()
	if !ok {
		return 0xFFFF // all released (active-low)
	}
	var b uint16 = 0xFFFF
	press := func(btn ebiten.StandardGamepadButton, mask uint16) {
		if ebiten.IsStandardGamepadButtonPressed(id, btn) {
			b &^= mask
		}
	}
	// Right,Left,Down,Up,Start,ATRG,CTRG,BTRG in the high byte (bits
	// 15-8), matching every digital-pad-shaped table in the manual.
	press(ebiten.StandardGamepadButtonLeftRight, 0x8000)
	press(ebiten.StandardGamepadButtonLeftLeft, 0x4000)
	press(ebiten.StandardGamepadButtonLeftBottom, 0x2000)
	press(ebiten.StandardGamepadButtonLeftTop, 0x1000)
	press(ebiten.StandardGamepadButtonCenterRight, 0x0800) // Start
	press(ebiten.StandardGamepadButtonRightBottom, 0x0400) // A
	press(ebiten.StandardGamepadButtonRightLeft, 0x0200)   // C (mapped to X face button)
	press(ebiten.StandardGamepadButtonRightRight, 0x0100)  // B
	return b
}

// calibratedChannel returns the current calibrated byte for the named
// channel ("ax"/"ay"/"ar"/"al"/"az") on the first connected gamepad,
// using m.calibration -- device SDL ID + channel name -> the explicit
// AxisCalibration the settings-screen wizard produced (calibration.go).
//
// No calibration entry for this device/channel -> returns the safe
// rest byte (0x80, since every calibratable channel here is either a
// stick axis or reused across trigger/stick contexts and 0x80 is
// harmless for both -- a trigger channel's own calibration, once
// present, maps its own Rest to 0x00 per mapToByte) rather than any
// approximated reading. This is the "never guess" boundary: an
// uncalibrated channel is inert, not wrong.
func calibratedChannel(m *manager, channel string) uint8 {
	id, ok := firstGamepadID()
	if !ok {
		return 0x80
	}
	sdlID := ebiten.GamepadSDLID(id)
	dev, ok := m.calibration[sdlID]
	if !ok {
		return 0x80
	}
	cal, ok := dev[channel]
	if !ok {
		return 0x80
	}
	axes := sampleAxes(id)
	if cal.RawAxis < 0 || cal.RawAxis >= len(axes) {
		return 0x80 // device axis layout changed since calibration -- don't guess, stay at rest
	}
	return mapToByte(axes[cal.RawAxis], cal)
}

// --- Saturn Mouse ---

var prevMouseX, prevMouseY = -1, -1 // shared across ports since only one physical OS cursor exists; realistically only one port is ever configured as mouse at a time

func pollMouse(m *manager, port int) {
	x, y := ebiten.CursorPosition()
	var dx, dy int32
	if prevMouseX != -1 {
		dx = int32(x - prevMouseX)
		dy = int32(y - prevMouseY)
	}
	prevMouseX, prevMouseY = x, y

	left := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	right := ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight)
	middle := ebiten.IsMouseButtonPressed(ebiten.MouseButtonMiddle)
	// Start: no standard PC mouse button maps to this (it's specific
	// to Sega's own 3-button-plus-Start mouse hardware) -- left
	// unconnected rather than guessing an arbitrary keybind for it.
	m.sendInput(inputEvent{kind: inputMouse, player: port, left: left, middle: middle, right: right, start: false, dx: dx, dy: dy})
}

// --- Saturn Keyboard ---

func pollKeyboard(m *manager, port int) {
	var justPressed, justReleased []ebiten.Key
	justPressed = inpututil.AppendJustPressedKeys(justPressed)
	justReleased = inpututil.AppendJustReleasedKeys(justReleased)

	// sendKeyboardEvent (blocking), not sendInput (drop-on-full-queue)
	// -- see sendKeyboardEvent's doc comment. Make/Break events are
	// discrete and non-idempotent, unlike every other input kind this
	// file sends.
	for _, k := range justPressed {
		if code, ok := ebitenKeyToSaturnScancode(k); ok {
			m.sendKeyboardEvent(inputEvent{kind: inputKeyboard, player: port, scancode: code, released: false})
		}
	}
	for _, k := range justReleased {
		if code, ok := ebitenKeyToSaturnScancode(k); ok {
			m.sendKeyboardEvent(inputEvent{kind: inputKeyboard, player: port, scancode: code, released: true})
		}
	}
}

// ebitenKeyToSaturnScancode maps ebiten's Key to the PS/2 Set 2
// scancode constants smpc_keyboard.go.newfile.txt defines (Code*),
// covering the practically-bindable keyboard, same coverage
// philosophy as input.go's keyByNameTable. Values are the literal
// byte constants, not re-derived from strings, to avoid a
// string-round-trip for something called every keystroke.
func ebitenKeyToSaturnScancode(k ebiten.Key) (byte, bool) {
	switch k {
	case ebiten.KeyArrowUp:
		return 0x75, true
	case ebiten.KeyArrowDown:
		return 0x72, true
	case ebiten.KeyArrowLeft:
		return 0x6B, true
	case ebiten.KeyArrowRight:
		return 0x74, true
	case ebiten.KeyEnter:
		return 0x5A, true
	case ebiten.KeyEscape:
		return 0x76, true
	case ebiten.KeySpace:
		return 0x29, true
	case ebiten.KeyTab:
		return 0x0D, true
	case ebiten.KeyBackspace:
		return 0x66, true
	case ebiten.KeyDelete:
		return 0x71, true
	case ebiten.KeyInsert:
		return 0x70, true
	case ebiten.KeyHome:
		return 0x6C, true
	case ebiten.KeyEnd:
		return 0x69, true
	case ebiten.KeyPageUp:
		return 0x7D, true
	case ebiten.KeyPageDown:
		return 0x7A, true
	case ebiten.KeyCapsLock:
		return 0x58, true
	case ebiten.KeyNumLock:
		return 0x77, true
	case ebiten.KeyScrollLock:
		return 0x7E, true
	case ebiten.KeyF1:
		return 0x05, true
	case ebiten.KeyF2:
		return 0x06, true
	case ebiten.KeyF3:
		return 0x04, true
	case ebiten.KeyF4:
		return 0x0C, true
	case ebiten.KeyF5:
		return 0x03, true
	case ebiten.KeyF6:
		return 0x0B, true
	case ebiten.KeyF7:
		return 0x83, true
	case ebiten.KeyF8:
		return 0x0A, true
	case ebiten.KeyF9:
		return 0x01, true
	case ebiten.KeyF10:
		return 0x09, true
	case ebiten.KeyF11:
		return 0x78, true
	case ebiten.KeyF12:
		return 0x07, true
	case ebiten.KeyA:
		return 0x1C, true
	case ebiten.KeyS:
		return 0x1B, true
	case ebiten.KeyD:
		return 0x23, true
	case ebiten.KeyQ:
		return 0x15, true
	case ebiten.KeyE:
		return 0x24, true
	case ebiten.KeyZ:
		return 0x1A, true
	case ebiten.KeyX:
		return 0x22, true
	case ebiten.KeyC:
		return 0x21, true
	default:
		return 0, false
	}
}

// --- Mega Drive 3/6-button pad ---
//
// Reuses gamepad face buttons directly (not the full inputPoller
// rebind system, which is scoped to uiface.SystemInfo.Buttons' own ID
// space) -- same UX-choice caveat as the analog devices above.

func pollMDPad(m *manager, port int, sixButton bool) {
	id, ok := firstGamepadID()
	press := func(btn ebiten.StandardGamepadButton) bool {
		return ok && ebiten.IsStandardGamepadButtonPressed(id, btn)
	}
	right := press(ebiten.StandardGamepadButtonLeftRight)
	left := press(ebiten.StandardGamepadButtonLeftLeft)
	down := press(ebiten.StandardGamepadButtonLeftBottom)
	up := press(ebiten.StandardGamepadButtonLeftTop)
	start := press(ebiten.StandardGamepadButtonCenterRight)
	a := press(ebiten.StandardGamepadButtonFrontTopLeft) // MD's A is a shoulder-adjacent button on a 6-button-shaped layout; L1 is a reasonable stand-in
	b := press(ebiten.StandardGamepadButtonRightBottom)
	c := press(ebiten.StandardGamepadButtonRightRight)
	mode := press(ebiten.StandardGamepadButtonCenterLeft) // Select/Back as Mode, a common convention
	x := press(ebiten.StandardGamepadButtonRightLeft)
	y := press(ebiten.StandardGamepadButtonRightTop)
	z := press(ebiten.StandardGamepadButtonFrontTopRight)

	if !sixButton {
		x, y, z, mode = false, false, false, false
	}

	// Pack into the same active-low bit layout coreThread's
	// inputMDPad case decodes (right,left,down,up,start,a,c,b in bits
	// 15-8), so the queued event stays a plain data struct rather than
	// needing 8 separate bool fields for this one case.
	var packed uint16 = 0xFF00 // bits 15-8 default "released"; low byte unused here
	set := func(pressed bool, mask uint16) {
		if pressed {
			packed &^= mask
		}
	}
	set(right, 0x8000)
	set(left, 0x4000)
	set(down, 0x2000)
	set(up, 0x1000)
	set(start, 0x0800)
	set(a, 0x0400)
	set(c, 0x0200)
	set(b, 0x0100)

	m.sendInput(inputEvent{kind: inputMDPad, player: port, padButtons: packed, mdMode: mode, mdX: x, mdY: y, mdZ: z, sixButton: sixButton})
}
