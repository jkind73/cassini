// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package ebiten

import (
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/jkind73/cassini/ui"
	"github.com/jkind73/cassini/uiface"
)

// ---------------------------------------------------------------------
// Keyboard key-name resolution
// ---------------------------------------------------------------------

// keyByName resolves the key names used in uiface.Button.DefaultKey
// (and in config.toml keybind overrides, see config.go) to ebiten.Key
// values. Names follow the JS/W3C "KeyboardEvent.code"-style naming
// ebiten itself uses for its Key constants (e.g. "ArrowUp", "ShiftLeft",
// "Digit1", "KeyA"), so the same strings a user would see in ebiten's
// own docs work here directly — no separate naming scheme to maintain.
//
// Coverage: every letter, every digit, every arrow, all function keys
// F1-F12, all standard modifier/editing/navigation keys, and the full
// numeric keypad. This is the complete practically-bindable keyboard —
// keys omitted (PrintScreen, ScrollLock, Pause, media keys) are not
// exposed by ebiten.Key at all, so there is nothing further to map.
var keyByNameTable = buildKeyTable()

func buildKeyTable() map[string]ebiten.Key {
	t := make(map[string]ebiten.Key, 128)

	letters := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	keyLetters := []ebiten.Key{
		ebiten.KeyA, ebiten.KeyB, ebiten.KeyC, ebiten.KeyD, ebiten.KeyE,
		ebiten.KeyF, ebiten.KeyG, ebiten.KeyH, ebiten.KeyI, ebiten.KeyJ,
		ebiten.KeyK, ebiten.KeyL, ebiten.KeyM, ebiten.KeyN, ebiten.KeyO,
		ebiten.KeyP, ebiten.KeyQ, ebiten.KeyR, ebiten.KeyS, ebiten.KeyT,
		ebiten.KeyU, ebiten.KeyV, ebiten.KeyW, ebiten.KeyX, ebiten.KeyY,
		ebiten.KeyZ,
	}
	for i, ch := range letters {
		t["Key"+string(ch)] = keyLetters[i]
		t[string(ch)] = keyLetters[i] // bare letter also accepted
	}

	digitKeys := []ebiten.Key{
		ebiten.Key0, ebiten.Key1, ebiten.Key2, ebiten.Key3, ebiten.Key4,
		ebiten.Key5, ebiten.Key6, ebiten.Key7, ebiten.Key8, ebiten.Key9,
	}
	for i, k := range digitKeys {
		name := string(rune('0' + i))
		t["Digit"+name] = k
		t[name] = k
	}

	kpKeys := []ebiten.Key{
		ebiten.KeyKP0, ebiten.KeyKP1, ebiten.KeyKP2, ebiten.KeyKP3, ebiten.KeyKP4,
		ebiten.KeyKP5, ebiten.KeyKP6, ebiten.KeyKP7, ebiten.KeyKP8, ebiten.KeyKP9,
	}
	for i, k := range kpKeys {
		t["Numpad"+string(rune('0'+i))] = k
	}
	t["NumpadAdd"] = ebiten.KeyKPAdd
	t["NumpadSubtract"] = ebiten.KeyKPSubtract
	t["NumpadMultiply"] = ebiten.KeyKPMultiply
	t["NumpadDivide"] = ebiten.KeyKPDivide
	t["NumpadDecimal"] = ebiten.KeyKPDecimal
	t["NumpadEnter"] = ebiten.KeyKPEnter

	fnKeys := []ebiten.Key{
		ebiten.KeyF1, ebiten.KeyF2, ebiten.KeyF3, ebiten.KeyF4,
		ebiten.KeyF5, ebiten.KeyF6, ebiten.KeyF7, ebiten.KeyF8,
		ebiten.KeyF9, ebiten.KeyF10, ebiten.KeyF11, ebiten.KeyF12,
	}
	for i, k := range fnKeys {
		t["F"+itoa(i+1)] = k
	}

	t["ArrowUp"] = ebiten.KeyArrowUp
	t["ArrowDown"] = ebiten.KeyArrowDown
	t["ArrowLeft"] = ebiten.KeyArrowLeft
	t["ArrowRight"] = ebiten.KeyArrowRight

	t["Enter"] = ebiten.KeyEnter
	t["Escape"] = ebiten.KeyEscape
	t["Space"] = ebiten.KeySpace
	t["Tab"] = ebiten.KeyTab
	t["Backspace"] = ebiten.KeyBackspace
	t["Delete"] = ebiten.KeyDelete
	t["Insert"] = ebiten.KeyInsert
	t["Home"] = ebiten.KeyHome
	t["End"] = ebiten.KeyEnd
	t["PageUp"] = ebiten.KeyPageUp
	t["PageDown"] = ebiten.KeyPageDown
	t["CapsLock"] = ebiten.KeyCapsLock
	t["NumLock"] = ebiten.KeyNumLock
	t["ContextMenu"] = ebiten.KeyContextMenu

	t["ShiftLeft"] = ebiten.KeyShiftLeft
	t["ShiftRight"] = ebiten.KeyShiftRight
	t["ControlLeft"] = ebiten.KeyControlLeft
	t["ControlRight"] = ebiten.KeyControlRight
	t["AltLeft"] = ebiten.KeyAltLeft
	t["AltRight"] = ebiten.KeyAltRight
	t["MetaLeft"] = ebiten.KeyMetaLeft
	t["MetaRight"] = ebiten.KeyMetaRight

	t["Comma"] = ebiten.KeyComma
	t["Period"] = ebiten.KeyPeriod
	t["Slash"] = ebiten.KeySlash
	t["Backslash"] = ebiten.KeyBackslash
	t["Semicolon"] = ebiten.KeySemicolon
	t["Quote"] = ebiten.KeyQuote
	t["Backquote"] = ebiten.KeyBackquote
	t["Minus"] = ebiten.KeyMinus
	t["Equal"] = ebiten.KeyEqual
	t["BracketLeft"] = ebiten.KeyBracketLeft
	t["BracketRight"] = ebiten.KeyBracketRight

	return t
}

// itoa avoids importing strconv for a one-off single/double digit
// conversion used only at table-build time.
func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

func keyByName(name string) (ebiten.Key, bool) {
	k, ok := keyByNameTable[name]
	return k, ok
}

// keyName is the reverse of keyByName, used by the settings screen
// (settings.go) to display and persist a captured key press. Built
// once from keyByNameTable rather than maintained as a second literal
// table, so the two can never drift out of sync with each other.
var nameByKey = buildReverseKeyTable()

func buildReverseKeyTable() map[ebiten.Key]string {
	// Prefer the canonical "KeyX"/"DigitN"/"ArrowX" spellings over the
	// bare-letter/bare-digit aliases keyByNameTable also accepts, so
	// captured bindings get written to config in one consistent form.
	m := make(map[ebiten.Key]string, len(keyByNameTable))
	for name, k := range keyByNameTable {
		if _, exists := m[k]; exists && len(name) == 1 {
			continue // don't let a bare-letter alias overwrite a canonical name already stored
		}
		m[k] = name
	}
	return m
}

func keyName(k ebiten.Key) (string, bool) {
	n, ok := nameByKey[k]
	return n, ok
}

// ---------------------------------------------------------------------
// Gamepad button-name resolution
// ---------------------------------------------------------------------

// gamepadButtonByName resolves uiface.Button.DefaultPad names to
// ebiten's StandardGamepadButton, using conventional face/shoulder/
// stick/dpad naming that matches every SDL-mapped controller (Xbox,
// DualShock/DualSense, Switch Pro, and generic HID pads all get
// normalized to the "standard" layout by ebiten/the OS gamepad DB).
func gamepadButtonByName(name string) (ebiten.StandardGamepadButton, bool) {
	switch strings.ToUpper(name) {
	case "A", "CROSS":
		return ebiten.StandardGamepadButtonRightBottom, true
	case "B", "CIRCLE":
		return ebiten.StandardGamepadButtonRightRight, true
	case "X", "SQUARE":
		return ebiten.StandardGamepadButtonRightLeft, true
	case "Y", "TRIANGLE":
		return ebiten.StandardGamepadButtonRightTop, true
	case "L", "L1", "LB":
		return ebiten.StandardGamepadButtonFrontTopLeft, true
	case "R", "R1", "RB":
		return ebiten.StandardGamepadButtonFrontTopRight, true
	case "L2", "LT", "ZL":
		return ebiten.StandardGamepadButtonFrontBottomLeft, true
	case "R2", "RT", "ZR":
		return ebiten.StandardGamepadButtonFrontBottomRight, true
	case "SELECT", "BACK", "SHARE", "MINUS":
		return ebiten.StandardGamepadButtonCenterLeft, true
	case "START", "OPTIONS", "PLUS":
		return ebiten.StandardGamepadButtonCenterRight, true
	case "HOME", "GUIDE", "PS":
		return ebiten.StandardGamepadButtonCenterCenter, true
	case "L3", "LEFTSTICK":
		return ebiten.StandardGamepadButtonLeftStick, true
	case "R3", "RIGHTSTICK":
		return ebiten.StandardGamepadButtonRightStick, true
	case "UP", "DPADUP":
		return ebiten.StandardGamepadButtonLeftTop, true
	case "DOWN", "DPADDOWN":
		return ebiten.StandardGamepadButtonLeftBottom, true
	case "LEFT", "DPADLEFT":
		return ebiten.StandardGamepadButtonLeftLeft, true
	case "RIGHT", "DPADRIGHT":
		return ebiten.StandardGamepadButtonLeftRight, true
	default:
		return 0, false
	}
}

// standardGamepadButtonName is the reverse of gamepadButtonByName,
// used by the settings screen (settings.go) to store a captured
// gamepad button press as a config-storable name. Returns the
// canonical (non-alias) name for each button, e.g. "A" not "CROSS" —
// both parse back to the same button via gamepadButtonByName, so
// which one is stored is a display/persistence choice, not a
// correctness one.
func standardGamepadButtonName(b ebiten.StandardGamepadButton) (string, bool) {
	switch b {
	case ebiten.StandardGamepadButtonRightBottom:
		return "A", true
	case ebiten.StandardGamepadButtonRightRight:
		return "B", true
	case ebiten.StandardGamepadButtonRightLeft:
		return "X", true
	case ebiten.StandardGamepadButtonRightTop:
		return "Y", true
	case ebiten.StandardGamepadButtonFrontTopLeft:
		return "L1", true
	case ebiten.StandardGamepadButtonFrontTopRight:
		return "R1", true
	case ebiten.StandardGamepadButtonFrontBottomLeft:
		return "L2", true
	case ebiten.StandardGamepadButtonFrontBottomRight:
		return "R2", true
	case ebiten.StandardGamepadButtonCenterLeft:
		return "SELECT", true
	case ebiten.StandardGamepadButtonCenterRight:
		return "START", true
	case ebiten.StandardGamepadButtonCenterCenter:
		return "HOME", true
	case ebiten.StandardGamepadButtonLeftStick:
		return "L3", true
	case ebiten.StandardGamepadButtonRightStick:
		return "R3", true
	case ebiten.StandardGamepadButtonLeftTop:
		return "UP", true
	case ebiten.StandardGamepadButtonLeftBottom:
		return "DOWN", true
	case ebiten.StandardGamepadButtonLeftLeft:
		return "LEFT", true
	case ebiten.StandardGamepadButtonLeftRight:
		return "RIGHT", true
	default:
		return "", false
	}
}

// ---------------------------------------------------------------------
// Per-player input polling
// ---------------------------------------------------------------------

// binding is the resolved (not string) form of one uiface.Button,
// computed once per SystemInfo rather than re-parsed every Update.
type binding struct {
	id      int
	key     ebiten.Key
	hasKey  bool
	pad     ebiten.StandardGamepadButton
	hasPad  bool
}

// inputPoller owns resolved keybindings and per-player previous-state
// for edge-agnostic (level-triggered) polling. It lives entirely on
// the UI thread; it only ever calls manager.sendInput / sendOption,
// never touches uiface.Emulator directly (see the optionEvent comment
// in manager.go for why that boundary is load-bearing, not stylistic).
type inputPoller struct {
	bindings  []binding
	overrides map[playerButton]binding
	players   int

	// prevState avoids re-sending an unchanged bitmask every single UI
	// frame (which could be 60-240Hz depending on display) when the
	// core only ticks at ~60Hz — cuts channel traffic without changing
	// behavior, since coreThread applies whatever the latest queued
	// value is regardless of how often it's resent.
	prevState [maxPlayers]uint32

	prevPointer [maxPlayers]pointerState
}

const maxPlayers = 8

type pointerState struct {
	x, y    int
	trigger bool
	valid   bool
}

func newInputPoller(info uiface.SystemInfo, overrides []ui.KeyBindOverride) *inputPoller {
	p := &inputPoller{players: info.Players}
	if p.players > maxPlayers {
		p.players = maxPlayers
	}
	for _, b := range info.Buttons {
		rb := binding{id: b.ID}
		if k, ok := keyByName(b.DefaultKey); ok {
			rb.key, rb.hasKey = k, true
		}
		if pb, ok := gamepadButtonByName(b.DefaultPad); ok {
			rb.pad, rb.hasPad = pb, true
		}
		p.bindings = append(p.bindings, rb)
	}

	// Overrides are keyed by (player, buttonID). player is carried on
	// the override, not the binding, because a given button ID can be
	// rebound differently per player (e.g. player 2 prefers WASD to
	// player 1's arrow keys on a shared keyboard). To support that
	// without duplicating the whole binding table per player, we keep
	// per-player override maps layered on top of the shared default
	// bindings at poll time (see inputPoller.effectiveBinding).
	p.setOverrides(overrides, p.bindings)
	return p
}

// setOverrides replaces the override table wholesale, letting the
// settings screen (settings.go) apply a rebind immediately without
// tearing down and reconstructing the poller (which would lose the
// prevState/prevPointer edge-detection history mid-session).
func (p *inputPoller) setOverrides(overrides []ui.KeyBindOverride, defaults []binding) {
	p.overrides = make(map[playerButton]binding, len(overrides))
	for _, ov := range overrides {
		key := playerButton{player: ov.Player, buttonID: ov.ButtonID}
		eb := binding{id: ov.ButtonID}
		for _, b := range defaults {
			if b.id == ov.ButtonID {
				eb = b
				break
			}
		}
		if ov.Key != "" {
			if k, ok := keyByName(ov.Key); ok {
				eb.key, eb.hasKey = k, true
			}
		}
		if ov.Pad != "" {
			if pb, ok := gamepadButtonByName(ov.Pad); ok {
				eb.pad, eb.hasPad = pb, true
			}
		}
		p.overrides[key] = eb
	}
}

type playerButton struct {
	player   int
	buttonID int
}

// effectiveBinding returns the override for (player, id) if one
// exists, else the shared default binding.
func (p *inputPoller) effectiveBinding(player int, def binding) binding {
	if ov, ok := p.overrides[playerButton{player: player, buttonID: def.id}]; ok {
		return ov
	}
	return def
}

// poll reads the current keyboard/gamepad state and forwards changed
// button masks / pointer state to m via non-blocking sends. Called
// once per ebiten Update() (UI thread only).
//
// Player 0 combines keyboard OR the first connected standard gamepad.
// Players 1..N-1 each map to the gamepad at that connection index, if
// one is present and standard-layout-mapped; unmapped extra players
// simply report no input, matching how every console UI handles an
// unplugged controller port.
func (p *inputPoller) poll(m *manager, dest destRect, srcW, srcH int) {
	var gamepadIDs []ebiten.GamepadID
	gamepadIDs = ebiten.AppendGamepadIDs(gamepadIDs[:0])

	for player := 0; player < p.players; player++ {
		var mask uint32
		for _, def := range p.bindings {
			b := p.effectiveBinding(player, def)
			pressed := false
			if player == 0 && b.hasKey && ebiten.IsKeyPressed(b.key) {
				pressed = true
			}
			if !pressed && b.hasPad {
				gid := gamepadIndexForPlayer(gamepadIDs, player)
				if gid >= 0 {
					id := gamepadIDs[gid]
					if ebiten.IsStandardGamepadLayoutAvailable(id) &&
						ebiten.IsStandardGamepadButtonPressed(id, b.pad) {
						pressed = true
					}
				}
			}
			if pressed {
				mask |= 1 << uint(b.id)
			}
		}
		if mask != p.prevState[player] {
			p.prevState[player] = mask
			m.sendInput(inputEvent{kind: inputButtons, player: player, buttons: mask})
		}
	}

	// Lightgun/mouse pointer: player 0 only, driven by the OS cursor,
	// transformed from window pixels into source-frame coordinates
	// using the same destRect Draw last computed. Saturn peripherals
	// (Virtua Gun, mouse) are single-port devices in practice, so this
	// intentionally does not extend to other players.
	if srcW > 0 && srcH > 0 && dest.w > 0 && dest.h > 0 {
		mx, my := ebiten.CursorPosition()
		relX := float64(mx-dest.x) / float64(dest.w)
		relY := float64(my-dest.y) / float64(dest.h)
		inBounds := relX >= 0 && relX <= 1 && relY >= 0 && relY <= 1
		sx := int(relX * float64(srcW))
		sy := int(relY * float64(srcH))
		trigger := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)

		cur := pointerState{x: sx, y: sy, trigger: trigger, valid: inBounds}
		if cur != p.prevPointer[0] {
			p.prevPointer[0] = cur
			// Off-screen coordinates are still forwarded with
			// trigger=false and clamped coordinates when out of
			// bounds, matching real lightgun "off-screen" behavior
			// (SMPC lightgun handling treats this as no-hit, not as
			// stale coordinates).
			ex, ey := sx, sy
			tr := trigger
			if !inBounds {
				ex, ey = clamp(sx, 0, srcW-1), clamp(sy, 0, srcH-1)
				tr = false
			}
			m.sendInput(inputEvent{kind: inputPointer, player: 0, x: ex, y: ey, trigger: tr})
		}
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// gamepadIndexForPlayer returns the index into ids for the given
// player slot, or -1 if no gamepad is connected at that slot.
// Connection order is stable within a session (ebiten preserves
// GamepadID assignment order), so player-to-controller assignment
// does not shuffle when an unrelated device is plugged/unplugged
// elsewhere on the system.
func gamepadIndexForPlayer(ids []ebiten.GamepadID, player int) int {
	if player < 0 || player >= len(ids) {
		return -1
	}
	return player
}
