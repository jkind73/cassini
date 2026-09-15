package cache

import (
	"encoding/xml"
	"strings"
	"testing"
)

// TestMameXMLDecode isolates just the XML-decoding half of
// ImportMameListXML (the part with real correctness risk, since it's
// written from documented schema knowledge rather than a fetched
// sample) from the DB-writing half, which needs a real SQLite backend
// this sandbox can't provide.
func TestMameXMLDecode(t *testing.T) {
	xmlData := `<?xml version="1.0"?>
<mame build="0.260">
  <machine name="sfish2j" sourcefile="sega/stv.cpp">
    <description>Sport Fishing 2 (Japan, v1.000)</description>
    <year>1997</year>
    <manufacturer>Sega</manufacturer>
    <rom name="epr-20091.ic8" size="2097152" crc="59ed40f4" sha1="eff0f54c70bce05ff3a289bf30b1027e1c8cd117"/>
    <rom name="fpr-19582.13" size="4194304" crc="deadbeef" sha1="0000000000000000000000000000000000000a"/>
    <input players="2"/>
    <dipswitch name="Difficulty"><dipvalue name="Easy"/></dipswitch>
  </machine>
  <machine name="stvbios" isbios="yes">
    <description>Sega Titan Video BIOS</description>
    <rom name="epr-23603.ic8" size="2097152" crc="f688ae60" sha1="1a31b6b1a4257fcb6ac6a91e67dd798f91505f48"/>
  </machine>
</mame>`

	var doc mameListXML
	if err := xml.NewDecoder(strings.NewReader(xmlData)).Decode(&doc); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(doc.Machines) != 2 {
		t.Fatalf("got %d machines, want 2", len(doc.Machines))
	}
	m := doc.Machines[0]
	if m.Name != "sfish2j" {
		t.Errorf("Name = %q", m.Name)
	}
	if m.Description != "Sport Fishing 2 (Japan, v1.000)" {
		t.Errorf("Description = %q", m.Description)
	}
	if m.Year != "1997" || m.Manufacturer != "Sega" {
		t.Errorf("Year=%q Manufacturer=%q", m.Year, m.Manufacturer)
	}
	if len(m.ROMs) != 2 {
		t.Fatalf("got %d roms, want 2", len(m.ROMs))
	}
	if m.ROMs[0].CRC != "59ed40f4" || m.ROMs[0].SHA1 != "eff0f54c70bce05ff3a289bf30b1027e1c8cd117" || m.ROMs[0].Size != 2097152 {
		t.Errorf("rom[0] = %+v", m.ROMs[0])
	}
	// Confirms unmapped elements (<input>, <dipswitch>) and unmapped
	// attributes (isbios) are silently and correctly ignored rather
	// than causing a decode error -- the property the doc comment
	// claims encoding/xml provides for free.
	if doc.Machines[1].Name != "stvbios" {
		t.Errorf("second machine Name = %q", doc.Machines[1].Name)
	}
}
