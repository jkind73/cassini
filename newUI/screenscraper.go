// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package config

import "github.com/jkind73/cassini/screenscraper"

// ScreenScraperCredentials bridges the persisted config fields into
// the screenscraper client's own Credentials type. All four fields
// are exactly what config.go's ScreenScraperCreds already stores --
// username/password (the user's ScreenScraper.fr member login) and
// dev ID/dev password (the per-application developer credentials) --
// entered by the user through the config UI (item 8's ScreenScraper
// panel) and never hardcoded, exactly as instructed. This function is
// the only place that translation happens, so the two types can't
// drift out of field-for-field sync silently.
func (cfg *Config) ScreenScraperCredentials() screenscraper.Credentials {
	return screenscraper.Credentials{
		Username:    cfg.ScreenScraper.Username,
		Password:    cfg.ScreenScraper.Password,
		DevID:       cfg.ScreenScraper.DevID,
		DevPassword: cfg.ScreenScraper.DevPassword,
	}
}
