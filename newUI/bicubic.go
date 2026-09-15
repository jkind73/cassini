// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package ebiten

// bicubicShaderSrc implements Catmull-Rom bicubic interpolation
// (a = -0.5, the standard Catmull-Rom parameterization), a real,
// well-defined 16-tap convolution filter distinct from bilinear --
// unlike hq2x/hq4x/xBRZ (see present.go's doc comment for why those
// remain a bilinear fallback), this is verifiable by hand from the
// standard cubic convolution formula rather than needing a rendered
// side-by-side comparison against a reference implementation, so it's
// implemented for real here.
//
// Weight function (separable, applied per axis):
//
//	w(x), 0<=|x|<1:  (a+2)|x|^3 - (a+3)|x|^2 + 1
//	w(x), 1<=|x|<2:  a|x|^3 - 5a|x|^2 + 8a|x| - 4a
//	w(x), |x|>=2:    0
//
// with a = -0.5. The 16 taps are unrolled explicitly rather than
// written as a loop -- Kage's exact for-loop semantics weren't worth
// risking here when the alternative (16 explicit lines) has zero
// syntax ambiguity and costs nothing at compile time.
const bicubicShaderSrc = `
package main

var SrcSize vec2
var DstOrigin vec2
var DstSize vec2

func cubicWeight(x float) float {
	a := -0.5
	ax := abs(x)
	if ax < 1.0 {
		return (a+2.0)*ax*ax*ax - (a+3.0)*ax*ax + 1.0
	}
	if ax < 2.0 {
		return a*ax*ax*ax - 5.0*a*ax*ax + 8.0*a*ax - 4.0*a
	}
	return 0.0
}

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	uv := (dstPos.xy - DstOrigin) / DstSize
	srcF := uv * SrcSize
	center := floor(srcF - 0.5) + 0.5
	frac := srcF - center

	wx0 := cubicWeight(frac.x + 1.0)
	wx1 := cubicWeight(frac.x)
	wx2 := cubicWeight(frac.x - 1.0)
	wx3 := cubicWeight(frac.x - 2.0)
	wy0 := cubicWeight(frac.y + 1.0)
	wy1 := cubicWeight(frac.y)
	wy2 := cubicWeight(frac.y - 1.0)
	wy3 := cubicWeight(frac.y - 2.0)

	p00 := imageSrc0At(center + vec2(-1.0, -1.0))
	p10 := imageSrc0At(center + vec2(0.0, -1.0))
	p20 := imageSrc0At(center + vec2(1.0, -1.0))
	p30 := imageSrc0At(center + vec2(2.0, -1.0))
	row0 := p00*wx0 + p10*wx1 + p20*wx2 + p30*wx3

	p01 := imageSrc0At(center + vec2(-1.0, 0.0))
	p11 := imageSrc0At(center + vec2(0.0, 0.0))
	p21 := imageSrc0At(center + vec2(1.0, 0.0))
	p31 := imageSrc0At(center + vec2(2.0, 0.0))
	row1 := p01*wx0 + p11*wx1 + p21*wx2 + p31*wx3

	p02 := imageSrc0At(center + vec2(-1.0, 1.0))
	p12 := imageSrc0At(center + vec2(0.0, 1.0))
	p22 := imageSrc0At(center + vec2(1.0, 1.0))
	p32 := imageSrc0At(center + vec2(2.0, 1.0))
	row2 := p02*wx0 + p12*wx1 + p22*wx2 + p32*wx3

	p03 := imageSrc0At(center + vec2(-1.0, 2.0))
	p13 := imageSrc0At(center + vec2(0.0, 2.0))
	p23 := imageSrc0At(center + vec2(1.0, 2.0))
	p33 := imageSrc0At(center + vec2(2.0, 2.0))
	row3 := p03*wx0 + p13*wx1 + p23*wx2 + p33*wx3

	result := row0*wy0 + row1*wy1 + row2*wy2 + row3*wy3
	return clamp(result, vec4(0.0), vec4(1.0))
}
`
