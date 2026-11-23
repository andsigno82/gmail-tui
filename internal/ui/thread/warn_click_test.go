package thread

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/ui/render"
)

// Piano 15, Task 16 (regola di sicurezza del coordinatore): un link o un bottone con
// Warn (testo con un dominio diverso dal reale) non si apre al clic: il thread
// riporta all'app l'URL e il dominio mostrato, che chiede conferma.

const htmlWarn = `<p>Accedi su <a href="https://evil.example.net/login">www.paypal.com</a> ora.</p>
<p><a class="btn" href="https://evil.example.net/pay">paypal.com/paga</a></p>`

func warnModel(t *testing.T) Model {
	t.Helper()
	full := testMsg()
	full.BodyHTML = htmlWarn
	m := newModel(t, full)
	m.SetSize(80, 40)
	return m
}

func TestClickSuLinkWarnNonApreEChiedeConferma(t *testing.T) {
	opened := false
	orig := openLink
	openLink = func(string) error { opened = true; return nil }
	t.Cleanup(func() { openLink = orig })

	m := warnModel(t)
	row, col := rowOf(t, m, "www.paypal.com")
	z, ok := m.ZoneAt(col, headerLines+row)
	if !ok || z.Kind != render.ZoneLink || !z.Warn {
		t.Fatalf("ZoneAt sul link = %+v,%v, voglio ZoneLink con Warn", z, ok)
	}
	if z.Shown != "www.paypal.com" {
		t.Errorf("Zone.Shown = %q, voglio il testo mostrato senza marcatore", z.Shown)
	}
	_, cmd := m.Update(tea.MouseClickMsg{X: col, Y: headerLines + row, Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("clic sul link Warn: nessuna Cmd")
	}
	got, ok := cmd().(LinkWarnMsg)
	if !ok || got.URL != "https://evil.example.net/login" || got.Shown != "www.paypal.com" {
		t.Errorf("messaggio = %#v, voglio LinkWarnMsg con URL e testo mostrato", got)
	}
	if opened {
		t.Error("un link con Warn non deve aprirsi subito")
	}
}

func TestClickSuBottoneWarnPortaWarnEShown(t *testing.T) {
	m := warnModel(t)
	row, col := rowOf(t, m, "paypal.com/paga")
	cmd, ok := m.LinkClick(col, headerLines+row)
	if !ok {
		t.Fatal("bottone non riconosciuto")
	}
	got, ok := cmd().(ButtonClickMsg)
	if !ok || !got.Warn || got.Shown != "paypal.com/paga" || got.URL != "https://evil.example.net/pay" {
		t.Errorf("messaggio = %#v, voglio ButtonClickMsg con Warn e Shown", got)
	}
}

// Richiesta di Andrea (2026-11-23): un link spezzato su più righe porta il testo
// mostrato COMPLETO su ogni sua zona, così la conferma cita il dominio intero.
func TestShownDiUnLinkSpezzatoEInteroSuTutteLeZone(t *testing.T) {
	const shown = "www.paypal-account-verification-center.com/signin/secure"
	full := testMsg()
	full.BodyHTML = `<p>Accedi: <a href="https://evil.example.net/login">` + shown + `</a> ora.</p>`
	m := newModel(t, full)
	m.SetSize(30, 40)
	rows := map[int]bool{}
	for y := headerLines; y < headerLines+30; y++ {
		for x := 0; x < 30; x++ {
			z, ok := m.ZoneAt(x, y)
			if !ok || !z.Warn {
				continue
			}
			rows[y] = true
			if z.Shown != shown {
				t.Errorf("riga %d: Shown = %q, voglio il testo intero %q", y, z.Shown, shown)
			}
		}
	}
	if len(rows) < 2 {
		t.Fatalf("il link doveva spezzarsi su più righe: zone su %d righe", len(rows))
	}
}
