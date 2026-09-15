// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

// Package ebiten implements ui.Manager on top of hajimehoshi/ebiten/v2.
//
// Thread model:
//   - The emulation core runs on its own dedicated goroutine ("core
//     thread"), driven by its own ticker at the console's native frame
//     rate. It never touches ebiten APIs.
//   - Ebiten's Update/Draw run on the OS main thread ("UI thread"), as
//     required by every native windowing toolkit.
//   - The two communicate through frameBuf (a lock-free triple buffer
//     for video) and an input channel (UI -> core, non-blocking send).
//   - Audio samples are pushed from the core thread into a
//     ring-buffered io.Reader that ebiten's audio player drains on its
//     own goroutine, so audio timing is decoupled from both the core
//     tick and the UI frame rate.
//
// This means core timing is NEVER gated on window events (resize,
// minimize, focus loss): the core thread free-runs regardless of what
// the UI thread is doing, and the UI simply displays whatever the most
// recently completed frame was.
package ebiten

import (
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"

	"github.com/jkind73/cassini/cache"
	"github.com/jkind73/cassini/config"
	"github.com/jkind73/cassini/discimg"
	"github.com/jkind73/cassini/romcartdb"
	"github.com/jkind73/cassini/screenscraper"
	"github.com/jkind73/cassini/ui"
	"github.com/jkind73/cassini/uiface"
)

// frameSlot holds one decoded video frame plus the metadata needed to
// draw it (stride/height can change frame-to-frame on real Saturn
// content due to VDP2 resolution switches). par is captured from
// uiface.Emulator.PixelAspectRatio() at the same instant as the pixels
// it describes — it is a core-thread-only query (see coreThread), so
// it is snapshotted here rather than re-queried from the UI thread.
type frameSlot struct {
	pix    []byte
	stride int
	height int
	par    float64
}

// tripleBuffer lets the core thread publish frames without ever
// blocking on the UI thread, and lets the UI thread read the latest
// complete frame without ever blocking on the core thread.
type tripleBuffer struct {
	slots   [3]frameSlot
	writing int32 // index core is currently filling
	ready   int32 // index of latest fully-written frame
	reading int32 // index UI is currently reading
}

func (b *tripleBuffer) publish(pix []byte, stride, h int, par float64) {
	s := &b.slots[b.writing]
	if cap(s.pix) < len(pix) {
		s.pix = make([]byte, len(pix))
	}
	s.pix = s.pix[:len(pix)]
	copy(s.pix, pix)
	s.stride, s.height, s.par = stride, h, par

	// Publish: writing slot becomes ready; pick a fresh slot to write
	// next that isn't the one the UI might currently be reading.
	old := atomic.SwapInt32(&b.ready, b.writing)
	next := int32(0)
	for next == old || next == atomic.LoadInt32(&b.reading) {
		next = (next + 1) % 3
	}
	atomic.StoreInt32(&b.writing, next)
}

func (b *tripleBuffer) latest() frameSlot {
	idx := atomic.LoadInt32(&b.ready)
	atomic.StoreInt32(&b.reading, idx)
	return b.slots[idx]
}

// inputEvent is one input update queued from the UI thread to the
// core thread. The channel is generously buffered and sends are
// non-blocking (dropped on overflow) so a slow core tick can never
// stall input polling on the UI thread.
type inputEvent struct {
	kind    inputKind
	player  int // also used as "port" (0/1) for accessory event kinds
	buttons uint32
	x, y    int
	trigger bool

	// Accessory event fields (see accessory.go). Only the fields
	// relevant to ev.kind are meaningful for a given event; kept as a
	// single struct rather than per-kind types to reuse the existing
	// inputEvent channel/queue machinery unchanged.
	left, middle, right, start bool  // mouse buttons
	dx, dy                     int32 // mouse delta
	padButtons                 uint16
	ax, ay, ar, al, az         uint8 // analog axes (3D pad/Mission Stick/Racing)
	mdMode, mdX, mdY, mdZ      bool  // MD6 extra buttons beyond padButtons' 3-button set
	sixButton                  bool  // MD3 vs MD6
	scancode                   byte  // keyboard
	released                   bool  // keyboard: true = key-up
}

type inputKind int

const (
	inputButtons inputKind = iota
	inputPointer
	inputMouse
	inputAnalogPad
	inputMissionStick
	inputRacing
	inputMDPad
	inputKeyboard
)

// optionEvent queues a SetOption call for the core thread.
//
// IMPORTANT: core.Emulator.SetOption (see emulator.go:451) is a single
// unsynchronized switch statement. Some of its cases mutate state the
// core reads mid-RunFrame on the core thread — scale_factor and
// texture_filtering feed vdp1's internal render resolution, and
// lightgun_mode/lightgun_port touch smpc input state. There is no
// mutex anywhere in that switch. Calling SetOption concurrently from
// the UI thread while coreThread is mid-RunFrame is a data race on
// shared core state and a direct threat to cycle-accurate, replay-
// deterministic behavior — even for keys that look presentation-only,
// because they share one function with keys that aren't.
//
// So the rule enforced here: uiface.Emulator.SetOption is NEVER called
// from the UI thread, full stop. Every option change — no matter how
// innocuous it looks — goes through this queue and is applied on the
// core thread between ticks, using the exact same non-blocking-send /
// apply-before-next-tick mechanism as input events.
//
// Five of SetOption's keys (aspect_ratio, display_shader,
// upscaler_filter, crt_bloom, crt_curvature) only ever write into
// e.displayProc, which core never reads back — DisplayProcessor.
// ProcessFrame is not called anywhere in emulator.go, vdp2.go, or
// vdp2_render.go. So today those five calls are inert on the core
// side. Rather than resurrect that dead CPU code path in the render
// loop (which would mean per-frame CPU pixel processing racing for
// time against the core thread on every single UI frame), presentation
// is implemented independently in present.go/shader.go as a GPU
// (Kage) shader, driven by UI-thread-only state. We still forward
// those five keys to SetOption too, purely so config/save-state round
// trips and any future core-side use of displayProc stay consistent
// with what's on screen — but the UI never depends on the core's copy.
type optionEvent struct {
	key, value string
}

type manager struct {
	icon []byte

	factory uiface.Factory
	emu     uiface.Emulator
	frames  tripleBuffer
	input   chan inputEvent
	opts    chan optionEvent

	audioCtx    *audio.Context
	audioStream *ringReader

	stopCore chan struct{}

	onSettingsChanged func(ui.SettingsSnapshot)

	// calibration is the live axis-calibration store, converted once
	// at startup from opts.Calibration and updated in place whenever
	// the settings screen produces a new calibration (see settings.go
	// and toConfigCalibration). Read by accessory.go's calibrated axis
	// lookups on every poll -- never mutated concurrently with those
	// reads since both happen on the UI thread only (Update()).
	calibration config.Calibration

	// Library holds everything the browser (Run) needs that
	// RunDirect doesn't: scan directories, the metadata cache, and an
	// optional ScreenScraper client. All nil-safe — a nil Cache/
	// ScreenScraper simply means those tiers of the resolver chain
	// (cache.Resolver, item 6) are skipped, not an error, matching how
	// RunDirect never needed any of this at all.
	lib Library
}

// Library configures the browser's data sources. Kept as a separate
// struct passed to New (rather than folded into ui.RunOptions) since
// this is Manager-construction-time configuration, not a per-run
// flag — it doesn't change between Run/RunDirect calls the way
// InitialWindow or KeyBindOverrides might.
type Library struct {
	ROMDirs   []string
	BIOSDirs  []string
	CartDirs  []string // KOF95/Ultraman ROM cart images; auto-selected by product number, never a UI choice -- see romcartdb
	Cache     *cache.Cache          // nil: browser works, no offline cache/fallback lookups
	Scraper   *screenscraper.Client // nil: browser works, no ScreenScraper tier
	MediaDir  string                // where downloaded cover art is cached to disk
}

// New returns a ui.Manager backed by ebiten. lib is only consulted by
// Run (the browser); RunDirect ignores it entirely, so passing a zero
// Library is fine for a direct-run-only caller.
func New(lib Library) ui.Manager {
	return &manager{
		input: make(chan inputEvent, 64),
		opts:  make(chan optionEvent, 16),
		lib:   lib,
	}
}

func (m *manager) SetAppIcon(icon []byte) { m.icon = icon }

func defaultString(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// toConfigAccessory converts ui.PortAccessory (the framework-agnostic
// shape) to config.PortAccessory (accessoryState's internal shape).
// ui/ebiten already depends on both packages elsewhere (config for
// Library, ui for the Manager contract), so this conversion doesn't
// introduce any new dependency edge -- it just avoids accessoryState
// needing to know about package ui's type at all.
func toConfigAccessory(a ui.PortAccessory) config.PortAccessory {
	return config.PortAccessory{Type: a.Type, KeyboardLayout: a.KeyboardLayout}
}

// toConfigCalibration/fromConfigCalibration convert between
// ui.RunOptions/SettingsSnapshot's calibration shape and
// config.Calibration -- ui/ebiten already depends on both packages
// (config for Library, ui for the Manager contract), so this
// conversion doesn't introduce a new dependency edge.
func toConfigCalibration(m map[string]map[string]ui.AxisCalibration) config.Calibration {
	out := config.Calibration{}
	for dev, channels := range m {
		out[dev] = map[string]config.AxisCalibration{}
		for ch, cal := range channels {
			out[dev][ch] = config.AxisCalibration{
				RawAxis: cal.RawAxis, Rest: cal.Rest, Min: cal.Min, Max: cal.Max, Inverted: cal.Inverted,
			}
		}
	}
	return out
}

func fromConfigCalibration(c config.Calibration) map[string]map[string]ui.AxisCalibration {
	out := map[string]map[string]ui.AxisCalibration{}
	for dev, channels := range c {
		out[dev] = map[string]ui.AxisCalibration{}
		for ch, cal := range channels {
			out[dev][ch] = ui.AxisCalibration{
				RawAxis: cal.RawAxis, Rest: cal.Rest, Min: cal.Min, Max: cal.Max, Inverted: cal.Inverted,
			}
		}
	}
	return out
}

func (m *manager) Run(factory uiface.Factory, opts ui.RunOptions) error {
	m.factory = factory
	info := factory.SystemInfo()

	if err := m.setupAudio(info); err != nil {
		return err
	}
	present := newPresentState()
	present.applyInitial(opts.Options)
	m.setupWindow(info, opts)

	g := &game{
		m:                m,
		present:          present,
		poller:           newInputPoller(info, opts.KeyBindOverrides),
		mode:             modeBrowser,
		baseOptions:      opts.Options,
		keyBindOverrides: opts.KeyBindOverrides,
		lightgunEnabled:  opts.Options["lightgun_mode"] == "true",
		lightgunPort:     defaultString(opts.Options["lightgun_port"], "1"),
		accessories:      newAccessoryState(toConfigAccessory(opts.Port1Accessory), toConfigAccessory(opts.Port2Accessory)),
	}
	m.onSettingsChanged = opts.OnSettingsChanged
	m.calibration = toConfigCalibration(opts.Calibration)
	g.browser = newBrowserState(m)
	g.browser.startScan()

	err := ebiten.RunGame(g)
	if g.mode == modePlaying {
		g.endSession()
	}
	if opts.OnWindowStateChanged != nil {
		opts.OnWindowStateChanged(captureWindowState())
	}
	return err
}

func (m *manager) RunDirect(factory uiface.Factory, opts ui.RunOptions) error {
	m.factory = factory
	info := factory.SystemInfo()

	if err := m.setupAudio(info); err != nil {
		return err
	}
	present := newPresentState()
	present.applyInitial(opts.Options)
	m.setupWindow(info, opts)

	g := &game{
		m:                m,
		present:          present,
		poller:           newInputPoller(info, opts.KeyBindOverrides),
		mode:             modePlaying,
		baseOptions:      opts.Options,
		keyBindOverrides: opts.KeyBindOverrides,
		lightgunEnabled:  opts.Options["lightgun_mode"] == "true",
		lightgunPort:     defaultString(opts.Options["lightgun_port"], "1"),
		accessories:      newAccessoryState(toConfigAccessory(opts.Port1Accessory), toConfigAccessory(opts.Port2Accessory)),
	}
	m.onSettingsChanged = opts.OnSettingsChanged
	m.calibration = toConfigCalibration(opts.Calibration)
	if err := g.startSession(opts.DiscPath, opts.BIOS, opts.Options); err != nil {
		return err
	}

	err := ebiten.RunGame(g)
	g.endSession()

	if opts.OnWindowStateChanged != nil {
		opts.OnWindowStateChanged(captureWindowState())
	}
	return err
}

// setupAudio creates the audio context/player once per process. The
// sample rate is fixed per-system (Saturn's SCSP output rate never
// changes game-to-game), so — unlike m.emu, which is per-session
// (see startSession) — this is set up once here and simply keeps
// draining whatever ringReader.write calls the current session's
// coreThread makes; a session boundary needs no audio-side reset.
func (m *manager) setupAudio(info uiface.SystemInfo) error {
	m.audioCtx = audio.NewContext(info.SampleRate)
	m.audioStream = newRingReader(info.SampleRate * 4) // ~1s of stereo s16 headroom
	player, err := m.audioCtx.NewPlayer(m.audioStream)
	if err != nil {
		return err
	}
	player.Play()
	return nil
}

func (m *manager) setupWindow(info uiface.SystemInfo, opts ui.RunOptions) {
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetWindowTitle(info.CoreName)
	if m.icon != nil {
		// ebiten.SetWindowIcon takes []image.Image; decode m.icon (webp)
		// at startup and pass here.
	}
	applyInitialWindow(opts.InitialWindow, info)
}

// applyInitialWindow sets startup geometry/mode. A zero Width/Height
// falls back to a size derived from the console's native resolution
// (2x integer scale) rather than a literal 0x0 window.
func applyInitialWindow(ws ui.WindowState, info uiface.SystemInfo) {
	w, h := ws.Width, ws.Height
	if w <= 0 || h <= 0 {
		w, h = info.ScreenWidth*2, info.MaxScreenHeight*2
	}
	ebiten.SetWindowSize(w, h)

	if ws.X >= 0 && ws.Y >= 0 {
		ebiten.SetWindowPosition(ws.X, ws.Y)
	}
	if ws.Fullscreen {
		ebiten.SetFullscreen(true)
	} else if ws.Maximized {
		ebiten.MaximizeWindow()
	}
}

// captureWindowState reads back actual current geometry/mode. Called
// once, after ebiten.RunGame has returned (main thread, render loop
// fully stopped), so these calls are the last ebiten API use before
// shutdown completes.
func captureWindowState() ui.WindowState {
	w, h := ebiten.WindowSize()
	x, y := ebiten.WindowPosition()
	return ui.WindowState{
		Width:      w,
		Height:     h,
		X:          x,
		Y:          y,
		Maximized:  ebiten.IsWindowMaximized(),
		Fullscreen: ebiten.IsFullscreen(),
	}
}

// coreThread is the entire emulation loop. It never calls into ebiten.
// It free-runs at the console's native rate regardless of UI frame
// pacing, window state, or resize events — this is what guarantees
// zero A/V-sync or timing regression from the UI swap.
func (m *manager) coreThread(info uiface.SystemInfo, initialOptions map[string]string) {
	for k, v := range initialOptions {
		m.emu.SetOption(k, v) // single-threaded here: loop below hasn't started
	}

	t := m.emu.GetTiming()
	period := time.Duration(float64(time.Second) / t.FPS)
	ticker := time.NewTicker(period)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCore:
			return
		case ev := <-m.input:
			switch ev.kind {
			case inputButtons:
				m.emu.SetInput(ev.player, ev.buttons)
			case inputPointer:
				m.emu.SetPointer(ev.player, ev.x, ev.y, ev.trigger)
			case inputMouse:
				m.emu.SetMouseData(ev.player, ev.left, ev.middle, ev.right, ev.start, ev.dx, ev.dy)
			case inputAnalogPad:
				m.emu.SetAnalogPadData(ev.player, ev.padButtons, ev.ax, ev.ay, ev.ar, ev.al)
			case inputMissionStick:
				m.emu.SetMissionStickData(ev.player, ev.padButtons, ev.ax, ev.ay, ev.az)
			case inputRacing:
				m.emu.SetRacingData(ev.player, ev.padButtons, ev.ax)
			case inputMDPad:
				b := ev.padButtons
				m.emu.SetMDPadData(ev.player,
					b&0x8000 == 0, b&0x4000 == 0, b&0x2000 == 0, b&0x1000 == 0, // right,left,down,up (active-low bits)
					b&0x0800 == 0, b&0x0400 == 0, b&0x0200 == 0, b&0x0100 == 0, // start,a,c,b
					ev.mdMode, ev.mdX, ev.mdY, ev.mdZ)
			case inputKeyboard:
				m.emu.PushKeyboardEvent(ev.player, ev.scancode, ev.released)
			}
			continue // apply input without waiting for next tick
		case ev := <-m.opts:
			m.emu.SetOption(ev.key, ev.value)
			continue // same as input: apply before the next tick, never mid-RunFrame
		case <-ticker.C:
			m.emu.RunFrame()
			m.frames.publish(
				m.emu.GetFramebuffer(),
				m.emu.GetFramebufferStride(),
				m.emu.GetActiveHeight(),
				m.emu.PixelAspectRatio(), // core-thread-only query, see frameSlot
			)
			m.audioStream.write(m.emu.GetAudioSamples())
		}
	}
}

// sendInput is called from the UI thread (Ebiten's Update). Never
// blocks: on a full queue it drops the event rather than stall input
// polling, which is preferable to stuttering the render loop.
func (m *manager) sendInput(ev inputEvent) {
	select {
	case m.input <- ev:
	default:
	}
}

// sendKeyboardEvent delivers a Saturn Keyboard Make/Break event with a
// stronger guarantee than sendInput's drop-on-full-queue send: this
// blocks until coreThread accepts it. Keyboard events are discrete and
// non-idempotent (unlike continuous state -- pad bitmask, mouse
// position, analog axes -- where a dropped update just means one
// stale frame that self-corrects on the next poll), so dropping a
// Make with its Break later delivered would desync the emulated
// keyboard's Make/Break bookkeeping from what actually happened on
// the host, exactly the kind of hardware-behavior deviation the
// 100%-accuracy standard rules out. coreThread's select loop drains
// m.input promptly every tick (worst case: blocked for one core frame
// period, ~16.6ms at 60fps, while mid-RunFrame) -- imperceptible for
// something that fires at most a few dozen times per second even
// during fast typing, and safe from deadlock since coreThread is the
// sole consumer and never itself blocks waiting on m.input.
// sendKeyboardEvent delivers one Make/Break event with a stronger
// delivery guarantee than sendInput's drop-on-full-queue send: no
// drops, no reordering, bounded latency.
//
// Why keyboard needs this and nothing else does: every other
// inputEvent kind carries continuous STATE (current pad bitmask,
// current mouse position, current analog axis value) — if one send is
// dropped, the very next poll resends the current value a few
// milliseconds later, so a drop is self-correcting and invisible.
// Keyboard Make/Break events are discrete and non-idempotent: a
// dropped Make with its Break later delivered leaves the game's
// keyboard driver seeing a release it never saw the matching press
// for — a real behavioral divergence from a real Saturn keyboard's
// own MCU-buffered delivery (SaturnKeyboard's FIFO models that
// hardware buffer specifically so events are queued, not silently
// discarded; a best-effort UI-side drop would undermine that on the
// delivery path alone, independent of whether the wire format itself
// is correct) — exactly the kind of hardware-behavior deviation the
// project's cycle-accuracy standard rules out, not a cosmetic rough
// edge.
//
// Implementation: a plain blocking channel send (no select/default).
// coreThread drains m.input promptly on every iteration of its select
// loop, so worst case this blocks the UI thread for one core frame
// period (~16.6ms at 60Hz, while a RunFrame() is in flight) —
// imperceptible for keyboard input, which occurs at most a few dozen
// times per second even during fast typing — and there is no deadlock
// risk since coreThread is the sole consumer and never itself blocks
// waiting on m.input. Go channels are FIFO, so Make always arrives
// before its matching Break and events across different keys stay
// ordered.
func (m *manager) sendKeyboardEvent(ev inputEvent) {
	m.input <- ev
}

// sendOption queues a SetOption call for the core thread. Unlike
// sendInput, this blocks briefly (regular channel send, buffer 16)
// rather than dropping — an ignored menu selection is a worse UX bug
// than a few microseconds of Update() latency, and 16-deep is already
// far more than a human can generate between core ticks.
func (m *manager) sendOption(key, value string) {
	m.opts <- optionEvent{key: key, value: value}
}

// game implements ebiten.Game. All methods run on the UI thread.
// present is owned exclusively by this thread — coreThread never
// touches it, so it needs no locking.
type mode int

const (
	modeBrowser mode = iota
	modePlaying
	modeSettings
)

type game struct {
	m       *manager
	present *presentState
	poller  *inputPoller
	accessories *accessoryState
	mode    mode
	prevMode mode // what to return to when exitSettings is called
	browser *browserState
	settings *settingsState

	// baseOptions is the process-wide option set (aspect ratio, fast
	// boot, etc, from config) that every session starts with. Kept
	// here so the browser's "launch selected game" path can pass the
	// same options startSession would otherwise only see once, at
	// RunDirect's single call.
	baseOptions map[string]string

	// keyBindOverrides/lightgunEnabled/lightgunPort mirror the
	// process's current settings, kept here (not just inside
	// settingsState, which is transient/nil outside modeSettings) so
	// re-entering settings later starts from the last-applied state,
	// and so startSession can apply the current lightgun settings to
	// every new session's Options without needing settingsState to
	// still exist.
	keyBindOverrides []ui.KeyBindOverride
	lightgunEnabled  bool
	lightgunPort     string

	// discReader is owned by the current session (nil in modeBrowser);
	// closed by endSession.
	discReader discimg.DiscReadCloser

	// escDown/f1Down debounce their respective hotkeys so holding the
	// key doesn't fire the action every single tick.
	escDown bool
	f1Down  bool

	// frameTex is a persistent GPU texture reused frame-to-frame, only
	// reallocated when the source resolution actually changes (VDP2
	// resolution switches mid-game are legal and must not stall or
	// stutter). Avoids an ebiten.Image + GC allocation on every single
	// UI frame.
	frameTex     *ebiten.Image
	frameTexW    int
	frameTexH    int
	winW, winH   int
	crtShader    *ebiten.Shader
	bicubicShader *ebiten.Shader
	shaderLoaded bool

	// lastDest/lastSrcW/lastSrcH are the geometry Draw most recently
	// computed, reused by Update (via poller.poll) for lightgun
	// coordinate transform. ebiten calls Update before Draw each tick,
	// so this is one tick stale on the very first frame only, which is
	// visually and functionally irrelevant for a pointer position.
	lastDest           destRect
	lastSrcW, lastSrcH int
}

// startSession creates a fresh emulator instance, applies BIOS/disc/
// options, and starts the core thread, then switches g into
// modePlaying. Called once by RunDirect before the first RunGame, and
// again by the browser (browser.go) each time the user launches a
// game — the emulator instance itself is genuinely per-session (a
// fresh factory.CreateEmulator() call every time), unlike present/
// poller/audio, which are process-lifetime and set up once by
// Run/RunDirect.
func (g *game) startSession(discPath string, bios map[string][]byte, options map[string]string) error {
	m := g.m
	info := m.factory.SystemInfo()
	m.emu = m.factory.CreateEmulator()

	for key, data := range bios {
		if err := m.emu.SetBIOS(key, data); err != nil {
			m.emu = nil
			return err
		}
	}

	if discPath != "" {
		disc, err := discimg.Open(discPath)
		if err != nil {
			m.emu = nil
			return err
		}
		m.emu.SetDisc(disc)
		g.discReader = disc

		// ROM cartridge auto-select (KOF95/Ultraman): the only two
		// Saturn titles using a game-specific ROM cart instead of the
		// general DRAM expansion cart. Deliberately not a UI setting —
		// detected purely from the disc's product number, same as
		// BIOS region auto-assignment (item 3). factory.DiscInfo is
		// the same, unmodified function every other identification
		// path in cassini already goes through — see romcartdb's
		// wiring for why this can't happen inside package core itself
		// (no filesystem access there).
		if info, ok := m.factory.DiscInfo(disc); ok {
			if _, needsCart := romcartdb.RequiresROMCart(info.ProductNumber); needsCart {
				found, err := romcartdb.Scan(m.lib.CartDirs)
				if err != nil {
					m.emu = nil
					return fmt.Errorf("rom cart scan: %w", err)
				}
				if f, ok := romcartdb.SelectForProduct(found, info.ProductNumber); ok {
					data, err := os.ReadFile(f.Path)
					if err != nil {
						m.emu = nil
						return fmt.Errorf("rom cart: read %s: %w", f.Path, err)
					}
					m.emu.SetROMCartridge(data)
				}
				// If not found: the game will correctly see "no
				// cartridge" (applyRAMCartOverride's core-side
				// handling, see emulator.go.patch.txt) rather than
				// cassini refusing to launch the disc at all — a
				// missing optional accessory shouldn't block play.
			}
		}
	}

	g.accessories.applyToSession(m.emu)
	m.emu.Start()

	m.stopCore = make(chan struct{})
	go m.coreThread(info, options)

	g.mode = modePlaying
	g.frameTex = nil // force texture realloc: the new session's resolution may differ
	return nil
}

// endSession stops the core thread, closes the emulator and disc, and
// returns g to modeBrowser. Safe to call when nothing is running
// (e.g. RunDirect's caller reaching this after a normal exit) — every
// step is nil/closed-channel guarded.
func (g *game) endSession() {
	m := g.m
	if m.stopCore != nil {
		close(m.stopCore)
		m.stopCore = nil
	}
	if m.emu != nil {
		m.emu.Close()
		m.emu = nil
	}
	if g.discReader != nil {
		g.discReader.Close()
		g.discReader = nil
	}
	g.mode = modeBrowser
}

// enterSettings opens the settings screen (item 8), remembering the
// mode to return to. Callable from either modeBrowser or modePlaying.
func (g *game) enterSettings() {
	info := g.m.factory.SystemInfo()
	g.prevMode = g.mode
	g.settings = newSettingsState(g.poller, info, g.keyBindOverrides, g.lightgunEnabled, g.lightgunPort, g.accessories.port1, g.accessories.port2, g.m.calibration)
	g.mode = modeSettings
}

// exitSettings applies and persists whatever changed during the
// settings visit, then returns to prevMode.
func (g *game) exitSettings() {
	snap := g.settings.snapshot()
	g.keyBindOverrides = snap.KeyBindOverrides
	g.lightgunEnabled = snap.LightgunEnabled
	g.lightgunPort = snap.LightgunPort

	if g.settings.changed {
		// Apply lightgun settings live if a session is running. Both
		// keys are confirmed-real SetOption cases (emulator.go) — see
		// the item 8 discussion for why nothing else is wired here.
		// Always routed through the core-thread queue, never called
		// directly — see optionEvent's doc comment.
		if g.mode == modePlaying || g.prevMode == modePlaying {
			g.m.sendOption("lightgun_mode", boolStr(g.lightgunEnabled))
			g.m.sendOption("lightgun_port", g.lightgunPort)
		}
		// Accessory type changes (port 1/2 device selection) are NOT
		// hot-swapped into an already-running session -- unlike
		// lightgun_mode/port, selecting a different peripheral means
		// calling a different Set*Mode entirely (SMPC's peripheral-ID
		// reporting itself changes, not just a data value feeding an
		// already-selected one), and real Saturn games generally only
		// poll for a peripheral type change at their own boot/reset,
		// not mid-session either. Takes effect starting the next
		// session (accessories.applyToSession runs at every
		// startSession call).
		g.accessories.port1 = config.PortAccessory{Type: snap.Port1Accessory.Type, KeyboardLayout: snap.Port1Accessory.KeyboardLayout}
		g.accessories.port2 = config.PortAccessory{Type: snap.Port2Accessory.Type, KeyboardLayout: snap.Port2Accessory.KeyboardLayout}
		// Calibration takes effect immediately (unlike accessory type
		// selection, which only applies at next session start): it's
		// consumed live by calibratedChannel on every accessory.go
		// poll, reading whatever m.calibration currently holds, with
		// no SMPC mode-selection call involved -- so there's no
		// "already running with the old peripheral ID" state to
		// reconcile the way there is for a full accessory-type swap.
		g.m.calibration = toConfigCalibration(snap.Calibration)
		// Keep baseOptions in sync so the *next* session (a different
		// game launched from the browser) starts with the same
		// lightgun settings too, not just the current one.
		if g.baseOptions == nil {
			g.baseOptions = map[string]string{}
		}
		g.baseOptions["lightgun_mode"] = boolStr(g.lightgunEnabled)
		g.baseOptions["lightgun_port"] = g.lightgunPort

		if g.m.onSettingsChanged != nil {
			g.m.onSettingsChanged(snap)
		}
	}

	g.settings = nil
	g.mode = g.prevMode
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func (g *game) Update() error {
	// F1 opens settings from either browser or gameplay; not
	// re-triggerable from inside settings itself (handled by
	// settingsState.update's own Escape handling instead).
	f1 := ebiten.IsKeyPressed(ebiten.KeyF1)
	if f1 && !g.f1Down && g.mode != modeSettings {
		g.enterSettings()
	}
	g.f1Down = f1
	if g.mode == modeSettings {
		return g.settings.update(g)
	}

	switch g.mode {
	case modeBrowser:
		return g.browser.update(g)
	case modePlaying:
		g.poller.poll(g.m, g.lastDest, g.lastSrcW, g.lastSrcH)
		g.accessories.poll(g.m)
		esc := ebiten.IsKeyPressed(ebiten.KeyEscape)
		if esc && !g.escDown && g.browser != nil {
			g.endSession() // only the browser flow can return to a browser; RunDirect sessions just exit with the window
		}
		g.escDown = esc
	}
	return nil
}

func (g *game) Draw(screen *ebiten.Image) {
	switch g.mode {
	case modeBrowser:
		g.browser.draw(screen, g)
	case modePlaying:
		g.drawGame(screen)
	case modeSettings:
		g.settings.draw(screen, g)
	}
}

func (g *game) drawGame(screen *ebiten.Image) {
	f := g.m.frames.latest()
	if len(f.pix) == 0 || f.stride <= 0 {
		return
	}
	w, h := f.stride/4, f.height
	if w <= 0 || h <= 0 {
		return
	}

	// Reallocate the source texture only on an actual resolution change
	// (e.g. 320x224 -> 704x480 VDP2 hi-res switch), not every frame.
	if g.frameTex == nil || g.frameTexW != w || g.frameTexH != h {
		if g.frameTex != nil {
			g.frameTex.Deallocate()
		}
		g.frameTex = ebiten.NewImage(w, h)
		g.frameTexW, g.frameTexH = w, h
	}
	// WritePixels expects tightly-packed RGBA; f.pix is already RGBA
	// with stride==w*4 and alpha always 0xFF (see vdp2_composite.go),
	// so this is a straight upload, no conversion.
	g.frameTex.WritePixels(f.pix)

	if !g.shaderLoaded {
		g.crtShader, _ = ebiten.NewShader([]byte(crtShaderSrc))
		g.bicubicShader, _ = ebiten.NewShader([]byte(bicubicShaderSrc))
		g.shaderLoaded = true
	}

	dst := computeDestRect(w, h, f.par, g.winW, g.winH, g.present.aspectMode)
	g.lastDest, g.lastSrcW, g.lastSrcH = dst, w, h

	if g.present.shaderMode == displayModeCRT && g.crtShader != nil {
		op := &ebiten.DrawRectShaderOptions{}
		op.Images[0] = g.frameTex
		op.Uniforms = map[string]any{
			"Intensity":  g.present.crtIntensity,
			"Bloom":      g.present.crtBloom,
			"Curvature":  boolToF32(g.present.crtCurvature),
			"SrcSize":    []float32{float32(w), float32(h)},
			"DstOrigin":  []float32{float32(dst.x), float32(dst.y)},
			"DstSize":    []float32{float32(dst.w), float32(dst.h)},
		}
		screen.DrawRectShader(dst.w, dst.h, g.crtShader, op)
		return
	}

	if g.present.filter == filterBicubic && g.bicubicShader != nil {
		op := &ebiten.DrawRectShaderOptions{}
		op.Images[0] = g.frameTex
		op.Uniforms = map[string]any{
			"SrcSize":   []float32{float32(w), float32(h)},
			"DstOrigin": []float32{float32(dst.x), float32(dst.y)},
			"DstSize":   []float32{float32(dst.w), float32(dst.h)},
		}
		screen.DrawRectShader(dst.w, dst.h, g.bicubicShader, op)
		return
	}

	op := &ebiten.DrawImageOptions{}
	op.Filter = g.present.filter.ebitenFilter()
	sx := float64(dst.w) / float64(w)
	sy := float64(dst.h) / float64(h)
	op.GeoM.Scale(sx, sy)
	op.GeoM.Translate(float64(dst.x), float64(dst.y))
	screen.DrawImage(g.frameTex, op)
}

func boolToF32(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

// Layout is ebiten's DPI/resize hook — called automatically whenever
// the window is resized or the OS DPI scale changes. No manual
// per-platform resize handling is needed; this is the entirety of it.
// It only records the logical window size for computeDestRect to use
// in the next Draw — it never touches the core or the frame buffer,
// so a resize can never stall or desync emulation.
func (g *game) Layout(outsideWidth, outsideHeight int) (int, int) {
	g.winW, g.winH = outsideWidth, outsideHeight
	return outsideWidth, outsideHeight
}
