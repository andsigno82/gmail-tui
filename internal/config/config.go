// Package config gestisce ~/.config/gmail-tui/config.toml.
package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
)

// ErrInvalid marca gli errori di validazione (valore non ammesso): Load
// restituisce comunque i campi decodificati, così il chiamante può riportare al
// default solo i campi sbagliati (Piano 7.2, fix review). Un errore di parsing o
// di lettura NON è ErrInvalid.
var ErrInvalid = errors.New("valore non valido")

// Valori ammessi per Layout e PreviewMarkRead (Piano 7.2, Task 2). Il valore
// vuoto è accettato e vale come default (single / delay).
const (
	LayoutSingle = "single"
	LayoutSplit  = "split"

	PreviewMarkReadImmediate = "immediate"
	PreviewMarkReadDelay     = "delay"
	PreviewMarkReadNever     = "never"
)

// LayoutValues restituisce i valori ammessi per Layout, nell'ordine del menu
// impostazioni (Piano 7.2, Task 2).
func LayoutValues() []string {
	return []string{LayoutSingle, LayoutSplit}
}

// PreviewMarkReadValues restituisce i valori ammessi per PreviewMarkRead, nell'
// ordine del menu impostazioni (Piano 7.2, Task 2).
func PreviewMarkReadValues() []string {
	return []string{PreviewMarkReadImmediate, PreviewMarkReadDelay, PreviewMarkReadNever}
}

// Config è la configurazione utente. Gli host IMAP/SMTP sono preset
// Gmail fissi (spec) e non sono configurabili in v1. Layout e PreviewMarkRead
// sono le impostazioni di lettura (Piano 7.2, Task 2).
type Config struct {
	Address         string `toml:"address"`
	Name            string `toml:"name"`
	Signature       string `toml:"signature"`
	Theme           string `toml:"theme"`
	Layout          string `toml:"layout"`
	PreviewMarkRead string `toml:"preview_mark_read"`
	// AddressBookCmd (Piano 9, Task 10, D9-G): comando esterno dei contatti per
	// l'autocompletamento di A/Cc, eseguito senza shell; "%s" = la query.
	AddressBookCmd []string `toml:"address_book_cmd"`
}

// Default restituisce la configurazione di fabbrica (Piano 7.2, Task 2: layout
// a una colonna, segna come letto in anteprima dopo 2 secondi).
func Default() Config {
	return Config{Theme: "default", Layout: LayoutSingle, PreviewMarkRead: PreviewMarkReadDelay}
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
	err = toml.NewEncoder(f).Encode(c)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// Sanitized riporta al default i soli campi di lettura con valore non
// ammesso, conservando tutto il resto (Piano 7.2, fix review: `gmail-tui
// login` non deve perdere nome e firma per un layout scritto male).
func (c Config) Sanitized() Config {
	d := Default()
	if !slices.Contains(LayoutValues(), c.Layout) {
		c.Layout = d.Layout
	}
	if !slices.Contains(PreviewMarkReadValues(), c.PreviewMarkRead) {
		c.PreviewMarkRead = d.PreviewMarkRead
	}
	if len(c.AddressBookCmd) > 0 && checkCommand(c.AddressBookCmd[0]) != nil {
		c.AddressBookCmd = d.AddressBookCmd // Piano 9, review H: comando non valido → solo locale
	}
	return c
}

// Validate controlla i campi con formato noto (Piano 7.2, Task 2: Layout e
// PreviewMarkRead accettano solo i valori ammessi, o il vuoto).
func (c Config) Validate() error {
	if c.Address != "" && !strings.Contains(c.Address, "@") {
		return fmt.Errorf("%w: address %q, manca @", ErrInvalid, c.Address)
	}
	if c.Layout != "" && !slices.Contains(LayoutValues(), c.Layout) {
		return fmt.Errorf("%w: layout %q, ammessi %s", ErrInvalid, c.Layout, strings.Join(LayoutValues(), ", "))
	}
	if c.PreviewMarkRead != "" && !slices.Contains(PreviewMarkReadValues(), c.PreviewMarkRead) {
		return fmt.Errorf("%w: preview_mark_read %q, ammessi %s", ErrInvalid, c.PreviewMarkRead, strings.Join(PreviewMarkReadValues(), ", "))
	}
	if len(c.AddressBookCmd) > 0 {
		if err := checkCommand(c.AddressBookCmd[0]); err != nil {
			return fmt.Errorf("%w: address_book_cmd: %v", ErrInvalid, err)
		}
	}
	return nil
}

// checkCommand accetta un percorso assoluto esistente o un nome senza separatori
// risolto nel PATH (Piano 9, review H). Il percorso relativo è rifiutato: il
// comando dei contatti non deve dipendere dalla directory di lavoro.
func checkCommand(argv0 string) error {
	if filepath.IsAbs(argv0) {
		if _, err := os.Stat(argv0); err != nil {
			return fmt.Errorf("comando %q non trovato", argv0)
		}
		return nil
	}
	if strings.ContainsAny(argv0, `/\`) {
		return fmt.Errorf("percorso relativo %q non ammesso: usa un percorso assoluto o un nome nel PATH", argv0)
	}
	if _, err := exec.LookPath(argv0); err != nil {
		return fmt.Errorf("comando %q non trovato nel PATH", argv0)
	}
	return nil
}
