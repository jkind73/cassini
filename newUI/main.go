// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	_ "embed"
	"flag"
	"log"
	"path/filepath"

	"github.com/jkind73/cassini/adapter"
	"github.com/jkind73/cassini/cache"
	"github.com/jkind73/cassini/config"
	"github.com/jkind73/cassini/screenscraper"
	"github.com/jkind73/cassini/ui"
	uiebiten "github.com/jkind73/cassini/ui/ebiten"
)

//go:embed icon.webp
var icon []byte

func main() {
	configPath := flag.String("config", config.DefaultPath(), "path to config.json")
	biosPath := flag.String("bios", "", "path to BIOS file (direct run; overrides config's last-used BIOS)")
	discPath := flag.String("disc", "", "path to disc file (direct run; overrides config's last-used disc)")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		// A missing file is not an error (config.Load returns
		// Default() for that case) — reaching here means the file
		// exists but is corrupt. Fall back to defaults rather than
		// refuse to start, but say so loudly: a silently-discarded
		// corrupt config could otherwise cost a user their keybind
		// remaps or ScreenScraper credentials without them noticing
		// until much later.
		log.Printf("config: %v (using built-in defaults for this run)", err)
		cfg = config.Default()
	}

	// CLIOverrides distinguishes "-disc not passed" from "-disc ''
	// passed" via nil vs non-nil pointers; flag.String always gives us
	// a non-nil *string, so we only take its address when the user
	// actually supplied a non-empty value. See resolve.go's doc
	// comment for why each flag is resolved independently rather than
	// as an all-or-nothing pair.
	var cli config.CLIOverrides
	if *discPath != "" {
		cli.DiscPath = discPath
	}
	if *biosPath != "" {
		cli.BIOSPath = biosPath
	}

	opts, err := config.ResolveRunOptions(cfg, cli)
	if err != nil {
		log.Fatal(err)
	}

	// Persist window geometry/mode back to config.json on exit,
	// regardless of which entry point (Run or RunDirect) was used, and
	// regardless of whether the emulator exited cleanly or the window
	// was simply closed. This is the only place cassini writes to
	// disk on shutdown, and it happens strictly after the UI thread's
	// render loop has fully stopped (see the ebiten Manager's
	// OnWindowStateChanged call site) — never concurrently with
	// anything else, so no synchronization is needed here.
	opts.OnWindowStateChanged = func(ws ui.WindowState) {
		cfg.Window = config.WindowState{
			Width: ws.Width, Height: ws.Height,
			X: ws.X, Y: ws.Y,
			Maximized: ws.Maximized, Fullscreen: ws.Fullscreen,
		}
		if err := config.Save(*configPath, cfg); err != nil {
			// Failing to persist window geometry is not fatal to the
			// session that just ended — it only affects next launch's
			// starting size/position — so this is logged, not fatal.
			log.Printf("config: failed to save %s: %v", *configPath, err)
		}
	}

	// Persist keybind/lightgun changes each time the user leaves the
	// in-app settings screen (item 8) — may fire multiple times per
	// run, unlike OnWindowStateChanged. Runs on the UI thread, same
	// synchronization-free guarantee as above.
	opts.OnSettingsChanged = func(snap ui.SettingsSnapshot) {
		cfg.Keybinds = nil
		for _, ov := range snap.KeyBindOverrides {
			cfg.Keybinds = append(cfg.Keybinds, config.KeyBind{
				Player: ov.Player, ButtonID: ov.ButtonID, Key: ov.Key, Pad: ov.Pad,
			})
		}
		cfg.Input.LightgunEnabled = snap.LightgunEnabled
		cfg.Input.LightgunPort = snap.LightgunPort
		cfg.Input.Port1Accessory = config.PortAccessory{Type: snap.Port1Accessory.Type, KeyboardLayout: snap.Port1Accessory.KeyboardLayout}
		cfg.Input.Port2Accessory = config.PortAccessory{Type: snap.Port2Accessory.Type, KeyboardLayout: snap.Port2Accessory.KeyboardLayout}
		newCal := config.Calibration{}
		for dev, channels := range snap.Calibration {
			newCal[dev] = map[string]config.AxisCalibration{}
			for ch, cal := range channels {
				newCal[dev][ch] = config.AxisCalibration{
					RawAxis: cal.RawAxis, Rest: cal.Rest, Min: cal.Min, Max: cal.Max, Inverted: cal.Inverted,
				}
			}
		}
		cfg.Calibration = newCal
		if err := config.Save(*configPath, cfg); err != nil {
			log.Printf("config: failed to save %s: %v", *configPath, err)
		}
	}

	factory := &adapter.Factory{}

	// Metadata cache (item 6) -- opened once for the process. A
	// failure here degrades the browser (no cross-session cache, no
	// RetroArch/MAME fallback lookups) rather than blocking startup:
	// cassini is still fully usable via -disc/-bios direct-run even if
	// the cache database can't be opened (e.g. a permissions problem
	// on the cache directory).
	var metaCache *cache.Cache
	if c, err := cache.Open(filepath.Join(cfg.Paths.CacheDir, "metadata.db")); err != nil {
		log.Printf("cache: failed to open metadata cache: %v (browser will run without it)", err)
	} else {
		metaCache = c
		defer metaCache.Close()
	}

	// ScreenScraper client (item 5) -- credentials are config-only,
	// filled in via the UI, never hardcoded (per instruction). A
	// Client is still constructed even with empty credentials (the
	// zero-value Credentials just means unauthenticated/rate-limited
	// requests, which ScreenScraper itself handles, not an error
	// cassini needs to detect up front) so the resolver chain's
	// ScreenScraper tier can be attempted whenever it's set later
	// without restructuring this wiring.
	scraper := screenscraper.New(cfg.ScreenScraperCredentials(), nil)

	// This is the only line that changes to swap UI frameworks.
	var mgr ui.Manager = uiebiten.New(uiebiten.Library{
		ROMDirs:  cfg.Paths.ROMDirs,
		BIOSDirs: cfg.Paths.BIOSDirs,
		CartDirs: cfg.Paths.CartDirs,
		Cache:    metaCache,
		Scraper:  scraper,
		MediaDir: filepath.Join(cfg.Paths.CacheDir, "media"),
	})
	mgr.SetAppIcon(icon)

	if opts.DiscPath != "" {
		if err := mgr.RunDirect(factory, opts); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := mgr.Run(factory, opts); err != nil {
		log.Fatal(err)
	}
}
