// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package ebiten

import (
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/jkind73/cassini/config"
)

// calibChannel describes one logical analog channel needing
// calibration for a given accessory type.
type calibChannel struct {
	name          string // config.Calibration's per-device channel key: "ax","ay","ar","al","az"
	label         string
	bidirectional bool // true: stick-style, needs rest+min+max. false: trigger-style, needs rest+max only.
}

// channelsFor lists exactly the channels each analog accessory type
// needs, matching smpc.go.patch.txt's data shapes precisely -- 3D
// Control Pad has AX/AY/AR/AL (ID 0x16), Mission Stick has AX/AY/AZ
// (0x15), Racing Controller has AX only (0x13).
var channelsFor = map[string][]calibChannel{
	"3dpad":        {{"ax", "Stick X", true}, {"ay", "Stick Y", true}, {"ar", "Right Trigger", false}, {"al", "Left Trigger", false}},
	"missionstick": {{"ax", "Stick X", true}, {"ay", "Stick Y", true}, {"az", "Throttle", false}},
	"racing":       {{"ax", "Steering", true}},
}

type calibStep int

const (
	calibStepRest calibStep = iota
	calibStepMax
	calibStepMin // skipped for unidirectional (trigger-style) channels
	calibStepConfirmed
)

// calibrationWizard walks the person through explicit rest/min/max
// sampling -- never guesses an axis index or a resting value; every
// number in the resulting config.AxisCalibration was directly sampled
// from the device while the person performed the requested physical
// action.
type calibrationWizard struct {
	deviceSDLID string
	gamepadID   ebiten.GamepadID
	channels    []calibChannel
	channelIdx  int
	step        calibStep

	restSnapshot []float64 // full raw-axis vector sampled at calibStepRest, for delta-based axis detection
	detectedAxis int
	restRaw      float64
	maxRaw       float64

	result    map[string]config.AxisCalibration
	failed    bool
	statusMsg string

	enterDown, escDown bool
}

func newCalibrationWizard(deviceSDLID string, gamepadID ebiten.GamepadID, accessoryType string) *calibrationWizard {
	w := &calibrationWizard{
		deviceSDLID: deviceSDLID,
		gamepadID:   gamepadID,
		channels:    channelsFor[accessoryType],
		result:      map[string]config.AxisCalibration{},
	}
	w.beginChannel()
	return w
}

func (w *calibrationWizard) currentChannel() calibChannel {
	return w.channels[w.channelIdx]
}

func (w *calibrationWizard) beginChannel() {
	w.step = calibStepRest
	ch := w.currentChannel()
	w.statusMsg = fmt.Sprintf("[%s] Release/center it, then press Enter", ch.label)
}

func (w *calibrationWizard) done() bool {
	return w.channelIdx >= len(w.channels)
}

// sampleAxes reads every raw axis on the device -- this, not
// StandardGamepadAxisValue, is the layer that lets calibration find
// an axis ebiten's normalized "standard layout" doesn't expose at
// all (analog triggers, in particular).
func sampleAxes(id ebiten.GamepadID) []float64 {
	n := ebiten.GamepadAxisCount(id)
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		out[i] = ebiten.GamepadAxisValue(id, ebiten.GamepadAxisType(i))
	}
	return out
}

// detectMovedAxis compares two raw-axis snapshots and returns the
// index of the axis with the largest movement, requiring it exceed a
// noise threshold so ordinary analog stick jitter at rest doesn't
// register as a false detection. Returns ok=false if nothing moved
// enough to be confident -- the wizard surfaces this as "no movement
// detected, try again" rather than guessing.
func detectMovedAxis(before, after []float64) (idx int, delta float64, ok bool) {
	const noiseThreshold = 0.15
	best := -1
	bestDelta := 0.0
	for i := 0; i < len(before) && i < len(after); i++ {
		d := after[i] - before[i]
		ad := d
		if ad < 0 {
			ad = -ad
		}
		if ad > bestDelta {
			bestDelta = ad
			best = i
			delta = d
		}
	}
	if best < 0 || bestDelta < noiseThreshold {
		return 0, 0, false
	}
	return best, delta, true
}

func (w *calibrationWizard) update() {
	esc := ebiten.IsKeyPressed(ebiten.KeyEscape)
	escJust := esc && !w.escDown
	w.escDown = esc
	if escJust {
		w.failed = true
		return
	}

	enter := ebiten.IsKeyPressed(ebiten.KeyEnter) || ebiten.IsKeyPressed(ebiten.KeySpace)
	enterJust := enter && !w.enterDown
	w.enterDown = enter
	if !enterJust {
		return
	}

	ch := w.currentChannel()

	switch w.step {
	case calibStepRest:
		w.restSnapshot = sampleAxes(w.gamepadID)
		w.step = calibStepMax
		w.statusMsg = fmt.Sprintf("[%s] Move it to full %s, hold, then press Enter", ch.label, maxDirectionLabel(ch))

	case calibStepMax:
		after := sampleAxes(w.gamepadID)
		axis, _, ok := detectMovedAxis(w.restSnapshot, after)
		if !ok {
			w.statusMsg = fmt.Sprintf("[%s] No movement detected on any axis -- try again, move it further", ch.label)
			return
		}
		w.detectedAxis = axis
		w.restRaw = w.restSnapshot[axis]
		w.maxRaw = after[axis]

		if !ch.bidirectional {
			w.finishChannel(w.restRaw, w.restRaw, w.maxRaw)
			return
		}
		w.step = calibStepMin
		w.statusMsg = fmt.Sprintf("[%s] Now move it to the opposite extreme, hold, then press Enter", ch.label)

	case calibStepMin:
		after := sampleAxes(w.gamepadID)
		if w.detectedAxis >= len(after) {
			w.statusMsg = "Device axis count changed mid-calibration -- press Esc and retry"
			return
		}
		minRaw := after[w.detectedAxis]
		w.finishChannel(minRaw, w.restRaw, w.maxRaw)
	}
}

// finishChannel records the calibrated channel and advances to the
// next one, or marks the wizard done.
func (w *calibrationWizard) finishChannel(minRaw, restRaw, maxRaw float64) {
	ch := w.currentChannel()
	w.result[ch.name] = config.AxisCalibration{
		RawAxis: w.detectedAxis,
		Rest:    restRaw,
		Min:     minRaw,
		Max:     maxRaw,
	}
	w.channelIdx++
	if w.done() {
		w.statusMsg = "Calibration complete -- press Enter to save, Esc to discard"
		w.step = calibStepConfirmed
		return
	}
	w.beginChannel()
}

func maxDirectionLabel(ch calibChannel) string {
	if ch.bidirectional {
		return "one extreme"
	}
	return "press"
}

// mapToByte converts a raw axis reading to the Saturn 0x00-0xFF wire
// value using an explicit calibration -- linear interpolation only.
// This function lives entirely in the host input layer and produces a
// single discrete uint8 per call, which is all the emulated
// peripheral (smpc.go.patch.txt) ever receives or acts on -- the core
// never sees anything but this final byte, never smooths or
// interpolates it further.
//
// Center/rest maps to 0x80 for a bidirectional (stick) channel, 0x00
// for a unidirectional (trigger) channel (Min==Rest signals
// unidirectional, set by finishChannel above); full positive throw
// maps to 0xFF, full negative throw to 0x00, matching the documented
// resting/full-throw values exactly.
func mapToByte(raw float64, cal config.AxisCalibration) uint8 {
	if cal.Min == cal.Rest {
		// Unidirectional (trigger-style).
		span := cal.Max - cal.Rest
		if span == 0 {
			return 0x00
		}
		t := (raw - cal.Rest) / span
		t = clamp01(t)
		return uint8(t*255 + 0.5)
	}

	// Bidirectional (stick-style): positive and negative throws are
	// calibrated (and therefore scaled) independently, since a real
	// analog stick's mechanical range is not always symmetric.
	if raw >= cal.Rest {
		span := cal.Max - cal.Rest
		if span == 0 {
			return 0x80
		}
		t := clamp01((raw - cal.Rest) / span)
		return uint8(128 + t*127 + 0.5)
	}
	span := cal.Rest - cal.Min
	if span == 0 {
		return 0x80
	}
	t := clamp01((cal.Rest - raw) / span)
	return uint8(128 - t*128 + 0.5)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// draw renders the wizard's current prompt. Deliberately plain-text
// only (no progress bar/graphic) -- the wizard is a rare,
// short-lived flow, and text keeps it trivial to read exactly what
// state it's in without adding UI chrome this codebase would then
// need to maintain.
func (w *calibrationWizard) draw(screen *ebiten.Image) {
	titleOp := &text.DrawOptions{}
	titleOp.GeoM.Translate(20, 20)
	titleOp.ColorScale.ScaleWithColor(color.RGBA{0xe0, 0xe0, 0xe0, 0xff})
	text.Draw(screen, "Axis Calibration  (Esc: cancel)", browserFace, titleOp)

	if !w.done() {
		progOp := &text.DrawOptions{}
		progOp.GeoM.Translate(20, 45)
		progOp.ColorScale.ScaleWithColor(color.RGBA{0x90, 0x90, 0x90, 0xff})
		text.Draw(screen, fmt.Sprintf("Channel %d of %d", w.channelIdx+1, len(w.channels)), browserFace, progOp)
	}

	statusOp := &text.DrawOptions{}
	statusOp.GeoM.Translate(20, 75)
	statusOp.ColorScale.ScaleWithColor(color.RGBA{0x4a, 0x9e, 0xff, 0xff})
	text.Draw(screen, w.statusMsg, browserFace, statusOp)
}
