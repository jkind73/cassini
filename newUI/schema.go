// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package cache

// schema is applied via a single idempotent migration on Open (see
// cache.go). SQLite's own "CREATE TABLE IF NOT EXISTS" makes this
// safe to re-run on every startup rather than needing a separate
// migration-tracking table -- appropriate for a cache (as opposed to
// a durable store where losing/rebuilding the DB would be costly:
// this DB is 100% derivable by re-scanning+re-scraping, so "just
// re-run the DDL" is sufficient rather than needing full migration
// tooling).
const schema = `
-- games: PRIMARY KEY is the disc's IP.BIN product number
-- (uiface.DiscInfo.ProductNumber), per instruction. This works
-- offline with zero network/hash-database dependency the moment a
-- disc is scanned (discimg.Scan already reads it via the existing,
-- unmodified Factory.DiscInfo), and is stable across re-dumps/re-rips
-- of the same disc, unlike a file hash which changes if someone
-- re-compresses to CHD or re-rips with different sector padding.
CREATE TABLE IF NOT EXISTS games (
	product_number   TEXT PRIMARY KEY,
	title            TEXT NOT NULL,
	region           TEXT NOT NULL DEFAULT '',
	disc_number      INTEGER NOT NULL DEFAULT 1,
	disc_total       INTEGER NOT NULL DEFAULT 1,

	-- source: 'screenscraper' | 'retroarch' | 'mame' | 'ipbin' | 'manual'
	-- Records which tier of the fallback chain (see resolver.go)
	-- produced this row's title, so the UI can show provenance and a
	-- later re-scrape can prefer upgrading a low-confidence source
	-- (ipbin/retroarch/mame) to a high-confidence one (screenscraper)
	-- without clobbering a manual user edit.
	source           TEXT NOT NULL,

	screenscraper_id TEXT NOT NULL DEFAULT '',
	screenscraper_json BLOB,

	path             TEXT NOT NULL DEFAULT '',
	updated_at       INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE INDEX IF NOT EXISTS idx_games_screenscraper_id ON games(screenscraper_id) WHERE screenscraper_id != '';

-- arcade_sets: ST-V cartridge titles have no IP.BIN/product-number
-- scheme (they're ROM carts, not discs), so they get their own table
-- keyed by MAME's own romset shortname instead -- the natural primary
-- key for anything identified via the MAME fallback (mame_listxml.go).
CREATE TABLE IF NOT EXISTS arcade_sets (
	mame_shortname   TEXT PRIMARY KEY,
	title            TEXT NOT NULL,
	year             TEXT NOT NULL DEFAULT '',
	manufacturer     TEXT NOT NULL DEFAULT '',
	source           TEXT NOT NULL,
	screenscraper_id TEXT NOT NULL DEFAULT '',
	screenscraper_json BLOB,
	path             TEXT NOT NULL DEFAULT '',
	updated_at       INTEGER NOT NULL DEFAULT (unixepoch())
);

-- media: box art / wheel / screenshot / etc, keyed off whichever of
-- games.product_number or arcade_sets.mame_shortname the asset
-- belongs to (exactly one of the two FK-style columns is set,
-- enforced by the CHECK below rather than two separate media tables,
-- since the row shape is otherwise identical).
CREATE TABLE IF NOT EXISTS media (
	product_number TEXT NOT NULL DEFAULT '',
	mame_shortname TEXT NOT NULL DEFAULT '',
	media_type     TEXT NOT NULL,
	region         TEXT NOT NULL DEFAULT '',
	local_path     TEXT NOT NULL DEFAULT '',
	source_url     TEXT NOT NULL DEFAULT '',
	cached_at      INTEGER NOT NULL DEFAULT (unixepoch()),
	PRIMARY KEY (product_number, mame_shortname, media_type, region),
	CHECK ((product_number != '') != (mame_shortname != ''))
);

-- retroarch_dat_entries / retroarch_dat_roms: imported once from a
-- user-supplied libretro-database "Sega - Saturn.dat" (ClrMamePro DAT
-- format -- see retroarch_dat.go). Matching is by ROM file hash
-- (crc32/sha1/md5), the DAT's actual designed lookup key, not by
-- product number -- see resolver.go for why.
CREATE TABLE IF NOT EXISTS retroarch_dat_entries (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	name        TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	serial      TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS retroarch_dat_roms (
	entry_id INTEGER NOT NULL REFERENCES retroarch_dat_entries(id) ON DELETE CASCADE,
	crc32    TEXT NOT NULL DEFAULT '',
	sha1     TEXT NOT NULL DEFAULT '',
	md5      TEXT NOT NULL DEFAULT '',
	size     INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_retroarch_roms_sha1  ON retroarch_dat_roms(sha1)  WHERE sha1 != '';
CREATE INDEX IF NOT EXISTS idx_retroarch_roms_crc32 ON retroarch_dat_roms(crc32) WHERE crc32 != '';

-- mame_entries / mame_entry_roms: imported once from a user-supplied
-- 'mame -listxml' (or a driver-scoped subset, e.g. 'mame stv -listxml')
-- output -- see mame_listxml.go. Matching is by ROM hash, same as
-- RetroArch DAT, which is also exactly how MAME's own -verifyroms
-- works.
CREATE TABLE IF NOT EXISTS mame_entries (
	shortname    TEXT PRIMARY KEY,
	description  TEXT NOT NULL DEFAULT '',
	year         TEXT NOT NULL DEFAULT '',
	manufacturer TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS mame_entry_roms (
	shortname TEXT NOT NULL REFERENCES mame_entries(shortname) ON DELETE CASCADE,
	crc32     TEXT NOT NULL DEFAULT '',
	sha1      TEXT NOT NULL DEFAULT '',
	size      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_mame_roms_sha1  ON mame_entry_roms(sha1)  WHERE sha1 != '';
CREATE INDEX IF NOT EXISTS idx_mame_roms_crc32 ON mame_entry_roms(crc32) WHERE crc32 != '';
`
