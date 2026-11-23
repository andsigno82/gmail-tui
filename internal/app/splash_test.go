package app

// Splash d'avvio (Piano 15, Task 16b; richiesta di Andrea, 2026-11-23): finché non
// arriva il primo caricamento della lista si mostra un titolo grande al centro con
// la fase corrente, non la lista vuota. Nessun ritardo artificiale.

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/config"
)

var ansiSeq = regexp.MustCompile("\x1b\\[[0-9;:]*[A-Za-z]")

// plain è la vista a schermo senza sequenze di stile.
func plain(m Model) string { return ansiSeq.ReplaceAllString(m.View().Content, "") }

func plainStr(s string) string { return ansiSeq.ReplaceAllString(s, "") }

func splashModel(t *testing.T, w, h int) Model {
	t.Helper()
	m := newTestModel(t)
	m.SetVersion("v9.9.9")
	mm, _ := step(m, tea.WindowSizeMsg{Width: w, Height: h})
	return mm
}

func TestSplashDuranteConnect(t *testing.T) {
	m := splashModel(t, 140, 35)
	v := plain(m)
	for _, want := range []string{"█", "connessione a imap.gmail.com…", "v9.9.9"} {
		if !strings.Contains(v, want) {
			t.Errorf("splash: manca %q\n%s", want, v)
		}
	}
	for _, bad := range []string{"No items", "nessun messaggio", "Nessun messaggio"} {
		if strings.Contains(v, bad) {
			t.Errorf("splash: non deve mostrare %q", bad)
		}
	}
}

func TestSplashFaseCaricamento(t *testing.T) {
	m := splashModel(t, 140, 35)
	m, _ = step(m, connResult{})
	v := plain(m)
	if !strings.Contains(v, "caricamento posta…") || strings.Contains(v, "connessione a") {
		t.Errorf("dopo il connect la fase è il caricamento\n%s", v)
	}
	if !strings.Contains(v, "█") {
		t.Error("lo splash resta finché non arriva la lista")
	}
}

// Sparisce appena arriva la lista, senza ritardi.
func TestSplashSparisceAllaLista(t *testing.T) {
	refs := testRefs(t)
	m := splashModel(t, 140, 35)
	m, _ = step(m, connResult{})
	m, _ = step(m, msgsLoaded{refs: refs, gen: m.listGen, limit: pageSize})
	v := plain(m)
	if strings.Contains(v, "█") || strings.Contains(v, "caricamento posta…") {
		t.Errorf("lo splash deve sparire alla lista\n%s", v)
	}
	if !strings.Contains(v, "prima") {
		t.Errorf("la lista deve comparire\n%s", v)
	}
}

// Un risultato stantio (altra generazione) non chiude lo splash.
func TestSplashNonSparisceConRisultatoStantio(t *testing.T) {
	m := splashModel(t, 140, 35)
	m, _ = step(m, connResult{})
	m, _ = step(m, msgsLoaded{refs: testRefs(t), gen: m.listGen + 7})
	if !strings.Contains(plain(m), "█") {
		t.Error("risultato stantio: lo splash resta")
	}
}

func TestSplashConnessioneFallita(t *testing.T) {
	m := splashModel(t, 140, 35)
	m, _ = step(m, connResult{err: errors.New("\x1b[31mboom\nrete\x07")})
	v := plain(m)
	if !strings.Contains(v, "boom rete") || !strings.Contains(v, "R per riprovare") {
		t.Errorf("errore e R per riprovare attesi\n%s", v)
	}
	if strings.Contains(v, "connessione a imap.gmail.com…") || strings.Contains(v, "\x07") {
		t.Errorf("niente spinner né controlli grezzi\n%s", v)
	}
	// R funziona come prima: riparte la sequenza e la fase torna "connessione".
	m, cmd := step(m, pressKey("R"))
	if cmd == nil || !strings.Contains(plain(m), "connessione a imap.gmail.com…") {
		t.Errorf("R deve rilanciare il connect (cmd=%v)\n%s", cmd != nil, plain(m))
	}
}

// La password dell'ambiente non compare mai nello splash.
func TestSplashMascheraLaPassword(t *testing.T) {
	t.Setenv("GMAIL_APP_PASSWORD", "abcdwxyzabcdwxyz")
	m := splashModel(t, 140, 35)
	m, _ = step(m, connResult{err: errors.New("login abcdwxyzabcdwxyz rifiutato")})
	if v := plain(m); strings.Contains(v, "abcdwxyzabcdwxyz") || !strings.Contains(v, "rifiutato") {
		t.Errorf("password non mascherata\n%s", v)
	}
}

// Il titolo è centrato nel corpo e la vista occupa tutto lo schermo.
func TestSplashCentrato(t *testing.T) {
	m := splashModel(t, 140, 35)
	lines := strings.Split(plain(m), "\n")
	if len(lines) != 35 {
		t.Fatalf("righe = %d, attese 35 (corpo + barra)", len(lines))
	}
	first, last := -1, -1
	for i, l := range lines[:34] {
		if strings.Contains(l, "█") {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 5 || last > 20 {
		t.Errorf("titolo non centrato verticalmente: righe %d-%d", first, last)
	}
	for _, l := range lines[first : last+1] {
		if w := lipgloss.Width(strings.TrimSpace(l)); w < 60 || w > 100 {
			t.Errorf("titolo non centrato orizzontalmente: larghezza %d in %q", w, l)
		}
	}
}

func TestSplashTitoloAdattivo(t *testing.T) {
	st := newTestModel(t).styles
	big := splashTitle(st, 100)
	compact := splashTitle(st, 59)
	tiny := splashTitle(st, 20)
	wBig, wCompact := lipgloss.Width(big), lipgloss.Width(compact)
	if wBig > 100 || wCompact > 59 || wCompact >= wBig {
		t.Errorf("larghezze: grande %d (<=100), compatto %d (<=59, < grande)", wBig, wCompact)
	}
	if lipgloss.Height(big) < 3 || lipgloss.Height(compact) < 3 {
		t.Error("il titolo è a blocchi su più righe")
	}
	if got := plainStr(tiny); got != "gmail-tui" {
		t.Errorf("sotto la larghezza minima: titolo testuale, ho %q", got)
	}
}

// Il titolo ha la sfumatura primary->accent: colori diversi lungo le colonne.
func TestSplashTitoloSfumato(t *testing.T) {
	st := newTestModel(t).styles
	out := splashTitle(st, 100)
	cols := map[string]bool{}
	for _, m := range regexp.MustCompile(`38;2;(\d+;\d+;\d+)`).FindAllStringSubmatch(out, -1) {
		cols[m[1]] = true
	}
	if len(cols) < 4 {
		t.Errorf("sfumatura assente: %d colori distinti", len(cols))
	}
}

// Una cartella davvero vuota dopo il caricamento: messaggio neutro centrato.
func TestStatoVuotoCartella(t *testing.T) {
	m := splashModel(t, 140, 35)
	m, _ = step(m, connResult{})
	m, _ = step(m, msgsLoaded{gen: m.listGen, limit: pageSize})
	v := plain(m)
	if !strings.Contains(v, "Nessun messaggio in Posta in arrivo") {
		t.Errorf("stato vuoto neutro atteso\n%s", v)
	}
	if strings.Contains(v, "No items") || strings.Contains(v, "█") {
		t.Errorf("niente 'No items' né splash\n%s", v)
	}
	var row string
	for _, l := range strings.Split(v, "\n") {
		if strings.Contains(l, "Nessun messaggio") {
			row = l
		}
	}
	if lead := len(row) - len(strings.TrimLeft(row, " ")); lead < 30 {
		t.Errorf("messaggio non centrato (rientro %d)", lead)
	}
}

// Anteprima a due colonne senza selezione: testo neutro centrato, non un errore.
func TestAnteprimaSenzaSelezioneNeutra(t *testing.T) {
	m, _ := newTestModelMock(t)
	m.SetPrefs(config.LayoutSplit, config.PreviewMarkReadDelay)
	m, _ = step(m, tea.WindowSizeMsg{Width: 140, Height: 35})
	m, _ = step(m, msgsLoaded{})
	v := plain(m)
	if strings.Contains(v, "nessun messaggio") {
		t.Errorf("il testo d'anteprima non deve sembrare un errore\n%s", v)
	}
	if !strings.Contains(v, "Seleziona un messaggio") {
		t.Errorf("testo neutro atteso nell'anteprima\n%s", v)
	}
}
