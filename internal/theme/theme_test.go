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
