// Integrazione della modalità comando `:` nel thread (Piano 4, Task 2):
// routing della radice (`:` catturato in viewThread e delegato), gate del
// mouse sui modali del thread, consumo dei messaggi di protocollo
// (SaveRequested → store, CommandFeedback → status) e cambio tema runtime
// (ThemeChanged → ri-temizzazione + persistenza config).
package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"gmail-tui/internal/config"
	"gmail-tui/internal/mail"
	"gmail-tui/internal/msg"
	"gmail-tui/internal/theme"
	"gmail-tui/internal/ui/inbox"
	"gmail-tui/internal/ui/thread"
)

// cmdRefs costruisce il messaggio di test: due allegati con PartIndex "buchi"
// (0 e 3) per verificare la mappatura n (1-based) → PartIndex filtrato.
func cmdRefs(tb testing.TB) (msg.MessageRef, *msg.Message) {
	tb.Helper()
	ref := msg.MessageRef{
		ID:      msg.NewID("INBOX", 9),
		From:    "carla@example.com",
		Subject: "con allegati",
		Date:    time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC),
	}
	full := &msg.Message{
		Ref:      ref,
		To:       []string{"bob@example.com"},
		BodyText: "corpo\n> riga citata",
		Attachments: []msg.Attachment{
			{PartIndex: 0, Filename: "report.pdf", ContentType: "application/pdf", Size: 1536},
			{PartIndex: 3, Filename: "dati.csv", ContentType: "text/csv", Size: 512},
		},
	}
	return ref, full
}

// cmdThreadModel apre la thread view su cmdRefs (ref già letto: niente auto
// MarkRead), con la size nota. Restituisce modello, Mock e ref.
func cmdThreadModel(tb testing.TB) (Model, *mail.Mock, msg.MessageRef) {
	tb.Helper()
	ref, full := cmdRefs(tb)
	mock := &mail.Mock{Msgs: []*msg.Message{full}}
	th, err := theme.Load("default")
	if err != nil {
		tb.Fatalf("tema default: %v", err)
	}
	m := New(mock, th)
	m, _ = step(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	mm, cmd := step(m, inbox.SelectedMsg{Ref: ref})
	var loaded msgLoaded
	for _, ms := range runCmd(cmd) {
		if l, ok := ms.(msgLoaded); ok {
			loaded = l
		}
	}
	mm, _ = step(mm, loaded)
	if mm.currentView() != viewThread || !mm.thread.Loaded() {
		tb.Fatalf("vista = %v (loaded %v), voglio thread caricato", mm.currentView(), mm.thread.Loaded())
	}
	// Lista inbox popolata: serve perché gli assert di ri-temizzazione
	// confrontino righe reali (a lista vuota la view non ha stili).
	mm, _ = step(mm, msgsLoaded{refs: testRefs(tb)})
	return mm, mock, ref
}

// openCmdMode apre la command line con `:` e digita s (tasto per tasto,
// come il terminale). Con s vuota apre e basta.
func openCmdMode(m Model, s string) Model {
	m, _ = step(m, pressKey(":"))
	for _, r := range s {
		m, _ = step(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// themeChangedOf estrae l'unico ThemeChanged dai messaggi dati.
func themeChangedOf(tb testing.TB, msgs []tea.Msg) thread.ThemeChanged {
	tb.Helper()
	var out []thread.ThemeChanged
	for _, ms := range msgs {
		if tc, ok := ms.(thread.ThemeChanged); ok {
			out = append(out, tc)
		}
	}
	if len(out) != 1 {
		tb.Fatalf("ThemeChanged trovati %d, voglio 1 (msgs: %T)", len(out), msgs)
	}
	return out[0]
}

// --- routing ------------------------------------------------------------------

// TestColonInThreadOpensCommandLine: `:` nella thread view apre la command
// line del thread (la radice cattura e delega); j/k restano testo finché è
// aperta e esc la chiude SENZA tornare alla lista.
func TestColonInThreadOpensCommandLine(t *testing.T) {
	m, _, _ := cmdThreadModel(t)

	mm := openCmdMode(m, "")
	if !mm.thread.CommandOpen() {
		t.Fatal("`: in thread non apre la command line")
	}
	// j/k come testo, non scroll né comandi.
	mm, cmd := step(mm, pressKey("j"))
	if got := mm.thread.CommandOpen(); !got {
		t.Error("j con command line aperta deve restare testo (aperta)")
	}
	_ = cmd // blink/focus del textinput: non eseguita (dorme)
	mm2, cmd2 := step(mm, pressKey("a"))
	if !mm2.thread.CommandOpen() {
		t.Error("a con command line aperta deve restare testo (command line chiusa = eseguita)")
	}
	for _, ms := range runCmd(cmd2) {
		if _, ok := ms.(opDone); ok {
			t.Error("a con command line aperta ha scattato l'op archivia: deve essere testo")
		}
	}

	// esc chiude e resta nel thread.
	mm3, cmd3 := step(mm2, pressKey("esc"))
	if cmd3 != nil {
		t.Errorf("esc con command line aperta: cmd = %v, voglio nil (niente BackMsg)", cmd3)
	}
	if mm3.thread.CommandOpen() || mm3.currentView() != viewThread {
		t.Error("esc deve chiudere la command line restando nel thread")
	}
}

// TestColonInInboxDoesNothing: `:` nell'inbox non apre nulla (il comando è
// solo del thread) e non rompe il routing.
func TestColonInInboxDoesNothing(t *testing.T) {
	m, _ := step(newTestModel(t), tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = step(m, msgsLoaded{refs: testRefs(t)})
	mm, cmd := step(m, pressKey(":"))
	if cmd != nil {
		t.Errorf("`: in inbox: cmd = %v, voglio nil", cmd)
	}
	if mm.currentView() != viewInbox {
		t.Errorf("vista dopo `: in inbox = %v, voglio inbox", mm.currentView())
	}
}

// TestMouseGatedWhileCommandOpen: con la command line aperta il mouse è
// ignorato (come sotto l'overlay help e col prompt label): nessun click
// efficace, command line intatta.
func TestMouseGatedWhileCommandOpen(t *testing.T) {
	m, _, _ := cmdThreadModel(t)
	m = openCmdMode(m, "")

	mm, cmd := step(m, tea.MouseClickMsg{X: 10, Y: 5, Button: tea.MouseLeft})
	if cmd != nil {
		t.Errorf("click con command line aperta: cmd = %v, voglio nil", cmd)
	}
	if !mm.thread.CommandOpen() {
		t.Error("il click non deve chiudere la command line")
	}
	mm, cmd = step(mm, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if cmd != nil {
		t.Errorf("rotella con command line aperta: cmd = %v, voglio nil", cmd)
	}
	if !mm.thread.CommandOpen() {
		t.Error("la rotella non deve chiudere la command line")
	}
}

// TestMouseGatedWhilePickerOpen: col filepicker di :save aperto il mouse è
// ignorato (un click non chiude né seleziona nulla dietro il picker).
func TestMouseGatedWhilePickerOpen(t *testing.T) {
	m, _, _ := cmdThreadModel(t)
	m = openCmdMode(m, "save 1")
	mm, cmd := step(m, pressKey("enter"))
	if cmd == nil {
		t.Fatal(":save 1 senza percorso: cmd nil, voglio la Init del filepicker")
	}
	if !mm.thread.PickerOpen() {
		t.Fatal("il picker non si è aperto")
	}
	mm2, cmd2 := step(mm, tea.MouseClickMsg{X: 10, Y: 5, Button: tea.MouseLeft})
	if cmd2 != nil {
		t.Errorf("click con picker aperto: cmd = %v, voglio nil", cmd2)
	}
	if !mm2.thread.PickerOpen() {
		t.Error("il click non deve chiudere il picker")
	}
	// esc annulla: torna la vista thread normale.
	mm3, cmd3 := step(mm2, pressKey("esc"))
	if cmd3 != nil {
		t.Errorf("esc sul picker: cmd = %v, voglio nil", cmd3)
	}
	if mm3.thread.PickerOpen() {
		t.Error("esc non chiude il picker")
	}
}

// --- :save ----------------------------------------------------------------------

// saveRequestedOf estrae l'unico thread.SaveRequested dai messaggi dati.
func saveRequestedOf(tb testing.TB, msgs []tea.Msg) thread.SaveRequested {
	tb.Helper()
	var out []thread.SaveRequested
	for _, ms := range msgs {
		if sr, ok := ms.(thread.SaveRequested); ok {
			out = append(out, sr)
		}
	}
	if len(out) != 1 {
		tb.Fatalf("SaveRequested trovati %d, voglio 1 (msgs: %T)", len(out), msgs)
	}
	return out[0]
}

// attachmentSavedOf estrae l'unico attachmentSaved dai messaggi dati.
func attachmentSavedOf(tb testing.TB, msgs []tea.Msg) attachmentSaved {
	tb.Helper()
	var out []attachmentSaved
	for _, ms := range msgs {
		if a, ok := ms.(attachmentSaved); ok {
			out = append(out, a)
		}
	}
	if len(out) != 1 {
		tb.Fatalf("attachmentSaved trovati %d, voglio 1 (msgs: %T)", len(out), msgs)
	}
	return out[0]
}

// TestSaveWithExplicitPathWritesMockRaw: `:save <n> <percorso>` esegue
// store.SaveAttachment(id, PartIndex, percorso) — il Mock scrive il contenuto
// di Raw — e mette l'esito "salvato in …" in status bar.
func TestSaveWithExplicitPathWritesMockRaw(t *testing.T) {
	m, mock, ref := cmdThreadModel(t)
	mock.Raw = map[msg.MessageID]map[int][]byte{ref.ID: {0: []byte("PDFBYTES")}}
	out := filepath.Join(t.TempDir(), "out.pdf")

	mm := openCmdMode(m, "save 1 "+out)
	mm2, cmd := step(mm, pressKey("enter"))
	if cmd == nil {
		t.Fatal("invio su :save: cmd nil, voglio la richiesta di salvataggio")
	}
	// La richiesta parte dal thread e la radice la trasforma nell'op store.
	sr := saveRequestedOf(t, runCmd(cmd))
	if sr.Path != out || sr.Part != 0 || sr.Open {
		t.Errorf("SaveRequested = %+v, voglio {Part:0 Path:%s Open:false}", sr, out)
	}
	mm3, saveCmd := step(mm2, sr)
	saved := attachmentSavedOf(t, runCmd(saveCmd))
	mm4, cmd4 := step(mm3, saved)
	if cmd4 != nil {
		t.Errorf("cmd dopo il salvataggio = %v, voglio nil (niente refresh per un salvataggio)", cmd4)
	}
	if got := mm4.statusText(); got != "salvato in "+out {
		t.Errorf("status = %q, voglio \"salvato in %s\"", got, out)
	}
	if mm4.pending != 0 {
		t.Errorf("pending = %d, voglio 0 dopo il salvataggio", mm4.pending)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("file salvato illeggibile: %v", err)
	}
	if string(data) != "PDFBYTES" {
		t.Errorf("contenuto = %q, voglio i byte di Mock.Raw", data)
	}
}

// TestSaveMapsNToPartIndexAtStore: `:save 2` passa al store il PartIndex del
// SECONDO elemento dell'elenco (3, non l'indice 1) — coerenza col fix Piano 2.
// Il Mock scrive su disco il contenuto di Raw[part]: il file giusto è la prova.
func TestSaveMapsNToPartIndexAtStore(t *testing.T) {
	m, mock, ref := cmdThreadModel(t)
	mock.Raw = map[msg.MessageID]map[int][]byte{ref.ID: {
		0: []byte("pdf"),
		3: []byte("csv"),
	}}
	out := filepath.Join(t.TempDir(), "d.csv")
	mm := openCmdMode(m, "save 2 "+out)
	mm2, cmd := step(mm, pressKey("enter"))
	sr := saveRequestedOf(t, runCmd(cmd))
	if sr.Part != 3 {
		t.Fatalf("SaveRequested.Part = %d, voglio 3 (PartIndex filtrato del secondo elemento)", sr.Part)
	}
	_, saveCmd := step(mm2, sr)
	attachmentSavedOf(t, runCmd(saveCmd))
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("file illeggibile: %v", err)
	}
	if string(data) != "csv" {
		t.Errorf("contenuto = %q, voglio il Raw del part 3 (n → PartIndex filtrato)", data)
	}
}

// TestSaveErrorShowsStatusErr: un salvataggio fallito finisce in status bar
// in stile errore e chiude l'op.
func TestSaveErrorShowsStatusErr(t *testing.T) {
	m, mock, _ := cmdThreadModel(t)
	mock.Err = errors.New("rete giù")
	out := filepath.Join(t.TempDir(), "out.pdf")

	mm := openCmdMode(m, "save 1 "+out)
	mm2, cmd := step(mm, pressKey("enter"))
	sr := saveRequestedOf(t, runCmd(cmd))
	mm3, saveCmd := step(mm2, sr)
	saved := attachmentSavedOf(t, runCmd(saveCmd))
	if saved.err == nil {
		t.Fatal("attachmentSaved.err = nil, voglio l'errore del Mock")
	}
	mm4, cmd4 := step(mm3, saved)
	if cmd4 != nil {
		t.Errorf("cmd dopo errore = %v, voglio nil", cmd4)
	}
	if !mm4.status.isErr {
		t.Error("status.isErr = false, voglio true")
	}
	if got := mm4.statusText(); !strings.Contains(got, "salva allegato") || !strings.Contains(got, "rete giù") {
		t.Errorf("status = %q, voglio \"salva allegato: rete giù\"", got)
	}
	if mm4.pending != 0 {
		t.Errorf("pending = %d, voglio 0", mm4.pending)
	}
}

// --- :open ----------------------------------------------------------------------

// TestOpenAttachmentFlow: `:open <n>` salva in tmpfile col nome originale e
// apre con lo stesso meccanismo di `o` (OpenCmd/OpenedMsg, feedback "aperto
// nel browser"). La exec vera non entra nei test.
func TestOpenAttachmentFlow(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	m, mock, ref := cmdThreadModel(t)
	mock.Raw = map[msg.MessageID]map[int][]byte{ref.ID: {3: []byte("CSVBYTES")}}

	mm := openCmdMode(m, "open 2")
	mm2, cmd := step(mm, pressKey("enter"))
	sr := saveRequestedOf(t, runCmd(cmd))
	if !sr.Open {
		t.Fatalf("SaveRequested.Open = false, voglio true (:open apre dopo il salvataggio)")
	}
	want := filepath.Join(os.TempDir(), "dati.csv")
	if sr.Path != want {
		t.Errorf("path = %q, voglio %q (tmpfile col nome originale)", sr.Path, want)
	}
	// La radice esegue il salvataggio PRIMA dell'apertura.
	mm3, saveCmd := step(mm2, sr)
	saved := attachmentSavedOf(t, runCmd(saveCmd))
	if saved.err != nil || !saved.open {
		t.Fatalf("attachmentSaved = %+v, voglio {open:true err:nil}", saved)
	}
	data, err := os.ReadFile(want)
	if err != nil || string(data) != "CSVBYTES" {
		t.Fatalf("tmpfile = (%q, %v), voglio i byte del Raw", data, err)
	}
	// La radice risponde con l'exec (OpenCmd): non la eseguiamo.
	mm4, execCmd := step(mm3, saved)
	if execCmd == nil {
		t.Fatal("dopo il salvataggio di :open: cmd nil, voglio thread.OpenCmd")
	}
	// Il browser è uscito: feedback in status bar.
	mm5, _ := step(mm4, thread.OpenedMsg{})
	if got := mm5.statusText(); got != "aperto nel browser" {
		t.Errorf("status dopo OpenedMsg = %q, voglio \"aperto nel browser\"", got)
	}
}

// --- feedback comandi ------------------------------------------------------------

// TestThemeListShowsStatus: `:theme` senza argomenti mette l'elenco dei temi
// in status bar (feedback non d'errore).
func TestThemeListShowsStatus(t *testing.T) {
	m, _, _ := cmdThreadModel(t)
	mm := openCmdMode(m, "theme")
	mm2, cmd := step(mm, pressKey("enter"))
	var fb thread.CommandFeedback
	for _, ms := range runCmd(cmd) {
		if f, ok := ms.(thread.CommandFeedback); ok {
			fb = f
		}
	}
	if fb.IsErr {
		t.Error("elenco temi con IsErr = true, voglio feedback informativo")
	}
	mm3, _ := step(mm2, fb)
	if got := mm3.statusText(); !strings.HasPrefix(got, "temi:") || !strings.Contains(got, "default") || !strings.Contains(got, "dracula") {
		t.Errorf("status = %q, voglio l'elenco dei temi", got)
	}
	if mm3.status.isErr {
		t.Error("elenco temi in stile errore")
	}
}

// TestUnknownCommandShowsStatusErr: comando sconosciuto → status d'errore con
// l'elenco dei comandi.
func TestUnknownCommandShowsStatusErr(t *testing.T) {
	m, _, _ := cmdThreadModel(t)
	mm := openCmdMode(m, "pippo")
	mm2, cmd := step(mm, pressKey("enter"))
	var fb thread.CommandFeedback
	for _, ms := range runCmd(cmd) {
		if f, ok := ms.(thread.CommandFeedback); ok {
			fb = f
		}
	}
	if !fb.IsErr {
		t.Error("comando sconosciuto con IsErr = false")
	}
	mm3, _ := step(mm2, fb)
	if !mm3.status.isErr {
		t.Error("status.isErr = false per comando sconosciuto")
	}
	for _, want := range []string{"pippo", ":save", ":open", ":theme"} {
		if !strings.Contains(mm3.statusText(), want) {
			t.Errorf("status senza %q: %q", want, mm3.statusText())
		}
	}
}

// --- :theme ----------------------------------------------------------------------

// TestThemeChangeReThemesRoot: ThemeChanged sostituisce gli stili della radice
// e di tutte le sub-viste (help, inbox, thread) al volo, con feedback in
// status. La persistenza gira nella cmd restituita (test dedicato).
func TestThemeChangeReThemesRoot(t *testing.T) {
	m, _, _ := cmdThreadModel(t)
	m.SetConfigPath(filepath.Join(t.TempDir(), "config.toml"))
	oldInbox := m.inbox.View()

	mm := openCmdMode(m, "theme dracula")
	mm2, cmd := step(mm, pressKey("enter"))
	tc := themeChangedOf(t, runCmd(cmd))
	mm3, _ := step(mm2, tc)

	if mm3.styles.SearchInput.Render("x") == m.styles.SearchInput.Render("x") {
		t.Error("gli stili della radice non sono cambiati")
	}
	if mm3.help.Styles.ShortKey.Render("q") != mm3.styles.SearchInput.Render("q") {
		t.Error("il modello help non è ri-costruito con i nuovi stili")
	}
	if mm3.inbox.View() == oldInbox {
		t.Error("l'inbox non è ri-temizzata (righe identiche al tema vecchio)")
	}
	if v := mm3.thread.View(); !strings.Contains(v, mm3.styles.ThreadHeaderKey.Render("Da:")) {
		t.Error("il thread non è ri-renderizzato con i nuovi stili")
	}
	if got := mm3.statusText(); got != "tema dracula applicato" {
		t.Errorf("status = %q, voglio \"tema dracula applicato\"", got)
	}
}

// TestThemeChangePersistsConfig: la persistenza salva il nuovo tema nella
// config (load → set → Save) PRESERVANDO gli altri campi.
func TestThemeChangePersistsConfig(t *testing.T) {
	m, _, _ := cmdThreadModel(t)
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	orig := config.Config{Address: "user@gmail.com", Name: "Andrea", Signature: "--\nsig", Theme: "default"}
	if err := orig.Save(cfgPath); err != nil {
		t.Fatalf("config di partenza: %v", err)
	}
	m.SetConfigPath(cfgPath)

	mm := openCmdMode(m, "theme dracula")
	mm2, cmd := step(mm, pressKey("enter"))
	tc := themeChangedOf(t, runCmd(cmd))
	mm3, persist := step(mm2, tc)
	if persist == nil {
		t.Fatal("cmd di persistenza nil, voglio saveThemeCmd")
	}
	pmsgs := runCmd(persist)
	if len(pmsgs) != 1 {
		t.Fatalf("msgs persistenza = %T, voglio un solo themePersisted", pmsgs)
	}
	tp, ok := pmsgs[0].(themePersisted)
	if !ok || tp.err != nil {
		t.Fatalf("themePersisted = (%T, %v), voglio {err:nil}", pmsgs[0], tp.err)
	}
	mm4, _ := step(mm3, tp)
	if got := mm4.statusText(); got != "tema dracula applicato" {
		t.Errorf("status dopo il salvataggio = %q, voglio il feedback del tema intatto", got)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("ricarica config: %v", err)
	}
	if cfg.Theme != "dracula" {
		t.Errorf("config.Theme = %q, voglio dracula", cfg.Theme)
	}
	if cfg.Address != orig.Address || cfg.Name != orig.Name || cfg.Signature != orig.Signature {
		t.Errorf("config = %+v, gli altri campi devono restare %+v", cfg, orig)
	}
}

// TestThemePersistErrorNonFatal: errore di salvataggio config → status d'errore
// ma il tema runtime resta (non si perde il cambio già applicato).
func TestThemePersistErrorNonFatal(t *testing.T) {
	m, _, _ := cmdThreadModel(t)
	// Percorso config = una DIRECTORY: qualunque lettura/scrittura fallisce.
	m.SetConfigPath(t.TempDir())

	mm := openCmdMode(m, "theme dracula")
	mm2, cmd := step(mm, pressKey("enter"))
	tc := themeChangedOf(t, runCmd(cmd))
	mm3, persist := step(mm2, tc)
	if mm3.styles.SearchInput.Render("x") == m.styles.SearchInput.Render("x") {
		t.Fatal("precondizione: tema runtime non applicato")
	}
	pmsgs := runCmd(persist)
	if len(pmsgs) != 1 {
		t.Fatalf("msgs persistenza = %T, voglio un solo themePersisted", pmsgs)
	}
	tp, ok := pmsgs[0].(themePersisted)
	if !ok || tp.err == nil {
		t.Fatalf("themePersisted = (%T, %v), voglio {err:…}", pmsgs[0], tp.err)
	}
	mm4, _ := step(mm3, tp)
	if !mm4.status.isErr || !strings.Contains(mm4.statusText(), "salvataggio tema") {
		t.Errorf("status = %q (isErr %v), voglio l'errore di salvataggio tema", mm4.statusText(), mm4.status.isErr)
	}
	if mm4.styles.SearchInput.Render("x") == m.styles.SearchInput.Render("x") {
		t.Error("l'errore di salvataggio non deve perdere il tema runtime")
	}
}

// TestThemeWithoutPathNoPersist: senza percorso config (os.UserConfigDir non
// disponibile) il tema si applica e la persistenza è silenziosamente omessa.
func TestThemeWithoutPathNoPersist(t *testing.T) {
	m, _, _ := cmdThreadModel(t)
	m.SetConfigPath("")

	mm := openCmdMode(m, "theme nord")
	mm2, cmd := step(mm, pressKey("enter"))
	tc := themeChangedOf(t, runCmd(cmd))
	mm3, persist := step(mm2, tc)
	if persist != nil {
		t.Errorf("cmd persistenza = %v, voglio nil (percorso sconosciuto)", persist)
	}
	if mm3.styles.SearchInput.Render("x") == m.styles.SearchInput.Render("x") {
		t.Error("tema non applicato")
	}
}

// --- help ------------------------------------------------------------------------

// TestThreadHelpIncludesViewportAndCommandKeys (polish P3 + Task 2, rebind
// P4 Task 3): l'help completo del thread documenta le chiavi viewport j/k,
// pgup/pgdn, space, d/u e il comando `:`; f/b NON sono più documentati
// (rebind: f = inoltra, b liberato). La barra breve include `: comandi`.
func TestThreadHelpIncludesViewportAndCommandKeys(t *testing.T) {
	m := newTestModel(t)
	var doc map[string]bool
	for _, g := range m.fullHelpFor(false, viewThread) {
		for _, b := range g {
			for _, k := range b.Keys() {
				if doc == nil {
					doc = map[string]bool{}
				}
				doc[k] = true
			}
		}
	}
	for _, want := range []string{"j", "k", "pgup", "pgdn", "space", "d", "u", ":", "o", "r", "f"} {
		if !doc[want] {
			t.Errorf("help completo thread senza il tasto %q", want)
		}
	}
	// b è liberato dal rebind: nessuna binding del thread lo documenta più.
	if doc["b"] {
		t.Error("help completo thread documenta ancora \"b\" (rebind: b libero)")
	}
	// f è documentato SOLO come inoltro (Forward): la binding Scroll non
	// deve più annoverarlo (check per descrizione, non per chiave).
	var scrollDoc map[string]bool
	for _, g := range m.fullHelpFor(false, viewThread) {
		for _, b := range g {
			if b.Help().Desc == "scroll pagina" {
				scrollDoc = map[string]bool{}
				for _, k := range b.Keys() {
					scrollDoc[k] = true
				}
			}
		}
	}
	if scrollDoc == nil || scrollDoc["f"] || scrollDoc["b"] {
		t.Errorf("binding Scroll = %v, voglio senza f/b (rebind)", scrollDoc)
	}
	short := bindingDescs(m.shortHelpFor(false, viewThread))
	if !short["comandi"] {
		t.Errorf("short help thread senza \"comandi\": %v", short)
	}
}
