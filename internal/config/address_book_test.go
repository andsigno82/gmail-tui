package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// Piano 9, Task 10 (D9-G): address_book_cmd è un array (niente shell).
func TestLoadReadsAddressBookCmdArray(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	// Il comando deve esistere (review H): si usa il binario di test, percorso assoluto.
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	src := fmt.Sprintf("address_book_cmd = [%q, \"email\", \"--parsable\", \"%%s\"]\n", exe)
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{exe, "email", "--parsable", "%s"}
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

// Piano 9, review H: address_book_cmd deve essere un percorso assoluto o un nome
// nel PATH; un percorso relativo o un comando mancante è un errore chiaro.
func TestAddressBookCmdValidation(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		argv    []string
		wantErr bool
	}{
		{"vuoto", nil, false},
		{"assoluto esistente", []string{exe, "%s"}, false},
		{"relativo con separatore", []string{"./khard", "%s"}, true},
		{"nome mancante nel PATH", []string{"gmail-tui-no-such-cmd-q9z", "%s"}, true},
		{"assoluto inesistente", []string{filepath.Join(t.TempDir(), "nope"), "%s"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := Default()
			c.AddressBookCmd = tc.argv
			err := c.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() = %v, errore atteso: %v", err, tc.wantErr)
			}
		})
	}
}

// Piano 9, review H: un address_book_cmd non valido torna al default (vuoto) senza
// perdere nome e firma, come layout e preview_mark_read.
func TestSanitizedDropsBadAddressBookCmd(t *testing.T) {
	c := Default()
	c.Name = "Anna"
	c.Signature = "firma"
	c.AddressBookCmd = []string{"./khard", "%s"}
	s := c.Sanitized()
	if len(s.AddressBookCmd) != 0 {
		t.Errorf("AddressBookCmd = %v dopo Sanitized, voglio vuoto", s.AddressBookCmd)
	}
	if s.Name != "Anna" || s.Signature != "firma" {
		t.Errorf("nome/firma persi: %q %q", s.Name, s.Signature)
	}
}
