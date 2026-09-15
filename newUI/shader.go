// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package ebiten

// crtShaderSrc mirrors the scanline / aperture-grille / bloom math in
// core/display_shader.go's applyCRTShader almost 1:1, so this is a
// faithful GPU port rather than a different-looking reimplementation
// — same reasoning as cross-referencing a known-good reference impl
// elsewhere in this codebase (Ymir), just against our own CPU version.
//
// Running as a fragment shader means this costs effectively nothing on
// either the core thread (never touches it) or the CPU (GPU-resident,
// runs once per output pixel per UI frame — decoupled from the core's
// FPS entirely, since Draw is called at display refresh rate, not
// emulation rate).
const crtShaderSrc = `
package main

var Intensity float
var Bloom float
var Curvature float
var SrcSize vec2
var DstOrigin vec2
var DstSize vec2

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	uv := (dstPos.xy - DstOrigin) / DstSize

	if Curvature > 0.5 {
		// Mild barrel distortion around center, matching the "curved
		// glass" framing described for crt_curvature in emulator.go.
		c := uv*2.0 - 1.0
		r2 := dot(c, c)
		c *= 1.0 + 0.08*r2
		uv = c*0.5 + 0.5
		if uv.x < 0.0 || uv.x > 1.0 || uv.y < 0.0 || uv.y > 1.0 {
			return vec4(0, 0, 0, 1)
		}
	}

	texel := SrcSize * uv
	src := imageSrc0At(texel)

	// Scanline dimming on odd source scanlines — same parity test and
	// darken factor as applyCRTShader.
	darkenFactor := 1.0 - Intensity
	if int(texel.y)%2 != 0 {
		src.rgb *= darkenFactor
	}

	// Aperture grille: same per-column 0.85 attenuation pattern,
	// x%3 based on destination pixel to keep grille density tied to
	// display resolution rather than source resolution.
	col := int(dstPos.x) % 3
	if col == 0 {
		src.g *= 0.85
		src.b *= 0.85
	} else if col == 1 {
		src.r *= 0.85
		src.b *= 0.85
	} else {
		src.r *= 0.85
		src.g *= 0.85
	}

	// Phosphor bloom — same +20%*bloom additive, clamped.
	if Bloom > 0.0 {
		src.rgb += src.rgb * Bloom * 0.2
		src.rgb = min(src.rgb, vec3(1.0, 1.0, 1.0))
	}

	return src
}
`
