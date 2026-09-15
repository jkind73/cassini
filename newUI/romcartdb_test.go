package romcartdb

import (
	"crypto/md5"
	"encoding/hex"
	"os"
	"testing"
)

func TestScanAndSelect(t *testing.T) {
	dir := t.TempDir()
	content := []byte("fake ultraman rom cart data for testing")
	sum := md5.Sum(content)
	hexSum := hex.EncodeToString(sum[:])

	// Patch in a temporary known entry matching our synthetic file's
	// real MD5, so this test doesn't depend on possessing the actual
	// copyrighted cart dump.
	orig := Known
	Known = append(Known, Entry{
		ProductNumber: "TEST-0001",
		Label:         "Test Cart",
		CartFilename:  "test.bin",
		MD5:           hexSum,
	})
	defer func() { Known = orig }()

	path := dir + "/test.bin"
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatal(err)
	}

	found, err := Scan([]string{dir})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("got %d found, want 1", len(found))
	}

	f, ok := SelectForProduct(found, "TEST-0001")
	if !ok || f.Path != path {
		t.Errorf("SelectForProduct = %+v, ok=%v", f, ok)
	}

	_, ok = SelectForProduct(found, "NONEXISTENT")
	if ok {
		t.Error("expected no match for NONEXISTENT product")
	}
}

func TestRequiresROMCart(t *testing.T) {
	if _, ok := RequiresROMCart("T-3101G"); !ok {
		t.Error("expected T-3101G (KOF95 Japan) to require a ROM cart")
	}
	if _, ok := RequiresROMCart("T-1521G"); ok {
		t.Error("T-1521G (Astra Superstars, a DRAM-cart title) should not require a ROM cart")
	}
}
