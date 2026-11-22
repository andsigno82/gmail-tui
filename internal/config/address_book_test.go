package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// Piano 9, Task 10 (D9-G): address_book_cmd è un array (niente shell).
func TestLoadReadsAddressBookCmdArray(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	src := "address_book_cmd = [\"khard\", \"email\", \"--parsable\", \"%s\"]\n"
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"khard", "email", "--parsable", "%s"}
	if !slices.Equal(c.AddressBookCmd, want) {
		t.Errorf("AddressBookCmd = %v, voglio %v", c.AddressBookCmd, want)
	}
}

// Senza il campo il comando resta vuoto: solo suggerimenti locali.
func TestAddressBookCmdDefaultsEmpty(t *testing.T) {
	if got := Default().AddressBookCmd; len(got) != 0 {
		t.Errorf("default AddressBookCmd = %v, voglio vuoto", got)
	}
}
