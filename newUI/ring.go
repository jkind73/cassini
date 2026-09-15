// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package ebiten

import "sync"

// ringReader is a simple byte ring buffer implementing io.Reader.
// The core thread writes s16 samples via write(); ebiten's audio
// player goroutine drains via Read(). Mutex-guarded rather than
// lock-free since audio isn't per-frame-latency-critical the way
// video is, and the critical sections are tiny (memcpy only).
type ringReader struct {
	mu   sync.Mutex
	buf  []byte
	r, w int
	full bool
}

func newRingReader(sizeSamples int) *ringReader {
	return &ringReader{buf: make([]byte, sizeSamples*2)} // s16 = 2 bytes/sample
}

// write appends interleaved s16 stereo samples from the core thread.
// Drops oldest data on overflow rather than blocking the core thread —
// an audio glitch under sustained overload is preferable to stalling
// emulation timing.
func (rb *ringReader) write(samples []int16) {
	if len(samples) == 0 {
		return
	}
	rb.mu.Lock()
	defer rb.mu.Unlock()

	for _, s := range samples {
		rb.buf[rb.w] = byte(s)
		rb.buf[rb.w+1] = byte(s >> 8)
		rb.w = (rb.w + 2) % len(rb.buf)
		if rb.full {
			rb.r = rb.w
		}
		if rb.w == rb.r {
			rb.full = true
		}
	}
}

// Read implements io.Reader for ebiten's audio player.
func (rb *ringReader) Read(p []byte) (int, error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	n := 0
	for n < len(p) && (rb.r != rb.w || rb.full) {
		p[n] = rb.buf[rb.r]
		rb.r = (rb.r + 1) % len(rb.buf)
		rb.full = false
		n++
	}
	// Zero-fill remainder (silence) rather than blocking — underrun is
	// audible as a brief gap, not a core-thread stall.
	for i := n; i < len(p); i++ {
		p[i] = 0
	}
	return len(p), nil
}
