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
	Fg        Slot   `toml:"fg"`
	Bg        Slot   `toml:"bg"`
	Muted     Slot   `toml:"muted"`
	Border    Slot   `toml:"border"`
	Primary   Slot   `toml:"primary"`
	Accent    Slot   `toml:"accent"`
	Error     Slot   `toml:"error"`
	Success   Slot   `toml:"success"`
	Warning   Slot   `toml:"warning"`
	Unread    Slot   `toml:"unread"`
	Starred   Slot   `toml:"starred"`
	Quoted    Slot   `toml:"quoted"`
	Help      Slot   `toml:"help"`
	Selection Slot   `toml:"selection"`
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
