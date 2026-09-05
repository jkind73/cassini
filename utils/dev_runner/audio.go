// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"io"
	"log"
	"math"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

// Audio constants for Saturn SCSP output.
const (
	audioSampleRate = 44100

	// ringBufferFrames is the audio ring depth in frames. The byte size is
	// derived at runtime from the core's frame rate (see newAudioPlayer).
	// Under timer-paced production the producer no longer blocks on a full
	// ring in steady state; the ring must be deep enough to absorb oto's
	// bursty multi-frame reads (~2 frames pulled every ~33ms, with jitter)
	// without hitting the block-on-full path, which would re-couple it to
	// oto's coarse read cadence.
	ringBufferFrames = 6

	// otoPlayerBufferBytes sizes the oto mux player buffer (~50ms at 44.1kHz
	// stereo int16) via player.SetBufferSize.
	otoPlayerBufferBytes = 19200
)

var (
	otoCtx      *oto.Context
	otoInitOnce sync.Once
	otoInitErr  error
)

func ensureOtoContext() (*oto.Context, error) {
	otoInitOnce.Do(func() {
		op := &oto.NewContextOptions{
			SampleRate:   audioSampleRate,
			ChannelCount: 2,
			Format:       oto.FormatSignedInt16LE,
			BufferSize:   50 * time.Millisecond,
		}

		type result struct {
			ctx   *oto.Context
			ready chan struct{}
			err   error
		}

		ch := make(chan result, 1)
		go func() {
			c, r, e := oto.NewContext(op)
			ch <- result{ctx: c, ready: r, err: e}
		}()

		select {
		case res := <-ch:
			if res.err != nil {
				otoInitErr = res.err
				return
			}
			select {
			case <-res.ready:
				otoCtx = res.ctx
			case <-time.After(500 * time.Millisecond):
				otoInitErr = fmt.Errorf("audio device ready signal timeout")
			}
		case <-time.After(500 * time.Millisecond):
			otoInitErr = fmt.Errorf("oto.NewContext timeout")
		}
	})
	return otoCtx, otoInitErr
}

// ---------------------------------------------------------------------------
// Audio Player
// ---------------------------------------------------------------------------

// audioPlayer wraps oto and a ring buffer. The producer writes one frame of
// samples per emulated frame into the ring; oto drains it. Pacing is NOT done
// here - the emulation loop paces itself on an absolute-deadline timer and
// uses the ring fill (Buffered) only as a slow rate-lock reference. The ring's
// block-on-full path remains as a backpressure safety net.
type audioPlayer struct {
	player     *oto.Player
	ringBuffer *audioRingBuffer
	audioBytes []byte
	// silentFrame is one frame's worth of zero bytes, written to the ring
	// when the core produces empty audio so oto always has samples to drain
	// and does not underrun on empty cold-start frames.
	silentFrame []byte
}

func newAudioPlayer(fps int) (*audioPlayer, error) {
	ctx, err := ensureOtoContext()
	if err != nil {
		return nil, fmt.Errorf("oto audio not available: %w", err)
	}

	bytesPerFrame := int(math.Round(float64(audioSampleRate) * 4 / float64(fps)))
	rb := newAudioRingBuffer(ringBufferFrames * bytesPerFrame)

	ap := &audioPlayer{
		ringBuffer:  rb,
		audioBytes:  make([]byte, 0, 4096),
		silentFrame: make([]byte, bytesPerFrame),
	}

	type playerResult struct {
		player *oto.Player
	}
	pCh := make(chan playerResult, 1)
	go func() {
		p := ctx.NewPlayer(rb)
		p.SetBufferSize(otoPlayerBufferBytes)
		p.SetVolume(1.0)
		p.Play()
		pCh <- playerResult{player: p}
	}()

	select {
	case res := <-pCh:
		ap.player = res.player
	case <-time.After(500 * time.Millisecond):
		log.Printf("Warning: ctx.NewPlayer timed out; audio disabled")
	}

	return ap, nil
}

// buffered returns the current ring fill in bytes. The timer-pacing
// controller reads this as its rate-lock reference.
func (a *audioPlayer) buffered() int {
	if a == nil || a.ringBuffer == nil {
		return 0
	}
	return a.ringBuffer.Buffered()
}

func (a *audioPlayer) queueSamples(samples []int16) {
	if a == nil || a.player == nil || a.ringBuffer == nil {
		return
	}
	if len(samples) == 0 {
		a.ringBuffer.Write(a.silentFrame)
		return
	}

	needed := len(samples) * 2
	if cap(a.audioBytes) < needed {
		a.audioBytes = make([]byte, 0, needed)
	}
	a.audioBytes = a.audioBytes[:0]
	for _, sample := range samples {
		a.audioBytes = append(a.audioBytes, byte(sample), byte(sample>>8))
	}

	a.ringBuffer.Write(a.audioBytes)
}

func (a *audioPlayer) close() {
	if a == nil {
		return
	}
	if a.ringBuffer != nil {
		a.ringBuffer.Close()
	}
	if a.player != nil {
		a.player.Close()
	}
}

// ---------------------------------------------------------------------------
// Audio ring buffer
// ---------------------------------------------------------------------------

type audioRingBuffer struct {
	buf      []byte
	readPos  int
	writePos int
	count    int
	capacity int
	mu       sync.Mutex
	cond     *sync.Cond
	closed   bool
}

func newAudioRingBuffer(capacity int) *audioRingBuffer {
	rb := &audioRingBuffer{
		buf:      make([]byte, capacity),
		capacity: capacity,
	}
	rb.cond = sync.NewCond(&rb.mu)
	return rb
}

func (rb *audioRingBuffer) Write(p []byte) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.closed {
		return
	}

	n := len(p)
	if n == 0 {
		return
	}

	if n > rb.capacity {
		p = p[n-rb.capacity:]
		n = rb.capacity
	}

	written := 0
	for written < n {
		for !rb.closed && rb.count == rb.capacity {
			rb.cond.Wait()
		}
		if rb.closed {
			return
		}

		// Only signal the reader on the empty->non-empty transition.
		// Signaling when the reader isn't parked still costs a syscall via
		// runtime.pthread_cond_signal, which shows up as scheduling overhead.
		wasEmpty := rb.count == 0

		avail := rb.capacity - rb.count
		chunk := n - written
		if chunk > avail {
			chunk = avail
		}

		firstChunk := rb.capacity - rb.writePos
		if firstChunk >= chunk {
			copy(rb.buf[rb.writePos:], p[written:written+chunk])
		} else {
			copy(rb.buf[rb.writePos:], p[written:written+firstChunk])
			copy(rb.buf[0:], p[written+firstChunk:written+chunk])
		}
		rb.writePos = (rb.writePos + chunk) % rb.capacity
		rb.count += chunk
		written += chunk

		if wasEmpty {
			rb.cond.Signal()
		}
	}
}

func (rb *audioRingBuffer) Read(p []byte) (int, error) {
	rb.mu.Lock()

	for rb.count == 0 {
		if rb.closed {
			rb.mu.Unlock()
			return 0, io.EOF
		}
		rb.cond.Wait()
	}

	// Only signal the writer on the full->non-full transition; avoids a
	// syscall every Read when the writer is not parked.
	wasFull := rb.count == rb.capacity

	n := len(p)
	if n > rb.count {
		n = rb.count
	}

	firstChunk := rb.capacity - rb.readPos
	if firstChunk >= n {
		copy(p, rb.buf[rb.readPos:rb.readPos+n])
	} else {
		copy(p, rb.buf[rb.readPos:])
		copy(p[firstChunk:], rb.buf[:n-firstChunk])
	}
	rb.readPos = (rb.readPos + n) % rb.capacity
	rb.count -= n

	if wasFull {
		rb.cond.Signal()
	}

	rb.mu.Unlock()

	return n, nil
}

func (rb *audioRingBuffer) Buffered() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.count
}

func (rb *audioRingBuffer) Close() {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.closed = true
	rb.cond.Broadcast()
}
