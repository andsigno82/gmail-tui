package app

import (
	"strings"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/composer"
)

// TestSendMalformedRecipientOpensNoConfirmation: un indirizzo malformato
// (A o Cc) non apre la conferma: la status bar cita campo e indirizzo, il
// composer resta aperto e la bozza è intatta (Piano 7, Task 6).
func TestSendMalformedRecipientOpensNoConfirmation(t *testing.T) {
	cases := []struct {
		name string
		cfg  composer.ComposerConfig
		want string
	}{
		{"A malformato", composer.ComposerConfig{To: "abc", Subject: "s", Body: "c"}, `A: indirizzo non valido "abc"`},
		{"Cc malformato", composer.ComposerConfig{To: "anna@x.dev", Cc: "nope", Subject: "s", Body: "c"}, `Cc: indirizzo non valido "nope"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &senderRecorder{}
			m, _ := senderModel(t, rec)
			mm, _ := step(m, teaSize())
			mm = sendRequested(t, draftComposer(mm, tc.cfg))
			if mm.confirmSend {
				t.Error("indirizzo malformato: conferma aperta")
			}
			if mm.currentView() != viewComposer {
				t.Errorf("vista = %v, voglio composer (bozza preservata)", mm.currentView())
			}
			if !mm.status.isErr || !strings.Contains(mm.statusText(), tc.want) {
				t.Errorf("status = %q (err=%v), voglio errore con %q", mm.statusText(), mm.status.isErr, tc.want)
			}
			// BodyText porta la firma in coda: si confronta il prefisso del corpo.
			if d := mm.composer.Draft(); d.Subject != "s" || !strings.HasPrefix(d.BodyText, "c") || strings.Join(d.To, ", ") != tc.cfg.To {
				t.Errorf("bozza dopo il rifiuto = %+v, voglio i campi intatti", d)
			}
			if len(rec.calls) != 0 {
				t.Errorf("sender chiamato %d volte, voglio 0", len(rec.calls))
			}
		})
	}
}

// TestSendValidRecipientKeepsConfirmation: indirizzi validi (anche con nome
// visualizzato) → flusso invariato, conferma aperta, nessun errore.
func TestSendValidRecipientKeepsConfirmation(t *testing.T) {
	rec := &senderRecorder{}
	m, _ := senderModel(t, rec)
	mm, _ := step(m, teaSize())
	mm = sendRequested(t, draftComposer(mm, composer.ComposerConfig{
		To: "Anna Rossi <anna@x.dev>", Cc: "bob@y.dev", Subject: "s", Body: "c",
	}))
	if !mm.confirmSend || mm.status.isErr {
		t.Errorf("indirizzi validi: confirm=%v status=%q (err=%v)", mm.confirmSend, mm.statusText(), mm.status.isErr)
	}
}

// TestSendValidationRunsAfterGuards: l'ordine dei guard conta. Con la bozza
// stante (vista cambiata), con un discard aperto o con un invio in volo il
// messaggio è scartato in silenzio: nessun errore di validazione in status.
func TestSendValidationRunsAfterGuards(t *testing.T) {
	bad := composer.SendRequestedMsg{Out: msg.Outgoing{To: []string{"abc"}, Subject: "s", BodyText: "c"}}

	rec := &senderRecorder{}
	m, _ := senderModel(t, rec)
	mm, _ := step(m, teaSize()) // vista inbox: richiesta stante
	before := mm.statusText()
	mm, _ = step(mm, bad)
	if mm.statusText() != before || mm.status.isErr {
		t.Errorf("richiesta stante: status = %q, voglio invariato (%q)", mm.statusText(), before)
	}

	mm = draftComposer(mm, composer.ComposerConfig{Subject: "s", Body: "c"})
	mm.discardOpen = true
	before = mm.statusText()
	mm, _ = step(mm, bad)
	if mm.statusText() != before || mm.status.isErr {
		t.Errorf("discard aperto: status = %q, voglio invariato", mm.statusText())
	}

	mm.discardOpen = false
	mm.sending = true
	mm, _ = step(mm, bad)
	if mm.statusText() != before || mm.status.isErr {
		t.Errorf("invio in volo: status = %q, voglio invariato", mm.statusText())
	}
}

// TestSendWithoutRecipientsIsBlocked (Piano 7, fix review 7): una bozza senza
// destinatari non apre la conferma: errore "nessun destinatario" in status, il
// composer resta aperto con la bozza intatta e il sender non è chiamato.
func TestSendWithoutRecipientsIsBlocked(t *testing.T) {
	rec := &senderRecorder{}
	m, _ := senderModel(t, rec)
	mm, _ := step(m, teaSize())
	mm = sendRequested(t, draftComposer(mm, composer.ComposerConfig{Subject: "Fwd: prima", Body: "corpo"}))
	if mm.confirmSend {
		t.Error("bozza senza destinatari: conferma aperta")
	}
	if !mm.status.isErr || mm.statusText() != "nessun destinatario" {
		t.Errorf("status = %q (err=%v), voglio errore \"nessun destinatario\"", mm.statusText(), mm.status.isErr)
	}
	if mm.currentView() != viewComposer {
		t.Errorf("vista = %v, voglio composer (bozza preservata)", mm.currentView())
	}
	if d := mm.composer.Draft(); d.Subject != "Fwd: prima" {
		t.Errorf("bozza dopo il rifiuto = %+v, voglio i campi intatti", d)
	}
	if len(rec.calls) != 0 {
		t.Errorf("sender chiamato %d volte, voglio 0", len(rec.calls))
	}
}

// TestSendCcOnlyKeepsConfirmation (Piano 7, fix review 7): un destinatario
// solo in Cc è un destinatario: la conferma si apre e mostra l'indirizzo.
func TestSendCcOnlyKeepsConfirmation(t *testing.T) {
	rec := &senderRecorder{}
	m, _ := senderModel(t, rec)
	mm, _ := step(m, teaSize())
	mm = sendRequested(t, draftComposer(mm, composer.ComposerConfig{Cc: "bob@y.dev", Subject: "s", Body: "c"}))
	if !mm.confirmSend || mm.status.isErr {
		t.Fatalf("solo Cc: confirm=%v status=%q (err=%v)", mm.confirmSend, mm.statusText(), mm.status.isErr)
	}
	if bar := plainBar(mm); !strings.Contains(bar, "Invio a bob@y.dev") {
		t.Errorf("conferma = %q, voglio l'indirizzo in Cc", bar)
	}
}
