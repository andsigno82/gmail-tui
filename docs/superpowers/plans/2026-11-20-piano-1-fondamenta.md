# gmail-tui — Piano 1: Fondamenta — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Le fondamenta testabili del client: module, config, sistema temi embedded, parsing messaggi, interfaccia `MailStore` + mock, cache bbolt e CLI `version`/`themes`/`doctor`.

**Architecture:** Solo package puri senza TUI e senza rete: `internal/config`, `internal/theme`, `internal/msg`, `internal/mail` (interfaccia + mock), `internal/mail/cache`. Ogni package è testabile in isolamento; i piani 2 (IMAP/SMTP) e 3 (TUI) consumano queste interfacce.

**Tech Stack:** Go 1.25 · BurntSushi/toml · jhillyerd/enmime · k3a/html2text · go.etcd.io/bbolt · std `flag`

**Nota esecuzione:** i subagenti in parallelo per l'implementazione vengono lanciati con modello **GLM-5.3-Flash** (preferenza dell'utente).

## Global Constraints

- Module: `gmail-tui` (path locale; rename definitivo con `go mod edit -module` a v0.1.0, spec §Domande aperte)
- Go 1.25 nel `go.mod`; nessun CGO (`CGO_ENABLED=0` buildmuni)
- Licenza MIT, copyright "The gmail-tui authors"
- Zero NIH: nessuna dipendenza aggiunta oltre a quelle elencate senza giustificarlo nel commit
- Tipi dati con unica fonte di verità in `internal/msg`; `internal/mail` li importa (deviazione dalla spec che li metteva in `store.go`: motivata DRY)
- A fine di ogni task: `go vet ./...` e `go test ./...` verdi, poi commit
- La UI (piani successivi) userà SOLO slot semantici dei temi, mai hex diretti

## File Structure

```
gmail-tui/
├── .github/workflows/ci.yml              (Task 9)
├── .gitignore                              (Task 1)
├── .golangci.yml                           (Task 9)
├── LICENSE                                 (Task 1)
├── go.mod                                  (Task 1, aggiornato dai tidy)
├── cmd/gmail-tui/main.go                   (Task 1 stub, Task 8 comandi)
├── internal/cmd/doctor.go + _test.go       (Task 8)
├── internal/config/config.go + _test.go    (Task 2)
├── internal/mail/store.go + mock.go + mock_test.go (Task 6)
├── internal/mail/cache/cache.go + _test.go (Task 7)
├── internal/msg/message.go + query.go      (Task 5)
├── internal/msg/parse.go + parse_test.go   (Task 5)
├── internal/theme/theme.go + _test.go      (Task 3)
└── internal/theme/themes/*.toml            (Task 3: default; Task 4: altri 11)
```

---

### Task 1: Modulo, licenza, stub main

**Files:**
- Create: `go.mod`, `LICENSE`, `.gitignore`, `cmd/gmail-tui/main.go`

**Interfaces:**
- Consumes: nulla
- Produces: binario `gmail-tui` con sottocomando `version`; var `version` linkabile con `-ldflags`

- [ ] **Step 1: crea `go.mod`**

```
module gmail-tui

go 1.25
```

- [ ] **Step 2: crea `LICENSE`**

```text
MIT License

Copyright (c) 2026 The gmail-tui authors

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

- [ ] **Step 3: crea `.gitignore`**

```gitignore
/gmail-tui
dist/
.DS_Store
```

- [ ] **Step 4: crea `cmd/gmail-tui/main.go`**

```go
package main

import (
	"fmt"
	"os"
)

// version viene iniettata a build time: -ldflags "-X main.version=0.1.0"
var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version":
			fmt.Println("gmail-tui " + version)
			return
		}
	}
	fmt.Fprintln(os.Stderr, "gmail-tui is under construction; available: version")
	os.Exit(1)
}
```

- [ ] **Step 5: verifica build**

Run: `cd /Users/andrea/dev/gmail-tui && go vet ./... && go build ./... && go run ./cmd/gmail-tui version`
Expected: `gmail-tui dev`

- [ ] **Step 6: Commit**

```bash
git add go.mod LICENSE .gitignore cmd/
git commit -m "feat: go module, MIT license, main stub with version command"
```

---

### Task 2: package config

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: nulla
- Produces: `config.Config{Address, Name, Signature, Theme string}`; `config.Default() Config`; `config.Path() (string, error)`; `config.Load(path string) (Config, error)` (file assente ⇒ default, nessun errore); `(Config) Save(path string) error`; `(Config) Validate() error`

- [ ] **Step 1: scrivi il test fallente**

```go
package config

import (
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
```

e il helper `writeFile` nel file di test:

```go
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
```

(con `os` negli import dei test).

- [ ] **Step 2: esegui, verifica che fallisca**

Run: `go test ./internal/config/`
Expected: FAIL — `undefined: Load` (nessun file config.go)

- [ ] **Step 3: implementazione minima**

```go
// Package config gestisce ~/.config/gmail-tui/config.toml.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config è la configurazione utente. Gli host IMAP/SMTP sono preset
// Gmail fissi (spec) e non sono configurabili in v1.
type Config struct {
	Address   string `toml:"address"`
	Name      string `toml:"name"`
	Signature string `toml:"signature"`
	Theme     string `toml:"theme"`
}

// Default restituisce la configurazione di fabbrica.
func Default() Config {
	return Config{Theme: "default"}
}

// Path restituisce il percorso della config (os.UserConfigDir + gmail-tui).
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config dir: %w", err)
	}
	return filepath.Join(dir, "gmail-tui", "config.toml"), nil
}

// Load legge la config; file assente ⇒ Default senza errore (first-run).
func Load(path string) (Config, error) {
	c := Default()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("leggo %s: %w", path, err)
	}
	if _, err := toml.Decode(string(data), &c); err != nil {
		return c, fmt.Errorf("parse %s: %w", path, err)
	}
	return c, c.Validate()
}

// Save scrive la config creando le directory padre.
func (c Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creo dir config: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("apro %s: %w", path, err)
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(c)
}

// Validate controlla i campi con formato noto.
func (c Config) Validate() error {
	if c.Address != "" && !strings.Contains(c.Address, "@") {
		return fmt.Errorf("address %q non valido: manca @", c.Address)
	}
	return nil
}
```

- [ ] **Step 4: esegui, verifica che passi**

Run: `go mod tidy && go test ./internal/config/ -v`
Expected: PASS (4 test). `go.mod` ora include `github.com/BurntSushi/toml`.

- [ ] **Step 5: Commit**

```bash
git add internal/config/ go.mod go.sum
git commit -m "feat(config): load/save/validate TOML config with defaults"
```

---

### Task 3: package theme — Theme, loader embedded, validazione

**Files:**
- Create: `internal/theme/theme.go`, `internal/theme/themes/default.toml`
- Test: `internal/theme/theme_test.go`

**Interfaces:**
- Consumes: nulla (solo std + BurntSushi/toml già presenti)
- Produces: `theme.Slot{Dark, Light string}` con `(Slot) Resolve() (dark, light string)` (fallback incrociato se un lato manca); `theme.Theme` con campi slot `Fg, Bg, Muted, Border, Primary, Accent, Error, Success, Warning, Unread, Starred, Quoted, Help, Selection`; `theme.Load(name string) (Theme, error)`; `theme.Names() []string` (ordinato); errore `theme.ErrNotFound`

- [ ] **Step 1: crea `internal/theme/themes/default.toml`** (tema neutro Charm-like, slots adattivi)

```toml
# Tema di default — palette neutra ispirata a Charm.
# Sintassi: ogni slot ha "dark" e/o "light"; se manca un lato, vale l'altro.

[fg]
dark  = "#e2e2e7"
light = "#24292f"
[bg]
dark  = "#16161e"
light = "#ffffff"
[muted]
dark  = "#8b8b9e"
light = "#6e7781"
[border]
dark  = "#3a3a4e"
light = "#d0d7de"
[primary]
dark  = "#906cf2"
light = "#6f42c1"
[accent]
dark  = "#f778ba"
light = "#bf3989"
[error]
dark  = "#f38ba8"
light = "#cf222e"
[success]
dark  = "#a6e3a1"
light = "#116329"
[warning]
dark  = "#f9e2af"
light = "#9a6700"
[unread]
dark  = "#f9e2af"
light = "#9a6700"
[starred]
dark  = "#fab387"
light = "#bc4c00"
[quoted]
dark  = "#7d8590"
light = "#57606a"
[help]
dark  = "#8b8b9e"
light = "#6e7781"
[selection]
dark  = "#2e2e42"
light = "#ddf4ff"
```

- [ ] **Step 2: scrivi il test fallente**

```go
package theme

import (
	"errors"
	"strings"
	"testing"
)

func TestLoadDefault(t *testing.T) {
	th, err := Load("default")
	if err != nil {
		t.Fatalf("Load(default): %v", err)
	}
	if th.Name != "default" {
		t.Fatalf("Name = %q, atteso \"default\"", th.Name)
	}
	d, l := th.Fg.Resolve()
	if d == "" || l == "" {
		t.Fatalf("fg non risolto: dark=%q light=%q", d, l)
	}
}

func TestLoadUnknown(t *testing.T) {
	_, err := Load("non-esiste")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("atteso ErrNotFound, avuto %v", err)
	}
}

func TestLoadRejectsPathTraversal(t *testing.T) {
	if _, err := Load("../evil"); err == nil {
		t.Fatal("nomi con path separator devono essere rifiutati")
	}
}

func TestNamesDefaultFirstRestSorted(t *testing.T) {
	names := Names()
	if len(names) == 0 || names[0] != "default" {
		t.Fatalf("Names() = %v, atteso \"default\" per primo", names)
	}
	for i := 2; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Fatalf("Names() dopo default non ordinato: %v", names)
		}
	}
}

// TestEveryEmbeddedThemeValid è il test tabellare che valida TUTTI i temi
// embedded: aggiungere un .toml = aggiungere un caso gratis (Task 4).
func TestEveryEmbeddedThemeValid(t *testing.T) {
	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(name); err != nil {
				t.Fatalf("tema embedded non valido: %v", err)
			}
		})
	}
}

func TestSlotResolveFallback(t *testing.T) {
	d, l := Slot{Dark: "#111111"}.Resolve()
	if d != "#111111" || l != "#111111" {
		t.Fatalf("fallback light→dark rotto: %q %q", d, l)
	}
	d, l = Slot{Light: "#222222"}.Resolve()
	if d != "#222222" || l != "#222222" {
		t.Fatalf("fallback dark→light rotto: %q %q", d, l)
	}
}

func TestValidateRejectsBadHex(t *testing.T) {
	th, err := Load("default")
	if err != nil {
		t.Fatal(err)
	}
	th.Fg = Slot{Dark: "non-hex"}
	if err := th.validate(); err == nil {
		t.Fatal("hex invalido deve fallire validate")
	}
	if !strings.Contains(err2String(th.validate()), "fg") {
		t.Fatalf("l'errore deve nominare lo slot: %v", th.validate())
	}
}

func err2String(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
```

- [ ] **Step 3: esegui, verifica che fallisca**

Run: `go test ./internal/theme/`
Expected: FAIL — `undefined: Load`

- [ ] **Step 4: implementazione minima**

```go
// Package theme carica i temi embedded (TOML) in slot semantici.
// La UI usa SOLO questi slot: mai colori diretti.
package theme

import (
	"embed"
	"fmt"
	"io/fs"
	"reflect"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

//go:embed themes/*.toml
var files embed.FS

// ErrNotFound è restituito per nomi tema inesistenti o non validi.
var ErrNotFound = fmt.Errorf("tema non trovato")

// Slot è un colore con variante dark/light per il terminale.
// Se un lato manca, vale l'altro (Resolve).
type Slot struct {
	Dark  string `toml:"dark"`
	Light string `toml:"light"`
}

// Resolve applica il fallback incrociato dark↔light.
func (s Slot) Resolve() (dark, light string) {
	d, l := s.Dark, s.Light
	if d == "" {
		d = l
	}
	if l == "" {
		l = d
	}
	return d, l
}

// Theme mappa slot semantici → colori. Il campo Name deriva dal filename.
type Theme struct {
	Name      string `toml:"-"`
	Fg        Slot    `toml:"fg"`
	Bg        Slot    `toml:"bg"`
	Muted     Slot    `toml:"muted"`
	Border    Slot    `toml:"border"`
	Primary   Slot    `toml:"primary"`
	Accent    Slot    `toml:"accent"`
	Error     Slot    `toml:"error"`
	Success   Slot    `toml:"success"`
	Warning   Slot    `toml:"warning"`
	Unread    Slot    `toml:"unread"`
	Starred   Slot    `toml:"starred"`
	Quoted    Slot    `toml:"quoted"`
	Help      Slot    `toml:"help"`
	Selection Slot    `toml:"selection"`
}

// Load legge il tema embedded per nome (= filename senza .toml).
func Load(name string) (Theme, error) {
	if !validName(name) {
		return Theme{}, fmt.Errorf("%w: %q", ErrNotFound, name)
	}
	data, err := files.ReadFile("themes/" + name + ".toml")
	if err != nil {
		return Theme{}, fmt.Errorf("%w: %q", ErrNotFound, name)
	}
	var t Theme
	if _, err := toml.Decode(string(data), &t); err != nil {
		return Theme{}, fmt.Errorf("tema %q malformato: %w", name, err)
	}
	t.Name = name
	if err := t.validate(); err != nil {
		return Theme{}, err
	}
	return t, nil
}

// Names elenca i temi embedded, ordinati, "default" primo.
func Names() []string {
	entries, err := fs.Glob(files, "themes/*.toml")
	if err != nil {
		return nil // irraggiungibile con embed
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(strings.TrimPrefix(e, "themes/"), ".toml"))
	}
	sort.Slice(names, func(i, j int) bool {
		if names[i] == "default" {
			return true
		}
		if names[j] == "default" {
			return false
		}
		return names[i] < names[j]
	})
	return names
}

func (t Theme) validate() error {
	v := reflect.ValueOf(t)
	slotType := reflect.TypeOf(Slot{})
	for i := 0; i < v.NumField(); i++ {
		field := v.Type().Field(i)
		if field.Type != slotType {
			continue
		}
		slot := v.Field(i).Interface().(Slot)
		d, l := slot.Resolve()
		name := strings.ToLower(field.Name)
		if d == "" {
			return fmt.Errorf("tema %q: slot %s vuoto", t.Name, name)
		}
		if err := checkHex(d); err != nil {
			return fmt.Errorf("tema %q slot %s dark: %w", t.Name, name, err)
		}
		if err := checkHex(l); err != nil {
			return fmt.Errorf("tema %q slot %s light: %w", t.Name, name, err)
		}
	}
	return nil
}

func validName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	return strings.Trim(name, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-") == ""
}

func checkHex(s string) error {
	if len(s) != 4 && len(s) != 7 || s[0] != '#' {
		return fmt.Errorf("hex %q non valido (atteso #rgb o #rrggbb)", s)
	}
	for _, c := range s[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return fmt.Errorf("hex %q non valido", s)
		}
	}
	return nil
}
```

- [ ] **Step 5: esegui, verifica che passi**

Run: `go test ./internal/theme/ -v`
Expected: PASS (7 test + subtest `default` del test tabellare)

- [ ] **Step 6: Commit**

```bash
git add internal/theme/
git commit -m "feat(theme): embedded TOML theme system with semantic slots and validation"
```

---

### Task 4: gli altri 11 temi classici

**Files:**
- Create: `internal/theme/themes/{catppuccin-latte,catppuccin-frappe,catppuccin-macchiato,catppuccin-mocha,dracula,one-dark,one-light,nord,gruvbox-dark,gruvbox-light,tokyo-night}.toml`

**Interfaces:**
- Consumes: `theme.Load`, `theme.Names`, il test tabellare `TestEveryEmbeddedThemeValid` (Task 3)
- Produces: 12 temi totali disponibili; palette ufficiali con attribuzione (file `internal/theme/themes/README.md`)

- [ ] **Step 1: crea `internal/theme/themes/README.md`**

```markdown
# Temi

Ogni file `.toml` è un tema: slot semantici → colori (`dark`/`light`).
Se un lato manca, vale l'altro. Aggiungere un tema = aggiungere un file:
il test tabellare in `theme_test.go` lo valida automaticamente.

Palette ufficiali (grazie ai rispettivi progetti, licenze MIT/Apache):
- Catppuccin (latte, frappé, macchiato, mocha) — https://catppuccin.com (MIT)
- Dracula — https://draculatheme.com (MIT)
- Atom One Dark / One Light — https://atom.io/themes/one-dark-syntax (MIT)
- Nord — https://nordtheme.com (MIT)
- Gruvbox — https://github.com/morhetz/gruvbox (MIT/Apache-2.0)
- Tokyo Night — https://github.com/folke/tokyo.night (Apache-2.0)
```

- [ ] **Step 2: crea i temi scuri** (solo `dark`, il light fallback è il tema stesso)

`catppuccin-mocha.toml`:
```toml
[fg]
dark = "#cdd6f4"
[bg]
dark = "#1e1e2e"
[muted]
dark = "#6c7086"
[border]
dark = "#313244"
[primary]
dark = "#89b4fa"
[accent]
dark = "#cba6f7"
[error]
dark = "#f38ba8"
[success]
dark = "#a6e3a1"
[warning]
dark = "#f9e2af"
[unread]
dark = "#f9e2af"
[starred]
dark = "#fab387"
[quoted]
dark = "#7f849c"
[help]
dark = "#6c7086"
[selection]
dark = "#45475a"
```

`catppuccin-frappe.toml`:
```toml
[fg]
dark = "#c6d0f5"
[bg]
dark = "#303446"
[muted]
dark = "#838ba7"
[border]
dark = "#414559"
[primary]
dark = "#8caaee"
[accent]
dark = "#ca9ee6"
[error]
dark = "#e78284"
[success]
dark = "#a6d189"
[warning]
dark = "#e5c890"
[unread]
dark = "#e5c890"
[starred]
dark = "#ef9f76"
[quoted]
dark = "#949cbb"
[help]
dark = "#838ba7"
[selection]
dark = "#51576d"
```

`catppuccin-macchiato.toml`:
```toml
[fg]
dark = "#cad3f5"
[bg]
dark = "#24273a"
[muted]
dark = "#8087a2"
[border]
dark = "#363a4f"
[primary]
dark = "#8aadf4"
[accent]
dark = "#c6a0f6"
[error]
dark = "#ed8796"
[success]
dark = "#a6da95"
[warning]
dark = "#eed49f"
[unread]
dark = "#eed49f"
[starred]
dark = "#f5a97f"
[quoted]
dark = "#939ab7"
[help]
dark = "#8087a2"
[selection]
dark = "#494d64"
```

`dracula.toml`:
```toml
[fg]
dark = "#f8f8f2"
[bg]
dark = "#282a36"
[muted]
dark = "#6272a4"
[border]
dark = "#44475a"
[primary]
dark = "#bd93f9"
[accent]
dark = "#ff79c6"
[error]
dark = "#ff5555"
[success]
dark = "#50fa7b"
[warning]
dark = "#f1fa8c"
[unread]
dark = "#f1fa8c"
[starred]
dark = "#ffb86c"
[quoted]
dark = "#8be9fd"
[help]
dark = "#6272a4"
[selection]
dark = "#44475a"
```

`one-dark.toml`:
```toml
[fg]
dark = "#abb2bf"
[bg]
dark = "#282c34"
[muted]
dark = "#5c6370"
[border]
dark = "#3e4451"
[primary]
dark = "#61afef"
[accent]
dark = "#c678dd"
[error]
dark = "#e06c75"
[success]
dark = "#98c379"
[warning]
dark = "#e5c07b"
[unread]
dark = "#e5c07b"
[starred]
dark = "#d19a66"
[quoted]
dark = "#56b6c2"
[help]
dark = "#5c6370"
[selection]
dark = "#3e4451"
```

`nord.toml`:
```toml
[fg]
dark = "#d8dee9"
[bg]
dark = "#2e3440"
[muted]
dark = "#7b88a1"
[border]
dark = "#434c5e"
[primary]
dark = "#88c0d0"
[accent]
dark = "#b48ead"
[error]
dark = "#bf616a"
[success]
dark = "#a3be8c"
[warning]
dark = "#ebcb8b"
[unread]
dark = "#ebcb8b"
[starred]
dark = "#d08770"
[quoted]
dark = "#81a1c1"
[help]
dark = "#7b88a1"
[selection]
dark = "#434c5e"
```

`gruvbox-dark.toml`:
```toml
[fg]
dark = "#ebdbb2"
[bg]
dark = "#282828"
[muted]
dark = "#928374"
[border]
dark = "#3c3836"
[primary]
dark = "#83a598"
[accent]
dark = "#d3869b"
[error]
dark = "#fb4934"
[success]
dark = "#b8bb26"
[warning]
dark = "#fabd2f"
[unread]
dark = "#fabd2f"
[starred]
dark = "#fe8019"
[quoted]
dark = "#8ec07c"
[help]
dark = "#928374"
[selection]
dark = "#504945"
```

`tokyo-night.toml`:
```toml
[fg]
dark = "#c0caf5"
[bg]
dark = "#1a1b26"
[muted]
dark = "#565f89"
[border]
dark = "#292e42"
[primary]
dark = "#7aa2f7"
[accent]
dark = "#bb9af7"
[error]
dark = "#f7768e"
[success]
dark = "#9ece6a"
[warning]
dark = "#e0af68"
[unread]
dark = "#e0af68"
[starred]
dark = "#ff9e64"
[quoted]
dark = "#7dcfff"
[help]
dark = "#565f89"
[selection]
dark = "#2f334d"
```

- [ ] **Step 3: crea i temi chiari** (solo `light`, fallback dark = light)

`catppuccin-latte.toml`:
```toml
[fg]
light = "#4c4f69"
[bg]
light = "#eff1f5"
[muted]
light = "#9ca0b0"
[border]
light = "#bcc0cc"
[primary]
light = "#1e66f5"
[accent]
light = "#8839ef"
[error]
light = "#d20f39"
[success]
light = "#40a02b"
[warning]
light = "#df8e1d"
[unread]
light = "#df8e1d"
[starred]
light = "#fe640b"
[quoted]
light = "#7c7f93"
[help]
light = "#9ca0b0"
[selection]
light = "#ccd0da"
```

`one-light.toml`:
```toml
[fg]
light = "#383a42"
[bg]
light = "#fafafa"
[muted]
light = "#a0a1a7"
[border]
light = "#d7d7db"
[primary]
light = "#4078f2"
[accent]
light = "#a626a4"
[error]
light = "#e45649"
[success]
light = "#50a14f"
[warning]
light = "#c18401"
[unread]
light = "#c18401"
[starred]
light = "#986801"
[quoted]
light = "#0184bc"
[help]
light = "#a0a1a7"
[selection]
light = "#f0f0f0"
```

`gruvbox-light.toml`:
```toml
[fg]
light = "#3c3836"
[bg]
light = "#fbf1c7"
[muted]
light = "#928374"
[border]
light = "#bdae93"
[primary]
light = "#076678"
[accent]
light = "#8f3f71"
[error]
light = "#9d0006"
[success]
light = "#79740e"
[warning]
light = "#b57614"
[unread]
light = "#b57614"
[starred]
light = "#af3a03"
[quoted]
light = "#427b58"
[help]
light = "#928374"
[selection]
light = "#ebdbb2"
```

- [ ] **Step 4: esegui il test tabellare**

Run: `go test ./internal/theme/ -run TestEveryEmbeddedThemeValid -v`
Expected: PASS con 12 subtest (default + 11 nuovi)

- [ ] **Step 5: Commit**

```bash
git add internal/theme/themes/
git commit -m "feat(theme): add catppuccin, dracula, one-dark/light, nord, gruvbox, tokyo-night"
```

---

### Task 5: package msg — tipi dati e parsing MIME

**Files:**
- Create: `internal/msg/message.go`, `internal/msg/query.go`, `internal/msg/parse.go`
- Test: `internal/msg/parse_test.go`

**Interfaces:**
- Consumes: enmime, k3a/html2text
- Produces (per Tasks 6-8 e Piani 2-3):
  - `msg.MessageID string` con `(MessageID) Split() (mailbox string, uid uint32, err error)` e `msg.NewID(mailbox string, uid uint32) MessageID`
  - `msg.MessageRef{ID MessageID; From, Subject string; Date time.Time; Unread, Starred bool; ThreadLen int}`
  - `msg.Attachment{PartIndex int; Filename, ContentType string; Size int64}`
  - `msg.Message{Ref MessageRef; To, Cc []string; BodyText string; HasHTML bool; Attachments []Attachment}`
  - `msg.Outgoing{From string; To, Cc, Bcc []string; Subject, BodyText string; Attachments []string}`
  - `msg.Query{Text, From, To, Label string; Unread, Starred, HasAttachment bool}` con `(Query) IsEmpty() bool`
  - `msg.Parse(id MessageID, raw io.Reader) (*Message, error)`

- [ ] **Step 1: scrivi il test fallente**

```go
package msg

import (
	"strings"
	"testing"
	"time"
)

const multipartMail = `From: Alice Rossi <alice@example.com>
To: bob@example.com
Cc: carol@example.com
Subject: Contratto aggiornato
Date: Thu, 20 Nov 2026 10:32:00 +0100
Message-ID: <c1@example.com>
MIME-Version: 1.0
Content-Type: multipart/mixed; boundary="BOUND"

--BOUND
Content-Type: text/plain; charset=utf-8

Ciao Bob,
trovi il contratto in allegato.
--BOUND
Content-Type: application/pdf; name="contratto.pdf"
Content-Disposition: attachment; filename="contratto.pdf"

JVBERi0xLjQ=
--BOUND--
`

const htmlOnlyMail = `From: Newsletter <news@example.org>
To: bob@example.com
Subject: Ultimi articoli
Date: Thu, 19 Nov 2026 08:00:00 +0100
MIME-Version: 1.0
Content-Type: text/html; charset=utf-8

<html><body><h1>Titolo</h1><p>Leggi <a href="https://example.org">qui</a>.</p></body></html>
`

func TestParseMultipart(t *testing.T) {
	m, err := Parse(NewID("INBOX", 42), strings.NewReader(multipartMail))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if m.Ref.Subject != "Contratto aggiornato" {
		t.Errorf("Subject = %q", m.Ref.Subject)
	}
	if m.Ref.From != "Alice Rossi <alice@example.com>" {
		t.Errorf("From = %q", m.Ref.From)
	}
	want := time.Date(2026, 11, 20, 10, 32, 0, 0, time.FixedZone("", 3600))
	if !m.Ref.Date.Equal(want) {
		t.Errorf("Date = %v, atteso %v", m.Ref.Date, want)
	}
	if m.BodyText != "Ciao Bob,\ntrovi il contratto in allegato.\n" &&
		!strings.Contains(m.BodyText, "trovi il contratto") {
		t.Errorf("BodyText inatteso: %q", m.BodyText)
	}
	if len(m.Attachments) != 1 {
		t.Fatalf("allegati = %d, atteso 1", len(m.Attachments))
	}
	a := m.Attachments[0]
	if a.Filename != "contratto.pdf" || a.ContentType != "application/pdf" {
		t.Errorf("allegato = %+v", a)
	}
	if a.PartIndex != 0 || a.Size == 0 {
		t.Errorf("allegato indice/size = %d/%d", a.PartIndex, a.Size)
	}
}

func TestParseHTMLFallback(t *testing.T) {
	m, err := Parse(NewID("INBOX", 43), strings.NewReader(htmlOnlyMail))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !m.HasHTML {
		t.Error("HasHTML atteso true")
	}
	if !strings.Contains(m.BodyText, "Titolo") || !strings.Contains(m.BodyText, "https://example.org") {
		t.Errorf("BodyText HTML→testo inatteso: %q", m.BodyText)
	}
}

func TestIDSplitRoundtrip(t *testing.T) {
	id := NewID("INBOX", 7)
	mb, uid, err := id.Split()
	if err != nil || mb != "INBOX" || uid != 7 {
		t.Fatalf("Split = %q %d %v", mb, uid, err)
	}
	if _, _, err := (MessageID("senza-duepunti")).Split(); err == nil {
		t.Error("ID malformato deve dare errore")
	}
}

func TestQueryIsEmpty(t *testing.T) {
	if !(Query{}).IsEmpty() {
		t.Error("Query zero-value deve essere empty")
	}
	if (Query{Unread: true}).IsEmpty() {
		t.Error("Query con filtri non deve essere empty")
	}
}
```

- [ ] **Step 2: esegui, verifica che fallisca**

Run: `go test ./internal/msg/`
Expected: FAIL — `undefined: Parse`

- [ ] **Step 3: implementa `message.go`**

```go
// Package msg contiene i tipi dati normalizzati della posta:
// unica fonte di verità per UI e backend.
package msg

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// MessageID identifica un messaggio: "MAILBOX:UID" (es. "INBOX:42").
type MessageID string

// NewID compone un MessageID da mailbox e UID IMAP.
func NewID(mailbox string, uid uint32) MessageID {
	return MessageID(mailbox + ":" + strconv.FormatUint(uint64(uid), 10))
}

// Split decostruisce l'ID.
func (id MessageID) Split() (mailbox string, uid uint32, err error) {
	s := string(id)
	i := strings.LastIndexByte(s, ':')
	if i <= 0 || i == len(s)-1 {
		return "", 0, fmt.Errorf("message id %q malformato", s)
	}
	n, err := strconv.ParseUint(s[i+1:], 10, 32)
	if err != nil {
		return "", 0, fmt.Errorf("message id %q: uid non numerico", s)
	}
	return s[:i], uint32(n), nil
}

// MessageRef è la riga di lista: tutto ciò che serve senza il corpo.
type MessageRef struct {
	ID        MessageID
	From      string
	Subject   string
	Date      time.Time
	Unread    bool
	Starred   bool
	ThreadLen int
}

// Attachment descrive un allegato (PartIndex = indice in Message.Attachments).
type Attachment struct {
	PartIndex   int
	Filename    string
	ContentType string
	Size        int64
}

// Message è il messaggio completo normalizzato.
type Message struct {
	Ref         MessageRef
	To, Cc      []string
	BodyText    string
	HasHTML     bool
	Attachments []Attachment
}

// Outgoing è un'email da inviare (Attachments = percorsi file).
type Outgoing struct {
	From        string
	To, Cc, Bcc []string
	Subject     string
	BodyText    string
	Attachments []string
}
```

E `query.go`:

```go
package msg

// Query è una ricerca: campi vuoti = filtro non attivo.
type Query struct {
	Text          string // testo libero (subject, from, corpo)
	From, To      string
	Label         string
	Unread        bool
	Starred       bool
	HasAttachment bool
}

// IsEmpty true se nessun filtro attivo.
func (q Query) IsEmpty() bool { return q == Query{} }
```

- [ ] **Step 4: implementa `parse.go`**

```go
package msg

import (
	"fmt"
	"io"
	"net/mail"
	"strings"

	"github.com/jhillyerd/enmime"
	"github.com/k3a/html2text"
)

// Parse normalizza una email MIME grezza in Message.
// Regola corpo (spec): text/plain vince; se solo HTML → HasHTML=true
// e BodyText = conversione HTML→testo.
// Un allegato non decodificabile non fallisce il messaggio: viene saltato.
func Parse(id MessageID, raw io.Reader) (*Message, error) {
	env, err := enmime.ReadEnvelope(raw)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", id, err)
	}
	m := &Message{Ref: MessageRef{
		ID:      id,
		Subject: env.GetHeader("Subject"),
	}}
	m.Ref.From = displayAddr(env, "From")
	m.To = addrList(env, "To")
	m.Cc = addrList(env, "Cc")
	if d, err := mail.ParseDate(env.GetHeader("Date")); err == nil {
		m.Ref.Date = d
	}
	switch {
	case strings.TrimSpace(env.Text) != "":
		m.BodyText = env.Text
	case strings.TrimSpace(env.HTML) != "":
		m.HasHTML = true
		m.BodyText = html2text.HTML2Text(env.HTML)
	}
	idx := 0
	for _, part := range env.Attachments {
		if part.FileName == "" && part.ContentType == "application/octet-stream" && len(part.Content) == 0 {
			continue // parte vuota/non utile
		}
		m.Attachments = append(m.Attachments, Attachment{
			PartIndex:   idx,
			Filename:    part.FileName,
			ContentType: part.ContentType,
			Size:        int64(len(part.Content)),
		})
		idx++
	}
	return m, nil
}

// displayAddr rende "Nome <addr>" (o l'indirizzo nudo) dal header.
func displayAddr(env *enmime.Envelope, header string) string {
	addrs, err := env.AddressList(header)
	if err != nil || len(addrs) == 0 {
		return env.GetHeader(header)
	}
	a := addrs[0]
	if a.Name != "" {
		return a.Name + " <" + a.Address + ">"
	}
	return a.Address
}

// addrList estrae gli indirizzi di un header come stringhe.
func addrList(env *enmime.Envelope, header string) []string {
	addrs, err := env.AddressList(header)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, a.Address)
	}
	return out
}
```

- [ ] **Step 5: esegui, verifica che passi**

Run: `go mod tidy && go test ./internal/msg/ -v`
Expected: PASS (4 test). `go.mod` ora include enmime e k3a/html2text (+indirect).

- [ ] **Step 6: Commit**

```bash
git add internal/msg/ go.mod go.sum
git commit -m "feat(msg): normalized mail types and MIME parsing with HTML fallback"
```

---

### Task 6: package mail — MailStore + Mock in-memory

**Files:**
- Create: `internal/mail/store.go`, `internal/mail/mock.go`
- Test: `internal/mail/mock_test.go`

**Interfaces:**
- Consumes: tutti i tipi di `internal/msg`
- Produces (per Piano 2 IMAP/SMTP e Piano 3 TUI):
  - `mail.Page{Offset, Limit int}`
  - `mail.ChangeKind` (`ChangeNewMail`, `ChangeFlags`, `ChangeDisconnected`)
  - `mail.Change{Kind ChangeKind; IDs []msg.MessageID}`
  - `mail.MailStore` — interfaccia identica alla spec §Interfaccia MailStore
  - `mail.Mock` — implementazione in-memory con campi pubblici `Msgs []*msg.Message` (ordinati per Data desc dal metodo, non richiesto all'inserimento), `Sent []msg.Outgoing`, `Err error`, `Raw map[msg.MessageID]map[int][]byte` (contenuti allegati per `SaveAttachment`), metodo `(m *Mock) Emit(c Change)` per i test

- [ ] **Step 1: scrivi il test fallente**

```go
package mail

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gmail-tui/internal/msg"
)

func newMock() *Mock {
	m := &Mock{Raw: map[msg.MessageID]map[int][]byte{}}
	add := func(uid uint32, from, subject string, unread bool, hoursAgo int, atts ...string) {
		id := msg.NewID("INBOX", uid)
		mm := &msg.Message{Ref: msg.MessageRef{
			ID: id, From: from, Subject: subject, Unread: unread,
			Date: time.Now().Add(-time.Duration(hoursAgo) * time.Hour),
		}}
		for i, name := range atts {
			mm.Attachments = append(mm.Attachments, msg.Attachment{
				PartIndex: i, Filename: name, ContentType: "application/octet-stream", Size: 4,
			})
			if m.Raw[id] == nil {
				m.Raw[id] = map[int][]byte{}
			}
			m.Raw[id][i] = []byte("data")
		}
		m.Msgs = append(m.Msgs, mm)
	}
	add(1, "alice@example.com", "Primo", true, 1)
	add(2, "bob@example.org", "Secondo", false, 2, "doc.pdf")
	add(3, "alice@example.com", "Terzo", true, 3)
	add(4, "carol@example.net", "Quarto", false, 4)
	add(5, "alice@example.com", "Quinto", false, 5)
	return m
}

func TestListSortedAndPaginated(t *testing.T) {
	m := newMock()
	got, err := m.ListMessages(context.Background(), msg.Query{}, Page{Offset: 1, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("attesi 2 ref, avuti %d", len(got))
	}
	if got[0].Subject != "Secondo" || got[1].Subject != "Terzo" {
		t.Fatalf("ordinamento data desc + offset rotto: %q %q", got[0].Subject, got[1].Subject)
	}
}

func TestListFilterUnread(t *testing.T) {
	m := newMock()
	got, _ := m.ListMessages(context.Background(), msg.Query{Unread: true}, Page{})
	if len(got) != 2 {
		t.Fatalf("non letti attesi 2, avuti %d", len(got))
	}
}

func TestSearchText(t *testing.T) {
	m := newMock()
	got, _ := m.Search(context.Background(), msg.Query{Text: "quinto"})
	if len(got) != 1 || got[0].Subject != "Quinto" {
		t.Fatalf("search text: %+v", got)
	}
	got, _ = m.Search(context.Background(), msg.Query{From: "alice@example.com"})
	if len(got) != 3 {
		t.Fatalf("search from: %d risultati", len(got))
	}
}

func TestFetchAndCacheCopySemantics(t *testing.T) {
	m := newMock()
	f1, err := m.FetchMessage(context.Background(), msg.NewID("INBOX", 1))
	if err != nil {
		t.Fatal(err)
	}
	f1.Ref.Unread = false // la copia non deve inquinare lo store
	f2, _ := m.FetchMessage(context.Background(), msg.NewID("INBOX", 1))
	if !f2.Ref.Unread {
		t.Fatal("FetchMessage deve restituire una copia difensiva")
	}
}

func TestMarkReadEmitsChange(t *testing.T) {
	m := newMock()
	ctx := context.Background()
	ch, err := m.Watch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id := msg.NewID("INBOX", 1)
	if err := m.MarkRead(ctx, []msg.MessageID{id}, false); err != nil {
		t.Fatal(err)
	}
	select {
	case c := <-ch:
		if c.Kind != ChangeFlags || len(c.IDs) != 1 || c.IDs[0] != id {
			t.Fatalf("change inatteso: %+v", c)
		}
	default:
		t.Fatal("MarkRead deve emettere ChangeFlags su Watch")
	}
}

func TestArchiveRemoves(t *testing.T) {
	m := newMock()
	if err := m.Archive(context.Background(), []msg.MessageID{msg.NewID("INBOX", 3)}); err != nil {
		t.Fatal(err)
	}
	got, _ := m.ListMessages(context.Background(), msg.Query{}, Page{})
	if len(got) != 4 {
		t.Fatalf("dopo archive attesi 4, avuti %d", len(got))
	}
}

func TestSendRecords(t *testing.T) {
	m := newMock()
	out := msg.Outgoing{From: "me@example.com", To: []string{"a@example.com"}, Subject: "ciao"}
	if err := m.Send(context.Background(), out); err != nil {
		t.Fatal(err)
	}
	if len(m.Sent) != 1 || m.Sent[0].Subject != "ciao" {
		t.Fatalf("Sent = %+v", m.Sent)
	}
}

func TestSaveAttachmentWritesFile(t *testing.T) {
	m := newMock()
	path := filepath.Join(t.TempDir(), "doc.pdf")
	if err := m.SaveAttachment(context.Background(), msg.NewID("INBOX", 2), 0, path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "data" {
		t.Fatalf("contenuto allegato sbagliato: %q %v", data, err)
	}
}

func TestErrorInjection(t *testing.T) {
	m := newMock()
	m.Err = errors.New("boom")
	if _, err := m.ListMessages(context.Background(), msg.Query{}, Page{}); err == nil {
		t.Fatal("Err iniettato deve propagarsi")
	}
}
```

- [ ] **Step 2: esegui, verifica che fallisca**

Run: `go test ./internal/mail/`
Expected: FAIL — `undefined: Mock`

- [ ] **Step 3: implementa `store.go`**

```go
// Package mail definisce il contratto MailStore (spec §Interfaccia
// MailStore): la UI parla SOLO con questa interfaccia, mai con IMAP.
package mail

import (
	"context"

	"gmail-tui/internal/msg"
)

// Page è la paginazione di ListMessages (Limit 0 = default 50).
type Page struct {
	Offset, Limit int
}

// ChangeKind indica cosa è cambiato nel feed Watch.
type ChangeKind int

const (
	ChangeNewMail ChangeKind = iota
	ChangeFlags
	ChangeDisconnected
)

// Change è un evento push (IMAP IDLE nel backend reale).
type Change struct {
	Kind ChangeKind
	IDs  []msg.MessageID
}

// MailStore è l'astrazione di backend posta.
type MailStore interface {
	Connect(ctx context.Context) error
	ListMessages(ctx context.Context, q msg.Query, pg Page) ([]msg.MessageRef, error)
	FetchMessage(ctx context.Context, id msg.MessageID) (*msg.Message, error)
	Search(ctx context.Context, q msg.Query) ([]msg.MessageRef, error)
	MarkRead(ctx context.Context, ids []msg.MessageID, read bool) error
	Archive(ctx context.Context, ids []msg.MessageID) error
	Star(ctx context.Context, ids []msg.MessageID, on bool) error
	ApplyLabel(ctx context.Context, ids []msg.MessageID, label string, on bool) error
	SaveAttachment(ctx context.Context, id msg.MessageID, part int, path string) error
	Send(ctx context.Context, m msg.Outgoing) error
	Watch(ctx context.Context) (<-chan Change, error)
}
```

- [ ] **Step 4: implementa `mock.go`**

```go
package mail

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"gmail-tui/internal/msg"
)

// Mock è un MailStore in-memory per i test della UI e della logica.
// Campo Err non-nil: ogni chiamata fallisce (injection per test errori).
type Mock struct {
	mu      sync.Mutex
	Msgs    []*msg.Message
	Sent    []msg.Outgoing
	Err     error
	Raw     map[msg.MessageID]map[int][]byte
	changes chan Change
}

// Emit invia un change a tutti i watcher (per i test).
func (m *Mock) Emit(c Change) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.init()
	select {
	case m.changes <- c:
	default: // watcher lento: l'evento si perde, la UI farà re-list
	}
}

func (m *Mock) init() {
	if m.changes == nil {
		m.changes = make(chan Change, 16)
	}
}

func (m *Mock) fail() error { return m.Err }

// Connect implementa MailStore.
func (m *Mock) Connect(context.Context) error { return m.fail() }

// ListMessages implementa MailStore: data desc + offset/limit.
func (m *Mock) ListMessages(_ context.Context, q msg.Query, pg Page) ([]msg.MessageRef, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, m.Err
	}
	refs := make([]msg.MessageRef, 0, len(m.Msgs))
	for _, mm := range m.Msgs {
		if matches(q, mm) {
			refs = append(refs, mm.Ref)
		}
	}
	sort.SliceStable(refs, func(i, j int) bool { return refs[i].Date.After(refs[j].Date) })
	if pg.Limit == 0 {
		pg.Limit = 50
	}
	if pg.Offset >= len(refs) {
		return nil, nil
	}
	end := pg.Offset + pg.Limit
	if end > len(refs) {
		end = len(refs)
	}
	return refs[pg.Offset:end], nil
}

// FetchMessage implementa MailStore (copia difensiva).
func (m *Mock) FetchMessage(_ context.Context, id msg.MessageID) (*msg.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return nil, m.Err
	}
	for _, mm := range m.Msgs {
		if mm.Ref.ID == id {
			cp := *mm
			cp.To = append([]string(nil), mm.To...)
			cp.Cc = append([]string(nil), mm.Cc...)
			cp.Attachments = append([]msg.Attachment(nil), mm.Attachments...)
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("messaggio %s non trovato", id)
}

// Search implementa MailStore: stesso filtro di ListMessages, senza pagine.
func (m *Mock) Search(ctx context.Context, q msg.Query) ([]msg.MessageRef, error) {
	return m.ListMessages(ctx, q, Page{Limit: 1000})
}

// MarkRead implementa MailStore.
func (m *Mock) MarkRead(_ context.Context, ids []msg.MessageID, read bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	for _, mm := range m.Msgs {
		for _, id := range ids {
			if mm.Ref.ID == id {
				mm.Ref.Unread = !read
			}
		}
	}
	m.init()
	select {
	case m.changes <- Change{Kind: ChangeFlags, IDs: ids}:
	default:
	}
	return nil
}

// Archive implementa MailStore: rimuove dalla lista (via da INBOX).
func (m *Mock) Archive(_ context.Context, ids []msg.MessageID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	kept := m.Msgs[:0]
	for _, mm := range m.Msgs {
		drop := false
		for _, id := range ids {
			if mm.Ref.ID == id {
				drop = true
			}
		}
		if !drop {
			kept = append(kept, mm)
		}
	}
	m.Msgs = kept
	m.init()
	select {
	case m.changes <- Change{Kind: ChangeFlags, IDs: ids}:
	default:
	}
	return nil
}

// Star implementa MailStore.
func (m *Mock) Star(_ context.Context, ids []msg.MessageID, on bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	for _, mm := range m.Msgs {
		for _, id := range ids {
			if mm.Ref.ID == id {
				mm.Ref.Starred = on
			}
		}
	}
	return nil
}

// ApplyLabel implementa MailStore: no-op nel mock (le label sono IMAP-only).
func (m *Mock) ApplyLabel(_ context.Context, ids []msg.MessageID, _ string, _ bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_ = ids
	return m.fail()
}

// SaveAttachment implementa MailStore usando i contenuti in m.Raw.
func (m *Mock) SaveAttachment(_ context.Context, id msg.MessageID, part int, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	data, ok := m.Raw[id][part]
	if !ok {
		return fmt.Errorf("allegato %s/%d non trovato", id, part)
	}
	return os.WriteFile(path, data, 0o644)
}

// Send implementa MailStore: registra in Sent.
func (m *Mock) Send(_ context.Context, out msg.Outgoing) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	m.Sent = append(m.Sent, out)
	return nil
}

// Watch implementa MailStore.
func (m *Mock) Watch(context.Context) (<-chan Change, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.init()
	return m.changes, nil
}

// matches applica msg.Query a un messaggio (case-insensitive).
func matches(q msg.Query, m *msg.Message) bool {
	if q.Unread && !m.Ref.Unread {
		return false
	}
	if q.Starred && !m.Ref.Starred {
		return false
	}
	if q.HasAttachment && len(m.Attachments) == 0 {
		return false
	}
	if q.From != "" && !strings.Contains(strings.ToLower(m.Ref.From), strings.ToLower(q.From)) {
		return false
	}
	if q.To != "" && !containsAny(m.To, q.To) {
		return false
	}
	if q.Text != "" {
		hay := strings.ToLower(m.Ref.Subject + " " + m.Ref.From + " " + m.BodyText)
		if !strings.Contains(hay, strings.ToLower(q.Text)) {
			return false
		}
	}
	// q.Label: non supportato dal mock (IMAP-only), ignorato di proposito.
	return true
}

func containsAny(addrs []string, want string) bool {
	for _, a := range addrs {
		if strings.Contains(strings.ToLower(a), strings.ToLower(want)) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 5: esegui, verifica che passi**

Run: `go test ./internal/mail/ -v`
Expected: PASS (9 test)

- [ ] **Step 6: Commit**

```bash
git add internal/mail/
git commit -m "feat(mail): MailStore interface and in-memory mock with change feed"
```

---

### Task 7: cache bbolt

**Files:**
- Create: `internal/mail/cache/cache.go`
- Test: `internal/mail/cache/cache_test.go`

**Interfaces:**
- Consumes: `msg.Message`, `msg.NewID`, `go.etcd.io/bbolt`
- Produces: `cache.Cache`; `cache.Open(path string) (*Cache, error)`; `(c *Cache) SetMessage(mailbox string, uidValidity uint32, m *msg.Message) error` (invalida automaticamente la mailbox se `uidValidity` cambia); `(c *Cache) GetMessage(mailbox string, uid uint32) (*msg.Message, bool, error)` (bool = hit); `(c *Cache) UIDValidity(mailbox string) (uint32, bool, error)`; `(c *Cache) DropMailbox(mailbox string) error`; `(c *Cache) Close() error`

- [ ] **Step 1: scrivi il test fallente**

```go
package cache

import (
	"path/filepath"
	"testing"
	"time"

	"gmail-tui/internal/msg"
)

func newCache(t *testing.T) *Cache {
	t.Helper()
	c, err := Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func sample(uid uint32) *msg.Message {
	return &msg.Message{Ref: msg.MessageRef{
		ID:      msg.NewID("INBOX", uid),
		Subject: "ciao",
		Date:    time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC),
	}, BodyText: "corpo"}
}

func TestSetGetRoundtrip(t *testing.T) {
	c := newCache(t)
	if err := c.SetMessage("INBOX", 1, sample(7)); err != nil {
		t.Fatalf("SetMessage: %v", err)
	}
	got, ok, err := c.GetMessage("INBOX", 7)
	if err != nil || !ok {
		t.Fatalf("GetMessage: ok=%v err=%v", ok, err)
	}
	if got.Ref.Subject != "ciao" || got.BodyText != "corpo" {
		t.Fatalf("roundtrip rotto: %+v", got)
	}
}

func TestGetMiss(t *testing.T) {
	c := newCache(t)
	_, ok, err := c.GetMessage("INBOX", 999)
	if err != nil || ok {
		t.Fatalf("miss atteso: ok=%v err=%v", ok, err)
	}
}

func TestUIDValidityInvalidation(t *testing.T) {
	c := newCache(t)
	if err := c.SetMessage("INBOX", 1, sample(7)); err != nil {
		t.Fatal(err)
	}
	// il server ha rigenerato le mailbox: uidValidity scatta da 1 a 2
	if err := c.SetMessage("INBOX", 2, sample(8)); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := c.GetMessage("INBOX", 7); ok {
		t.Fatal("vecchio UID deve essere invalidato dal cambio uidValidity")
	}
	got, ok, _ := c.GetMessage("INBOX", 8)
	if !ok || got == nil {
		t.Fatal("nuovo UID dopo invalidazione non trovato")
	}
	v, ok, _ := c.UIDValidity("INBOX")
	if !ok || v != 2 {
		t.Fatalf("UIDValidity = %d ok=%v, atteso 2", v, ok)
	}
}

func TestDropMailbox(t *testing.T) {
	c := newCache(t)
	_ = c.SetMessage("Label", 1, sample(7))
	if err := c.DropMailbox("Label"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := c.GetMessage("Label", 7); ok {
		t.Fatal("DropMailbox non ha svuotato")
	}
}
```

- [ ] **Step 2: esegui, verifica che fallisca**

Run: `go test ./internal/mail/cache/`
Expected: FAIL — `undefined: Open`

- [ ] **Step 3: implementazione minima**

```go
// Package cache persiste messaggi normalizzati in bbolt (spec §cache).
// Schema: bucket "meta:<mailbox>" chiave "uidvalidity";
// bucket "msgs:<mailbox>" chiave = uid, valore = Message in JSON.
package cache

import (
	"encoding/json"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"

	"gmail-tui/internal/msg"
)

// Cache è la cache locale dei messaggi.
type Cache struct {
	db *bolt.DB
}

// Open apre/crea il database bbolt in path.
func Open(path string) (*Cache, error) {
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, fmt.Errorf("cache open %s: %w", path, err)
	}
	return &Cache{db: db}, nil
}

// SetMessage salva un messaggio; se uidValidity è cambiata da quella
// memorizzata, la mailbox viene svuotata prima (invalidazione spec).
func (c *Cache) SetMessage(mailbox string, uidValidity uint32, m *msg.Message) error {
	return c.db.Update(func(tx *bolt.Tx) error {
		meta, err := tx.CreateBucketIfNotExists([]byte("meta:" + mailbox))
		if err != nil {
			return err
		}
		if prev := meta.Get([]byte("uidvalidity")); prev != nil && binaryUint32(prev) != uidValidity {
			if err := tx.DeleteBucket([]byte("msgs:" + mailbox)); err != nil && err != bolt.ErrBucketNotFound {
				return err
			}
		}
		if err := meta.Put([]byte("uidvalidity"), uint32Bytes(uidValidity)); err != nil {
			return err
		}
		msgs, err := tx.CreateBucketIfNotExists([]byte("msgs:" + mailbox))
		if err != nil {
			return err
		}
		data, err := json.Marshal(m)
		if err != nil {
			return fmt.Errorf("marshal %s: %w", m.Ref.ID, err)
		}
		return msgs.Put(uidKey(m.Ref.ID), data)
	})
}

// GetMessage recupera per mailbox+uid; secondo valore false = miss.
func (c *Cache) GetMessage(mailbox string, uid uint32) (*msg.Message, bool, error) {
	var out *msg.Message
	err := c.db.View(func(tx *bolt.Tx) error {
		msgs := tx.Bucket([]byte("msgs:" + mailbox))
		if msgs == nil {
			return nil
		}
		data := msgs.Get(uidKey(msg.NewID(mailbox, uid)))
		if data == nil {
			return nil
		}
		var m msg.Message
		if err := json.Unmarshal(data, &m); err != nil {
			return fmt.Errorf("unmarshal %s/%d: %w", mailbox, uid, err)
		}
		out = &m
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return out, out != nil, nil
}

// UIDValidity restituisce l'ultima uidvalidity memorizzata per mailbox.
func (c *Cache) UIDValidity(mailbox string) (uint32, bool, error) {
	var v uint32
	var ok bool
	err := c.db.View(func(tx *bolt.Tx) error {
		meta := tx.Bucket([]byte("meta:" + mailbox))
		if meta == nil {
			return nil
		}
		if b := meta.Get([]byte("uidvalidity")); b != nil {
			v, ok = binaryUint32(b), true
		}
		return nil
	})
	return v, ok, err
}

// DropMailbox svuota i dati della mailbox.
func (c *Cache) DropMailbox(mailbox string) error {
	return c.db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket([]byte("msgs:" + mailbox)); err != nil && err != bolt.ErrBucketNotFound {
			return err
		}
		if err := tx.DeleteBucket([]byte("meta:" + mailbox)); err != nil && err != bolt.ErrBucketNotFound {
			return err
		}
		return nil
	})
}

// Close chiude il database.
func (c *Cache) Close() error { return c.db.Close() }

// uidKey deriva la chiave bucket dal MessageID (8 byte big-endian).
func uidKey(id msg.MessageID) []byte {
	_, uid, err := id.Split()
	if err != nil {
		return []byte(id) // fallback: id grezza come stringa
	}
	return uint32Bytes(uid)
}

func uint32Bytes(v uint32) []byte {
	return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
}

func binaryUint32(b []byte) uint32 {
	if len(b) != 4 {
		return 0
	}
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}
```

- [ ] **Step 4: esegui, verifica che passi**

Run: `go mod tidy && go test ./internal/mail/cache/ -v`
Expected: PASS (4 test)

- [ ] **Step 5: Commit**

```bash
git add internal/mail/cache/ go.mod go.sum
git commit -m "feat(cache): bbolt message cache with uidvalidity invalidation"
```

---

### Task 8: CLI `themes` e `doctor`

**Files:**
- Create: `internal/cmd/doctor.go`
- Modify: `cmd/gmail-tui/main.go`
- Test: `internal/cmd/doctor_test.go`

**Interfaces:**
- Consumes: `config.{Path,Load}`, `theme.{Names,Load}`, `os.UserCacheDir`
- Produces: `cmd.Doctor(out io.Writer, cfgPath string, getenv func(string) string) int` (0 ok, 1 problemi — usabile dal main e dai test); sottocomandi `gmail-tui themes` e `gmail-tui doctor`

- [ ] **Step 1: scrivi il test fallente**

```go
package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorReportsDefaultsAndMissingPassword(t *testing.T) {
	var out bytes.Buffer
	dir := t.TempDir()
	code := Doctor(&out, filepath.Join(dir, "config.toml"), func(string) string { return "" })
	if code != 0 {
		t.Fatalf("config assente non è errore fatale: code=%d", code)
	}
	s := out.String()
	if !strings.Contains(s, "defaults") {
		t.Errorf("attesa riga config defaults, avuto:\n%s", s)
	}
	if !strings.Contains(s, "password: missing") {
		t.Errorf("attesa riga password missing, avuto:\n%s", s)
	}
}

func TestDoctorReportsBadTheme(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	cfgToml := "address = \"a@example.com\"\ntheme = \"non-esiste\"\n"
	if err := osWriteFile(path, cfgToml); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := Doctor(&out, path, func(string) string { return "" })
	if code != 1 {
		t.Fatalf("tema rotto deve dare exit 1, avuto %d", code)
	}
	if !strings.Contains(out.String(), "theme non-esiste:") {
		t.Errorf("atteso dettaglio tema rotto, avuto:\n%s", out.String())
	}
}

func TestDoctorOKWithEnvPassword(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := osWriteFile(path, "address = \"a@example.com\"\ntheme = \"dracula\"\n"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := Doctor(&out, path, func(k string) string {
		if k == "GMAIL_APP_PASSWORD" {
			return "xxxx xxxx xxxx xxxx"
		}
		return ""
	})
	if code != 0 {
		t.Fatalf("tutto ok atteso exit 0, avuto %d:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "theme: dracula ok") {
		t.Errorf("attesa riga theme ok, avuto:\n%s", out.String())
	}
}
```

helper nel test file:

```go
func osWriteFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
```

(`os` negli import dei test).

- [ ] **Step 2: esegui, verifica che fallisca**

Run: `go test ./internal/cmd/`
Expected: FAIL — `undefined: Doctor`

- [ ] **Step 3: implementa `internal/cmd/doctor.go`**

```go
// Package cmd contiene i sottocomandi CLI non interattivi.
package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gmail-tui/internal/config"
	"gmail-tui/internal/theme"
)

// Doctor diagnostica la configurazione e stampa un report.
// exit code: 0 tutto ok (anche con config assente), 1 problemi bloccanti.
func Doctor(out io.Writer, cfgPath string, getenv func(string) string) int {
	problems := 0

	cfg, err := config.Load(cfgPath)
	switch {
	case err != nil:
		fmt.Fprintf(out, "config: %s ERRORE: %v\n", cfgPath, err)
		problems++
		cfg = config.Default()
	case fileExists(cfgPath):
		fmt.Fprintf(out, "config: %s (address=%q)\n", cfgPath, cfg.Address)
	default:
		fmt.Fprintln(out, "config: nessuna, uso defaults (first-run wizard in arrivo)")
	}

	if _, err := theme.Load(cfg.Theme); err != nil {
		fmt.Fprintf(out, "theme %s: ERRORE: %v\n", cfg.Theme, err)
		problems++
	} else {
		fmt.Fprintf(out, "theme: %s ok\n", cfg.Theme)
	}

	if getenv("GMAIL_APP_PASSWORD") == "" {
		fmt.Fprintln(out, "password: missing — export GMAIL_APP_PASSWORD o \"gmail-tui login\" (in arrivo)")
	} else {
		fmt.Fprintln(out, "password: GMAIL_APP_PASSWORD ok")
	}

	if dir, err := os.UserCacheDir(); err == nil {
		cacheDir := filepath.Join(dir, "gmail-tui")
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			fmt.Fprintf(out, "cache: %s NON SCRIVIBILE: %v\n", cacheDir, err)
			problems++
		} else {
			fmt.Fprintf(out, "cache: %s ok\n", cacheDir)
		}
	}

	return problems
}

// Themes stampa i temi disponibili (uno per riga).
func Themes(out io.Writer) {
	for _, name := range theme.Names() {
		fmt.Fprintln(out, name)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
```

- [ ] **Step 4: aggiorna `cmd/gmail-tui/main.go`**

```go
package main

import (
	"fmt"
	"os"

	"gmail-tui/internal/cmd"
	"gmail-tui/internal/config"
)

// version viene iniettata a build time: -ldflags "-X main.version=0.1.0"
var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version":
			fmt.Println("gmail-tui " + version)
			return
		case "themes":
			cmd.Themes(os.Stdout)
			return
		case "doctor":
			cfgPath, err := config.Path()
			if err != nil {
				fmt.Fprintf(os.Stderr, "config path: %v\n", err)
				os.Exit(1)
			}
			os.Exit(cmd.Doctor(os.Stdout, cfgPath, os.Getenv))
		}
	}
	fmt.Fprintln(os.Stderr, "comandi: version | themes | doctor")
	os.Exit(1)
}
```

- [ ] **Step 5: esegui tutto e prova a mano**

Run: `go test ./... && go run ./cmd/gmail-tui themes && go run ./cmd/gmail-tui doctor`
Expected: tutti i test PASS; `themes` stampa 12 nomi (default primo); `doctor` exit 0 con `password: missing`

- [ ] **Step 6: Commit**

```bash
git add internal/cmd/ cmd/
git commit -m "feat(cli): themes and doctor subcommands"
```

---

### Task 9: CI GitHub Actions + golangci-lint

**Files:**
- Create: `.github/workflows/ci.yml`, `.golangci.yml`

**Interfaces:**
- Consumes: repo con tutti i test verdi
- Produces: pipeline CI (lint + test con race + build matrix macOS/Linux/Windows)

- [ ] **Step 1: crea `.golangci.yml`**

```yaml
version: "2"
linters:
  enable:
    - errcheck
    - govet
    - staticcheck
    - unused
```

- [ ] **Step 2: crea `.github/workflows/ci.yml`**

```yaml
name: CI
on:
  push:
    branches: [main]
  pull_request:

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25"
      - uses: golangci/golangci-lint-action@v7
        with:
          version: latest

  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25"
      - run: go vet ./...
      - run: go test -race ./...

  build:
    strategy:
      matrix:
        os: [macos-latest, ubuntu-latest, windows-latest]
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25"
      - run: go build ./...
        env:
          CGO_ENABLED: "0"
```

- [ ] **Step 3: valida la YAML localmente e lancia i check come farà la CI**

Run: `ruby -ryaml -e 'YAML.load_file(".github/workflows/ci.yml"); YAML.load_file(".golangci.yml"); puts "YAML OK"' && go vet ./... && go test -race ./... && CGO_ENABLED=0 go build ./...`
Expected: `YAML OK`, vet pulito, test PASS senza race, build OK

- [ ] **Step 4: Commit**

```bash
git add .github/ .golangci.yml
git commit -m "ci: golangci-lint, race tests, cross-os build matrix"
```

---

## Verifica finale del piano

- [ ] `go vet ./...` verde
- [ ] `go test -race ./...` verde
- [ ] `go run ./cmd/gmail-tui version && go run ./cmd/gmail-tui themes && go run ./cmd/gmail-tui doctor` funzionano
- [ ] `git log --oneline` mostra 9 commit, uno per task

## Prossimo piano (quando questo è eseguito)

Piano 2 — Backend Gmail: `internal/mail/imap` (go-imap v2: Connect/List/Fetch/flags/IDLE/SEARCH), `internal/mail/smtp` (invio), comandi CLI `fetch`/`send` per provarli dal terminale, integrazione GreenMail in CI.
