package cache

import "testing"

func TestParseClrMamePro(t *testing.T) {
	data := `clrmamepro (
	name "Sega - Saturn"
	description "Sega - Saturn"
)

game (
	name "NiGHTS into Dreams (USA)"
	description "NiGHTS into Dreams (USA)"
	serial "T-8106H"
	rom ( name "NiGHTS into Dreams (USA) (Track 1).bin" size 168571872 crc a1b2c3d4 md5 abcdef0123456789abcdef0123456789 sha1 df94c5b4d47eb3cc404d88b33a8fda237eaf4720 )
	rom ( name "NiGHTS into Dreams (USA) (Track 2).bin" size 11642880 crc deadbeef sha1 faa8ea183a6d7bbe5d4e03bb1332519800d3fbc3 )
)

game (
	name "Panzer Dragoon (Japan)"
	description "Panzer Dragoon (Japan)"
	rom ( name "Panzer Dragoon (Japan).bin" size 500000000 crc 12345678 sha1 2b8cb4f87580683eb4d760e4ed210813d667f0a2 )
)
`
	tmp := t.TempDir() + "/test.dat"
	if err := writeFile(tmp, data); err != nil {
		t.Fatal(err)
	}

	entries, err := parseClrMamePro(tmp)
	if err != nil {
		t.Fatalf("parseClrMamePro: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].fields["name"] != "NiGHTS into Dreams (USA)" {
		t.Errorf("entry[0].name = %q", entries[0].fields["name"])
	}
	if entries[0].fields["serial"] != "T-8106H" {
		t.Errorf("entry[0].serial = %q", entries[0].fields["serial"])
	}
	if len(entries[0].roms) != 2 {
		t.Fatalf("entry[0] got %d roms, want 2", len(entries[0].roms))
	}
	if entries[0].roms[0]["sha1"] != "df94c5b4d47eb3cc404d88b33a8fda237eaf4720" {
		t.Errorf("rom[0].sha1 = %q", entries[0].roms[0]["sha1"])
	}
	if entries[1].fields["serial"] != "" {
		t.Errorf("entry[1] should have no serial, got %q", entries[1].fields["serial"])
	}
}

func writeFile(path, content string) error {
	return osWriteFile(path, []byte(content))
}
