// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

// Package config handles cassini's persisted configuration:
// window state, keybind overrides, BIOS/ROM directory paths,
// ScreenScraper credentials, and presentation options. Stdlib only
// (encoding/json, os, path/filepath) — no third-party dependency for
// something this simple.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const currentVersion = 1

// WindowState mirrors ui.WindowState field-for-field. Duplicated
// rather than imported so package config has zero dependency on
// package ui (config is loaded before any UI framework is selected,
// and should stay that way — a future non-ebiten Manager still uses
// this same config format unchanged).
type WindowState struct {
	Width      int  `json:"width"`
	Height     int  `json:"height"`
	X          int  `json:"x"`
	Y          int  `json:"y"`
	Maximized  bool `json:"maximized"`
	Fullscreen bool `json:"fullscreen"`
}

// KeyBind mirrors ui.KeyBindOverride.
type KeyBind struct {
	Player   int    `json:"player"`
	ButtonID int    `json:"button_id"`
	Key      string `json:"key,omitempty"`
	Pad      string `json:"pad,omitempty"`
}

// ScreenScraperCreds holds ScreenScraper.fr credentials. Both the
// per-user login (Username/Password, created free on screenscraper.fr)
// and the per-application dev credentials (DevID/DevPassword, issued
// to a registered application) are required by their API — see the
// ScreenScraper client (item 5) for how these are actually used.
//
// Stored in plaintext in a local, single-user config file, consistent
// with how every other desktop emulator frontend (RetroArch,
// Launchbox, etc.) stores these same credentials. This file is not
// intended to be shared, synced to a public repo, or committed to
// version control — Save creates it with 0600 permissions
// specifically because of this.
type ScreenScraperCreds struct {
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
	DevID       string `json:"dev_id,omitempty"`
	DevPassword string `json:"dev_password,omitempty"`
}

// Paths holds directories cassini scans and caches into.
type Paths struct {
	ROMDirs  []string `json:"rom_dirs"`
	BIOSDirs []string `json:"bios_dirs"`
	CartDirs []string `json:"cart_dirs"` // KOF95/Ultraman ROM cart images; see romcartdb -- auto-selected, never exposed as a per-game UI choice
	CacheDir string   `json:"cache_dir"`
}

// Display holds presentation defaults, using the exact same value
// strings core.Emulator.SetOption and ui/ebiten's presentState.apply
// both already parse (see emulator.go's SetOption switch and
// present.go's apply switch) — one vocabulary, no translation layer,
// so there is no place for the config format and the runtime option
// format to drift apart.
type Display struct {
	AspectRatio      string `json:"aspect_ratio"`    // "4x3" | "16x9" | "integer" | "stretch"
	DisplayShader    string `json:"display_shader"`  // "normal" | "crt"
	UpscalerFilter   string `json:"upscaler_filter"` // "nearest" | "bilinear" | "bicubic" | "hq2x" | "hq4x" | "xbrz"
	CRTBloom         bool   `json:"crt_bloom"`
	CRTCurvature     bool   `json:"crt_curvature"`
	ScaleFactor      string `json:"scale_factor"` // "1x".."4x"
	TextureFiltering bool   `json:"texture_filtering"`
}

// Input holds settings the settings screen (item 8) writes to,
// backed entirely by SetOption keys confirmed to exist in
// emulator.go's real switch statement (lightgun_mode, lightgun_port)
// -- no speculative fields for accessories the core doesn't actually
// support (see the item 8 discussion: 3D pad, mouse, and RAM cart
// size selection all have no backend today, so nothing for them lives
// here yet).
// PortAccessory selects which peripheral is connected to one SMPC
// port. Type values match uiface.Emulator's Set*Mode method names
// (lowercased): "pad" (default), "3dpad", "missionstick", "racing",
// "mouse", "keyboard", "md3", "md6". All backed by real, verified
// core support (see smpc.go.patch.txt) -- no speculative types for
// anything without a real backend.
type PortAccessory struct {
	Type           string `json:"type"`
	KeyboardLayout string `json:"keyboard_layout,omitempty"` // "jp" | "western", only used when Type == "keyboard"
}

// AxisCalibration is one calibrated raw-axis-to-Saturn-channel
// mapping, captured via the interactive calibration wizard (never
// auto-detected/guessed at runtime -- see calibration.go). RawAxis is
// the host device's raw axis index (ebiten.GamepadAxisType as an
// int); Rest/Min/Max are the raw [-1.0,1.0] values sampled during
// calibration at, respectively, the resting position, one extreme,
// and the other extreme (for a unidirectional trigger, Rest and Min
// coincide -- the wizard only prompts for two points in that case,
// see calibration.go's TriggerChannel flag).
type AxisCalibration struct {
	RawAxis  int     `json:"raw_axis"`
	Rest     float64 `json:"rest"`
	Min      float64 `json:"min"`
	Max      float64 `json:"max"`
	Inverted bool    `json:"inverted"`
}

// Calibration maps a device (keyed by ebiten.GamepadSDLID, a stable
// per-physical-device identifier -- NOT ebiten.GamepadID, which is
// only a runtime connection-order index that changes across reconnects
// and sessions) to its calibrated channels (keyed by logical channel
// name: "ax", "ay", "ar", "al", "az" -- shared across accessory types,
// since these represent the same physical gesture on the same
// physical device regardless of which Saturn peripheral is currently
// selected).
type Calibration map[string]map[string]AxisCalibration

type Input struct {
	LightgunEnabled bool          `json:"lightgun_enabled"`
	LightgunPort    string        `json:"lightgun_port"` // "1" or "2"
	Port1Accessory  PortAccessory `json:"port1_accessory"`
	Port2Accessory  PortAccessory `json:"port2_accessory"`
}

// Config is the full persisted configuration.
type Config struct {
	Version int `json:"version"`

	Window        WindowState        `json:"window"`
	Keybinds      []KeyBind          `json:"keybinds"`
	Input         Input              `json:"input"`
	Calibration   Calibration        `json:"calibration"` // device SDL ID -> channel -> AxisCalibration, see calibration.go
	ScreenScraper ScreenScraperCreds `json:"screenscraper"`
	Paths         Paths              `json:"paths"`
	Display       Display            `json:"display"`
	FastBoot      bool               `json:"fast_boot"`

	// LastDiscPath/LastBIOSPath are used only when Run() (the browser)
	// is entered with no CLI override, to reopen the last-played game.
	// They are never used to silently override an explicit -disc/-bios
	// flag — see ResolveRunOptions.
	LastDiscPath string `json:"last_disc_path,omitempty"`
	LastBIOSPath string `json:"last_bios_path,omitempty"`
}

// Default returns cassini's built-in configuration, used for a fresh
// install and as the base that Load unmarshals on top of (so a config
// file from an older version missing newer fields still gets sane
// values for them instead of Go zero-values).
func Default() *Config {
	return &Config{
		Version: currentVersion,
		Window: WindowState{
			Width: 0, Height: 0, // 0 => Manager picks a size from SystemInfo
			X: -1, Y: -1, // -1 => let the OS/WM place the window
		},
		Paths: Paths{
			CacheDir: defaultCacheDir(),
		},
		Display: Display{
			AspectRatio:    "4x3",
			DisplayShader:  "normal",
			UpscalerFilter: "nearest",
			ScaleFactor:    "1x",
		},
		Input: Input{
			LightgunEnabled: false,
			LightgunPort:    "1",
			Port1Accessory:  PortAccessory{Type: "pad"},
			Port2Accessory:  PortAccessory{Type: "pad"},
		},
		FastBoot:    true,
		Calibration: Calibration{},
	}
}

func defaultCacheDir() string {
	dir, err := os.UserCacheDir()
	if err != nil || dir == "" {
		return "cassini-cache"
	}
	return filepath.Join(dir, "cassini")
}

// DefaultPath returns the standard per-OS location for cassini's
// config file: os.UserConfigDir()/cassini/config.json, e.g.
// ~/.config/cassini/config.json on Linux, %AppData%\cassini\config.json
// on Windows, ~/Library/Application Support/cassini/config.json on
// macOS (all via the stdlib's own OS-specific logic in UserConfigDir).
// Falls back to ./cassini-config.json if the OS reports no config dir
// at all (unusual, but Load/Save must still function).
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return "cassini-config.json"
	}
	return filepath.Join(dir, "cassini", "config.json")
}

// Load reads and parses the config file at path. A missing file is
// not an error — it returns Default() so first-run behaves correctly
// with no setup required. A file that exists but fails to parse *is*
// an error: silently discarding a corrupt config (which might contain
// a user's keybind remaps or ScreenScraper credentials) is worse than
// surfacing it and letting the caller decide (main.go logs and falls
// back to Default(), see item 10's final wiring).
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Default(), nil
		}
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	cfg := Default() // unmarshal on top of defaults, see doc comment above
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	cfg.Version = currentVersion // always write forward; no migration needed yet since this is v1
	normalize(cfg)
	return cfg, nil
}

// normalize clamps/repairs values that could otherwise crash or
// misbehave downstream if a config file was hand-edited badly (a
// negative window size, an empty-but-present AspectRatio string from
// an old partial write, etc). This is deliberately conservative: it
// only replaces values that would be actively invalid, never values
// that are merely unusual.
func normalize(cfg *Config) {
	if cfg.Window.Width < 0 {
		cfg.Window.Width = 0
	}
	if cfg.Window.Height < 0 {
		cfg.Window.Height = 0
	}
	if cfg.Window.X < -1 {
		cfg.Window.X = -1
	}
	if cfg.Window.Y < -1 {
		cfg.Window.Y = -1
	}
	if cfg.Display.AspectRatio == "" {
		cfg.Display.AspectRatio = "4x3"
	}
	if cfg.Display.DisplayShader == "" {
		cfg.Display.DisplayShader = "normal"
	}
	if cfg.Display.UpscalerFilter == "" {
		cfg.Display.UpscalerFilter = "nearest"
	}
	if cfg.Display.ScaleFactor == "" {
		cfg.Display.ScaleFactor = "1x"
	}
	if cfg.Paths.CacheDir == "" {
		cfg.Paths.CacheDir = defaultCacheDir()
	}
	if cfg.Calibration == nil {
		cfg.Calibration = Calibration{}
	}
	if cfg.Input.LightgunPort != "1" && cfg.Input.LightgunPort != "2" {
		cfg.Input.LightgunPort = "1"
	}
	validAccessory := map[string]bool{
		"pad": true, "3dpad": true, "missionstick": true, "racing": true,
		"mouse": true, "keyboard": true, "md3": true, "md6": true,
	}
	if !validAccessory[cfg.Input.Port1Accessory.Type] {
		cfg.Input.Port1Accessory.Type = "pad"
	}
	if !validAccessory[cfg.Input.Port2Accessory.Type] {
		cfg.Input.Port2Accessory.Type = "pad"
	}
	if cfg.Input.Port1Accessory.KeyboardLayout != "jp" {
		cfg.Input.Port1Accessory.KeyboardLayout = "western"
	}
	if cfg.Input.Port2Accessory.KeyboardLayout != "jp" {
		cfg.Input.Port2Accessory.KeyboardLayout = "western"
	}
}

// Save writes cfg to path atomically: it writes to a temp file in the
// same directory, syncs it, then renames it over path. The rename is
// atomic on every platform Go supports (POSIX rename(2); Windows
// MoveFileEx with MOVEFILE_REPLACE_EXISTING via os.Rename), so a crash
// or power loss mid-write can never leave a half-written, unparseable
// config file — the reader only ever sees the old complete file or the
// new complete file, never a mix.
//
// The file is created with 0600 (owner read/write only) because it
// can contain ScreenScraper credentials in plaintext.
func Save(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("config: create dir %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".config-*.json.tmp")
	if err != nil {
		return fmt.Errorf("config: create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	// If anything below fails before the rename, remove the temp file
	// rather than leaving stray .tmp files behind on every failed save.
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("config: write temp file: %w", err)
	}
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return fmt.Errorf("config: chmod temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("config: sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("config: close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("config: rename into place: %w", err)
	}
	return nil
}
