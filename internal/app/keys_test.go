package app

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

// pressCtrlC costruisce la pressione di ctrl+c (v2: Key con ModCtrl, nessuna
// costante KeyCtrlC — String() rende "ctrl+c" via ultraviolet).
func pressCtrlC() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
}

// --- keymap centrale -------------------------------------------------------

// TestKeyMapBindingsComplete: ogni binding della keymap è abilitato e documenta
// chiave e descrizione (il help è generato SOLO dalle binding: una mancante
// sparirebbe dall'help).
func TestKeyMapBindingsComplete(t *testing.T) {
	v := reflect.ValueOf(keys)
	typ := v.Type()
	for i := 0; i < v.NumField(); i++ {
		b, ok := v.Field(i).Interface().(key.Binding)
		if !ok {
			t.Fatalf("campo %s non è una key.Binding", typ.Field(i).Name)
		}
		if !b.Enabled() {
			t.Errorf("binding %s disabilitato", typ.Field(i).Name)
		}
		if len(b.Keys()) == 0 {
			t.Errorf("binding %s senza tasti", typ.Field(i).Name)
		}
		if b.Help().Key == "" || b.Help().Desc == "" {
			t.Errorf("binding %s senza help (key=%q desc=%q)", typ.Field(i).Name, b.Help().Key, b.Help().Desc)
		}
	}
}

// routedKeys sono i tasti che il routing di Update e le viste delegate
// consumano davvero (fonte: model.go + inbox/thread): ognuno deve essere
// documentato da ALMENO una binding della keymap centrale — se un giorno un
// tasto cambia, il test ricorda di aggiornare help e keymap insieme.
var routedKeys = []string{
	"q", "esc", // uscita (inbox) / back (thread) / cancel (composer)
	"ctrl+c", // uscita immediata
	"/", "c", // ricerca, composer (P4 Task 3)
	"r", "f", // risposta / inoltro (P4 Task 3: f liberato dal rebind scroll)
	"a", "s", "v", "l", // mutazioni
	"m",      // cartelle e label (selettore, P7 Task 4)
	"#",      // sposta nel cestino (conferma, P7 Task 5)
	"enter",  // apri messaggio (inbox), cerca (search), applica (label)
	"j", "k", // cursore lista / scroll thread
	"pgup", "pgdn", "space", // scroll pagina viewport (thread, P3 polish)
	"d", "u", // scroll mezza pagina viewport (thread: f/b liberati dal rebind, P4 Task 3)
	"o", // apri nel browser (thread)
	":", // command line :save/:open/:theme (thread, P4 Task 2)
	"?", // help completo
}

// TestRoutedKeysAreDocumented: ogni tasto routed è coperto da una binding.
func TestRoutedKeysAreDocumented(t *testing.T) {
	v := reflect.ValueOf(keys)
	documented := map[string]bool{}
	for i := 0; i < v.NumField(); i++ {
		b := v.Field(i).Interface().(key.Binding)
		for _, k := range b.Keys() {
			documented[k] = true
		}
	}
	for _, k := range routedKeys {
		if !documented[k] {
			t.Errorf("tasto routed %q non documentato in nessuna binding della keymap", k)
		}
	}
}

// TestShortHelpPerView: l'help breve è contestuale alla vista (Task 9): inbox
// offre uscita+ricerca+mutazioni+help, il thread il back e il browser, la
// ricerca invio/esc, il prompt label invio/esc.
func TestShortHelpPerView(t *testing.T) {
	m := newTestModel(t)
	inbox := bindingDescs(m.shortHelpFor(false, viewInbox))
	for _, want := range []string{"esci", "cerca", "archivia", "aiuto"} {
		if !inbox[want] {
			t.Errorf("short help inbox senza %q: %v", want, inbox)
		}
	}
	thread := bindingDescs(m.shortHelpFor(false, viewThread))
	for _, want := range []string{"torna alla lista", "apri nel browser"} {
		if !thread[want] {
			t.Errorf("short help thread senza %q: %v", want, thread)
		}
	}
	search := bindingDescs(m.shortHelpFor(false, viewSearch))
	if !search["cerca"] || !search["annulla"] {
		t.Errorf("short help search senza cerca/annulla: %v", search)
	}
	label := bindingDescs(m.shortHelpFor(true, viewInbox))
	if !label["annulla"] {
		t.Errorf("short help prompt label senza \"annulla\": %v", label)
	}
}

// TestFullHelpGroupsNonEmpty: l'help completo è organizzato in gruppi (colonne)
// per vista, nessuno vuoto; il gruppo aiuto è sempre presente.
func TestFullHelpGroupsNonEmpty(t *testing.T) {
	m := newTestModel(t)
	for _, tc := range []struct {
		labelOpen bool
		v         view
		minGroups int
	}{
		{false, viewInbox, 3},
		{false, viewThread, 3},
		{false, viewSearch, 2},
	} {
		groups := m.fullHelpFor(tc.labelOpen, tc.v)
		if len(groups) < tc.minGroups {
			t.Errorf("vista %v: %d gruppi, voglio almeno %d", tc.v, len(groups), tc.minGroups)
		}
		var hasHelp bool
		for _, g := range groups {
			if len(g) == 0 {
				t.Errorf("vista %v: gruppo vuoto nell'help completo", tc.v)
			}
			for _, b := range g {
				if b.Help().Desc == "aiuto" {
					hasHelp = true
				}
			}
		}
		if !hasHelp {
			t.Errorf("vista %v: help completo senza il gruppo aiuto", tc.v)
		}
	}
}

// TestHelpStylesThemed: gli stili del modello help derivano dagli slot del
// tema (i default di bubbles hardcodano hex): il rendering delle chiavi è
// identico allo stile SearchInput (Accent), le descrizioni a ListDate (Muted).
func TestHelpStylesThemed(t *testing.T) {
	m := newTestModel(t)
	if m.help.Styles.ShortKey.Render("q") != m.styles.SearchInput.Render("q") {
		t.Error("help ShortKey non usa lo slot SearchInput del tema")
	}
	if m.help.Styles.ShortDesc.Render("esci") != m.styles.ListDate.Render("esci") {
		t.Error("help ShortDesc non usa lo slot ListDate del tema")
	}
}

// --- barra help breve nella status bar --------------------------------------

// TestStatusBarShowsShortHelp: la status bar mostra SEMPRE l'help breve
// (bubbles/help) quando non c'è un prompt: a status vuota l'help sostituisce
// il vecchio hint fisso, a status piena lo affianca.
func TestStatusBarShowsShortHelp(t *testing.T) {
	m := newTestModel(t)
	bar := m.statusBar()
	for _, want := range []string{"esci", "cerca", "aiuto"} {
		if !strings.Contains(bar, want) {
			t.Errorf("status bar senza l'help breve (%q): %q", want, bar)
		}
	}
	// Con uno status attivo l'help resta visibile accanto al testo.
	mm, _ := step(m, statusMsg{text: "fatto"})
	bar = mm.statusBar()
	if !strings.Contains(bar, "fatto") || !strings.Contains(bar, "esci") {
		t.Errorf("status bar = %q, voglio testo + help breve", bar)
	}
}

// --- overlay help completo ---------------------------------------------------

// TestQuestionTogglesHelpOverlay: `?` apre l'help completo (dedicato, sopra il
// corpo), `?` e esc lo chiudono (esc NON esce dall'app mentre l'overlay è
// aperto).
func TestQuestionTogglesHelpOverlay(t *testing.T) {
	m := newTestModel(t)
	mm, _ := step(m, tea.WindowSizeMsg{Width: 80, Height: 24})

	mm, cmd := step(mm, pressKey("?"))
	if cmd != nil {
		t.Errorf("?: cmd = %v, voglio nil", cmd)
	}
	if !mm.showHelp {
		t.Fatal("? non apre l'overlay help")
	}
	// Il full help mostra voci assenti dalla barra breve (es. "sposta" del
	// movimento) e l'hint di chiusura.
	if v := mm.View().Content; !strings.Contains(v, "sposta") || !strings.Contains(v, "chiudi") {
		t.Errorf("view overlay help = %q, voglio il full help + hint chiudi", v)
	}

	// esc chiude senza uscire dall'app.
	mm, cmd = step(mm, pressKey("esc"))
	if cmd != nil {
		t.Errorf("esc nell'overlay: cmd = %v, voglio nil (niente quit)", cmd)
	}
	if mm.showHelp {
		t.Error("esc non chiude l'overlay help")
	}
	if mm.currentView() != viewInbox {
		t.Errorf("vista dopo esc = %v, voglio inbox", mm.currentView())
	}

	// ? chiude un overlay aperto (toggle).
	mm, _ = step(mm, pressKey("?"))
	mm, _ = step(mm, pressKey("?"))
	if mm.showHelp {
		t.Error("il secondo ? non chiude l'overlay")
	}
}

// TestHelpOverlaySwallowsKeys: con l'overlay aperto gli altri tasti sono
// ignorati — nessuna azione sotto l'overlay (q non esce, / non apre ricerca).
func TestHelpOverlaySwallowsKeys(t *testing.T) {
	m := newTestModel(t)
	mm, _ := step(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	mm, _ = step(mm, pressKey("?"))

	mm, cmd := step(mm, pressKey("q"))
	if cmd != nil {
		t.Errorf("q nell'overlay: cmd = %v, voglio nil", cmd)
	}
	mm, cmd = step(mm, pressKey("/"))
	if cmd != nil {
		t.Errorf("/ nell'overlay: cmd = %v, voglio nil", cmd)
	}
	if mm.currentView() != viewInbox {
		t.Errorf("vista dopo / nell'overlay = %v, voglio inbox (overlay assorbe)", mm.currentView())
	}
}

// TestHelpHiddenInSearchAndLabel: in ricerca e col prompt label aperto `?` è
// testo della query / del nome, non toggle dell'overlay.
func TestHelpHiddenInSearchAndLabel(t *testing.T) {
	m := newTestModel(t)
	m.view = viewSearch
	_ = m.search.Focus() // come fa il routing al primo `/` (blink cmd scartata)
	mm, _ := step(m, pressKey("?"))
	if mm.showHelp {
		t.Error("? in ricerca apre l'overlay: deve essere testo della query")
	}
	if mm.search.Value() != "?" {
		t.Errorf("query = %q, voglio \"?\" (testo nell'input)", mm.search.Value())
	}

	m2 := newTestModel(t)
	mm2, _ := step(m2, tea.WindowSizeMsg{Width: 80, Height: 24})
	mm2, _ = step(mm2, msgsLoaded{refs: testRefs(t)}) // `l` senza ref è un no-op (Task 7): serve una riga
	mm2, cmdL := step(mm2, pressKey("l"))             // apre il prompt (l'input viene focalizzato)
	_ = cmdL                                          // blink cmd del focus: non eseguita (dorme)
	mm2, _ = step(mm2, pressKey("?"))
	if mm2.showHelp {
		t.Error("? col prompt label aperto apre l'overlay: deve essere testo")
	}
	if mm2.label.Value() != "?" {
		t.Errorf("label = %q, voglio \"?\"", mm2.label.Value())
	}
}

// --- schermata min-size ------------------------------------------------------

// TestMinSizeWarningView: sotto 80×24 la View è la schermata di avviso dedicata
// (View-only: mostra la size corrente e il minimo richiesto); a 80×24 esatti
// la TUI è normale.
func TestMinSizeWarningView(t *testing.T) {
	m := newTestModel(t)
	for _, tc := range []struct {
		w, h int
		warn bool
	}{{79, 24, true}, {80, 23, true}, {40, 10, true}, {80, 24, false}, {100, 30, false}} {
		mm, _ := step(m, tea.WindowSizeMsg{Width: tc.w, Height: tc.h})
		v := mm.View().Content
		if tc.warn {
			if !strings.Contains(v, "troppo piccolo") || !strings.Contains(v, "80×24") {
				t.Errorf("%dx%d: view = %q, voglio l'avviso min-size", tc.w, tc.h, v)
			}
			if !strings.Contains(v, strconv.Itoa(tc.w)+"×"+strconv.Itoa(tc.h)) {
				t.Errorf("%dx%d: l'avviso non mostra la size corrente", tc.w, tc.h)
			}
		} else if strings.Contains(v, "troppo piccolo") {
			t.Errorf("%dx%d: view mostra l'avviso ma è sopra il minimo", tc.w, tc.h)
		}
	}
}

// TestMinSizeViewIsDedicated: la schermata min-size sostituisce TUTTO il corpo
// (niente lista, niente status bar).
func TestMinSizeViewIsDedicated(t *testing.T) {
	m := newTestModel(t)
	mm, _ := step(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	mm, _ = step(mm, msgsLoaded{refs: testRefs(t)})
	mm, _ = step(mm, tea.WindowSizeMsg{Width: 40, Height: 10})
	v := mm.View().Content
	if strings.Contains(v, "No items.") {
		t.Error("la view min-size rende il corpo normale (lista vuota)")
	}
	if strings.Contains(v, spinner.MiniDot.Frames[0]) {
		t.Error("la view min-size rende la status bar")
	}
}

// TestMinSizeKeysStillWork: la schermata min-size è View-only — q esce comunque
// (l'utente non resta intrappolato).
func TestMinSizeKeysStillWork(t *testing.T) {
	m := newTestModel(t)
	mm, _ := step(m, tea.WindowSizeMsg{Width: 40, Height: 10})
	_, cmd := step(mm, pressKey("q"))
	if cmd == nil {
		t.Fatal("q nella view min-size: cmd nil, voglio quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("cmd = %T, voglio tea.QuitMsg", cmd())
	}
}

// --- uscita pulita (mandato Task 8) -----------------------------------------

// TestQuitCancelsWatchContext: q/esc nell'inbox escono con quitCmd, che
// cancella il ctx del feed Watch PRIMA del QuitMsg (il loop waitForChange si
// sblocca e muore invece di restare parcheggiato).
func TestQuitCancelsWatchContext(t *testing.T) {
	for _, k := range []string{"q", "esc"} {
		m := newTestModel(t)
		mm, cmd := step(m, pressKey(k))
		if cmd == nil {
			t.Fatalf("%s in inbox: cmd nil, voglio quitCmd", k)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("%s: msg = %T, voglio tea.QuitMsg", k, cmd())
		}
		select {
		case <-mm.ctx.Done():
		default:
			t.Errorf("%s: ctx del feed non cancellato dal quitCmd", k)
		}
	}
}

// TestCtrlCQuitsFromAnywhere: ctrl+c è l'uscita universale (inbox, ricerca,
// prompt label, overlay help) e cancella il ctx del feed come q/esc.
func TestCtrlCQuitsFromAnywhere(t *testing.T) {
	states := []func(*Model){
		func(m *Model) {},                      // inbox
		func(m *Model) { m.view = viewSearch }, // ricerca
		func(m *Model) { m.labelOpen = true },  // prompt label
		func(m *Model) { m.showHelp = true },   // overlay help
	}
	for i, prep := range states {
		m := newTestModel(t)
		prep(&m)
		mm, cmd := step(m, pressCtrlC())
		if cmd == nil {
			t.Fatalf("stato %d: ctrl+c cmd nil, voglio quitCmd", i)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("stato %d: msg = %T, voglio tea.QuitMsg", i, cmd())
		}
		select {
		case <-mm.ctx.Done():
		default:
			t.Errorf("stato %d: ctx non cancellato da ctrl+c", i)
		}
	}
}

// --- banner demo --------------------------------------------------------------

// TestDemoBanner: SetDemoBanner imposta lo status iniziale "demo mode — …"
// reso con lo stile DemoBanner; sopravvive al load di Init (non-silent) ed è
// sostituito dal primo evento vero.
func TestDemoBanner(t *testing.T) {
	m := newTestModel(t)
	m.SetDemoBanner()
	if got := m.statusText(); got != demoBannerText {
		t.Fatalf("status dopo SetDemoBanner = %q, voglio %q", got, demoBannerText)
	}
	if !strings.Contains(m.View().Content, demoBannerText) {
		t.Error("il banner demo non compare nella view")
	}

	// Il load di Init NON cancella il banner: dopo il risultato (pending=0,
	// niente spinner davanti) la barra è ESATTAMENTE il banner in stile
	// DemoBanner — senza help aggiunto (il banner è da solo, ben visibile).
	mm, _ := step(m, msgsLoaded{refs: testRefs(t)})
	if got := mm.statusText(); got != demoBannerText {
		t.Errorf("status dopo il load = %q, voglio il banner intatto", got)
	}
	if got := mm.statusBar(); got != mm.styles.DemoBanner.Render(demoBannerText) {
		t.Errorf("status bar = %q, voglio lo stile DemoBanner (%q)", got, mm.styles.DemoBanner.Render(demoBannerText))
	}
	// Un SECONDO load esplicito (ricerca con query vuota) torna ad azzerare:
	// il flag di avvio si consuma al primo load.
	mm, _ = step(mm, msgsLoaded{refs: testRefs(t)})
	if got := mm.statusText(); got != "" {
		t.Errorf("status dopo il secondo load = %q, voglio \"\"", got)
	}
	// Il primo evento vero lo sostituisce.
	mm, _ = step(mm, opDone{label: "stella on"})
	if mm.statusText() != "stella on" {
		t.Errorf("status dopo un'op = %q, voglio \"stella on\"", mm.statusText())
	}
}

// TestStartupWarningOverridesBanner: un warning d'avvio (tema di fallback)
// sostituisce il banner demo, è reso in stile status normale e — come il
// banner — sopravvive al load di Init.
func TestStartupWarningOverridesBanner(t *testing.T) {
	m := newTestModel(t)
	m.SetDemoBanner()
	const warn = `tema "x" non disponibile — uso default`
	m.SetStartupWarning(warn)
	if got := m.statusText(); got != warn {
		t.Errorf("status = %q, voglio il warning del tema", got)
	}
	// Il load di Init NON cancella il warning (come il banner): consumato il
	// load (pending=0), la barra è il warning in stile StatusBar con l'help
	// breve accanto.
	mm, _ := step(m, msgsLoaded{refs: testRefs(t)})
	if got := mm.statusText(); got != warn {
		t.Errorf("status dopo il load = %q, voglio il warning intatto", got)
	}
	want := mm.styles.StatusBar.Render(mm.joinHelp(warn))
	if got := mm.statusBar(); got != want {
		t.Errorf("status bar = %q, voglio lo stile StatusBar con help (%q)", got, want)
	}
}

// --- view flags (mouse + altscreen) -----------------------------------------

// TestViewRequestsMouseAndAltScreen: la View dichiara MouseMode cell motion
// (click + rotella: ciò che list/viewport usano) e altscreen — in tea v2 è
// per-View, non ci sono opzioni di programma. Senza MouseMode il mouse non
// arriva mai all'Update nel terminale reale.
func TestViewRequestsMouseAndAltScreen(t *testing.T) {
	m := newTestModel(t)
	v := m.View()
	if v.MouseMode != tea.MouseModeCellMotion {
		t.Errorf("MouseMode = %v, voglio MouseModeCellMotion", v.MouseMode)
	}
	if !v.AltScreen {
		t.Error("AltScreen = false, voglio true (TUI a schermo intero)")
	}
}

// TestHelpWidthAdaptsToResize: la size del terminale alimenta anche il modello
// help (truncamento della barra breve).
func TestHelpWidthAdaptsToResize(t *testing.T) {
	m := newTestModel(t)
	mm, _ := step(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if mm.help.Width() != 100 {
		t.Errorf("help width = %d, voglio 100", mm.help.Width())
	}
}

// --- helper -------------------------------------------------------------------

// bindingDescs indicizza le binding per descrizione.
func bindingDescs(bs []key.Binding) map[string]bool {
	out := make(map[string]bool, len(bs))
	for _, b := range bs {
		out[b.Help().Desc] = true
	}
	return out
}
