package app

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/config"
	"github.com/andsigno82/gmail-tui/internal/theme"
	"github.com/andsigno82/gmail-tui/internal/ui/inbox"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
	"github.com/andsigno82/gmail-tui/internal/ui/toast"
)

// settingsModel prepara un radice con inbox caricata, config in t.TempDir e
// identità di test. Il tema di partenza è "default".
func settingsModel(t *testing.T) (Model, string) {
	t.Helper()
	m, _ := newTestModelMock(t, fullMessage(testRefs(t)[0]))
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	m.SetConfigPath(cfgPath)
	m.SetIdentity("me@example.com", "", "")
	mm, _ := step(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	mm, _ = step(mm, msgsLoaded{refs: testRefs(t)})
	return mm, cfgPath
}

// keyRight, keyLeft e keyDown sono i tasti freccia del menu.
var (
	keyRight = tea.KeyPressMsg{Code: tea.KeyRight}
	keyDown  = tea.KeyPressMsg{Code: tea.KeyDown}
	keyUp    = tea.KeyPressMsg{Code: tea.KeyUp}
)

// typeText invia il testo carattere per carattere al campo in modifica.
func typeText(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		m, _ = step(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// runSettingsCmd esegue la cmd del menu (salvataggio) e ne applica i Msg.
func runSettingsCmd(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for _, ms := range runCmd(cmd) {
		m, _ = step(m, ms)
	}
	return m
}

// TestCommaOpensSettingsFromInboxAndThread: `,` apre il menu da inbox e thread.
func TestCommaOpensSettingsFromInboxAndThread(t *testing.T) {
	m, _ := settingsModel(t)
	mm, _ := step(m, pressKey(","))
	if !mm.menu.open {
		t.Fatal("`,` nell'inbox: menu non aperto")
	}
	mm, _ = step(mm, pressKey("esc"))
	if mm.menu.open {
		t.Fatal("esc: menu ancora aperto")
	}

	mm, cmd := step(m, inbox.SelectedMsg{Ref: testRefs(t)[0]})
	for _, ms := range runCmd(cmd) {
		mm, _ = step(mm, ms)
	}
	if mm.currentView() != viewThread {
		t.Fatalf("vista = %v, voglio thread", mm.currentView())
	}
	mm, _ = step(mm, pressKey(","))
	if !mm.menu.open {
		t.Error("`,` nel thread: menu non aperto")
	}
}

// TestCommaIgnoredOutsideInboxAndThread: nella ricerca e nel composer `,` è
// testo, il menu non si apre.
func TestCommaIgnoredOutsideInboxAndThread(t *testing.T) {
	m, _ := settingsModel(t)
	mm, _ := step(m, pressKey("/"))
	mm, _ = step(mm, pressKey(","))
	if mm.menu.open {
		t.Error("`,` nella ricerca ha aperto il menu")
	}
	mm, _ = step(m, pressKey("c"))
	mm, _ = step(mm, pressKey(","))
	if mm.menu.open {
		t.Error("`,` nel composer ha aperto il menu")
	}
}

// TestSettingsRightChangesThemeImmediately: ←→ sul tema cambia gli stili subito.
func TestSettingsRightChangesThemeImmediately(t *testing.T) {
	m, _ := settingsModel(t)
	before := m.styles.StatusBar.Render("x")
	mm, _ := step(m, pressKey(","))
	mm, _ = step(mm, keyRight)
	if mm.th.Name == "default" {
		t.Fatal("tema invariato dopo →")
	}
	if mm.styles.StatusBar.Render("x") == before {
		t.Error("stili invariati dopo il cambio tema: anteprima assente")
	}
}

// TestSettingsEscRestoresThemeAndLayout: esc ripristina tema e layout di apertura
// e chiude il menu senza salvare.
func TestSettingsEscRestoresThemeAndLayout(t *testing.T) {
	m, cfgPath := settingsModel(t)
	mm, _ := step(m, pressKey(","))
	mm, _ = step(mm, keyRight) // tema successivo
	mm, _ = step(mm, keyDown)  // layout
	mm, _ = step(mm, keyRight) // due colonne
	if mm.layout != config.LayoutSplit {
		t.Fatalf("layout = %q, voglio split (anteprima)", mm.layout)
	}
	mm, cmd := step(mm, pressKey("esc"))
	if cmd != nil {
		t.Errorf("esc: cmd = %v, voglio nil (nessun salvataggio)", cmd)
	}
	if mm.menu.open {
		t.Error("esc: menu ancora aperto")
	}
	if mm.th.Name != "default" {
		t.Errorf("tema dopo esc = %q, voglio default", mm.th.Name)
	}
	if mm.styles.StatusBar.Render("x") != styles.New(mustTheme(t, "default")).StatusBar.Render("x") {
		t.Error("stili dopo esc diversi da quelli del tema di apertura")
	}
	if mm.layout != config.LayoutSingle {
		t.Errorf("layout dopo esc = %q, voglio single", mm.layout)
	}
	if _, err := os.Stat(cfgPath); err == nil {
		t.Error("esc ha scritto la config: l'annullamento non salva")
	}
}

// TestSettingsEnterSavesConfig: invio salva tutte le voci in config.
func TestSettingsEnterSavesConfig(t *testing.T) {
	m, cfgPath := settingsModel(t)
	mm, _ := step(m, pressKey(","))
	mm, _ = step(mm, keyRight) // tema successivo
	mm, _ = step(mm, keyDown)
	mm, _ = step(mm, keyRight) // split
	mm, _ = step(mm, keyDown)
	mm, _ = step(mm, keyRight) // mai (da dopo 2 secondi)
	mm, _ = step(mm, keyDown)  // nome
	mm, _ = step(mm, pressKey("enter"))
	mm = typeText(t, mm, "Ada")
	mm, _ = step(mm, pressKey("enter")) // chiude la modifica del nome
	mm, _ = step(mm, keyDown)           // firma
	mm, _ = step(mm, pressKey("enter"))
	mm = typeText(t, mm, "ciao")
	mm, _ = step(mm, pressKey("esc")) // chiude la modifica della firma, testo tenuto
	mm, _ = step(mm, keyUp)
	mm, _ = step(mm, keyUp) // segna come letto
	mm, cmd := step(mm, pressKey("enter"))
	if cmd == nil {
		t.Fatal("invio: cmd nil, voglio il salvataggio")
	}
	if mm.menu.open {
		t.Error("invio: menu ancora aperto")
	}
	mm = runSettingsCmd(t, mm, cmd)

	c, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Theme != mm.th.Name || c.Theme == "default" {
		t.Errorf("Theme in config = %q, voglio %q (diverso da default)", c.Theme, mm.th.Name)
	}
	if c.Layout != config.LayoutSplit {
		t.Errorf("Layout = %q, voglio split", c.Layout)
	}
	if c.PreviewMarkRead != config.PreviewMarkReadNever {
		t.Errorf("PreviewMarkRead = %q, voglio never", c.PreviewMarkRead)
	}
	if c.Name != "Ada" || c.Signature != "ciao" {
		t.Errorf("Name/Signature = %q/%q, voglio Ada/ciao", c.Name, c.Signature)
	}
	if !strings.Contains(toastText(mm), "salvate") || !toastIs(mm, toast.Success) {
		t.Errorf("toast = %q, voglio conferma di salvataggio", toastText(mm))
	}
}

// TestSettingsNameAndSignatureUsedByNextComposer: nome e firma valgono dal
// prossimo messaggio composto (mittente "Nome <indirizzo>", firma appesa).
func TestSettingsNameAndSignatureUsedByNextComposer(t *testing.T) {
	m, _ := settingsModel(t)
	mm, _ := step(m, pressKey(","))
	mm, _ = step(mm, keyDown)
	mm, _ = step(mm, keyDown)
	mm, _ = step(mm, keyDown)
	mm, _ = step(mm, pressKey("enter"))
	mm = typeText(t, mm, "Ada Lovelace")
	mm, _ = step(mm, pressKey("enter"))
	mm, _ = step(mm, keyDown)
	mm, _ = step(mm, pressKey("enter"))
	mm = typeText(t, mm, "ciao")
	mm, _ = step(mm, pressKey("esc"))
	mm, _ = step(mm, keyUp)
	mm, _ = step(mm, keyUp)
	mm, cmd := step(mm, pressKey("enter"))
	mm = runSettingsCmd(t, mm, cmd)

	mm, _ = step(mm, pressKey("c"))
	d := mm.composer.Draft()
	if d.From != `"Ada Lovelace" <me@example.com>` {
		t.Errorf("From = %q, voglio il nome con l'indirizzo", d.From)
	}
	if !strings.HasSuffix(d.BodyText, "\n\n--\nciao") {
		t.Errorf("corpo senza firma appesa: %q", d.BodyText)
	}
}

// TestSettingsNameNonASCIIEncoded: il nome non ASCII esce in RFC 2047.
func TestSettingsNameNonASCIIEncoded(t *testing.T) {
	got := identityFrom("me@example.com", "Àda")
	if !strings.Contains(got, "=?utf-8?") || !strings.HasSuffix(got, "<me@example.com>") {
		t.Errorf("identityFrom = %q, voglio RFC 2047 + indirizzo", got)
	}
	if identityFrom("me@example.com", "") != "me@example.com" {
		t.Error("senza nome il mittente è il solo indirizzo")
	}
}

// TestSettingsIgnoresOperativeKeysAndMouse: con il menu aperto `a`/`#` non
// operano e il mouse non muove la lista.
func TestSettingsIgnoresOperativeKeysAndMouse(t *testing.T) {
	m, _ := settingsModel(t)
	mm, _ := step(m, pressKey(","))
	mm, cmd := step(mm, pressKey("a"))
	if cmd != nil {
		t.Errorf("`a` con menu aperto: cmd = %v, voglio nil", cmd)
	}
	mm, cmd = step(mm, pressKey("#"))
	if cmd != nil || mm.trashOpen {
		t.Error("`#` con menu aperto ha aperto la conferma cestino")
	}
	before := mm.inbox.SelectedIndex()
	mm, cmd = step(mm, tea.MouseClickMsg{Y: 1, Button: tea.MouseLeft})
	if cmd != nil || mm.inbox.SelectedIndex() != before {
		t.Error("click con menu aperto ha mosso la lista")
	}
	if !mm.menu.open {
		t.Error("il menu si è chiuso su un tasto ignorato")
	}
}

// TestSettingsCtrlCQuits: ctrl+c esce da qualunque stato, anche dal menu.
func TestSettingsCtrlCQuits(t *testing.T) {
	m, _ := settingsModel(t)
	mm, _ := step(m, pressKey(","))
	_, cmd := step(mm, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+c nel menu: cmd nil, voglio l'uscita")
	}
}

// TestSettingsSaveErrorKeepsRuntimeValues: un errore di salvataggio è status
// d'errore; i valori restano applicati a runtime.
func TestSettingsSaveErrorKeepsRuntimeValues(t *testing.T) {
	m, _ := settingsModel(t)
	m.SetConfigPath(t.TempDir()) // una directory non è una config leggibile
	mm, _ := step(m, pressKey(","))
	mm, _ = step(mm, keyDown)
	mm, _ = step(mm, keyRight) // layout split (anteprima)
	mm, cmd := step(mm, pressKey("enter"))
	mm = runSettingsCmd(t, mm, cmd)
	if !toastIs(mm, toast.Error) || !strings.Contains(toastText(mm), "salvataggio impostazioni") {
		t.Errorf("toast = %q, voglio errore di salvataggio", toastText(mm))
	}
	if mm.layout != config.LayoutSplit {
		t.Errorf("layout a runtime = %q, voglio split nonostante l'errore", mm.layout)
	}
}

// TestSettingsWithoutConfigPathNotSaved: senza percorso di config il menu
// applica i valori ma non scrive nulla.
func TestSettingsWithoutConfigPathNotSaved(t *testing.T) {
	m, _ := settingsModel(t)
	m.SetConfigPath("")
	mm, _ := step(m, pressKey(","))
	mm, cmd := step(mm, pressKey("enter"))
	if msgs := dropExpired(runCmd(cmd)); len(msgs) != 0 {
		t.Errorf("senza config: cmd = %v, voglio nessun salvataggio", msgs)
	}
	if mm.menu.open || !strings.Contains(toastText(mm), "non salvate") {
		t.Errorf("toast = %q (menu aperto %v)", toastText(mm), mm.menu.open)
	}
}

// TestSettingsHelpListsMenu: la barra breve e l'overlay documentano il menu.
func TestSettingsHelpListsMenu(t *testing.T) {
	m, _ := settingsModel(t)
	if !bindingDescs(m.shortHelpFor(false, viewInbox))["impostazioni"] {
		t.Error("barra breve inbox senza \"impostazioni\"")
	}
	mm, _ := step(m, pressKey(","))
	if !bindingDescs(mm.shortHelpFor(false, viewInbox))["salva"] {
		t.Error("barra breve del menu senza \"salva\"")
	}
}

// mustTheme carica un tema embedded o termina il test.
func mustTheme(t *testing.T, name string) theme.Theme {
	t.Helper()
	th, err := theme.Load(name)
	if err != nil {
		t.Fatalf("theme.Load(%s): %v", name, err)
	}
	return th
}

// Piano 7.2, fix review: la barra di aiuto del menu elenca i tasti, incluso ctrl+s.
func TestSettingsShowsHelpLine(t *testing.T) {
	m, _ := settingsModel(t)
	mm, _ := step(m, pressKey(","))
	plain := plainText(mm.body())
	for _, want := range []string{"ctrl+s salva", "esc annulla", "↑↓ voce"} {
		if !strings.Contains(plain, want) {
			t.Errorf("menu senza la riga di aiuto %q:\n%s", want, plain)
		}
	}
}

// Piano 7.2, fix review: un pad di 1 colonna separa il bordo dalla voce ("│ Tema").
func TestSettingsHasLeftPadding(t *testing.T) {
	m, _ := settingsModel(t)
	mm, _ := step(m, pressKey(","))
	for _, line := range strings.Split(plainText(mm.body()), "\n") {
		if i := strings.Index(line, "Tema"); i >= 0 {
			if !strings.HasSuffix(line[:i], " ") {
				t.Errorf("voce Tema senza padding a sinistra: %q", line)
			}
			return
		}
	}
	t.Error("voce Tema non trovata nel menu")
}

// Piano 7.2, fix review: la modale è centrata orizzontalmente nel corpo.
func TestSettingsCentered(t *testing.T) {
	m, _ := settingsModel(t)
	mm, _ := step(m, pressKey(","))
	// Piano 15, Task 16: la finestra sta sopra la lista; si misura il riquadro (bordo
	// superiore) e non il margine di spazi, perche ai lati resta la lista.
	for _, line := range strings.Split(plainText(mm.body()), "\n") {
		l, r := strings.Index(line, "╭"), strings.Index(line, "╮")
		if l < 0 || r < 0 {
			continue
		}
		left := len([]rune(line[:l]))
		right := mm.width - len([]rune(line[:r])) - 1
		if left == 0 || left < right-3 || right < left-3 { // 2 colonne d'ombra a destra
			t.Errorf("riquadro del menu non centrato (sinistra %d, destra %d): %q", left, right, line)
		}
		return
	}
	t.Error("nessun bordo del menu nel corpo")
}

// Piano 7.2, fix review: ctrl+s salva da qualunque voce, anche durante la
// modifica del Nome (invio sul campo non salva).
func TestSettingsCtrlSSavesWhileEditingName(t *testing.T) {
	m, path := settingsModel(t)
	mm, _ := step(m, pressKey(","))
	for range 3 {
		mm, _ = step(mm, pressKey("j"))
	}
	mm, _ = step(mm, pressKey("enter"))
	mm = typeText(t, mm, "Ada")
	mm, cmd := step(mm, pressKey("ctrl+s"))
	if mm.menu.open {
		t.Error("ctrl+s durante la modifica del nome: menu ancora aperto")
	}
	mm = runSettingsCmd(t, mm, cmd)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("config non leggibile: %v", err)
	}
	if cfg.Name != "Ada" {
		t.Errorf("nome salvato = %q, voglio Ada", cfg.Name)
	}
}

// plainText toglie le sequenze ANSI dal rendering.
func plainText(s string) string {
	return regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(s, "")
}

// Piano 7.2, fix review: l'indirizzo vuoto dà From vuoto, non "\"Ada\" <@>".
func TestIdentityFromEmptyAddress(t *testing.T) {
	if got := identityFrom("", "Ada"); got != "" {
		t.Errorf("identityFrom(\"\", Ada) = %q, voglio stringa vuota", got)
	}
	if got := identityFrom("ada@example.com", ""); got != "ada@example.com" {
		t.Errorf("identityFrom senza nome = %q", got)
	}
}

// Piano 7.2, fix review: una config con un valore non valido (layout "grid")
// non blocca il salvataggio dal menu: il file viene riscritto valido e
// l'indirizzo resta.
func TestSaveSettingsRepairsInvalidConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("address = \"ada@example.com\"\nlayout = \"grid\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := saveSettingsCmd(path, settingsValues{Theme: "default", Layout: "split", PreviewMarkRead: "delay", Name: "Ada"})()
	if s, ok := got.(settingsSaved); !ok || s.err != nil {
		t.Fatalf("settingsSaved = %+v, voglio nessun errore", got)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("file ancora non valido dopo il salvataggio: %v", err)
	}
	if cfg.Address != "ada@example.com" || cfg.Layout != "split" || cfg.Name != "Ada" {
		t.Errorf("config dopo il salvataggio = %+v", cfg)
	}
}

// Piano 7.2, fix review: stesso ripristino per `:theme` (saveThemeCmd).
func TestSaveThemeRepairsInvalidConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("address = \"ada@example.com\"\npreview_mark_read = \"later\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := saveThemeCmd(path, "dracula")()
	if s, ok := got.(themePersisted); !ok || s.err != nil {
		t.Fatalf("themePersisted = %+v, voglio nessun errore", got)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("file ancora non valido: %v", err)
	}
	if cfg.Theme != "dracula" || cfg.Address != "ada@example.com" {
		t.Errorf("config = %+v", cfg)
	}
}

// Piano 7.2, fix review: un file illeggibile (parsing rotto) NON viene
// sovrascritto dal salvataggio del menu: errore in status, file intatto.
func TestSaveSettingsKeepsUnparsableFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	broken := "questo non è toml = [\n"
	if err := os.WriteFile(path, []byte(broken), 0o600); err != nil {
		t.Fatal(err)
	}
	got := saveSettingsCmd(path, settingsValues{Theme: "default", Layout: "single", PreviewMarkRead: "delay"})()
	if s, ok := got.(settingsSaved); !ok || s.err == nil {
		t.Errorf("settingsSaved = %+v, voglio errore", got)
	}
	if data, _ := os.ReadFile(path); string(data) != broken {
		t.Errorf("file sovrascritto: %q", data)
	}
}
