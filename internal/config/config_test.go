package config

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
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
	want := Config{Address: "a@example.com", Name: "Alice", Signature: "ciao", Theme: "dracula", Layout: "split", PreviewMarkRead: "never"}
	if err := want.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
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

// Piano 7.2, Task 2: layout e preview_mark_read. File senza le chiavi → default.
func TestLoadNewSettingsDefaultsWhenOmitted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := writeFile(path, "address = \"a@example.com\"\n"); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Layout != LayoutSingle {
		t.Errorf("Layout = %q, voglio %q di default", c.Layout, LayoutSingle)
	}
	if c.PreviewMarkRead != PreviewMarkReadDelay {
		t.Errorf("PreviewMarkRead = %q, voglio %q di default", c.PreviewMarkRead, PreviewMarkReadDelay)
	}
}

// Piano 7.2, Task 2: i valori scritti nel file si caricano.
func TestLoadReadsLayoutAndPreviewMarkRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := writeFile(path, "layout = \"split\"\npreview_mark_read = \"never\"\n"); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Layout != "split" || c.PreviewMarkRead != "never" {
		t.Fatalf("avuto (%q, %q), voglio (split, never)", c.Layout, c.PreviewMarkRead)
	}
}

// Piano 7.2, Task 2: Validate rifiuta i valori sconosciuti; vuoto è ammesso.
func TestValidateNewSettings(t *testing.T) {
	valid := []Config{
		{},
		{Layout: "single"},
		{Layout: "split"},
		{PreviewMarkRead: "immediate"},
		{PreviewMarkRead: "delay"},
		{PreviewMarkRead: "never"},
	}
	for _, c := range valid {
		if err := c.Validate(); err != nil {
			t.Errorf("Validate(%+v) = %v, voglio nil", c, err)
		}
	}
	invalid := []Config{
		{Layout: "grid"},
		{Layout: "Split"},
		{PreviewMarkRead: "later"},
	}
	for _, c := range invalid {
		if err := c.Validate(); err == nil {
			t.Errorf("Validate(%+v) = nil, voglio errore", c)
		}
	}
}

// Piano 7.2, Task 2: un file con layout sconosciuto non si carica.
func TestLoadRejectsUnknownLayout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := writeFile(path, "layout = \"grid\"\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load con layout sconosciuto deve fallire")
	}
}

// Piano 7.2, Task 2: elenco dei valori ammessi (per il menu impostazioni).
func TestValueListsMatchConstants(t *testing.T) {
	wantLayout := []string{LayoutSingle, LayoutSplit}
	if got := LayoutValues(); !slices.Equal(got, wantLayout) {
		t.Errorf("LayoutValues() = %v, voglio %v", got, wantLayout)
	}
	wantPreview := []string{PreviewMarkReadImmediate, PreviewMarkReadDelay, PreviewMarkReadNever}
	if got := PreviewMarkReadValues(); !slices.Equal(got, wantPreview) {
		t.Errorf("PreviewMarkReadValues() = %v, voglio %v", got, wantPreview)
	}
}
