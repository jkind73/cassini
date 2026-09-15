// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package ebiten

import (
	"testing"

	"github.com/jkind73/cassini/config"
)

func TestMapToByte_Stick_Center(t *testing.T) {
	cal := config.AxisCalibration{Rest: 0.02, Min: -0.95, Max: 0.97} // realistic: slight drift, asymmetric range
	got := mapToByte(0.02, cal)
	if got != 0x80 {
		t.Errorf("rest should map to 0x80, got 0x%02X", got)
	}
}

func TestMapToByte_Stick_FullPositive(t *testing.T) {
	cal := config.AxisCalibration{Rest: 0.0, Min: -1.0, Max: 1.0}
	got := mapToByte(1.0, cal)
	if got != 0xFF {
		t.Errorf("full positive throw should map to 0xFF, got 0x%02X", got)
	}
}

func TestMapToByte_Stick_FullNegative(t *testing.T) {
	cal := config.AxisCalibration{Rest: 0.0, Min: -1.0, Max: 1.0}
	got := mapToByte(-1.0, cal)
	if got != 0x00 {
		t.Errorf("full negative throw should map to 0x00, got 0x%02X", got)
	}
}

func TestMapToByte_Stick_AsymmetricRange(t *testing.T) {
	// A real stick where the negative direction has a shorter
	// mechanical range than positive -- exactly why independent
	// min/max calibration matters instead of assuming symmetry.
	cal := config.AxisCalibration{Rest: 0.0, Min: -0.5, Max: 1.0}
	if got := mapToByte(-0.5, cal); got != 0x00 {
		t.Errorf("calibrated min should still map to 0x00 despite shorter range, got 0x%02X", got)
	}
	if got := mapToByte(-0.25, cal); got != 64 { // halfway to min -> halfway to 0
		t.Errorf("halfway to min should map to ~64, got %d", got)
	}
}

func TestMapToByte_Trigger_Released(t *testing.T) {
	cal := config.AxisCalibration{Rest: -1.0, Min: -1.0, Max: 1.0} // Min==Rest signals unidirectional
	got := mapToByte(-1.0, cal)
	if got != 0x00 {
		t.Errorf("released trigger should map to 0x00, got 0x%02X", got)
	}
}

func TestMapToByte_Trigger_FullyPressed(t *testing.T) {
	cal := config.AxisCalibration{Rest: -1.0, Min: -1.0, Max: 1.0}
	got := mapToByte(1.0, cal)
	if got != 0xFF {
		t.Errorf("fully pressed trigger should map to 0xFF, got 0x%02X", got)
	}
}

func TestMapToByte_Trigger_HalfPressed(t *testing.T) {
	cal := config.AxisCalibration{Rest: 0.0, Min: 0.0, Max: 1.0}
	got := mapToByte(0.5, cal)
	if got < 126 || got > 129 {
		t.Errorf("half-pressed trigger should map to ~127, got %d", got)
	}
}

func TestMapToByte_OutOfRange_Clamped(t *testing.T) {
	cal := config.AxisCalibration{Rest: 0.0, Min: -1.0, Max: 1.0}
	if got := mapToByte(1.5, cal); got != 0xFF { // beyond calibrated max, e.g. device drift
		t.Errorf("beyond-max should clamp to 0xFF, got 0x%02X", got)
	}
	if got := mapToByte(-1.5, cal); got != 0x00 {
		t.Errorf("beyond-min should clamp to 0x00, got 0x%02X", got)
	}
}

func TestDetectMovedAxis_FindsCorrectAxis(t *testing.T) {
	before := []float64{0.01, -0.02, 0.0, 0.03}
	after := []float64{0.01, -0.02, 0.9, 0.03} // axis 2 moved
	idx, _, ok := detectMovedAxis(before, after)
	if !ok || idx != 2 {
		t.Errorf("expected axis 2, got idx=%d ok=%v", idx, ok)
	}
}

func TestDetectMovedAxis_IgnoresNoise(t *testing.T) {
	before := []float64{0.0, 0.0}
	after := []float64{0.05, -0.03} // small jitter, below threshold
	_, _, ok := detectMovedAxis(before, after)
	if ok {
		t.Error("small jitter should not register as a detected movement")
	}
}
