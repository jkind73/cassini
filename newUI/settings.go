// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package ebiten

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/jkind73/cassini/config"
	"github.com/jkind73/cassini/ui"
	"github.com/jkind73/cassini/uiface"
)

// settingsTab selects which panel is shown. Two tabs, matching
// exactly the two accessories with real backend support today (see
// the item 8 discussion in project history: 3D pad, mouse, and RAM
// cart size all lack any core implementation to configure).
type settingsTab int

const (
	tabInput settingsTab = iota
	tabLightgun
	tabAccessory
	numTabs
)

func (t settingsTab) String() string {
	switch t {
	case tabInput:
		return "Input"
	case tabLightgun:
		return "Lightgun"
	case tabAccessory:
		return "Accessory"
	default:
		return "?"
	}
}

// accessoryTypes lists every accessory type in picker order, matching
// config.PortAccessory.Type's valid values exactly (see config.go's
// normalize function). "pad" first as the always-available default.
var accessoryTypes = []string{"pad", "3dpad", "missionstick", "racing", "mouse", "keyboard", "md3", "md6"}

func accessoryLabel(t string) string {
	switch t {
	case "pad":
		return "Standard Pad"
	case "3dpad":
		return "3D Control Pad"
	case "missionstick":
		return "Mission Stick"
	case "racing":
		return "Racing Controller"
	case "mouse":
		return "Saturn Mouse"
	case "keyboard":
		return "Saturn Keyboard"
	case "md3":
		return "Mega Drive 3-Button"
	case "md6":
		return "Mega Drive 6-Button"
	default:
		return t
	}
}

type settingsState struct {
	// poller is the currently-active game's inputPoller, so keyboard/
	// gamepad rebinds (setOverride) take effect immediately if a
	// session is running. nil when settings is opened from the
	// browser with no session active -- rebinds still accumulate in
	// s.overrides and apply to the next session via
	// game.startSession's newInputPoller construction either way.
	poller *inputPoller

	tab            settingsTab
	buttons        []uiface.Button
	player         int
	selectedButton int
	capturing      bool // true while waiting for the next key/gamepad press to bind

	// overrides accumulates changes made during this settings-screen
	// visit, keyed the same way inputPoller keys them. Seeded from the
	// caller's current overrides on entry (see newSettingsState), so
	// opening settings and leaving without changing anything
	// round-trips to the same state, not a reset to defaults.
	overrides map[playerButton]ui.KeyBindOverride

	lightgunEnabled bool
	lightgunPort    string // "1" or "2"

	accessoryPort  int // 0 or 1 -- which port the accessory tab is currently editing
	port1Accessory config.PortAccessory
	port2Accessory config.PortAccessory

	calibration config.Calibration // device SDL ID -> channel -> AxisCalibration, edited in place by calibWizard on confirm
	calibWizard *calibrationWizard // non-nil while a calibration is in progress; update()/draw() delegate entirely to it

	changed bool // whether anything was modified this visit -- gates whether OnSettingsChanged fires at all

	tabLeftDown, tabRightDown, upDown, downDown, playerDown, enterDown, escDown bool
	calibKeyDown                                                               bool
}

func newSettingsState(poller *inputPoller, info uiface.SystemInfo, currentOverrides []ui.KeyBindOverride, lightgunEnabled bool, lightgunPort string, port1, port2 config.PortAccessory, calibration config.Calibration) *settingsState {
	s := &settingsState{
		poller:          poller,
		buttons:         info.Buttons,
		overrides:       make(map[playerButton]ui.KeyBindOverride, len(currentOverrides)),
		lightgunEnabled: lightgunEnabled,
		lightgunPort:    lightgunPort,
		port1Accessory:  port1,
		port2Accessory:  port2,
		calibration:     cloneCalibration(calibration),
	}
	for _, ov := range currentOverrides {
		s.overrides[playerButton{player: ov.Player, buttonID: ov.ButtonID}] = ov
	}
	return s
}

func cloneCalibration(c config.Calibration) config.Calibration {
	out := config.Calibration{}
	for dev, channels := range c {
		out[dev] = map[string]config.AxisCalibration{}
		for ch, cal := range channels {
			out[dev][ch] = cal
		}
	}
	return out
}

// snapshot returns the accumulated state as a ui.SettingsSnapshot for
// OnSettingsChanged.
func (s *settingsState) snapshot() ui.SettingsSnapshot {
	out := ui.SettingsSnapshot{
		LightgunEnabled: s.lightgunEnabled,
		LightgunPort:    s.lightgunPort,
		Port1Accessory:  ui.PortAccessory{Type: s.port1Accessory.Type, KeyboardLayout: s.port1Accessory.KeyboardLayout},
		Port2Accessory:  ui.PortAccessory{Type: s.port2Accessory.Type, KeyboardLayout: s.port2Accessory.KeyboardLayout},
		Calibration:     fromConfigCalibration(s.calibration),
	}
	for _, ov := range s.overrides {
		out.KeyBindOverrides = append(out.KeyBindOverrides, ov)
	}
	return out
}

func (s *settingsState) update(g *game) error {
	if s.calibWizard != nil {
		s.calibWizard.update()
		if s.calibWizard.failed {
			s.calibWizard = nil // discard, no changes applied
			return nil
		}
		if s.calibWizard.step == calibStepConfirmed && (ebiten.IsKeyPressed(ebiten.KeyEnter) || ebiten.IsKeyPressed(ebiten.KeySpace)) {
			if s.calibration[s.calibWizard.deviceSDLID] == nil {
				s.calibration[s.calibWizard.deviceSDLID] = map[string]config.AxisCalibration{}
			}
			for ch, cal := range s.calibWizard.result {
				s.calibration[s.calibWizard.deviceSDLID][ch] = cal
			}
			s.changed = true
			s.calibWizard = nil
		}
		return nil
	}

	escJustPressed := ebiten.IsKeyPressed(ebiten.KeyEscape) && !s.escDown
	s.escDown = ebiten.IsKeyPressed(ebiten.KeyEscape)

	if s.capturing {
		s.pollCapture()
		if escJustPressed {
			s.capturing = false // cancel capture without changing the binding
		}
		return nil
	}

	if escJustPressed {
		g.exitSettings()
		return nil
	}

	left := ebiten.IsKeyPressed(ebiten.KeyArrowLeft) || ebiten.IsKeyPressed(ebiten.KeyQ)
	right := ebiten.IsKeyPressed(ebiten.KeyArrowRight) || ebiten.IsKeyPressed(ebiten.KeyE)
	if right && !s.tabRightDown {
		s.tab = (s.tab + 1) % numTabs
	}
	if left && !s.tabLeftDown {
		s.tab = (s.tab - 1 + numTabs) % numTabs
	}
	s.tabLeftDown, s.tabRightDown = left, right

	switch s.tab {
	case tabInput:
		s.updateInputTab()
	case tabLightgun:
		s.updateLightgunTab()
	case tabAccessory:
		s.updateAccessoryTab()
	}
	return nil
}

func (s *settingsState) updateInputTab() {
	up := ebiten.IsKeyPressed(ebiten.KeyArrowUp) || ebiten.IsKeyPressed(ebiten.KeyW)
	down := ebiten.IsKeyPressed(ebiten.KeyArrowDown) || ebiten.IsKeyPressed(ebiten.KeyS)
	if up && !s.upDown && s.selectedButton > 0 {
		s.selectedButton--
	}
	if down && !s.downDown && s.selectedButton < len(s.buttons)-1 {
		s.selectedButton++
	}
	s.upDown, s.downDown = up, down

	playerKey := ebiten.IsKeyPressed(ebiten.KeyTab)
	if playerKey && !s.playerDown {
		s.player = (s.player + 1) % maxPlayers
	}
	s.playerDown = playerKey

	enter := ebiten.IsKeyPressed(ebiten.KeyEnter) || ebiten.IsKeyPressed(ebiten.KeySpace)
	if enter && !s.enterDown && len(s.buttons) > 0 {
		s.capturing = true
	}
	s.enterDown = enter
}

func (s *settingsState) updateLightgunTab() {
	enter := ebiten.IsKeyPressed(ebiten.KeyEnter) || ebiten.IsKeyPressed(ebiten.KeySpace)
	if enter && !s.enterDown {
		s.lightgunEnabled = !s.lightgunEnabled
		s.changed = true
	}
	s.enterDown = enter

	if s.lightgunEnabled {
		up := ebiten.IsKeyPressed(ebiten.KeyArrowUp) || ebiten.IsKeyPressed(ebiten.KeyW)
		down := ebiten.IsKeyPressed(ebiten.KeyArrowDown) || ebiten.IsKeyPressed(ebiten.KeyS)
		if (up && !s.upDown) || (down && !s.downDown) {
			if s.lightgunPort == "1" {
				s.lightgunPort = "2"
			} else {
				s.lightgunPort = "1"
			}
			s.changed = true
		}
		s.upDown, s.downDown = up, down
	}
}

// pollCapture checks for any newly-pressed key or standard gamepad
// button and, if found, binds it to the currently-selected button for
// the current player, then leaves capture mode. Uses
// inpututil.AppendJustPressedKeys/AppendJustPressedStandardGamepadButtons
// (verified against real ebiten source during this session) rather
// than scanning a fixed key list, so it can detect any key ebiten
// recognizes, not just the ones in input.go's curated keyByNameTable
// -- though only keys that ARE in that table can be converted back to
// a storable name, so an unrecognized key press is silently ignored
// rather than bound (matches this codebase's established pattern of
// not guessing at data it can't correctly represent).
func (s *settingsState) pollCapture() {
	if len(s.buttons) == 0 {
		return
	}
	buttonID := s.buttons[s.selectedButton].ID

	var pressed []ebiten.Key
	pressed = inpututil.AppendJustPressedKeys(pressed)
	for _, k := range pressed {
		if k == ebiten.KeyEscape {
			continue // handled as "cancel capture" by update(), not a bindable key
		}
		if name, ok := keyName(k); ok {
			s.setOverride(buttonID, name, "")
			s.capturing = false
			return
		}
	}

	var ids []ebiten.GamepadID
	ids = ebiten.AppendGamepadIDs(ids)
	for _, id := range ids {
		var btns []ebiten.StandardGamepadButton
		btns = inpututil.AppendJustPressedStandardGamepadButtons(id, btns)
		if len(btns) > 0 {
			if name, ok := standardGamepadButtonName(btns[0]); ok {
				s.setOverride(buttonID, "", name)
				s.capturing = false
				return
			}
		}
	}
}

// setOverride records a capture result. Exactly one of keyName/padName
// is non-empty; the other side of the existing override for this
// (player, buttonID), if any, is preserved -- capturing a new
// keyboard key doesn't erase an existing gamepad binding for the same
// button, and vice versa.
func (s *settingsState) setOverride(buttonID int, keyName, padName string) {
	pb := playerButton{player: s.player, buttonID: buttonID}
	ov := s.overrides[pb]
	ov.Player, ov.ButtonID = s.player, buttonID
	if keyName != "" {
		ov.Key = keyName
	}
	if padName != "" {
		ov.Pad = padName
	}
	s.overrides[pb] = ov
	s.changed = true

	// Apply immediately, live, if a session is currently running.
	// inputPoller state is UI-thread-only and never read by the core
	// thread (unlike SetOption, which must always be queued through
	// m.opts -- see manager.go's optionEvent comment), so calling
	// setOverrides directly here is safe.
	if s.poller != nil {
		s.poller.setOverrides(s.overridesSlice(), s.poller.bindings)
	}
}

func (s *settingsState) overridesSlice() []ui.KeyBindOverride {
	out := make([]ui.KeyBindOverride, 0, len(s.overrides))
	for _, ov := range s.overrides {
		out = append(out, ov)
	}
	return out
}

func (s *settingsState) draw(screen *ebiten.Image, g *game) {
	screen.Fill(color.RGBA{0x14, 0x14, 0x18, 0xff})

	if s.calibWizard != nil {
		s.calibWizard.draw(screen)
		return
	}

	headerOp := &text.DrawOptions{}
	headerOp.GeoM.Translate(20, 20)
	headerOp.ColorScale.ScaleWithColor(color.RGBA{0xe0, 0xe0, 0xe0, 0xff})
	text.Draw(screen, "Settings  (Left/Right: tab, Esc: back)", browserFace, headerOp)

	for i := settingsTab(0); i < numTabs; i++ {
		tabOp := &text.DrawOptions{}
		tabOp.GeoM.Translate(20+float64(i)*100, 45)
		col := color.RGBA{0x90, 0x90, 0x90, 0xff}
		if i == s.tab {
			col = color.RGBA{0x4a, 0x9e, 0xff, 0xff}
		}
		tabOp.ColorScale.ScaleWithColor(col)
		text.Draw(screen, "["+i.String()+"]", browserFace, tabOp)
	}

	switch s.tab {
	case tabInput:
		s.drawInputTab(screen)
	case tabLightgun:
		s.drawLightgunTab(screen)
	case tabAccessory:
		s.drawAccessoryTab(screen)
	}
}

func (s *settingsState) drawInputTab(screen *ebiten.Image) {
	playerOp := &text.DrawOptions{}
	playerOp.GeoM.Translate(20, 75)
	playerOp.ColorScale.ScaleWithColor(color.RGBA{0xc0, 0xc0, 0xc0, 0xff})
	text.Draw(screen, "Player "+itoa(s.player+1)+"  (Tab: switch player, Enter: rebind)", browserFace, playerOp)

	for i, b := range s.buttons {
		y := 100 + i*20
		def := binding{id: b.ID}
		if k, ok := keyByName(b.DefaultKey); ok {
			def.key, def.hasKey = k, true
		}
		eff := def
		if ov, ok := s.overrides[playerButton{player: s.player, buttonID: b.ID}]; ok {
			if ov.Key != "" {
				if k, ok := keyByName(ov.Key); ok {
					eff.key, eff.hasKey = k, true
				}
			}
			if ov.Pad != "" {
				eff.hasPad = true
			}
		}

		label := b.Name + ": "
		if s.capturing && i == s.selectedButton {
			label += "press a key or button..."
		} else if eff.hasKey {
			if name, ok := keyName(eff.key); ok {
				label += name
			} else {
				label += "(bound)"
			}
		} else {
			label += "(unbound)"
		}

		rowOp := &text.DrawOptions{}
		rowOp.GeoM.Translate(20, float64(y))
		col := color.RGBA{0xe0, 0xe0, 0xe0, 0xff}
		if i == s.selectedButton {
			col = color.RGBA{0x4a, 0x9e, 0xff, 0xff}
		}
		rowOp.ColorScale.ScaleWithColor(col)
		text.Draw(screen, label, browserFace, rowOp)
	}
}

func (s *settingsState) drawLightgunTab(screen *ebiten.Image) {
	enabledOp := &text.DrawOptions{}
	enabledOp.GeoM.Translate(20, 80)
	enabledOp.ColorScale.ScaleWithColor(color.RGBA{0xe0, 0xe0, 0xe0, 0xff})
	state := "Off"
	if s.lightgunEnabled {
		state = "On"
	}
	text.Draw(screen, "Lightgun: "+state+"  (Enter: toggle)", browserFace, enabledOp)

	if s.lightgunEnabled {
		portOp := &text.DrawOptions{}
		portOp.GeoM.Translate(20, 105)
		portOp.ColorScale.ScaleWithColor(color.RGBA{0xc0, 0xc0, 0xc0, 0xff})
		text.Draw(screen, "Port: "+s.lightgunPort+"  (Up/Down: change port)", browserFace, portOp)
	}
}

func (s *settingsState) currentPortAccessory() *config.PortAccessory {
	if s.accessoryPort == 0 {
		return &s.port1Accessory
	}
	return &s.port2Accessory
}

func (s *settingsState) updateAccessoryTab() {
	portKey := ebiten.IsKeyPressed(ebiten.KeyTab)
	if portKey && !s.playerDown {
		s.accessoryPort = 1 - s.accessoryPort
	}
	s.playerDown = portKey

	up := ebiten.IsKeyPressed(ebiten.KeyArrowUp) || ebiten.IsKeyPressed(ebiten.KeyW)
	down := ebiten.IsKeyPressed(ebiten.KeyArrowDown) || ebiten.IsKeyPressed(ebiten.KeyS)
	acc := s.currentPortAccessory()
	idx := 0
	for i, t := range accessoryTypes {
		if t == acc.Type {
			idx = i
			break
		}
	}
	if up && !s.upDown {
		idx = (idx - 1 + len(accessoryTypes)) % len(accessoryTypes)
		acc.Type = accessoryTypes[idx]
		s.changed = true
	}
	if down && !s.downDown {
		idx = (idx + 1) % len(accessoryTypes)
		acc.Type = accessoryTypes[idx]
		s.changed = true
	}
	s.upDown, s.downDown = up, down

	if acc.Type == "keyboard" {
		left := ebiten.IsKeyPressed(ebiten.KeyArrowLeft) || ebiten.IsKeyPressed(ebiten.KeyQ)
		right := ebiten.IsKeyPressed(ebiten.KeyArrowRight) || ebiten.IsKeyPressed(ebiten.KeyE)
		// Note: Left/Right also drive tab switching in update()'s
		// shared handler above -- when the keyboard accessory is
		// selected, layout toggling additionally piggybacks on those
		// same keys, matching this codebase's existing pattern of
		// reusing Left/Right contextually per tab (see
		// updateLightgunTab's Up/Down reuse).
		if (left && !s.tabLeftDown) || (right && !s.tabRightDown) {
			if acc.KeyboardLayout == "jp" {
				acc.KeyboardLayout = "western"
			} else {
				acc.KeyboardLayout = "jp"
			}
			s.changed = true
		}
	}

	// Calibration entry point: only offered for accessory types with
	// analog channels (channelsFor, calibration.go), and only when a
	// standard-layout gamepad is actually connected -- calibration
	// needs a device to sample.
	if chans, ok := channelsFor[acc.Type]; ok && len(chans) > 0 {
		cKey := ebiten.IsKeyPressed(ebiten.KeyC)
		if cKey && !s.calibKeyDown {
			if id, gok := firstGamepadID(); gok {
				s.calibWizard = newCalibrationWizard(ebiten.GamepadSDLID(id), id, acc.Type)
			}
		}
		s.calibKeyDown = cKey
	} else {
		s.calibKeyDown = false
	}
}

func (s *settingsState) drawAccessoryTab(screen *ebiten.Image) {
	portOp := &text.DrawOptions{}
	portOp.GeoM.Translate(20, 75)
	portOp.ColorScale.ScaleWithColor(color.RGBA{0xc0, 0xc0, 0xc0, 0xff})
	text.Draw(screen, "Port "+itoa(s.accessoryPort+1)+"  (Tab: switch port, Up/Down: change accessory)", browserFace, portOp)

	acc := s.currentPortAccessory()
	for i, t := range accessoryTypes {
		y := 100 + i*20
		rowOp := &text.DrawOptions{}
		rowOp.GeoM.Translate(20, float64(y))
		col := color.RGBA{0xe0, 0xe0, 0xe0, 0xff}
		if t == acc.Type {
			col = color.RGBA{0x4a, 0x9e, 0xff, 0xff}
		}
		rowOp.ColorScale.ScaleWithColor(col)
		text.Draw(screen, accessoryLabel(t), browserFace, rowOp)
	}

	if acc.Type == "keyboard" {
		layoutOp := &text.DrawOptions{}
		layoutOp.GeoM.Translate(20, float64(100+len(accessoryTypes)*20+10))
		layoutOp.ColorScale.ScaleWithColor(color.RGBA{0xc0, 0xc0, 0xc0, 0xff})
		layout := "Western"
		if acc.KeyboardLayout == "jp" {
			layout = "Japanese"
		}
		text.Draw(screen, "Layout: "+layout+"  (Left/Right: change)", browserFace, layoutOp)
	}
}
