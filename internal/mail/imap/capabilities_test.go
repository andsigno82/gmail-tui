package imap

import (
	"context"
	"slices"
	"testing"

	"github.com/emersion/go-imap/v2"
)

// TestCapabilitiesAnnounced (Piano 7.1, Task 4): la probe di doctor legge le
// capability annunciate dal server dopo il login, senza SELECT.
func TestCapabilitiesAnnounced(t *testing.T) {
	ts := startTestServerCaps(t, imap.CapSet{
		imap.CapIMAP4rev1:  {},
		imap.CapMove:       {},
		imap.CapUIDPlus:    {},
		imap.CapSpecialUse: {},
	})
	got, err := Capabilities(t.Context(), Options{
		Addr:     ts.addr(),
		Username: "tester",
		Password: "secret",
		Insecure: true,
	})
	if err != nil {
		t.Fatalf("Capabilities: %v", err)
	}
	for _, want := range []string{"MOVE", "UIDPLUS", "SPECIAL-USE"} {
		if !slices.Contains(got, want) {
			t.Errorf("capability %s mancante in %v", want, got)
		}
	}
	if slices.Contains(got, "LIST-EXTENDED") {
		t.Errorf("LIST-EXTENDED annunciata senza averla configurata: %v", got)
	}
}

// TestCapabilitiesBadPassword: login rifiutato ⇒ errore, nessuna capability.
func TestCapabilitiesBadPassword(t *testing.T) {
	ts := startTestServer(t)
	got, err := Capabilities(t.Context(), Options{
		Addr:     ts.addr(),
		Username: "tester",
		Password: "WRONG",
		Insecure: true,
	})
	if err == nil {
		t.Fatalf("login errato: err = nil, capability %v", got)
	}
}

// TestCapabilitiesHonoursContext: un ctx già scaduto torna subito con l'errore
// del ctx, senza attendere il dial.
func TestCapabilitiesHonoursContext(t *testing.T) {
	ts := startTestServer(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Capabilities(ctx, Options{Addr: ts.addr(), Username: "tester", Password: "secret", Insecure: true}); err == nil {
		t.Fatal("ctx annullato: err = nil, voglio errore")
	}
}
