package mcpserver

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// Piano 14, Task 1: la password non compare nel messaggio d'errore, anche se
// l'errore originale la contiene dentro un errore avvolto con %w.
func TestRedactRemovesSecretFromWrappedError(t *testing.T) {
	const secret = "abcd-efgh-ijkl-mnop"
	err := fmt.Errorf("login fallito con %s: %w", secret, errors.New("auth "+secret))

	got := Redact(err, secret)
	if got == nil {
		t.Fatal("Redact ha restituito nil per un errore non nil")
	}
	if strings.Contains(got.Error(), secret) {
		t.Fatalf("la password compare ancora: %q", got.Error())
	}
	if !strings.Contains(got.Error(), "***") {
		t.Errorf("segnaposto *** mancante: %q", got.Error())
	}
}

// Piano 14, Task 1: nil resta nil; un segreto vuoto non altera il messaggio.
func TestRedactNilAndEmptySecret(t *testing.T) {
	if Redact(nil, "x") != nil {
		t.Fatal("Redact(nil) deve restare nil")
	}
	err := errors.New("ciao")
	if got := Redact(err, ""); got == nil || got.Error() != "ciao" {
		t.Fatalf("segreto vuoto altera il messaggio: %v", got)
	}
}
