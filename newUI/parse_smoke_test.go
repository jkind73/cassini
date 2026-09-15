// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

package screenscraper

import (
	"encoding/json"
	"testing"
)

func TestParseGame_JeuWrapped(t *testing.T) {
	raw := json.RawMessage(`{"id":"360712","noms":[{"region":"wor","text":"NiGHTS into Dreams"}],"medias":[{"type":"box-2D","region":"wor","url":"https://example.invalid/box.png","format":"png"}]}`)
	wrapped := json.RawMessage(`{"jeu":` + string(raw) + `}`)

	g, err := parseGame(wrapped)
	if err != nil {
		t.Fatalf("parseGame: %v", err)
	}
	if g.ID != "360712" {
		t.Errorf("ID = %q, want 360712", g.ID)
	}
	if g.Name() != "NiGHTS into Dreams" {
		t.Errorf("Name() = %q, want %q", g.Name(), "NiGHTS into Dreams")
	}
	m, ok := g.MediaByType("box-2D")
	if !ok || m.URL != "https://example.invalid/box.png" {
		t.Errorf("MediaByType(box-2D) = %+v, ok=%v", m, ok)
	}
}

func TestParseGame_Empty(t *testing.T) {
	_, err := parseGame(json.RawMessage(`{"jeu":{}}`))
	if err != ErrGameNotFound {
		t.Errorf("err = %v, want ErrGameNotFound", err)
	}
}

func TestParseSystems(t *testing.T) {
	items := []json.RawMessage{
		json.RawMessage(`{"id":"22","nom":"Saturn"}`),
		json.RawMessage(`{"id":"75","nom":"ST-V"}`),
	}
	systems, err := parseSystems(items)
	if err != nil {
		t.Fatalf("parseSystems: %v", err)
	}
	if len(systems) != 2 || systems[0].ID != 22 || systems[0].Name() != "Saturn" {
		t.Errorf("systems = %+v", systems)
	}
	s, ok := ResolveSystemID(systems, "Saturn")
	if !ok || s.ID != 22 {
		t.Errorf("ResolveSystemID(Saturn) = %+v, ok=%v", s, ok)
	}
	if s.ID != SaturnSystemID {
		t.Errorf("resolved Saturn ID %d does not match documented SaturnSystemID %d", s.ID, SaturnSystemID)
	}
}

func TestAPIError(t *testing.T) {
	e := &APIError{Header: Header{Success: "false", Error: "Erreur : login/mdp incorrect"}}
	if e.Error() == "" {
		t.Fatal("Error() returned empty string")
	}
}
