// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

// Package ui defines the UIManager boundary. main.go depends only on
// this package; concrete frontends (ui/ebiten, or any future
// replacement) implement Manager. Swapping frameworks means swapping
// which package main.go imports here — nothing in uiface or the core
// changes.
package ui

import "github.com/jkind73/cassini/uiface"

// WindowState is the window geometry/mode a Manager applies at
// startup (if non-zero) and reports back at shutdown via
// RunOptions.OnWindowStateChanged. Kept in package ui (not pushed down
// into ui/ebiten and not pulled up into a config package) so any
// future Manager implementation reports geometry the same way, and so
// ui/ebiten never needs to import a config package to do its job.
type WindowState struct {
	Width, Height int
	X, Y          int // -1, -1 means "let the OS/window manager choose"
	Maximized     bool
	Fullscreen    bool
}

// KeyBindOverride replaces one button's default binding. Key/Pad use
// the same name spellings as uiface.Button.DefaultKey/DefaultPad
// (ebiten Key-constant-style names for Key; standard-gamepad names
// like "A"/"L1"/"Start" for Pad). Leaving a field empty keeps that
// device's default binding for the button untouched — a
// KeyBindOverride only overrides the device(s) it actually specifies.
type KeyBindOverride struct {
	Player   int
	ButtonID int
	Key      string
	Pad      string
}

// PortAccessory selects which peripheral is connected to one SMPC
// port, mirroring config.PortAccessory's shape (ui package doesn't
// import config, same discipline as WindowState/KeyBindOverride).
type PortAccessory struct {
	Type           string // "pad" (default), "3dpad", "missionstick", "racing", "mouse", "keyboard", "md3", "md6"
	KeyboardLayout string // "jp" | "western", only used when Type == "keyboard"
}

// AxisCalibration mirrors config.AxisCalibration's shape (ui package
// doesn't import config, same discipline as WindowState/PortAccessory).
type AxisCalibration struct {
	RawAxis  int
	Rest     float64
	Min      float64
	Max      float64
	Inverted bool
}

// SettingsSnapshot is the full settings-screen state, reported once
// via OnSettingsChanged when the user leaves the settings screen.
type SettingsSnapshot struct {
	KeyBindOverrides []KeyBindOverride
	LightgunEnabled  bool
	LightgunPort     string // "1" or "2"
	Port1Accessory   PortAccessory
	Port2Accessory   PortAccessory
	// Calibration: device SDL ID -> channel name ("ax"/"ay"/"ar"/"al"/"az") -> AxisCalibration.
	Calibration map[string]map[string]AxisCalibration
}

// RunOptions are the flags/config main.go collects before handing off
// to a Manager. Kept separate from uiface.SystemInfo because these are
// process-level (CLI/config-file), not core-level.
type RunOptions struct {
	// Direct-run mode: if DiscPath is set, skip the frontend's game
	// browser and load this disc immediately.
	DiscPath string
	BIOS     map[string][]byte // key -> raw BIOS bytes, matches uiface.BIOSOption.Key
	Options  map[string]string // core option key -> value, e.g. "fast_boot": "true"

	KeyBindOverrides []KeyBindOverride

	// Port1Accessory/Port2Accessory select which peripheral is
	// connected to each SMPC port. Zero-value PortAccessory means
	// "pad" (standard digital pad, always available).
	Port1Accessory PortAccessory
	Port2Accessory PortAccessory

	// Calibration: device SDL ID -> channel name ("ax"/"ay"/"ar"/"al"/
	// "az") -> AxisCalibration, produced by the settings screen's
	// interactive calibration wizard (see calibration.go). Never
	// auto-populated or guessed -- every entry traces to an explicit
	// rest/min/max sample the person performed. A device/channel with
	// no entry here is treated as uncalibrated: its analog value stays
	// at the safe rest byte (0x80 for sticks, 0x00 for triggers)
	// rather than an approximated reading.
	Calibration map[string]map[string]AxisCalibration

	// InitialWindow is applied once at startup. A zero-value
	// Width/Height means "use the Manager's own default sizing"
	// (typically derived from uiface.SystemInfo), not a literal 0x0
	// window.
	InitialWindow WindowState

	// OnWindowStateChanged, if set, is called exactly once, after the
	// window has closed and immediately before RunDirect/Run returns,
	// with the final geometry/mode. It runs on the same goroutine that
	// called RunDirect/Run (the UI thread), after the render loop has
	// fully stopped — never concurrently with anything else, so a
	// caller persisting this to disk needs no synchronization.
	OnWindowStateChanged func(WindowState)

	// OnSettingsChanged, if set, is called each time the user leaves
	// the in-app settings screen (item 8) with any change made during
	// that visit. Like OnWindowStateChanged, it runs on the UI thread,
	// so a caller persisting this to disk needs no synchronization —
	// but unlike OnWindowStateChanged it can fire multiple times per
	// run (once per settings-screen visit), not just once at shutdown.
	OnSettingsChanged func(SettingsSnapshot)
}

// Manager owns the window, the render/input/audio loop, and the
// emulation-core goroutine's lifecycle. Every method must be safe to
// call from the OS main thread only (windowing toolkits generally
// require this) — Manager is responsible for marshaling any
// core-thread communication itself.
type Manager interface {
	// SetAppIcon sets the window/taskbar icon. Must be called before Run.
	SetAppIcon(icon []byte)

	// Run opens the full frontend (game browser, config screens, etc).
	// opts.DiscPath/BIOS are ignored (the browser is exactly how the
	// user picks those); InitialWindow, KeyBindOverrides, Options, and
	// OnWindowStateChanged all still apply. Blocks until the window is
	// closed.
	Run(factory uiface.Factory, opts RunOptions) error

	// RunDirect skips the browser and loads discPath immediately with
	// the given BIOS/core options. Blocks until the window is closed.
	RunDirect(factory uiface.Factory, opts RunOptions) error
}
