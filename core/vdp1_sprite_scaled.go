// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package core

// startScaledSprite runs setup once (zoom-point resolution, dest
// rectangle, HSS state, gouraud table), then enters the chunked
// rasterizer. Iterator walks (dy, dx) over the dest rectangle and
// midpoint-samples the source.
func (v *VDP1) startScaledSprite(cmd *vdp1Command, budget int32) (consumed int32, done bool) {
	s := &v.spriteResume
	s.charAddr = uint32(cmd.srca) * 8
	s.charW = int((cmd.size>>8)&0x3F) * 8
	s.charH = int(cmd.size & 0xFF)
	s.colorMode = (cmd.pmod >> 3) & 0x07
	s.cc = cmd.pmod & 0x07
	s.ecdOff = cmd.pmod&0x0080 != 0
	s.spdOn = cmd.pmod&0x0040 != 0
	s.msbOn = cmd.pmod&0x8000 != 0
	s.mesh = cmd.pmod&0x0100 != 0
	s.userClip = (cmd.pmod >> 9) & 3
	s.flipH = cmd.ctrl&0x0010 != 0
	s.flipV = cmd.ctrl&0x0020 != 0
	hss := cmd.pmod&0x1000 != 0
	s.isScaled = true

	if s.charW == 0 || s.charH == 0 {
		return 0, true
	}

	zp := (cmd.ctrl >> 8) & 0x0F
	lx := int(v.localX)
	ly := int(v.localY)

	var dstX1, dstY1, dstX2, dstY2 int

	if zp == 0 {
		// Two-coordinate mode: A=upper-left, C=lower-right
		ax := int(int16(cmd.xa)) + lx
		ay := int(int16(cmd.ya)) + ly
		cx := int(int16(cmd.xc)) + lx
		cy := int(int16(cmd.yc)) + ly

		if ax <= cx {
			dstX1, dstX2 = ax, cx
		} else {
			dstX1, dstX2 = cx, ax
		}
		if ay <= cy {
			dstY1, dstY2 = ay, cy
		} else {
			dstY1, dstY2 = cy, ay
		}
	} else {
		ax := int(int16(cmd.xa)) + lx
		ay := int(int16(cmd.ya)) + ly
		dispW := int(int16(cmd.xb))
		dispH := int(int16(cmd.yb))

		// VDP1 Spec (Sec. 6.5): 0 width or height defaults to 1:1 character size
		if dispW == 0 {
			dispW = s.charW
		}
		if dispH == 0 {
			dispH = s.charH
		}

		if dispW < 0 || dispH < 0 {
			return 0, true
		}

		zpH := zp & 0x3
		zpV := (zp >> 2) & 0x3

		// Valid zpH: 1 (Left), 2 (Center), 3 (Right)
		// Valid zpV: 0 (Upper), 1 (Center), 2 (Lower)
		if zpH == 0 || zpV > 2 {
			return 0, true
		}

		switch zpH {
		case 1: // Left
			dstX1 = ax
			dstX2 = ax + dispW - 1
		case 2: // Center
			dstX1 = ax - dispW/2
			dstX2 = dstX1 + dispW - 1
		case 3: // Right
			dstX1 = ax - dispW + 1
			dstX2 = ax
		}

		switch zpV {
		case 0: // Upper / Top
			dstY1 = ay
			dstY2 = ay + dispH - 1
		case 1: // Center
			dstY1 = ay - dispH/2
			dstY2 = dstY1 + dispH - 1
		case 2: // Lower / Bottom
			dstY1 = ay - dispH + 1
			dstY2 = ay
		}
	}

	s.destW = dstX2 - dstX1 + 1
	s.destH = dstY2 - dstY1 + 1
	s.dstX1 = dstX1
	s.dstY1 = dstY1
	if s.destW <= 0 || s.destH <= 0 {
		return 0, true
	}

	// HSS subsamples the source by parity (per FBCR.EOS) only along the
	// horizontal (X) read direction of a line being shrunk; vertical
	// sampling is unaffected (manual Fig 6.5 retains every source row,
	// odd rows included). At 1:1 or enlarged, sampling is unmodified.
	// The end-code-disable side-effect rides on the X-shrink case.
	s.hssShrinkX = hss && s.destW < s.charW
	s.hssOddParity = v.fbcr&0x10 != 0
	s.hssEcdOff = s.hssShrinkX

	s.clipX, s.clipY = v.clipBounds()

	if cmd.pmod&0x0800 == 0 && preClipReject(dstX1, dstY1, dstX2, dstY2, s.clipX, s.clipY) {
		return vdp1PreClipLineCycles * int32(s.destH), true
	}

	if s.cc >= 4 {
		s.gt = v.readGouraudTable(cmd.grda)
	}

	s.outerIdx = 0 // dy
	s.innerIdx = 0 // dx
	s.endCodeCount = 0
	s.prevSrcX = -1

	// Effective flip for the rasterizer. In two-coordinate mode (zp==0)
	// a coordinate inversion (A.x > C.x or A.y > C.y) acts as an
	// additional flip, XOR'd with the CTRL DIR bits. In zoom-point mode
	// the rectangle is always left-to-right / top-to-bottom, so only the
	// CTRL DIR bits apply.
	s.effFlipH = s.flipH
	s.effFlipV = s.flipV
	if zp == 0 {
		ax := int(int16(cmd.xa)) + lx
		ay := int(int16(cmd.ya)) + ly
		cx := int(int16(cmd.xc)) + lx
		cy := int(int16(cmd.yc)) + ly
		if ax > cx {
			s.effFlipH = !s.effFlipH
		}
		if ay > cy {
			s.effFlipV = !s.effFlipV
		}
	}

	return v.runScaledSprite(budget)
}

func (v *VDP1) resumeScaledSprite(budget int32) (consumed int32, done bool) {
	return v.runScaledSprite(budget)
}

// runScaledSprite walks (dy, dx) in chunks of pixelsPerYieldChunk
// and yields when the budget is reached.
func (v *VDP1) runScaledSprite(budget int32) (consumed int32, done bool) {
	s := &v.spriteResume
	cycles := int32(0)

	for s.outerIdx < s.destH {
		hasDrawn := false

		srcY := ((2*s.outerIdx + 1) * s.charH) / (2 * s.destH)
		if s.effFlipV {
			srcY = s.charH - 1 - srcY
		}

		fbY := s.dstY1 + s.outerIdx
		if fbY < 0 || fbY > s.clipY {
			s.outerIdx++
			s.innerIdx = 0
			s.endCodeCount = 0
			s.prevSrcX = -1
			cycles += 5
			if cycles >= budget && s.outerIdx < s.destH {
				v.cmdPhase = phaseScaledSprite
				return cycles, false
			}
			continue
		}

		for s.innerIdx < s.destW {
			chunkEnd := s.innerIdx + pixelsPerYieldChunk
			if chunkEnd > s.destW {
				chunkEnd = s.destW
			}
			for s.innerIdx < chunkEnd {
				srcX := ((2*s.innerIdx + 1) * s.charW) / (2 * s.destW)
				if s.effFlipH {
					srcX = s.charW - 1 - srcX
				}
				if s.hssShrinkX {
					if s.hssOddParity {
						srcX |= 1
					} else {
						srcX &^= 1
					}
				}

				dot := v.readCharDot(s.charAddr, srcX, srcY, s.charW, s.colorMode)

				// 1. End Code check (only active when ECD = 0 / !s.ecdOff)
				if !s.ecdOff && v.isEndCode(dot, s.colorMode) {
					if !s.hssEcdOff {
						if hasDrawn {
							if srcX != s.prevSrcX {
								s.prevSrcX = srcX
								s.endCodeCount++
								if s.endCodeCount >= 2 {
									s.innerIdx = s.destW
									break
								}
							}
						}
					}
					s.innerIdx++
					cycles++
					continue
				}
				if !s.spdOn && dot == 0 {
					s.innerIdx++
					cycles++
					continue
				}

				hasDrawn = true

				fbX := s.dstX1 + s.innerIdx
				pixel := v.dotToPixel(dot, v.cmdSnapshot.colr, s.colorMode)
				var gouraud uint16
				if s.cc >= 4 {
					top := lerpGouraud(s.gt[0], s.gt[1], s.innerIdx, s.destW)
					bot := lerpGouraud(s.gt[3], s.gt[2], s.innerIdx, s.destW)
					gouraud = lerpGouraud(top, bot, s.outerIdx, s.destH)
				}
				v.writePixel(fbX, fbY, pixel, s.cc, gouraud, s.msbOn, s.mesh, s.userClip, s.clipX, s.clipY)

				s.innerIdx++
				cycles++
			}

			if cycles >= budget {
				v.cmdPhase = phaseScaledSprite
				return cycles, false
			}
		}

		s.outerIdx++
		s.innerIdx = 0
		s.endCodeCount = 0
		s.prevSrcX = -1
		if cycles >= budget && s.outerIdx < s.destH {
			v.cmdPhase = phaseScaledSprite
			return cycles, false
		}
	}

	v.cmdPhase = phaseIdle
	if s.cc >= 4 {
		cycles += 4
	}
	if s.colorMode == 1 {
		cycles += 16
	}
	return cycles, true
}
