// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

// Package cache is cassini's local SQLite metadata cache: scraped
// ScreenScraper results, plus RetroArch-DAT and MAME-listxml fallback
// identification, so a re-launch never needs the network to show a
// game's title again.
//
// DRIVER CHOICE, and its verification status: this uses
// modernc.org/sqlite (a pure-Go, cgo-free SQLite implementation), not
// mattn/go-sqlite3 (the more commonly seen cgo-based driver).
// Deliberate: cassini already carries one cgo dependency (chdlib, for
// CHD support) with the cross-compilation cost that entails (see
// third_party/SETUP.md); adding a second cgo dependency here for
// something SQLite-shaped, when a mature pure-Go implementation
// exists, would compound that cost for no real benefit.
//
// UNLIKE chdlib, I was not able to compile- or runtime-verify this
// package's use of modernc.org/sqlite in the environment I built this
// in -- its source isn't hosted anywhere within my available network
// access (it's not on GitHub; modernc.org packages are typically
// mirrored from gitlab.com/cznic, which wasn't reachable), so I
// couldn't clone-and-build it the way I did for libchdr. The
// database/sql usage pattern here (driver name "sqlite", standard
// sql.Open/sql.DB semantics) is the standard, well-documented way
// every database/sql driver works, and modernc.org/sqlite specifically
// advertises exactly this registration -- but treat this package as
// "written to the documented contract, not independently proven to
// compile" until you've run `go get modernc.org/sqlite && go build
// ./cache/...` yourself, unlike chdlib which I could and did prove.
package cache

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Cache wraps a *sql.DB against cassini's schema.
type Cache struct {
	db *sql.DB
}

// Open opens (creating if necessary) the SQLite database at path and
// applies schema. Uses WAL mode so concurrent reads (e.g. the browser
// UI listing games) never block a writer (e.g. a background scrape
// finishing), and foreign_keys=ON so the media/roms tables' ON DELETE
// CASCADE actually takes effect (SQLite has this off by default per
// connection).
func Open(path string) (*Cache, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("cache: open %s: %w", path, err)
	}

	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("cache: %s: %w", pragma, err)
		}
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("cache: apply schema: %w", err)
	}

	return &Cache{db: db}, nil
}

func (c *Cache) Close() error { return c.db.Close() }
