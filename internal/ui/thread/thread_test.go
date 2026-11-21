package thread

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"gmail-tui/internal/msg"
	"gmail-tui/internal/theme"
	"gmail-tui/internal/ui/styles"
)

// testStyles carica gli stili sul tema default.
func testStyles(tb testing.TB) styles.Styles {
	tb.Helper()
	th, err := theme.Load("default")
	if err != nil {
		tb.Fatalf("tema default: %v", err)
	}
	return styles.New(th)
}

func testRef() msg.MessageRef {
	return msg.MessageRef{
		ID:      msg.NewID("INBOX", 7),
		From:    "Mario Rossi <mario@example.com>",
		Subject: "piano settimana",
		Date:    time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC),
		Unread:  true,
	}
}

func testMsg() *msg.Message {
	return &msg.Message{
		Ref:      testRef(),
		To:       []string{"anna@example.com", "luca@example.com"},
		BodyText: "Ciao,\n> il 20 novembre andiamo\n> come concordato\n\nA lunedì,\nMario",
		Attachments: []msg.Attachment{
			{PartIndex: 0, Filename: "report.pdf", ContentType: "application/pdf", Size: 1536},
			{PartIndex: 1, Filename: "dati.csv", ContentType: "text/csv", Size: 512},
		},
	}
}

// newModel costruisce il modello già dimensionato (il terminale reale invia
// WindowSizeMsg all'avvio, la radice lo propaga con SetSize).
func newModel(tb testing.TB, full *msg.Message) Model {
	tb.Helper()
	return New(testRef(), full, testStyles(tb))
}

func pressKey(k string) tea.KeyPressMsg {
	switch k {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	default:
		return tea.KeyPressMsg{Code: rune(k[0]), Text: k}
	}
}

// --- header -----------------------------------------------------------------

// TestHeaderKeyValues: l'header mostra le coppie chiave/valore Da/A/Oggetto/
// Data, con le etichette nello stile ThreadHeaderKey e i valori in
// ThreadHeader (assert sull'ANSI dello stile, non sul testo nudo).
func TestHeaderKeyValues(t *testing.T) {
	st := testStyles(t)
	m := newModel(t, testMsg())
	m.SetSize(80, 24)
	v := m.View()

	pairs := [][2]string{
		{"Da:", "Mario Rossi <mario@example.com>"},
		{"A:", "anna@example.com, luca@example.com"},
		{"Oggetto:", "piano settimana"},
		{"Data:", "20 nov 2026 10:00"},
	}
	for _, p := range pairs {
		if !strings.Contains(v, st.ThreadHeaderKey.Render(p[0])) {
			t.Errorf("header senza la chiave stilizzata %q", p[0])
		}
		if !strings.Contains(v, st.ThreadHeader.Render(p[1])) {
			t.Errorf("header senza il valore stilizzato %q", p[1])
		}
	}
}

// --- quote dimming ----------------------------------------------------------

// TestQuoteDimming: le righe che iniziano con ">" sono rese con lo stile
// Quoted (line-per-line, prima del viewport); le righe normali no.
func TestQuoteDimming(t *testing.T) {
	st := testStyles(t)
	m := newModel(t, testMsg())
	m.SetSize(80, 24)
	v := m.View()

	if !strings.Contains(v, st.Quoted.Render("> il 20 novembre andiamo")) {
		t.Error("la riga citata non è nello stile Quoted")
	}
	if strings.Contains(v, st.Quoted.Render("Ciao,")) {
		t.Error("una riga normale non deve avere lo stile Quoted")
	}
	if !strings.Contains(v, "Ciao,") {
		t.Error("la riga normale deve restare nel corpo")
	}
}

// --- viewport ---------------------------------------------------------------

// TestViewportScrollJK: j/k scorrono il corpo nel viewport (delegati a
// bubbles/viewport), con clamp ai limiti del contenuto.
func TestViewportScrollJK(t *testing.T) {
	mm := testMsg()
	mm.Attachments = nil
	mm.BodyText = strings.TrimSuffix(strings.Repeat("riga corpo\n", 40), "\n")
	m := newModel(t, mm)
	m.SetSize(80, 24) // header 5 righe → viewport 19; maxY = 40-19 = 21

	if m.YOffset() != 0 {
		t.Fatalf("YOffset iniziale = %d, voglio 0", m.YOffset())
	}
	m, _ = m.Update(pressKey("j"))
	if m.YOffset() != 1 {
		t.Errorf("YOffset dopo j = %d, voglio 1", m.YOffset())
	}
	m, _ = m.Update(pressKey("k"))
	if m.YOffset() != 0 {
		t.Errorf("YOffset dopo k = %d, voglio 0", m.YOffset())
	}
	for range 40 {
		m, _ = m.Update(pressKey("j"))
	}
	if m.YOffset() != 21 {
		t.Errorf("YOffset al limite = %d, voglio 21 (clamp al fondo)", m.YOffset())
	}
}

// TestViewportWheel: la rotella scorre il corpo — la viewport v2 gestisce
// MouseWheelMsg nativamente (MouseWheelEnabled di default, delta 3).
func TestViewportWheel(t *testing.T) {
	mm := testMsg()
	mm.Attachments = nil
	mm.BodyText = strings.TrimSuffix(strings.Repeat("riga corpo\n", 40), "\n")
	m := newModel(t, mm)
	m.SetSize(80, 24)

	m, cmd := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if cmd != nil {
		t.Errorf("wheel: cmd = %v, voglio nil", cmd)
	}
	if m.YOffset() != 3 {
		t.Errorf("YOffset dopo wheel giù = %d, voglio 3 (delta default)", m.YOffset())
	}
	m, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if m.YOffset() != 0 {
		t.Errorf("YOffset dopo wheel su = %d, voglio 0", m.YOffset())
	}
}

// TestSetSizeFillsHeight: la vista occupa esattamente le righe date
// (header + viewport + allegati), così la radice mette la status bar
// sull'ultima riga senza buchi.
func TestSetSizeFillsHeight(t *testing.T) {
	m := newModel(t, testMsg())
	m.SetSize(80, 24)
	if got := strings.Count(m.View(), "\n"); got != 23 {
		t.Errorf("righe con allegati = %d, voglio 24", got+1)
	}

	mm := testMsg()
	mm.Attachments = nil
	m2 := newModel(t, mm)
	m2.SetSize(80, 24)
	if got := strings.Count(m2.View(), "\n"); got != 23 {
		t.Errorf("righe senza allegati = %d, voglio 24", got+1)
	}
}

// --- allegati ---------------------------------------------------------------

// TestAttachmentsListedHumanized: gli allegati sono elencati in coda come
// "📎 nome (dimensione)" con dimensione umana (B/KiB/MiB).
func TestAttachmentsListedHumanized(t *testing.T) {
	m := newModel(t, testMsg())
	m.SetSize(80, 24)
	v := m.View()
	for _, want := range []string{"📎 report.pdf (1.5 KiB)", "📎 dati.csv (512 B)"} {
		if !strings.Contains(v, want) {
			t.Errorf("vista senza la riga allegato %q", want)
		}
	}
	// dopo il corpo (il viewport sta in mezzo), non prima dell'header
	if strings.Index(v, "A lunedì") > strings.Index(v, "📎 report.pdf") {
		t.Error("gli allegati devono stare in coda, dopo il corpo")
	}
}

func TestNoAttachmentsNoSection(t *testing.T) {
	mm := testMsg()
	mm.Attachments = nil
	m := newModel(t, mm)
	m.SetSize(80, 24)
	if strings.Contains(m.View(), "📎") {
		t.Error("senza allegati non deve esserci la sezione 📎")
	}
}

// TestHumanSize: dimensioni umane binarie, con il ".0" tagliato.
func TestHumanSize(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1 KiB"},
		{1536, "1.5 KiB"},
		{2048, "2 KiB"},
		{1024 * 1024, "1 MiB"},
		{3*1024*1024 + 512*1024, "3.5 MiB"},
		{1024 * 1024 * 1024, "1 GiB"},
	}
	for _, c := range cases {
		if got := humanSize(c.n); got != c.want {
			t.Errorf("humanSize(%d) = %q, voglio %q", c.n, got, c.want)
		}
	}
}

// --- back -------------------------------------------------------------------

// TestBackMsgOnEscAndQ: esc/q producono BackMsg (la radice torna all'inbox).
func TestBackMsgOnEscAndQ(t *testing.T) {
	for _, k := range []string{"esc", "q"} {
		m := newModel(t, testMsg())
		mm, cmd := m.Update(pressKey(k))
		if cmd == nil {
			t.Fatalf("%s: cmd nil, voglio BackMsg", k)
		}
		if _, ok := cmd().(BackMsg); !ok {
			t.Errorf("%s: msg = %T, voglio BackMsg", k, cmd())
		}
		_ = mm
	}
}

// --- open nel browser -------------------------------------------------------

// TestOEmitsOpenRequested: `o` su un messaggio con HTML produce la Cmd che
// scrive il tmpfile .html e richiede l'apertura (OpenRequested col percorso).
// Il browser NON viene eseguito nei test: si asserta solo il tmpfile.
func TestOEmitsOpenRequested(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	mm := testMsg()
	mm.HasHTML = true
	mm.BodyHTML = "<html><body><b>ciao</b></body></html>"
	m := newModel(t, mm)

	m2, cmd := m.Update(pressKey("o"))
	if cmd == nil {
		t.Fatal("o con HTML: cmd nil, voglio OpenRequested")
	}
	req, ok := cmd().(OpenRequested)
	if !ok {
		t.Fatalf("o con HTML: msg = %T, voglio OpenRequested", cmd())
	}
	if req.Err != nil {
		t.Fatalf("OpenRequested.Err = %v, voglio nil", req.Err)
	}
	if !strings.HasSuffix(req.Path, ".html") {
		t.Errorf("OpenRequested.Path = %q, voglio suffisso .html", req.Path)
	}
	data, err := os.ReadFile(req.Path)
	if err != nil {
		t.Fatalf("tmpfile non leggibile: %v", err)
	}
	if string(data) != mm.BodyHTML {
		t.Errorf("contenuto tmpfile = %q, voglio il BodyHTML del messaggio", data)
	}
	_ = m2
}

// TestOAlternativeEmitsOpenRequested: messaggio multipart/alternative
// (BodyText + BodyHTML, HasHTML false): `o` apre comunque il browser — il
// gate è BodyHTML != "", NON HasHTML (fix: la posta reale è per lo più
// alternative, prima il tasto non faceva nulla su quella forma).
func TestOAlternativeEmitsOpenRequested(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	mm := testMsg()
	mm.BodyHTML = `<html><body><a href="https://example.org">qui</a></body></html>`
	if mm.HasHTML || mm.BodyHTML == "" {
		t.Fatal("precondizione: messaggio alternative (HasHTML false, BodyHTML presente)")
	}
	m := newModel(t, mm)

	_, cmd := m.Update(pressKey("o"))
	if cmd == nil {
		t.Fatal("o su alternative: cmd nil, voglio OpenRequested (gate su BodyHTML)")
	}
	req, ok := cmd().(OpenRequested)
	if !ok {
		t.Fatalf("o su alternative: msg = %T, voglio OpenRequested", cmd())
	}
	if req.Err != nil {
		t.Fatalf("OpenRequested.Err = %v, voglio nil", req.Err)
	}
	data, err := os.ReadFile(req.Path)
	if err != nil {
		t.Fatalf("tmpfile non leggibile: %v", err)
	}
	if string(data) != mm.BodyHTML {
		t.Errorf("contenuto tmpfile = %q, voglio il BodyHTML del messaggio", data)
	}
}

// TestONoHTMLNilCmd: senza HTML (o senza messaggio) `o` non fa nulla.
func TestONoHTMLNilCmd(t *testing.T) {
	m := newModel(t, testMsg()) // HasHTML false
	if _, cmd := m.Update(pressKey("o")); cmd != nil {
		t.Errorf("o senza HTML: cmd = %v, voglio nil", cmd)
	}
}

// TestOpenCmdBrowserBinary: la Cmd di apertura usa il comando di sistema
// giusto (open su darwin, xdg-open altrove). L'esecuzione vera non entra nei
// test: skip esplicito se il comando non è installato.
func TestOpenCmdBrowserBinary(t *testing.T) {
	want := "xdg-open"
	if runtime.GOOS == "darwin" {
		want = "open"
	}
	if _, err := exec.LookPath(want); err != nil {
		t.Skipf("comando %s assente su %s", want, runtime.GOOS)
	}
	c := browserCmd("/tmp/msg.html")
	// exec.Command risolve il binario nel PATH assoluto: si confronta il nome.
	if filepath.Base(c.Path) != want {
		t.Errorf("browserCmd.Path = %q, voglio %q", c.Path, want)
	}
	if len(c.Args) != 2 || c.Args[1] != "/tmp/msg.html" {
		t.Errorf("browserCmd.Args = %v, voglio [%s /tmp/msg.html]", c.Args, want)
	}
}

// --- markread / stato -------------------------------------------------------

// TestMarkReadMutatesRef: MarkRead aggiorna la copia locale del ref (l'header
// resta coerente senza re-list; il back preserva la lista, il refresh avviene
// via Watch/nuova ricerca).
func TestMarkReadMutatesRef(t *testing.T) {
	m := newModel(t, testMsg())
	if !m.Ref().Unread {
		t.Fatal("ref iniziale: Unread = false, voglio true")
	}
	m.MarkRead()
	if m.Ref().Unread {
		t.Error("dopo MarkRead: Unread = true, voglio false")
	}
}

// TestNewNilMessageNoPanic: un New con full nil (difesa) non panica e rende
// comunque l'header; Loaded() segnala l'assenza del corpo.
func TestNewNilMessageNoPanic(t *testing.T) {
	st := testStyles(t)
	m := New(testRef(), nil, st)
	if m.Loaded() {
		t.Error("Loaded con full nil = true, voglio false")
	}
	v := m.View()
	if !strings.Contains(v, st.ThreadHeaderKey.Render("Oggetto:")) {
		t.Errorf("vista senza header con full nil: %q", v)
	}
}
