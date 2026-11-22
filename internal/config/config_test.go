package config

import (
	"errors"
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

// Piano 11, Task 1: notify_command è una lista di argomenti TOML, mai una
// stringa interpretata da una shell.
func TestLoadNotifyCommandArgv(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := writeFile(path, "notify_command = [\"notify-send\", \"Posta\", \"nuova\"]\n"); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"notify-send", "Posta", "nuova"}
	if !slices.Equal(c.NotifyCommand, want) {
		t.Fatalf("NotifyCommand: avuto %q, atteso %q", c.NotifyCommand, want)
	}
}

// Piano 11, Task 1: il round-trip conserva la lista; senza comando la chiave
// non compare e il Load torna nil.
func TestSaveLoadNotifyCommandRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	want := Config{Theme: "default", NotifyCommand: []string{"osascript", "-e", "display notification"}}
	if err := want.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !slices.Equal(got.NotifyCommand, want.NotifyCommand) {
		t.Fatalf("roundtrip: avuto %q, atteso %q", got.NotifyCommand, want.NotifyCommand)
	}
	none := Config{Theme: "default"}
	if err := none.Save(path); err != nil {
		t.Fatalf("Save senza comando: %v", err)
	}
	got, err = Load(path)
	if err != nil {
		t.Fatalf("Load senza comando: %v", err)
	}
	if got.NotifyCommand != nil {
		t.Fatalf("senza comando atteso nil, avuto %q", got.NotifyCommand)
	}
}

// Piano 11, review punto 5: l'eseguibile è un percorso assoluto o un nome senza
// separatori (risolto nel PATH). ./x e scripts/x sono rifiutati con errore chiaro.
func TestNotifyCommandExecutableForms(t *testing.T) {
	valid := []string{"/usr/local/bin/notify", "notify-send"}
	for _, exe := range valid {
		c := Config{NotifyCommand: []string{exe}}
		if err := c.Validate(); err != nil {
			t.Errorf("%q: Validate = %v, voglio nil", exe, err)
		}
	}
	invalid := []string{"./notify.sh", "scripts/notify", "../notify"}
	for _, exe := range invalid {
		c := Config{NotifyCommand: []string{exe}}
		if err := c.Validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("%q: Validate = %v, voglio ErrInvalid", exe, err)
		}
		if s := c.Sanitized(); s.NotifyCommand != nil {
			t.Errorf("%q: Sanitized deve disattivare il comando, avuto %q", exe, s.NotifyCommand)
		}
	}
}

// Piano 11, Task 1: un primo elemento vuoto non è valido; Sanitized azzera solo
// il comando e conserva gli altri campi.
func TestNotifyCommandEmptyExecutableIsInvalid(t *testing.T) {
	c := Config{Theme: "dracula", Address: "a@example.com", NotifyCommand: []string{"  ", "x"}}
	if err := c.Validate(); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Validate: atteso ErrInvalid, avuto %v", err)
	}
	s := c.Sanitized()
	if s.NotifyCommand != nil {
		t.Fatalf("Sanitized: comando non valido atteso azzerato, avuto %q", s.NotifyCommand)
	}
	if s.Theme != "dracula" || s.Address != "a@example.com" {
		t.Fatalf("Sanitized: altri campi alterati: %+v", s)
	}
}
