// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/jkind73/cassini/ui"
)

// CLIOverrides holds the subset of RunOptions that can be set from
// the command line. Every field is a pointer/optional-shaped so
// ResolveRunOptions can distinguish "flag not passed" (use config,
// then default) from "flag explicitly passed" (always wins), which a
// plain string can't do once flag.String's zero value ("") is also a
// theoretically valid path.
type CLIOverrides struct {
	DiscPath *string // -disc
	BIOSPath *string // -bios
}

// ResolveRunOptions builds the final ui.RunOptions cassini will run
// with, applying precedence CLI > config file > built-in default at
// the level of each individual field — not at the level of "did any
// flag get passed at all". This matters concretely: e.g. -disc alone
// (no -bios) still uses the *config file's* BIOS path rather than
// falling all the way back to "no BIOS", because the two flags are
// independent overrides, not an all-or-nothing pair.
func ResolveRunOptions(cfg *Config, cli CLIOverrides) (ui.RunOptions, error) {
	opts := ui.RunOptions{
		Options: map[string]string{
			"fast_boot":         strconv.FormatBool(cfg.FastBoot),
			"aspect_ratio":      cfg.Display.AspectRatio,
			"display_shader":    cfg.Display.DisplayShader,
			"upscaler_filter":   cfg.Display.UpscalerFilter,
			"crt_bloom":         strconv.FormatBool(cfg.Display.CRTBloom),
			"crt_curvature":     strconv.FormatBool(cfg.Display.CRTCurvature),
			"scale_factor":      cfg.Display.ScaleFactor,
			"texture_filtering": strconv.FormatBool(cfg.Display.TextureFiltering),
			"lightgun_mode":     strconv.FormatBool(cfg.Input.LightgunEnabled),
			"lightgun_port":     cfg.Input.LightgunPort,
		},
		Port1Accessory: ui.PortAccessory{Type: cfg.Input.Port1Accessory.Type, KeyboardLayout: cfg.Input.Port1Accessory.KeyboardLayout},
		Port2Accessory: ui.PortAccessory{Type: cfg.Input.Port2Accessory.Type, KeyboardLayout: cfg.Input.Port2Accessory.KeyboardLayout},
		Calibration:    calibrationToUI(cfg.Calibration),
		InitialWindow: ui.WindowState{
			Width: cfg.Window.Width, Height: cfg.Window.Height,
			X: cfg.Window.X, Y: cfg.Window.Y,
			Maximized: cfg.Window.Maximized, Fullscreen: cfg.Window.Fullscreen,
		},
	}

	for _, kb := range cfg.Keybinds {
		opts.KeyBindOverrides = append(opts.KeyBindOverrides, ui.KeyBindOverride{
			Player: kb.Player, ButtonID: kb.ButtonID, Key: kb.Key, Pad: kb.Pad,
		})
	}

	// DiscPath: CLI > config's LastDiscPath > "" (empty means "enter
	// the game browser instead of direct-run", handled by main.go).
	discPath := cfg.LastDiscPath
	if cli.DiscPath != nil && *cli.DiscPath != "" {
		discPath = *cli.DiscPath
	}
	opts.DiscPath = discPath

	// BIOSPath: same precedence, but a resolved path also has to
	// actually be read into bytes here (RunOptions.BIOS wants raw
	// content, not a path), and a CLI-specified path that fails to
	// read must be a hard error — silently falling back to "no BIOS"
	// when the user explicitly named a file is a much more confusing
	// failure mode than a config-file path failing (which can
	// reasonably fall back, since the user didn't just ask for it).
	biosPath := cfg.LastBIOSPath
	explicitCLIBios := false
	if cli.BIOSPath != nil && *cli.BIOSPath != "" {
		biosPath = *cli.BIOSPath
		explicitCLIBios = true
	}
	if biosPath != "" {
		data, err := os.ReadFile(biosPath)
		switch {
		case err == nil:
			opts.BIOS = map[string][]byte{"main_bios": data}
		case explicitCLIBios:
			return ui.RunOptions{}, fmt.Errorf("resolve run options: -bios %q: %w", biosPath, err)
		default:
			// Config-file path went stale (file moved/deleted) — fall
			// back to no BIOS rather than fail startup outright; the
			// BIOS auto-assignment step (item 3) will offer a proper
			// picker in the browser UI (Run), and RunDirect callers
			// without a config BIOS still get a clear "no BIOS
			// loaded" state rather than a crash.
		}
	}

	return opts, nil
}

// calibrationToUI converts Config.Calibration to ui.RunOptions'
// calibration shape (device SDL ID -> channel -> AxisCalibration).
func calibrationToUI(c Calibration) map[string]map[string]ui.AxisCalibration {
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
