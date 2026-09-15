// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

// Package screenscraper is a client for the ScreenScraper.fr metadata
// API (api2/*.php).
//
// VERIFICATION NOTE, read before extending this package: the request
// side (endpoints, parameter names, base URL, the documented `media`
// type-key catalogue, and the recommended call order
// ssinfraInfos->ssuserInfos->jeuRecherche->jeuInfos->media*) is
// cross-confirmed across multiple independent sources: several
// third-party client implementations in different languages
// (muldjord/skyscraper in C++, anibaldeboni/screech in Go,
// gboquizosanchez/screenscraper in PHP, SaraVieira/screenscraper-js,
// dsolonenko/screenscraper-api-dart) that all agree on the same
// parameter names, plus a documentation-focused third-party API
// reference. The response ENVELOPE (header/response/serveurs/ssuser)
// is transcribed from anibaldeboni/screech's actual Go source
// (pkg.go.dev-rendered struct definitions), not guessed.
//
// The `jeu` (game) object ScreenScraper returns from jeuInfos.php is
// large (names/synopsis/dates per region, genres, developer,
// publisher, ratings, classifications, and a media list) and this
// package does NOT claim a verified field-for-field struct for all of
// it -- I could not independently confirm its exact JSON field names
// without making an authenticated call using real developer
// credentials, which cassini's design deliberately doesn't have (see
// config.ScreenScraperCreds: those are the end user's own credentials,
// filled in via the UI, not something available at development time).
// Rather than guess field names -- which would silently unmarshal to
// zero values if wrong, the worst kind of failure -- Game keeps the
// full raw JSON (RawJSON) alongside a small set of fields whose shape
// (a region-keyed array of {region, text}) is consistently documented
// across every client surveyed above, which is the strongest
// confidence available short of a live authenticated call. Extend
// Game's typed fields against a real captured response (log
// RawJSON once you have working credentials) rather than from memory.
package screenscraper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

const baseURL = "https://www.screenscraper.fr/api2/"

// Softname is cassini's own client identifier, reported on every
// request per ScreenScraper's convention (every client surveyed sends
// a fixed per-application softname, not a per-user value).
const Softname = "cassini"

// SaturnSystemID is ScreenScraper's systemeid for the Sega Saturn,
// confirmed by cross-checking four independent pages on
// screenscraper.fr itself, all listing "Saturn" under
// plateforme=22: systemeinfos.php?plateforme=22,
// romsinfos.php?plateforme=22, gamesinfos.php?plateforme=22, and a
// gameinfos.php?plateforme=22 game detail page.
const SaturnSystemID = 22

// STVSystemID is intentionally not hardcoded here: I was not able to
// independently verify ScreenScraper's numeric systemeid for ST-V
// (Sega Titan Video) the way SaturnSystemID was confirmed above.
// Rather than guess a plausible-looking number, callers should
// resolve it at runtime via Client.Systems() + ResolveSystemID("ST-V")
// (see systems.go) and cache the result -- which is also the more
// robust approach in general, since it stays correct if ScreenScraper
// ever renumbers a system, rather than trusting a number frozen into
// source code.

// Credentials mirrors config.ScreenScraperCreds field-for-field (kept
// as a separate type so this package has no dependency on package
// config -- the same "who depends on whom" discipline used between
// package ui and package config).
type Credentials struct {
	Username    string // ssid
	Password    string // sspassword
	DevID       string
	DevPassword string
}

// Client is a ScreenScraper API client. Safe for concurrent use (it
// holds no mutable state beyond the http.Client, which is itself
// safe for concurrent use).
type Client struct {
	HTTP  *http.Client
	Creds Credentials
}

// New returns a Client. If httpClient is nil, http.DefaultClient is
// used.
func New(creds Credentials, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{HTTP: httpClient, Creds: creds}
}

// authParams returns the auth/identity query parameters every
// endpoint requires, per the request-side sources cited in the
// package doc comment.
func (c *Client) authParams() url.Values {
	v := url.Values{}
	v.Set("devid", c.Creds.DevID)
	v.Set("devpassword", c.Creds.DevPassword)
	v.Set("softname", Softname)
	v.Set("output", "json")
	if c.Creds.Username != "" {
		v.Set("ssid", c.Creds.Username)
	}
	if c.Creds.Password != "" {
		v.Set("sspassword", c.Creds.Password)
	}
	return v
}

// Header is ScreenScraper's response envelope header, transcribed
// from anibaldeboni/screech's verified Go struct.
type Header struct {
	APIVersion       string `json:"APIversion"`
	DateTime         string `json:"dateTime"`
	CommandRequested string `json:"commandRequested"`
	Success          string `json:"success"`
	Error            string `json:"error"`
}

// envelope is the {"header":..., "response":...} wrapper every
// ScreenScraper endpoint returns.
type envelope struct {
	Header   Header          `json:"header"`
	Response json.RawMessage `json:"response"`
}

// APIError is returned when ScreenScraper's own header reports
// failure (header.success != "true" / non-empty header.error). The
// message is ScreenScraper's own text (often French), surfaced as-is
// rather than translated or pattern-matched into a sentinel error
// type this package isn't confident it can classify correctly across
// every phrasing ScreenScraper uses (documented failure modes seen
// across client source comments include: closed for non-members,
// blacklisted client, daily quota exceeded, thread limit exceeded --
// but I don't have a verified exhaustive/stable list of the exact
// header.error strings for each, so callers needing to distinguish
// these programmatically should match on Message themselves against
// what they observe, rather than this package guessing wrong
// sentinels).
type APIError struct {
	Header Header
}

func (e *APIError) Error() string {
	if e.Header.Error != "" {
		return fmt.Sprintf("screenscraper: %s", e.Header.Error)
	}
	return "screenscraper: request failed (no error detail in response header)"
}

// do performs a GET against endpoint with params merged onto
// authParams(), decodes the envelope, and returns the raw
// response.Response payload for the caller to unmarshal further
// (each endpoint's response shape differs).
func (c *Client) do(ctx context.Context, endpoint string, params url.Values) (json.RawMessage, error) {
	v := c.authParams()
	for key, vals := range params {
		for _, val := range vals {
			v.Add(key, val)
		}
	}

	u := baseURL + endpoint + "?" + v.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("screenscraper: build request: %w", err)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("screenscraper: %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("screenscraper: %s: read response: %w", endpoint, err)
	}

	// ScreenScraper is documented (across several client
	// implementations' error handling) to sometimes return a
	// completely empty body or a non-JSON plaintext error under
	// overload -- detect that distinctly from a JSON parse failure so
	// callers can tell "server hiccup, retry" from "our request was
	// malformed".
	if len(body) == 0 {
		return nil, fmt.Errorf("screenscraper: %s: empty response body (server overloaded or rejected the request before producing JSON)", endpoint)
	}

	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("screenscraper: %s: response was not valid JSON (first 200 bytes: %.200s): %w", endpoint, body, err)
	}

	if env.Header.Success != "" && env.Header.Success != "true" {
		return nil, &APIError{Header: env.Header}
	}
	return env.Response, nil
}

// romTypeParam and helpers used by multiple endpoints.
func intParam(v url.Values, key string, n int) {
	if n > 0 {
		v.Set(key, strconv.Itoa(n))
	}
}
