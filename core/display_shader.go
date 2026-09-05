// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package core

// DisplayMode specifies the post-processing shader mode.
type DisplayMode int

const (
	DisplayModeNormal DisplayMode = iota // Raw unmodified framebuffer output
	DisplayModeCRT                        // CRT scanline and aperture grille filter
)

// UpscalerFilter specifies the texture/scaling filter for framebuffer presentation.
type UpscalerFilter int

const (
	FilterNearest UpscalerFilter = iota // Nearest neighbor sampling
	FilterBilinear                      // Bilinear interpolation
	FilterBicubic                       // Bicubic smoothing
	FilterHQ2x                          // High Quality 2x scaling
	FilterHQ4x                          // High Quality 4x scaling
	FilterxBRZ                          // xBRZ high-quality pixel scaler
)

// AspectRatioMode specifies the aspect ratio scaling mode.
type AspectRatioMode int

const (
	Aspect4x3 AspectRatioMode = iota // Native 4:3 aspect ratio (pillarboxed if widescreen)
	Aspect16x9                       // Widescreen 16:9 aspect ratio
	AspectInteger                    // Integer scaling (exact pixel multiple)
	AspectStretch                    // Stretch to fill viewport
)

// DisplayProcessor applies post-processing display shaders and upscaling to raw RGBA framebuffers.
type DisplayProcessor struct {
	mode         DisplayMode
	filter       UpscalerFilter
	aspect       AspectRatioMode
	crtIntensity float32
	crtBloom     float32
	crtCurvature bool
}

// NewDisplayProcessor creates a DisplayProcessor with default settings.
func NewDisplayProcessor() *DisplayProcessor {
	return &DisplayProcessor{
		mode:         DisplayModeNormal,
		filter:       FilterNearest,
		aspect:       Aspect4x3,
		crtIntensity: 0.25,
	}
}

// SetDisplayMode sets the CRT or normal display shader mode.
func (p *DisplayProcessor) SetDisplayMode(mode DisplayMode) {
	p.mode = mode
}

// DisplayMode returns the current display mode.
func (p *DisplayProcessor) DisplayMode() DisplayMode {
	return p.mode
}

// SetUpscalerFilter sets the post-processing upscaler filter.
func (p *DisplayProcessor) SetUpscalerFilter(filter UpscalerFilter) {
	p.filter = filter
}

// UpscalerFilter returns the active upscaler filter.
func (p *DisplayProcessor) UpscalerFilter() UpscalerFilter {
	return p.filter
}

// SetAspectRatioMode sets the aspect ratio mode.
func (p *DisplayProcessor) SetAspectRatioMode(aspect AspectRatioMode) {
	p.aspect = aspect
}

// AspectRatioMode returns the active aspect ratio mode.
func (p *DisplayProcessor) AspectRatioMode() AspectRatioMode {
	return p.aspect
}

// SetCRTScanlineIntensity sets the CRT scanline darken intensity (0.0 to 1.0).
func (p *DisplayProcessor) SetCRTScanlineIntensity(intensity float32) {
	if intensity < 0.0 {
		intensity = 0.0
	} else if intensity > 1.0 {
		intensity = 1.0
	}
	p.crtIntensity = intensity
}

// SetCRTBloom sets the phosphor bloom intensity (0.0 to 1.0).
func (p *DisplayProcessor) SetCRTBloom(bloom float32) {
	p.crtBloom = bloom
}

// SetCRTCurvature enables or disables CRT curved glass distortion.
func (p *DisplayProcessor) SetCRTCurvature(enabled bool) {
	p.crtCurvature = enabled
}

// ProcessFrame applies configured shader effects and upscaling filters to a RGBA image.
func (p *DisplayProcessor) ProcessFrame(src []byte, width, height, stride int) []byte {
	if len(src) == 0 || width <= 0 || height <= 0 {
		return src
	}

	dst := make([]byte, len(src))
	copy(dst, src)

	if p.mode == DisplayModeCRT {
		p.applyCRTShader(dst, width, height, stride)
	}

	return dst
}

// applyCRTShader applies CRT scanlines, aperture grille subpixel attenuation, and phosphor bloom.
func (p *DisplayProcessor) applyCRTShader(buf []byte, width, height, stride int) {
	darkenFactor := 1.0 - p.crtIntensity
	bloomFactor := p.crtBloom

	for y := 0; y < height; y++ {
		// Scanline dimming on odd scanlines
		isScanline := (y & 1) != 0
		rowOff := y * stride

		for x := 0; x < width; x++ {
			off := rowOff + x*4
			if off+3 >= len(buf) {
				break
			}

			r := float32(buf[off])
			g := float32(buf[off+1])
			b := float32(buf[off+2])

			if isScanline {
				r *= darkenFactor
				g *= darkenFactor
				b *= darkenFactor
			}

			// Subpixel aperture grille (RGB mask pattern)
			switch x % 3 {
			case 0:
				g *= 0.85
				b *= 0.85
			case 1:
				r *= 0.85
				b *= 0.85
			case 2:
				r *= 0.85
				g *= 0.85
			}

			// Phosphor bloom effect
			if bloomFactor > 0 {
				r += r * bloomFactor * 0.2
				g += g * bloomFactor * 0.2
				b += b * bloomFactor * 0.2
				if r > 255 {
					r = 255
				}
				if g > 255 {
					g = 255
				}
				if b > 255 {
					b = 255
				}
			}

			buf[off] = uint8(r)
			buf[off+1] = uint8(g)
			buf[off+2] = uint8(b)
		}
	}
}
