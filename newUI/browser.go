// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package ebiten

import (
	"bytes"
	"context"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/basicfont"

	"github.com/jkind73/cassini/biosdb"
	"github.com/jkind73/cassini/cache"
	"github.com/jkind73/cassini/discimg"
)

// browserFace wraps the stdlib-adjacent golang.org/x/image/font/basicfont
// bitmap font via text/v2's NewGoXFace, so the browser needs no
// embedded font asset at all -- no license question, no font file to
// ship, and a fixed-width bitmap face is genuinely fine for a game
// list (not a typography-sensitive UI). NewGoXFace and the
// text.Draw/DrawOptions shapes used throughout this file were
// verified directly against hajimehoshi/ebiten's real source
// (text/v2/gox.go, text/v2/layout.go), not assumed from memory.
var browserFace = text.NewGoXFace(basicfont.Face7x13)

// browserEntry is one row in the game list.
type browserEntry struct {
	ProductNumber string
	Title         string
	Region        string
	Path          string
	Source        string // cache.Game.Source ("screenscraper"/"retroarch"/"mame"/"ipbin") -- shown so the user can see provenance
	CoverURL      string // taken directly from ScreenScraper's own media URL (cache.Media.SourceURL) -- already a complete, fetchable URL, no reconstruction needed
	CoverImage    *ebiten.Image
	coverLoading  bool
}

type browserState struct {
	m *manager

	entries  []browserEntry
	selected int
	status   string // "Scanning...", "N game(s) found", an error, etc

	entryCh chan browserEntry
	coverCh chan coverResult

	upDown, downDown, enterDown bool // edge-detect state for nav keys
}

type coverResult struct {
	index int
	img   image.Image
}

func newBrowserState(m *manager) *browserState {
	return &browserState{
		m:       m,
		entryCh: make(chan browserEntry, 32),
		coverCh: make(chan coverResult, 4),
		status:  "Scanning...",
		selected: -1,
	}
}

// startScan runs the ROM scan and metadata resolution on a background
// goroutine (NOT the core thread -- no emulator instance exists yet in
// browser mode, so there is no cycle-accuracy concern here; this is
// ordinary background I/O, same category as the existing config/cache
// file I/O). Results stream back to the UI thread via entryCh, drained
// non-blockingly in update().
func (b *browserState) startScan() {
	go func() {
		discs, err := discimg.Scan(b.m.lib.ROMDirs, b.m.factory)
		if err != nil {
			b.entryCh <- browserEntry{Title: "(scan error: " + err.Error() + ")"}
			return
		}

		var resolver *cache.Resolver
		if b.m.lib.Cache != nil {
			resolver = cache.NewResolver(b.m.lib.Cache, b.m.lib.Scraper)
		}

		for _, d := range discs {
			if !d.HasDiscInfo {
				continue // not a recognized Saturn disc -- nothing to list
			}
			entry := browserEntry{
				ProductNumber: d.DiscInfo.ProductNumber,
				Title:         d.DiscInfo.Title,
				Path:          d.Path,
				Source:        "ipbin",
			}
			if resolver != nil {
				if g, err := resolver.ResolveDisc(context.Background(), d); err == nil {
					entry.Title, entry.Region, entry.Source = g.Title, g.Region, g.Source
				}
				if media, err := b.m.lib.Cache.MediaForGame(entry.ProductNumber); err == nil {
					for _, med := range media {
						if med.MediaType == "box-2D" || med.MediaType == "wheel-hd" {
							entry.CoverURL = med.SourceURL
							break
						}
					}
				}
			}
			b.entryCh <- entry
		}
	}()
}

// loadCover fetches entry i's cover art (from a local cache file if
// present, else directly from e.CoverURL -- the exact URL
// ScreenScraper's own response gave us, see the resolver's
// cacheMedia; no URL reconstruction happens here) and decodes it,
// sending the result to coverCh for the UI thread to turn into an
// *ebiten.Image in update(). Runs on its own goroutine per call,
// triggered lazily by selection (called every update() tick, but
// guarded by coverLoading/CoverImage so it only actually starts a
// fetch once per entry) rather than eagerly for the whole list, to
// avoid downloading art for games the user never scrolls to.
func (b *browserState) loadCover(i int) {
	if i < 0 || i >= len(b.entries) {
		return
	}
	e := &b.entries[i]
	if e.CoverImage != nil || e.coverLoading || e.CoverURL == "" {
		return
	}
	e.coverLoading = true

	go func(idx int, url, productNumber string) {
		localPath := ""
		if b.m.lib.MediaDir != "" {
			localPath = filepath.Join(b.m.lib.MediaDir, productNumber+".img")
		}

		var data []byte
		if localPath != "" {
			if d, err := os.ReadFile(localPath); err == nil {
				data = d
			}
		}
		if data == nil {
			resp, err := http.Get(url)
			if err != nil {
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return
			}
			var buf bytes.Buffer
			if _, err := buf.ReadFrom(resp.Body); err != nil {
				return
			}
			data = buf.Bytes()
		}

		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return
		}
		if localPath != "" {
			os.WriteFile(localPath, data, 0644)
		}
		b.coverCh <- coverResult{index: idx, img: img}
	}(i, e.CoverURL, e.ProductNumber)
}

func (b *browserState) update(g *game) error {
	// Drain newly-scanned entries and loaded cover art.
drain:
	for {
		select {
		case e := <-b.entryCh:
			b.entries = append(b.entries, e)
			if b.selected < 0 {
				b.selected = 0
			}
			b.status = strconv.Itoa(len(b.entries)) + " game(s) found"
		case cr := <-b.coverCh:
			if cr.index >= 0 && cr.index < len(b.entries) {
				b.entries[cr.index].CoverImage = ebiten.NewImageFromImage(cr.img)
			}
		default:
			break drain
		}
	}

	if len(b.entries) == 0 {
		return nil
	}

	up := ebiten.IsKeyPressed(ebiten.KeyArrowUp) || ebiten.IsKeyPressed(ebiten.KeyW)
	down := ebiten.IsKeyPressed(ebiten.KeyArrowDown) || ebiten.IsKeyPressed(ebiten.KeyS)
	enter := ebiten.IsKeyPressed(ebiten.KeyEnter) || ebiten.IsKeyPressed(ebiten.KeySpace)

	if up && !b.upDown {
		b.selected--
	}
	if down && !b.downDown {
		b.selected++
	}
	if b.selected < 0 {
		b.selected = 0
	}
	if b.selected >= len(b.entries) {
		b.selected = len(b.entries) - 1
	}
	b.upDown, b.downDown = up, down

	b.loadCover(b.selected)

	if enter && !b.enterDown {
		b.launch(g, b.entries[b.selected])
	}
	b.enterDown = enter

	return nil
}

// launch runs BIOS auto-assignment (biosdb.SelectForRegion, item 3)
// for the selected disc's region and starts a session.
func (b *browserState) launch(g *game, e browserEntry) {
	biosFound, err := biosdb.Scan(b.m.lib.BIOSDirs)
	if err != nil {
		b.status = "BIOS scan failed: " + err.Error()
		return
	}

	region := biosdb.RegionUnknown
	switch e.Region {
	case "jp", "asia":
		region = biosdb.RegionJapan
	case "us":
		region = biosdb.RegionUSA
	case "eu":
		region = biosdb.RegionEurope
	}

	chosen, ok := biosdb.SelectForRegion(biosFound, biosdb.SystemSaturnConsole, region)
	if !ok {
		b.status = "No compatible Saturn BIOS found in configured BIOS directories"
		return
	}

	data, err := readBIOSBytes(chosen)
	if err != nil {
		b.status = "Failed to read BIOS: " + err.Error()
		return
	}

	if err := g.startSession(e.Path, map[string][]byte{"main_bios": data}, g.baseOptions); err != nil {
		b.status = "Failed to start: " + err.Error()
	}
}

// readBIOSBytes reads a biosdb.Found's underlying bytes, handling the
// zip-archived case (Inzip != "") the same way biosdb.Scan discovered
// it -- reused here rather than re-hashed, since biosdb.Scan already
// did the identification work.
func readBIOSBytes(f biosdb.Found) ([]byte, error) {
	if f.Inzip == "" {
		return os.ReadFile(f.Path)
	}
	return readZipEntry(f.Path, f.Inzip)
}

func (b *browserState) draw(screen *ebiten.Image, g *game) {
	screen.Fill(color.RGBA{0x14, 0x14, 0x18, 0xff})

	titleOp := &text.DrawOptions{}
	titleOp.GeoM.Translate(20, 20)
	titleOp.ColorScale.ScaleWithColor(color.RGBA{0xe0, 0xe0, 0xe0, 0xff})
	text.Draw(screen, "cassini", browserFace, titleOp)

	statusOp := &text.DrawOptions{}
	statusOp.GeoM.Translate(20, 40)
	statusOp.ColorScale.ScaleWithColor(color.RGBA{0x90, 0x90, 0x90, 0xff})
	text.Draw(screen, b.status, browserFace, statusOp)

	const rowHeight = 20
	const listX = 20
	const listY = 70
	const coverW = 120

	for i, e := range b.entries {
		y := listY + i*rowHeight
		if y > g.winH {
			break // don't draw (or waste time measuring) rows below the visible window
		}
		label := e.Title
		if label == "" {
			label = e.ProductNumber
		}

		if i == b.selected {
			marker := ebiten.NewImage(6, rowHeight-4)
			marker.Fill(color.RGBA{0x4a, 0x9e, 0xff, 0xff})
			markerOp := &ebiten.DrawImageOptions{}
			markerOp.GeoM.Translate(float64(listX+coverW), float64(y))
			screen.DrawImage(marker, markerOp)
		}

		rowOp := &text.DrawOptions{}
		rowOp.GeoM.Translate(float64(listX+coverW+10), float64(y))
		col := color.RGBA{0xe0, 0xe0, 0xe0, 0xff}
		if i == b.selected {
			col = color.RGBA{0x4a, 0x9e, 0xff, 0xff}
		}
		rowOp.ColorScale.ScaleWithColor(col)
		text.Draw(screen, label, browserFace, rowOp)
	}

	// Cover art for the selected entry only, drawn once loaded.
	if b.selected >= 0 && b.selected < len(b.entries) {
		if img := b.entries[b.selected].CoverImage; img != nil {
			iw := img.Bounds().Dx()
			scale := float64(coverW) / float64(iw)
			coverOp := &ebiten.DrawImageOptions{}
			coverOp.GeoM.Scale(scale, scale)
			coverOp.GeoM.Translate(float64(listX), float64(listY))
			screen.DrawImage(img, coverOp)
		}
	}
}

// (game counts use strconv.Itoa — input.go's itoa helper is
// intentionally narrow, 0-99 only, sized for its F-key use case, and
// would silently truncate a 100+ game library's count)
