// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package screenscraper

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// DownloadMedia fetches one media asset (box art, wheel, screenshot,
// etc) via mediaJeu.php and streams it to w. mediaType should be one
// of the documented keys from the package doc comment's table
// (optionally region-suffixed, e.g. "wheel-hd(wor)"), typically taken
// directly from a Game's Medias list (Game.MediaByType) rather than
// hand-constructed.
func (c *Client) DownloadMedia(ctx context.Context, systemID int, gameID string, mediaType string, w io.Writer) error {
	v := c.authParams()
	v.Set("systemeid", strconv.Itoa(systemID))
	v.Set("jeuid", gameID)
	v.Set("media", mediaType)

	u := baseURL + "mediaJeu.php?" + v.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("screenscraper: build media request: %w", err)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("screenscraper: download media: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("screenscraper: download media: HTTP %d", resp.StatusCode)
	}
	// Unlike the JSON endpoints, media downloads are the raw asset
	// bytes on success -- no envelope to unwrap. A non-2xx status is
	// the only failure signal available at this layer without
	// content-sniffing the body, which this function deliberately
	// doesn't attempt (an actual image/video/pdf byte stream is not
	// something to pattern-match against expected error text).
	if _, err := io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("screenscraper: download media: write: %w", err)
	}
	return nil
}

// MediaURL builds the request URL without performing the request --
// useful for a UI that wants to hand the URL to an async image loader
// rather than download synchronously through this client.
func (c *Client) MediaURL(systemID int, gameID string, mediaType string) string {
	v := c.authParams()
	v.Set("systemeid", strconv.Itoa(systemID))
	v.Set("jeuid", gameID)
	v.Set("media", mediaType)
	return baseURL + "mediaJeu.php?" + v.Encode()
}
