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
