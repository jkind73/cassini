// Copyright 2026 The cassini Authors
// SPDX-License-Identifier: GPL-3.0-or-later

// Package uiface defines the contract between the cassini core and any
// UI frontend. It replaces github.com/user-none/eblitui/coreif as the
// interface adapter.go targets, so the core has zero dependency on
// eblitui (or any other single UI framework) at compile time.
//
// This is a mechanical port of the coreif shapes cassini already
// implements in adapter.go — field names/semantics unchanged, so
// swapping the import in adapter.go is a rename, not a rewrite.
package uiface

// Button describes one logical input with default bindings.
type Button struct {
	Name       string
	ID         int
	DefaultKey string
	DefaultPad string
}

type CoreOptionType int

const (
	CoreOptionBool CoreOptionType = iota
	CoreOptionSelect
)

type CoreOptionCategory int

const (
	CoreOptionCategoryCore CoreOptionCategory = iota
	CoreOptionCategoryInput
	CoreOptionCategoryVideo
	CoreOptionCategoryAudio
)

type CoreOption struct {
	Key         string
	Label       string
	Description string
	Type        CoreOptionType
	Default     string
	Values      []string
	Category    CoreOptionCategory
}

type BIOSVariant struct {
	Label    string
	CRC32    uint32 // MAME-convention identification hash
	SHA1     string // hex, lowercase, 40 chars — MAME-convention verification hash
	Filename string
}

type BIOSOption struct {
	Key      string
	Label    string
	Required bool
	Variants []BIOSVariant
}

type MetadataVariant struct {
	Name          string
	RDBName       string
	ThumbnailRepo string
}

// SystemInfo is static metadata the UI needs before any emulator
// instance exists (menu construction, input mapping defaults, BIOS
// picker, etc).
type SystemInfo struct {
	Name             string
	ConsoleName      string
	ScreenWidth      int
	MaxScreenHeight  int
	PixelAspectRatio float64
	SampleRate       int
	Buttons          []Button
	Players          int
	Disc             bool
	ConsoleID        int
	BigEndianMemory  bool
	DataDirName      string
	CoreName         string
	CoreVersion      string
	SerializeSize    int
	MetadataVariants []MetadataVariant
	CoreOptions      []CoreOption
	BIOSOptions      []BIOSOption
}

// Region is the disc's declared compatible-area set, parsed from the
// IP System ID's Compatible Area Symbols field ($40, 10 bytes,
// space-padded, e.g. "JTU "). A disc can declare more than one area;
// Region reports the best match against the four symbols Sega
// actually used (J/T/U/E), preferring, in order, an exact single-area
// disc, then the first matching symbol found — good enough for BIOS
// auto-assignment (biosdb.SelectForRegion), which only needs "what
// region should the BIOS match", not the disc's full compatibility
// set.
type Region int

const (
	RegionUnknown Region = iota
	RegionJapan
	RegionAsiaNTSC
	RegionUSA
	RegionEurope
)

// DiscInfo is what Factory.DiscInfo extracts from sector 0.
type DiscInfo struct {
	ProductNumber string
	DiscNumber    int
	DiscTotal     int
	Title         string
	Region        Region
}

// DiscReader is the primitive disc-image accessor. Both raw
// bin/cue-style and .chd-style backends implement this; the UI layer
// owns the concrete backend, the core only ever sees this interface.
type DiscReader interface {
	ReadSector(index int) ([]byte, error)
	NumTracks() int
	Track(n int) (start, length int, audio bool)
	NumTrackIndexes(track int) int
	TrackIndex(track, index int) int
}

// Timing is per-frame timing info the UI needs for audio/video sync
// (vsync pacing, audio buffer sizing).
type Timing struct {
	FPS       float64
	Scanlines int
}

// Emulator is the full per-instance contract. This is exactly what
// adapter.go's `emulator` type already implements against coreif —
// only the import path changes.
type Emulator interface {
	RunFrame()
	GetFramebuffer() []byte
	GetFramebufferStride() int
	GetActiveHeight() int
	GetAudioSamples() []int16
	SetInput(player int, buttons uint32)
	SetPointer(player int, x, y int, trigger bool)
	SetOption(key, value string)
	SetRom(data []byte)
	Start()
	Close()

	ReadMemory(addr uint32, buf []byte) uint32
	Serialize() ([]byte, error)
	Deserialize(data []byte) error
	SetDisc(disc DiscReader)
	SetBIOS(key string, data []byte) error
	SetROMCartridge(data []byte) // mirrors SetBIOS: host supplies bytes for KOF95/Ultraman's cart, see romcartdb

	// Accessory support, all verified byte-exact against the official
	// Sega SMPC User's Manual (see smpc.go.patch.txt for citations).
	// Each pair is Set*Mode(port, enabled[, variant]) to select the
	// accessory, plus Set*Data(...) to feed it live input, matching
	// the granular-method style the rest of this interface already
	// uses (e.g. SetPointer implicitly enabling the lightgun above).
	SetMouseMode(port int, enabled bool)
	SetMouseData(port int, left, middle, right, start bool, dx, dy int32)
	SetAnalogPadMode(port int, enabled bool)
	SetAnalogPadData(port int, buttons uint16, ax, ay, ar, al uint8)
	SetMissionStickData(port int, buttons uint16, ax, ay, az uint8)
	SetRacingMode(port int, enabled bool)
	SetRacingData(port int, buttons uint16, ax uint8)
	SetMDPadMode(port int, enabled, sixButton bool)
	SetMDPadData(port int, right, left, down, up, start, a, c, b, mode, x, y, z bool)
	SetKeyboardMode(port int, enabled bool)
	PushKeyboardEvent(port int, scancode uint8, released bool)
	GetTiming() Timing
	HasSRAM() bool
	GetSRAM() []byte
	SetSRAM(data []byte)
	PixelAspectRatio() float64
}

// Factory creates Emulator instances and answers static/per-disc
// questions that don't require a running instance.
type Factory interface {
	SystemInfo() SystemInfo
	CreateEmulator() Emulator
	DiscInfo(disc DiscReader) (DiscInfo, bool)
}
