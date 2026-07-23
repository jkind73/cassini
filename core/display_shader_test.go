// Copyright 2026 The erings Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package core

import "testing"

func TestDisplayProcessorCRTShader(t *testing.T) {
	proc := NewDisplayProcessor()

	if proc.DisplayMode() != DisplayModeNormal {
		t.Errorf("default DisplayMode = %v, want DisplayModeNormal", proc.DisplayMode())
	}
	if proc.UpscalerFilter() != FilterNearest {
		t.Errorf("default UpscalerFilter = %v, want FilterNearest", proc.UpscalerFilter())
	}
	if proc.AspectRatioMode() != Aspect4x3 {
		t.Errorf("default AspectRatioMode = %v, want Aspect4x3", proc.AspectRatioMode())
	}

	proc.SetDisplayMode(DisplayModeCRT)
	proc.SetUpscalerFilter(FilterBilinear)
	proc.SetAspectRatioMode(AspectInteger)
	proc.SetCRTScanlineIntensity(0.5)

	if proc.DisplayMode() != DisplayModeCRT {
		t.Errorf("DisplayMode = %v, want DisplayModeCRT", proc.DisplayMode())
	}
	if proc.UpscalerFilter() != FilterBilinear {
		t.Errorf("UpscalerFilter = %v, want FilterBilinear", proc.UpscalerFilter())
	}
	if proc.AspectRatioMode() != AspectInteger {
		t.Errorf("AspectRatioMode = %v, want AspectInteger", proc.AspectRatioMode())
	}

	// Create 2x2 white RGBA framebuffer
	width, height := 2, 2
	stride := width * 4
	src := []byte{
		255, 255, 255, 255, 255, 255, 255, 255,
		255, 255, 255, 255, 255, 255, 255, 255,
	}

	dst := proc.ProcessFrame(src, width, height, stride)

	if len(dst) != len(src) {
		t.Fatalf("ProcessFrame returned slice len = %d, want %d", len(dst), len(src))
	}

	// Verify scanline dimming on y=1
	// Pixel (0,1): offset 8 -> red channel masked for subpixel (x=0 mask: g*0.85, b*0.85; y=1 scanline dimmed * 0.5)
	if dst[8] >= 255 {
		t.Errorf("Scanline pixel y=1 red channel = %d, want < 255 (dimmed)", dst[8])
	}
}

func TestEmulatorDisplayProcessorIntegration(t *testing.T) {
	e := NewEmulator()

	proc := e.DisplayProcessor()
	if proc == nil {
		t.Fatal("DisplayProcessor() returned nil")
	}

	fb := e.GetFramebuffer()
	if fb == nil {
		t.Fatal("GetFramebuffer() returned nil")
	}
}
