package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatalf("file assente non deve essere un errore: %v", err)
	}
	if c.Theme != "default" {
		t.Fatalf("theme di default atteso \"default\", avuto %q", c.Theme)
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	want := Config{Address: "a@example.com", Name: "Alice", Signature: "ciao", Theme: "dracula"}
	if err := want.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got != want {
		t.Fatalf("roundtrip: avuto %+v, atteso %+v", got, want)
	}
}

func TestLoadKeepsDefaultThemeWhenOmitted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := writeFile(path, "address = \"a@example.com\"\n"); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Theme != "default" {
		t.Fatalf("theme default preservato, avuto %q", c.Theme)
	}
}

func TestValidateRejectsBadAddress(t *testing.T) {
	c := Config{Address: "non-una-mail"}
	if err := c.Validate(); err == nil {
		t.Fatal("address senza @ deve fallire la validazione")
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
