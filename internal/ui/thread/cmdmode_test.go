package thread

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/theme"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

// --- helper -------------------------------------------------------------------

// pressEnter costruisce la pressione di invio (il pressKey di thread_test.go
// copre solo esc e i caratteri singoli).
func pressEnter() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyEnter}
}

// typeCommand apre la command line con `:` e digita s un rune alla volta
// (mentre è aperta ogni tasto — inclusi j/k/a/s — è testo dell'input).
func typeCommand(m Model, s string) Model {
	m, _ = m.Update(pressKey(":"))
	for _, r := range s {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// execCommand digita il comando e preme invio: restituisce il modello e i Msg
// prodotti dalla Cmd di esecuzione (feedback/richieste, srotolati dai batch).
func execCommand(t *testing.T, m Model, s string) (Model, []tea.Msg) {
	t.Helper()
	m = typeCommand(m, s)
	mm, cmd := m.Update(pressEnter())
	return mm, runCmds(cmd)
}

// runCmds esegue una cmd srotolando i tea.BatchMsg (come fa il runtime).
func runCmds(c tea.Cmd) []tea.Msg {
	if c == nil {
		return nil
	}
	res := c()
	batch, ok := res.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{res}
	}
	var out []tea.Msg
	for _, c := range batch {
		out = append(out, runCmds(c)...)
	}
	return out
}

// scrollableModel come newModel ma col corpo lungo (come TestViewportScrollJK):
// serve quando la precondizione è "j scorre la viewport". La riga citata sta
// IN TESTA (visibile anche a offset 1) e gli allegati restano, per gli assert
// di ri-renderizzazione di SetStyles.
func scrollableModel(t *testing.T) Model {
	t.Helper()
	mm := testMsg()
	mm.BodyText = "> riga citata\n" + strings.TrimSuffix(strings.Repeat("riga corpo\n", 40), "\n")
	m := newModel(t, mm)
	m.SetSize(80, 24)
	return m
}

// feedbackOf estrae l'unico CommandFeedback dai messaggi dati.
func feedbackOf(t *testing.T, msgs []tea.Msg) CommandFeedback {
	t.Helper()
	var out []CommandFeedback
	for _, ms := range msgs {
		if fb, ok := ms.(CommandFeedback); ok {
			out = append(out, fb)
		}
	}
	if len(out) != 1 {
		t.Fatalf("CommandFeedback trovati %d, voglio 1 (msgs: %T)", len(out), msgs)
	}
	return out[0]
}

// saveOf estrae l'unico SaveRequested dai messaggi dati.
func saveOf(t *testing.T, msgs []tea.Msg) SaveRequested {
	t.Helper()
	var out []SaveRequested
	for _, ms := range msgs {
		if sr, ok := ms.(SaveRequested); ok {
			out = append(out, sr)
		}
	}
	if len(out) != 1 {
		t.Fatalf("SaveRequested trovati %d, voglio 1 (msgs: %T)", len(out), msgs)
	}
	return out[0]
}

// --- parse --------------------------------------------------------------------

// TestParseCommand: il parse è split-spaces con i due-punti opzionali
// (l'input dell'utente li contiene nel prompt, ma il valore digitato no).
func TestParseCommand(t *testing.T) {
	cases := []struct {
		raw  string
		name string
		args []string
	}{
		{":save 1 /tmp/a.pdf", "save", []string{"1", "/tmp/a.pdf"}},
		{"save 1 /tmp/a.pdf", "save", []string{"1", "/tmp/a.pdf"}},
		{":open 2", "open", []string{"2"}},
		{":theme dracula", "theme", []string{"dracula"}},
		{":theme", "theme", nil},
		{":save", "save", nil},
		{"  :open   2  ", "open", []string{"2"}},
		{":", "", nil},
		{"", "", nil},
		{"  ", "", nil},
	}
	for _, c := range cases {
		got := parseCommand(c.raw)
		if got.name != c.name || !reflect.DeepEqual(got.args, c.args) {
			t.Errorf("parseCommand(%q) = {%q %v}, voglio {%q %v}", c.raw, got.name, got.args, c.name, c.args)
		}
	}
}

// --- apertura/chiusura command line --------------------------------------------

// TestColonOpensCommandLine: `:` apre la command line (input inline sotto il
// corpo, placeholder con l'uso dei comandi); la vista resta a altezza esatta.
func TestColonOpensCommandLine(t *testing.T) {
	m := newModel(t, testMsg())
	m.SetSize(80, 24)

	mm, cmd := m.Update(pressKey(":"))
	if cmd == nil {
		t.Fatal("`: cmd nil, voglio il focus dell'input")
	}
	if !mm.CommandOpen() {
		t.Error("`: CommandOpen = false, voglio true")
	}
	if !mm.ModalOpen() {
		t.Error("command line aperta ma ModalOpen = false")
	}
	v := mm.View()
	if !strings.Contains(v, "open <n>") {
		t.Errorf("la command line non mostra l'uso dei comandi: %q", v)
	}
	if got := strings.Count(v, "\n"); got != 23 {
		t.Errorf("righe con command line = %d, voglio 24 (altezza esatta)", got+1)
	}
}

// TestKeysGoToCommandInput: con la command line aperta TUTTI i tasti sono
// testo (j/k non scorrono, a/s non operano), come il prompt label.
func TestKeysGoToCommandInput(t *testing.T) {
	m := scrollableModel(t)
	m, _ = m.Update(pressKey("j")) // scroll di riferimento
	if m.YOffset() != 1 {
		t.Fatalf("precondizione: j scorre la viewport (YOffset = %d)", m.YOffset())
	}
	m, _ = m.Update(pressKey("k"))

	m = typeCommand(m, "ja:")
	if got := m.cmdInput.Value(); got != "ja:" {
		t.Errorf("input = %q, voglio \"ja:\" (tasti come testo)", got)
	}
	if m.YOffset() != 0 {
		t.Errorf("YOffset con command line aperta = %d, voglio 0 (j è testo)", m.YOffset())
	}
	if !m.CommandOpen() {
		t.Error("i tasti di comando devono restare testo (command line ancora aperta)")
	}
}

// TestEscClosesCommandWithoutBackMsg: esc chiude la command line SENZA
// produrre BackMsg (esc/q chiudono l'input, non il thread).
func TestEscClosesCommandWithoutBackMsg(t *testing.T) {
	m := newModel(t, testMsg())
	m = typeCommand(m, "save")
	mm, cmd := m.Update(pressKey("esc"))
	if cmd != nil {
		t.Fatalf("esc nella command line: cmd = %v (%T), voglio nil (niente BackMsg)", cmd, cmd)
	}
	if mm.CommandOpen() {
		t.Error("esc non chiude la command line")
	}
	if mm.ModalOpen() {
		t.Error("dopo esc ModalOpen deve essere false")
	}
	if !strings.Contains(mm.View(), "▣ report.pdf") {
		t.Error("dopo esc la vista thread normale deve tornare (allegati in coda)")
	}
}

// TestEnterEmptyClosesSilently: invio su input vuoto (o solo ":") chiude
// senza eseguire nulla.
func TestEnterEmptyClosesSilently(t *testing.T) {
	for _, raw := range []string{"", ":"} {
		m := newModel(t, testMsg())
		m = typeCommand(m, raw)
		mm, cmd := m.Update(pressEnter())
		if cmd != nil {
			t.Errorf("invio su %q: cmd = %v, voglio nil (nessun comando)", raw, cmd)
		}
		if mm.CommandOpen() {
			t.Errorf("invio su %q non chiude la command line", raw)
		}
	}
}

// --- comando sconosciuto -------------------------------------------------------

// TestUnknownCommandFeedback: un comando sconosciuto produce un feedback di
// errore con l'elenco dei comandi disponibili.
func TestUnknownCommandFeedback(t *testing.T) {
	m := newModel(t, testMsg())
	mm, msgs := execCommand(t, m, "pippo")
	fb := feedbackOf(t, msgs)
	if !fb.IsErr {
		t.Error("comando sconosciuto: IsErr = false, voglio true")
	}
	for _, want := range []string{":save", ":open", ":theme", "pippo"} {
		if !strings.Contains(fb.Text, want) {
			t.Errorf("feedback senza %q: %q", want, fb.Text)
		}
	}
	if mm.CommandOpen() {
		t.Error("dopo l'esecuzione la command line deve chiudersi")
	}
}

// --- :save ----------------------------------------------------------------------

// TestSaveExplicitPath: `:save <n> <percorso>` produce SaveRequested con
// PartIndex dell'allegato n (1-based) e percorso esplicito.
func TestSaveExplicitPath(t *testing.T) {
	m := newModel(t, testMsg())
	mm, msgs := execCommand(t, m, "save 1 /tmp/x/report.pdf")
	sr := saveOf(t, msgs)
	if sr.ID != testRef().ID {
		t.Errorf("SaveRequested.ID = %v, voglio %v", sr.ID, testRef().ID)
	}
	if sr.Part != 0 {
		t.Errorf("SaveRequested.Part = %d, voglio 0 (PartIndex del primo allegato)", sr.Part)
	}
	if sr.Path != "/tmp/x/report.pdf" {
		t.Errorf("SaveRequested.Path = %q, voglio /tmp/x/report.pdf", sr.Path)
	}
	if sr.Open {
		t.Error("SaveRequested.Open = true, voglio false (:save non apre)")
	}
	if mm.CommandOpen() {
		t.Error("dopo l'esecuzione la command line deve chiudersi")
	}
}

// TestSaveMapsNToPartIndex: n è 1-based sull'ELENCO a fondo messaggio, il Part
// è il PartIndex filtrato (qui attachments[1].PartIndex = 1 ≠ indice 1... il
// testMsg ha PartIndex 0 e 1; un messaggio con PartIndex "buchi" copre la
// mappatura: n=2 → PartIndex del SECONDO elemento dell'elenco).
func TestSaveMapsNToPartIndex(t *testing.T) {
	mm := testMsg()
	mm.Attachments = []msg.Attachment{
		{PartIndex: 0, Filename: "report.pdf", ContentType: "application/pdf", Size: 1536},
		{PartIndex: 3, Filename: "dati.csv", ContentType: "text/csv", Size: 512},
	}
	m := newModel(t, mm)
	_, msgs := execCommand(t, m, "save 2 /tmp/d.csv")
	if sr := saveOf(t, msgs); sr.Part != 3 {
		t.Errorf("SaveRequested.Part = %d, voglio 3 (PartIndex del secondo elemento)", sr.Part)
	}
}

// TestSaveUsageAndInvalidArgs: manca n, n non numerico, n fuori range →
// feedback di errore, nessuna richiesta.
func TestSaveUsageAndInvalidArgs(t *testing.T) {
	cases := []string{
		"save",       // manca n
		"save abc",   // n non numerico
		"save 0",     // 1-based: 0 fuori range
		"save 3",     // oltre la lunghezza (2 allegati)
		"save 1 a b", // troppi argomenti
	}
	for _, raw := range cases {
		m := newModel(t, testMsg())
		mm, msgs := execCommand(t, m, raw)
		fb := feedbackOf(t, msgs)
		if !fb.IsErr {
			t.Errorf("%q: IsErr = false, voglio true", raw)
		}
		if len(msgs) != 1 {
			t.Errorf("%q: msgs = %T, voglio solo il feedback (nessuna richiesta)", raw, msgs)
		}
		if mm.PickerOpen() {
			t.Errorf("%q: il filepicker non deve aprirsi", raw)
		}
	}
}

// TestSaveNoAttachmentsInRange: ":save 1" su messaggio senza allegati è errore
// chiaro (non range silenzioso).
func TestSaveNoAttachmentsInRange(t *testing.T) {
	mm := testMsg()
	mm.Attachments = nil
	m := newModel(t, mm)
	_, msgs := execCommand(t, m, "save 1 /tmp/x")
	fb := feedbackOf(t, msgs)
	if !fb.IsErr || !strings.Contains(fb.Text, "allegat") {
		t.Errorf("feedback = %+v, voglio errore sugli allegati", fb)
	}
}

// --- filepicker (:save senza percorso) ------------------------------------------

// pickerTempDir prepara la directory corrente del test con una cartella "sub"
// e un file "dest.pdf", e ci sposta il processo (t.Chdir ripristina da solo).
func pickerTempDir(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "dest.pdf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(tmp)
	return tmp
}

// openPickerOn esegue ":save 1" (senza percorso) e alimenta il filepicker col
// risultato della sua Init (readDir), come farebbe il runtime.
func openPickerOn(t *testing.T, m Model) Model {
	t.Helper()
	mm, msgs := execCommand(t, m, "save 1")
	if !mm.PickerOpen() {
		t.Fatal(":save senza percorso: PickerOpen = false, voglio true")
	}
	if !mm.ModalOpen() {
		t.Fatal("picker aperto ma ModalOpen = false")
	}
	for _, ms := range msgs {
		mm, _ = mm.Update(ms) // readDirMsg del filepicker
	}
	return mm
}

// TestSaveNoPathOpensPicker: `:save <n>` senza percorso apre il filepicker
// bubbles sulla directory corrente, con hint di conferma, a altezza esatta.
func TestSaveNoPathOpensPicker(t *testing.T) {
	pickerTempDir(t)
	m := newModel(t, testMsg())
	m.SetSize(80, 24)
	mm, msgs := execCommand(t, m, "save 1")
	if !mm.PickerOpen() {
		t.Fatal(":save senza percorso: PickerOpen = false, voglio true")
	}
	if len(msgs) == 0 {
		t.Fatal("il filepicker deve emettere la sua Init (lettura directory)")
	}
	for _, ms := range msgs {
		mm, _ = mm.Update(ms)
	}
	v := mm.View()
	if !strings.Contains(v, "invio conferma") || !strings.Contains(v, "esc annulla") {
		t.Errorf("vista picker senza hint: %q", v)
	}
	if !strings.Contains(v, "dest.pdf") {
		t.Errorf("vista picker senza il contenuto della directory: %q", v)
	}
	if got := strings.Count(v, "\n"); got != 23 {
		t.Errorf("righe con picker = %d, voglio 24 (altezza esatta)", got+1)
	}
}

// TestPickerEscCancels: esc sul filepicker annulla (torna alla vista thread,
// nessuna richiesta di salvataggio).
func TestPickerEscCancels(t *testing.T) {
	pickerTempDir(t)
	m := openPickerOn(t, newModel(t, testMsg()))
	mm, cmd := m.Update(pressKey("esc"))
	if cmd != nil {
		t.Errorf("esc sul picker: cmd = %v, voglio nil", cmd)
	}
	if mm.PickerOpen() {
		t.Error("esc non chiude il filepicker")
	}
	if mm.ModalOpen() {
		t.Error("dopo esc dal picker ModalOpen deve essere false")
	}
	if !strings.Contains(mm.View(), "▣ report.pdf") {
		t.Error("dopo esc la vista thread normale deve tornare")
	}
}

// TestPickerSelectFile: invio su un file del picker chiede il salvataggio
// ESATTAMENTE su quel percorso.
func TestPickerSelectFile(t *testing.T) {
	tmp := pickerTempDir(t)
	m := openPickerOn(t, newModel(t, testMsg()))
	m, _ = m.Update(pressKey("j")) // cursore su dest.pdf (le dir prima: sub)
	mm, cmd := m.Update(pressEnter())
	if cmd == nil {
		t.Fatal("invio sul picker: cmd nil, voglio SaveRequested")
	}
	sr := saveOf(t, runCmds(cmd))
	if want := filepath.Join(tmp, "dest.pdf"); sr.Path != want {
		t.Errorf("SaveRequested.Path = %q, voglio %q", sr.Path, want)
	}
	if sr.Part != 0 || sr.Open {
		t.Errorf("SaveRequested = %+v, voglio {Part:0 Open:false}", sr)
	}
	if mm.PickerOpen() {
		t.Error("il picker resta aperto dopo la selezione")
	}
}

// TestPickerSelectDir: invio su una directory salva DENTRO la directory col
// nome originale dell'allegato (destinazione = directory scelta).
func TestPickerSelectDir(t *testing.T) {
	tmp := pickerTempDir(t)
	m := openPickerOn(t, newModel(t, testMsg())) // cursore su sub (prima voce)
	mm, cmd := m.Update(pressEnter())
	if cmd == nil {
		t.Fatal("invio su directory: cmd nil, voglio SaveRequested")
	}
	sr := saveOf(t, runCmds(cmd))
	if want := filepath.Join(tmp, "sub", "report.pdf"); sr.Path != want {
		t.Errorf("SaveRequested.Path = %q, voglio %q (dir + nome originale)", sr.Path, want)
	}
	if mm.PickerOpen() {
		t.Error("il picker resta aperto dopo la selezione")
	}
}

// --- :open ----------------------------------------------------------------------

// TestOpenTmpfileOriginalName: `:open <n>` chiede il salvataggio in un tmpfile
// in os.TempDir con il NOME ORIGINALE, flag Open a true.
func TestOpenTmpfileOriginalName(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	m := newModel(t, testMsg())
	mm, msgs := execCommand(t, m, "open 2")
	sr := saveOf(t, msgs)
	if !sr.Open {
		t.Error("SaveRequested.Open = false, voglio true (:open apre dopo il salvataggio)")
	}
	if sr.Part != 1 {
		t.Errorf("SaveRequested.Part = %d, voglio 1 (PartIndex del secondo allegato)", sr.Part)
	}
	want := filepath.Join(os.TempDir(), "dati.csv")
	if sr.Path != want {
		t.Errorf("SaveRequested.Path = %q, voglio %q (tmpfile con nome originale)", sr.Path, want)
	}
	if mm.CommandOpen() {
		t.Error("dopo l'esecuzione la command line deve chiudersi")
	}
}

// TestOpenInvalidArgs: `:open` senza/col n sbagliato o con argomenti extra è
// errore, nessuna richiesta.
func TestOpenInvalidArgs(t *testing.T) {
	cases := []string{"open", "open abc", "open 0", "open 3", "open 1 extra"}
	for _, raw := range cases {
		m := newModel(t, testMsg())
		mm, msgs := execCommand(t, m, raw)
		fb := feedbackOf(t, msgs)
		if !fb.IsErr {
			t.Errorf("%q: IsErr = false, voglio true", raw)
		}
		if len(msgs) != 1 {
			t.Errorf("%q: msgs = %T, voglio solo il feedback", raw, msgs)
		}
		if mm.PickerOpen() || mm.CommandOpen() {
			t.Errorf("%q: stato modal imprevisto dopo l'errore", raw)
		}
	}
}

// --- :theme ----------------------------------------------------------------------

// TestThemeListNames: `:theme` senza argomenti elenca i temi disponibili in
// un feedback NON di errore.
func TestThemeListNames(t *testing.T) {
	m := newModel(t, testMsg())
	_, msgs := execCommand(t, m, "theme")
	fb := feedbackOf(t, msgs)
	if fb.IsErr {
		t.Error("`:theme` senza argomenti: IsErr = true, voglio elenco informativo")
	}
	for _, want := range []string{"default", "dracula", "nord"} {
		if !strings.Contains(fb.Text, want) {
			t.Errorf("elenco temi senza %q: %q", want, fb.Text)
		}
	}
}

// TestThemeLoadProducesThemeChanged: `:theme <nome>` carica il tema embedded e
// produce ThemeChanged{Th, St} con gli stili NUOVI (assert su un colore che
// cambia fra default e dracula).
func TestThemeLoadProducesThemeChanged(t *testing.T) {
	m := newModel(t, testMsg())
	mm, msgs := execCommand(t, m, "theme dracula")
	if len(msgs) != 1 {
		t.Fatalf("msgs = %T, voglio un solo ThemeChanged", msgs)
	}
	tc, ok := msgs[0].(ThemeChanged)
	if !ok {
		t.Fatalf("msg = %T, voglio ThemeChanged", msgs[0])
	}
	if tc.Th.Name != "dracula" {
		t.Errorf("ThemeChanged.Th.Name = %q, voglio dracula", tc.Th.Name)
	}
	def := testStyles(t)
	if tc.St.SearchInput.Render("x") == def.SearchInput.Render("x") {
		t.Error("ThemeChanged.St: lo stile SearchInput non è cambiato rispetto al default")
	}
	if mm.CommandOpen() {
		t.Error("dopo l'esecuzione la command line deve chiudersi")
	}
}

// TestThemeUnknownIsErr: tema inesistente → feedback di errore col nome e
// l'elenco dei temi validi; NESSUN cambio di tema.
func TestThemeUnknownIsErr(t *testing.T) {
	m := newModel(t, testMsg())
	mm, msgs := execCommand(t, m, "theme zzz")
	fb := feedbackOf(t, msgs)
	if !fb.IsErr || !strings.Contains(fb.Text, "zzz") || !strings.Contains(fb.Text, "default") {
		t.Errorf("feedback = %+v, voglio errore con nome + elenco temi", fb)
	}
	for _, ms := range msgs {
		if _, ok := ms.(ThemeChanged); ok {
			t.Error("tema inesistente: non deve produrre ThemeChanged")
		}
	}
	if mm.CommandOpen() {
		t.Error("dopo l'esecuzione la command line deve chiudersi")
	}
}

// TestThemeUsage: `:theme` con più di un argomento è errore d'uso.
func TestThemeUsage(t *testing.T) {
	m := newModel(t, testMsg())
	_, msgs := execCommand(t, m, "theme a b")
	fb := feedbackOf(t, msgs)
	if !fb.IsErr || !strings.Contains(fb.Text, ":theme <nome>") {
		t.Errorf("feedback = %+v, voglio l'uso di :theme", fb)
	}
}

// --- messaggio non caricato ------------------------------------------------------

// TestCommandsNeedLoadedMessage: senza messaggio caricato (fetch in corso)
// :save/:open sono errore chiaro, :theme funziona comunque.
func TestCommandsNeedLoadedMessage(t *testing.T) {
	st := testStyles(t)
	m := New(testRef(), nil, st)
	_, msgs := execCommand(t, m, "save 1 /tmp/x")
	fb := feedbackOf(t, msgs)
	if !fb.IsErr || !strings.Contains(fb.Text, "caricato") {
		t.Errorf(":save senza messaggio: feedback = %+v", fb)
	}
	_, msgs = execCommand(t, m, "open 1")
	if fb = feedbackOf(t, msgs); !fb.IsErr {
		t.Errorf(":open senza messaggio: feedback = %+v", fb)
	}
	// :theme non tocca il messaggio: funziona.
	_, msgs = execCommand(t, m, "theme nord")
	if len(msgs) != 1 {
		t.Fatalf("msgs = %T, voglio il solo ThemeChanged", msgs)
	}
	if tc, ok := msgs[0].(ThemeChanged); !ok || tc.Th.Name != "nord" {
		t.Errorf("msg = %v, voglio ThemeChanged{nord}", msgs[0])
	}
}

// --- reteming in place -----------------------------------------------------------

// TestSetStylesRerenders: SetStyles sostituisce gli stili e RI-renderizza
// header, corpo citato e righe allegati (sono pre-stilizzati in New) senza
// perdere lo stato (ref, scroll, command line).
func TestSetStylesRerenders(t *testing.T) {
	m := scrollableModel(t) // offset 0: la riga citata (in testa) è visibile

	th2, err := theme.Load("dracula")
	if err != nil {
		t.Fatalf("tema dracula: %v", err)
	}
	st2 := styles.New(th2)
	m.SetStyles(st2)

	v := m.View()
	if !strings.Contains(v, st2.ThreadHeaderKey.Render("Da:")) {
		t.Error("header non ri-renderizzato con i nuovi stili")
	}
	if !strings.Contains(v, st2.QuoteBar[0].Render("▎")) {
		t.Error("corpo citato non ri-renderizzato con i nuovi stili")
	}
	if !strings.Contains(v, st2.ChipAttach.Render("▣ report.pdf · application/pdf · 1,5 KB")) {
		t.Error("righe allegato non ri-renderizzate con i nuovi stili")
	}
	if m.Ref().ID != testRef().ID {
		t.Error("SetStyles deve preservare il ref")
	}

	// Lo scroll resta funzionale dopo il reteming (stato preservato).
	m, _ = m.Update(pressKey("j"))
	if m.YOffset() != 1 {
		t.Errorf("YOffset dopo j e SetStyles = %d, voglio 1", m.YOffset())
	}

	// La command line aperta sopravvive e riceve i nuovi stili (prompt).
	m = typeCommand(m, "theme")
	v = m.View()
	if !strings.Contains(v, st2.SearchInput.Render(": ")) {
		t.Error("prompt command line non restilizzato dopo SetStyles")
	}
}

// TestColonIsTextWhileOpen: un secondo `:` mentre la command line è aperta è
// testo nell'input, non riapertura/annullamento.
func TestColonIsTextWhileOpen(t *testing.T) {
	m := newModel(t, testMsg())
	m = typeCommand(m, ":")
	if got := m.cmdInput.Value(); got != ":" {
		t.Errorf("input = %q, voglio \":\" (testo)", got)
	}
	if !m.CommandOpen() {
		t.Error("il secondo : non deve chiudere la command line")
	}
}
