package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/ui/toast"
)

// init abbassa la durata dei toast: i test eseguono le cmd in modo sincrono e
// non devono attendere 3 s reali (Piano 7.3, Task 2).
func init() {
	toast.Duration = time.Millisecond
}

// dropExpired toglie gli ExpiredMsg dei timer dei toast dai Msg prodotti da una
// cmd: le asserzioni sul resto della cmd non devono dipendere dal timer.
func dropExpired(msgs []tea.Msg) []tea.Msg {
	var out []tea.Msg
	for _, m := range msgs {
		if _, ok := m.(toast.ExpiredMsg); !ok {
			out = append(out, m)
		}
	}
	return out
}

// Piano 7.3, Task 2: gli esiti delle azioni diventano toast; la barra di stato
// resta neutra e tiene solo lo stato persistente e le conferme modali.

// TestEsitoAzioneEToastNonBarra: un esito di op (opDone) finisce nella pila dei
// toast con stile Success e NON nel testo della barra.
func TestEsitoAzioneEToastNonBarra(t *testing.T) {
	m := newTestModel(t)
	mm, _ := step(m, opDone{label: "archiviata"})
	if got := mm.toast.Texts(); len(got) != 1 || got[0] != "archiviata" {
		t.Fatalf("toast = %q, voglio [archiviata]", got)
	}
	if k := mm.toast.Kinds(); len(k) != 1 || k[0] != toast.Success {
		t.Errorf("kind toast = %v, voglio [Success]", k)
	}
	if got := mm.statusText(); got != "" {
		t.Errorf("la barra conserva l'esito: %q, voglio vuota", got)
	}
	if strings.Contains(mm.statusBar(), "archiviata") {
		t.Errorf("statusBar contiene l'esito: %q", mm.statusBar())
	}
}

// TestToastSuccessScadeConExpiredMsg: il toast di successo sparisce col proprio
// ExpiredMsg (ID 1: primo toast di un modello appena costruito).
func TestToastSuccessScadeConExpiredMsg(t *testing.T) {
	m := newTestModel(t)
	mm, _ := step(m, opDone{label: "letta"})
	if got := mm.toast.Texts(); len(got) != 1 {
		t.Fatalf("toast = %q, voglio 1 toast prima della scadenza", got)
	}
	mm, _ = step(mm, toast.ExpiredMsg{ID: 1})
	if got := mm.toast.Texts(); len(got) != 0 {
		t.Errorf("toast ancora in pila dopo ExpiredMsg: %q", got)
	}
}

// TestToastErroreRestaFinoAlTasto: un errore di op resta oltre il suo
// ExpiredMsg e sparisce al primo tasto premuto (il tasto viene eseguito).
func TestToastErroreRestaFinoAlTasto(t *testing.T) {
	m := newTestModel(t)
	mm, _ := step(m, opDone{label: "stella on", err: errors.New("rete giù")})
	if k := mm.toast.Kinds(); len(k) != 1 || k[0] != toast.Error {
		t.Fatalf("kind toast = %v, voglio [Error]", k)
	}
	mm, _ = step(mm, toast.ExpiredMsg{ID: 1})
	if got := mm.toast.Texts(); len(got) != 1 {
		t.Fatalf("l'errore è sparito con ExpiredMsg: %q", got)
	}
	mm, _ = step(mm, pressKey("j"))
	if got := mm.toast.Texts(); len(got) != 0 {
		t.Errorf("il tasto non ha chiuso l'errore: %q", got)
	}
}

// TestStatoPersistenteRestaInBarra: l'indicatore ⟳ di riconnessione è stato
// persistente: resta nella barra neutra e non diventa un toast.
func TestStatoPersistenteRestaInBarra(t *testing.T) {
	mock := &mail.Mock{Msgs: mockMsgs(t)}
	m := watchModel(t, mock)
	mm, _ := step(m, changeEvt{c: mail.Change{Kind: mail.ChangeDisconnected}})
	if got := mm.statusText(); got != reconnectIndicator {
		t.Errorf("barra = %q, voglio %q", got, reconnectIndicator)
	}
	if !strings.Contains(mm.statusBar(), "⟳ riconnessione") {
		t.Errorf("statusBar senza ⟳: %q", mm.statusBar())
	}
	if got := mm.toast.Texts(); len(got) != 0 {
		t.Errorf("il ⟳ è finito in un toast: %q", got)
	}
}

// TestNuovaPostaEToastInfo: "nuova posta in arrivo" è feedback informativo, quindi
// toast Info (la barra non lo tiene più).
func TestNuovaPostaEToastInfo(t *testing.T) {
	mock := &mail.Mock{Msgs: mockMsgs(t)}
	m := watchModel(t, mock)
	mm, _ := step(m, changeEvt{c: mail.Change{Kind: mail.ChangeNewMail}})
	if got := mm.toast.Texts(); len(got) != 1 || got[0] != newMailText {
		t.Fatalf("toast = %q, voglio [%q]", got, newMailText)
	}
	if k := mm.toast.Kinds(); len(k) != 1 || k[0] != toast.Info {
		t.Errorf("kind toast = %v, voglio [Info]", k)
	}
}

// TestConfermaInvioRestaNellaBarraColorata: la conferma pre-invio è una modale:
// occupa la barra con lo stile StatusErr, non un toast (regressione guard).
func TestConfermaInvioRestaNellaBarraColorata(t *testing.T) {
	m := newTestModel(t)
	m.confirmSend = true
	want := m.renderBar(m.styles.StatusErr, sendConfirmText(m.draft))
	if got := m.statusBar(); got != want {
		t.Errorf("conferma invio: barra = %q, voglio lo stile StatusErr (%q)", got, want)
	}
}

// TestViewToastInAltoADestra: un esito compare come toast sopra il corpo, sulla
// riga 1 (margine di 1 riga dall'alto) e col bordo destro in quella riga.
func TestViewToastInAltoADestra(t *testing.T) {
	m := newTestModel(t)
	m, _ = step(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	mm, _ := step(m, opDone{label: "archiviata"})
	lines := strings.Split(mm.View().Content, "\n")
	if len(lines) < 2 {
		t.Fatalf("View troppo corta: %d righe", len(lines))
	}
	if !strings.Contains(lines[1], "╭") || !strings.Contains(lines[1], "╮") {
		t.Errorf("riga 1 senza bordo del toast: %q", lines[1])
	}
}

// TestViewSenzaToastUgualeAlContenuto: senza toast la View è il contenuto
// nudo (nessun compositing): gli snapshot di oggi restano identici.
func TestViewSenzaToastUgualeAlContenuto(t *testing.T) {
	m := newTestModel(t)
	m, _ = step(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if got, want := m.View().Content, m.content(); got != want {
		t.Errorf("View senza toast diversa dal contenuto:\n%q\nvoglio:\n%q", got, want)
	}
}

// toastText è il testo dell'ultimo toast in pila ("" se la pila è vuota).
func toastText(m Model) string {
	ts := m.toast.Texts()
	if len(ts) == 0 {
		return ""
	}
	return ts[len(ts)-1]
}

// toastIs dice se l'ultimo toast in pila è di tipo k (Piano 7.3, Task 2).
func toastIs(m Model, k toast.Kind) bool {
	ks := m.toast.Kinds()
	return len(ks) > 0 && ks[len(ks)-1] == k
}
