// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package ebiten

import "github.com/hajimehoshi/ebiten/v2"

type displayMode int

const (
	displayModeNormal displayMode = iota
	displayModeCRT
)

type aspectMode int

const (
	aspect4x3 aspectMode = iota
	aspect16x9
	aspectInteger
	aspectStretch
)

type upscalerFilter int

const (
	filterNearest upscalerFilter = iota
	filterBilinear
	filterBicubic
	filterHQ2x
	filterHQ4x
	filterXBRZ
)

// ebitenFilter maps to what ebiten can actually do natively, plus
// bicubic (implemented for real as a Kage shader, see bicubic.go).
// hq2x/hq4x/xBRZ remain a bilinear fallback -- not hand-waved, I
// actually checked: xBRZ's real reference source (Zenju's original,
// GPLv3, github.com/BradleyPudge/XBRZ mirror) is 1269 lines of
// per-pixel 4x4 neighborhood pattern classification, YUV-threshold
// edge detection, and corner-rounding blend-weight computation. A
// faithful port to a completely different execution model (Kage
// per-fragment GPU shader vs. the reference's per-source-pixel CPU
// block writer) within reasonable scope, with no way to render and
// visually diff the output against the reference implementation in
// this environment, risks shipping something that looks like edge
// smoothing but isn't actually xBRZ -- worse than the honest
// fallback this already was. A dedicated pass with real visual
// testing against reference images is the right way to do this
// properly; hq2x/hq4x carry the same reasoning (Stepin's original
// hqx has its own large hand-tuned pattern tables).
func (f upscalerFilter) ebitenFilter() ebiten.Filter {
	switch f {
	case filterNearest:
		return ebiten.FilterNearest
	default:
		return ebiten.FilterLinear
	}
}

// presentState is UI-thread-only display configuration. It is
// deliberately NOT part of manager/coreThread state: core's own
// displayProc (emulator.go) is never read back by RunFrame or
// GetFramebuffer, so mirroring it here — rather than querying it —
// keeps presentation entirely off the core thread. See the big
// comment on optionEvent in manager.go for the full reasoning.
type presentState struct {
	aspectMode   aspectMode
	shaderMode   displayMode
	filter       upscalerFilter
	crtIntensity float32
	crtBloom     float32
	crtCurvature bool
}

func newPresentState() *presentState {
	return &presentState{crtIntensity: 0.25}
}

// applyInitial seeds state from the same opts.Options map that gets
// forwarded to coreThread's initial SetOption calls, using identical
// value parsing to emulator.go's SetOption switch so the two never
// drift apart. Called once, before RunGame starts (single-threaded).
func (p *presentState) applyInitial(opts map[string]string) {
	for k, v := range opts {
		p.apply(k, v)
	}
}

// apply is also what a settings menu should call on the UI thread
// (paired with manager.sendOption for the core-thread copy — see the
// example in game.Update).
func (p *presentState) apply(key, value string) {
	switch key {
	case "aspect_ratio":
		switch value {
		case "4x3", "4:3":
			p.aspectMode = aspect4x3
		case "16x9", "16:9", "widescreen":
			p.aspectMode = aspect16x9
		case "integer":
			p.aspectMode = aspectInteger
		case "stretch":
			p.aspectMode = aspectStretch
		}
	case "display_shader":
		switch value {
		case "crt":
			p.shaderMode = displayModeCRT
		case "normal", "off", "none":
			p.shaderMode = displayModeNormal
		}
	case "upscaler_filter":
		switch value {
		case "bilinear":
			p.filter = filterBilinear
		case "bicubic":
			p.filter = filterBicubic
		case "hq2x":
			p.filter = filterHQ2x
		case "hq4x":
			p.filter = filterHQ4x
		case "xbrz":
			p.filter = filterXBRZ
		case "nearest", "off":
			p.filter = filterNearest
		}
	case "crt_bloom":
		if value == "true" || value == "on" {
			p.crtBloom = 0.5
		} else if value == "false" || value == "off" {
			p.crtBloom = 0.0
		}
	case "crt_curvature":
		p.crtCurvature = value == "true" || value == "1" || value == "on"
	}
}

type destRect struct {
	x, y, w, h int
}

// computeDestRect places the emulated frame inside the window per the
// active aspectMode. par is the pixel aspect ratio captured alongside
// the frame (frameSlot.par) — using the value from the same instant as
// the pixels avoids any mismatch during a resolution change, since a
// stale PAR queried later could pair the new frame's pixels with the
// previous mode's aspect for one Draw call.
//
// Pure UI-thread arithmetic — no core interaction, so window resizing
// (dragging the edge, fullscreen toggle, monitor DPI change) can never
// perturb emulation timing regardless of how often Layout/Draw fire.
func computeDestRect(srcW, srcH int, par float64, winW, winH int, mode aspectMode) destRect {
	if winW <= 0 || winH <= 0 {
		return destRect{}
	}
	if par <= 0 {
		par = 1
	}

	switch mode {
	case aspectStretch:
		return destRect{0, 0, winW, winH}

	case aspectInteger:
		dispW := float64(srcW) * par
		scale := 1
		for {
			w := float64(scale+1) * dispW
			h := float64(scale+1) * float64(srcH)
			if w > float64(winW) || h > float64(winH) {
				break
			}
			scale++
		}
		if scale < 1 {
			scale = 1
		}
		w := int(dispW) * scale
		h := srcH * scale
		return destRect{(winW - w) / 2, (winH - h) / 2, w, h}

	case aspect16x9:
		target := 16.0 / 9.0
		return fitAspect(winW, winH, target)

	default: // aspect4x3
		target := (float64(srcW) * par) / float64(srcH)
		return fitAspect(winW, winH, target)
	}
}

// fitAspect letterboxes/pillarboxes to fit targetAspect (w/h) inside
// the window, centered.
func fitAspect(winW, winH int, targetAspect float64) destRect {
	winAspect := float64(winW) / float64(winH)
	var w, h int
	if winAspect > targetAspect {
		h = winH
		w = int(float64(h) * targetAspect)
	} else {
		w = winW
		h = int(float64(w) / targetAspect)
	}
	return destRect{(winW - w) / 2, (winH - h) / 2, w, h}
}
