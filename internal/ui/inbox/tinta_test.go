package inbox

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// bgPerRune restituisce, per ogni carattere visibile di s, lo sfondo SGR attivo
// ("rgb(r,g,b)" oppure "" se nessuno). Serve a verificare che ogni cella della
// riga abbia lo sfondo atteso, senza buchi dovuti a reset fra i segmenti.
func bgPerRune(s string) []string {
	var out []string
	bg := ""
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			if j < len(s) && s[j] == 'm' {
				bg = applySGR(bg, strings.Split(s[i+2:j], ";"))
			}
			i = j + 1
			continue
		}
		out = append(out, bg)
		i++
	}
	return out
}

// applySGR aggiorna lo sfondo corrente con i parametri di una sequenza SGR.
func applySGR(bg string, p []string) string {
	for i := 0; i < len(p); i++ {
		switch p[i] {
		case "", "0", "49":
			bg = ""
		case "48":
			if i+4 < len(p) && p[i+1] == "2" {
				bg = fmt.Sprintf("rgb(%s,%s,%s)", p[i+2], p[i+3], p[i+4])
			}
			i += 4
		case "38":
			if i+1 < len(p) && p[i+1] == "2" {
				i += 4
			} else if i+1 < len(p) && p[i+1] == "5" {
				i += 2
			}
		}
	}
	return bg
}

// requireBgSuTutto verifica che ogni cella visibile di riga abbia lo sfondo want
// e che la riga occupi tutta la larghezza della lista (80).
func requireBgSuTutto(t *testing.T, riga, want string) {
	t.Helper()
	if w := lipgloss.Width(riga); w != 80 {
		t.Fatalf("larghezza riga = %d, voglio 80", w)
	}
	for i, bg := range bgPerRune(riga) {
		if bg != want {
			t.Fatalf("cella %d sfondo %q, voglio %q (riga %q)", i, bg, want, riga)
		}
	}
}

// TestRigaSpuntataTintaSuTuttaLaLarghezza (richiesta di Andrea, 2026-11-23): con
// profilo dark forzato, una riga spuntata ma non sotto il cursore ha come sfondo
// la tinta (bg mescolato al 50% con selection: default dark #16161e+#2e2e42 =
// #222230) su tutte le celle, date comprese. La riga sotto il cursore mantiene
// lo sfondo Selection (#2e2e42).
func TestRigaSpuntataTintaSuTuttaLaLarghezza(t *testing.T) {
	orig := compat.HasDarkBackground
	compat.HasDarkBackground = true
	t.Cleanup(func() { compat.HasDarkBackground = orig })

	m := newTestModel(t, testRefs())
	m.SetSelection(map[msg.MessageID]bool{
		msg.NewID("INBOX", 1): true, // sotto il cursore: prevale Selection
		msg.NewID("INBOX", 2): true, // spuntata, non sotto il cursore: tinta
	})
	r := rows(m)
	if len(r) < 2 {
		t.Fatalf("righe = %d, voglio almeno 2", len(r))
	}
	requireBgSuTutto(t, r[0], "rgb(46,46,66)")
	requireBgSuTutto(t, r[1], "rgb(34,34,48)")
}

// TestRigaSpuntataConEtichetteSullaTinta: le etichette della riga spuntata restano
// sulla tinta (nessuna cella senza sfondo), anche con etichette utente.
func TestRigaSpuntataConEtichetteSullaTinta(t *testing.T) {
	orig := compat.HasDarkBackground
	compat.HasDarkBackground = true
	t.Cleanup(func() { compat.HasDarkBackground = orig })

	m := newTestModel(t, testRefs())
	convs := []mail.Conversation{mail.SingleConversation(testRefs()[0]), mail.SingleConversation(testRefs()[1])}
	convs[1].Labels = []string{"lavoro"}
	m.SetConversations(convs)
	m.SetSelection(map[msg.MessageID]bool{msg.NewID("INBOX", 2): true})
	r := rows(m)
	requireBgSuTutto(t, r[1], "rgb(34,34,48)")
	if !strings.Contains(r[1], "[lavoro]") {
		t.Errorf("riga = %q, voglio l'etichetta [lavoro]", r[1])
	}
}
