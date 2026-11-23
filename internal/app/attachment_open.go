package app

// Apertura di allegati e link cliccati (Piano 15, Task 16; D15-T14, Q15-1).
// Un allegato si apre solo se e di un tipo sicuro: Content-Type in allowlist E
// estensione coerente (su macOS `open` esegue .app, .command, .pkg: il tipo non si
// decide dal nome da solo). Il file e scritto in una cartella temporanea 0700 con un
// nome generato (mai quello del messaggio; cartella della sessione in una radice
// dedicata, rimossa all'uscita: Piano 15, Task 16b) e aperto col comando di sistema, senza
// shell e con l'ambiente filtrato (internal/childenv). Mai eseguito.

import (
	"context"
	"fmt"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/childenv"
	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/msg/richtext"
	"github.com/andsigno82/gmail-tui/internal/ui/thread"
	"github.com/andsigno82/gmail-tui/internal/ui/toast"
)

// openFile apre un file col comando di sistema (open su darwin, xdg-open altrove,
// senza shell, ambiente filtrato). Variabile per i test: nei test non si apre mai
// nulla di vero.
var openFile = func(path string) error { return systemOpenCmd(path).Run() }

// openURL apre un URL gia sanificato nel browser di sistema, come i link del corpo
// (thread.OpenURL). Variabile per i test.
var openURL = thread.OpenURL

// systemOpenCmd sceglie il comando di sistema per aprire un percorso.
func systemOpenCmd(path string) *exec.Cmd {
	var c *exec.Cmd
	if runtime.GOOS == "darwin" {
		c = exec.Command("open", path)
	} else {
		c = exec.Command("xdg-open", path)
	}
	c.Env = childenv.Filtered() // la password App e le GMAIL_TUI_* non escono
	return c
}

// safeTypes e l'allowlist (Q15-1): Content-Type -> estensioni coerenti, la prima e
// quella usata per il nome generato.
var safeTypes = map[string][]string{
	"application/pdf": {".pdf"},
	"image/png":       {".png"},
	"image/jpeg":      {".jpg", ".jpeg"},
	"image/gif":       {".gif"},
	"text/plain":      {".txt"},
}

// safeAttachmentExt dice se un allegato e apribile e con quale estensione generata:
// il Content-Type (senza parametri, minuscolo) deve essere in allowlist e l'estensione
// del nome (minuscola) una di quelle coerenti. Altrimenti ("", false).
func safeAttachmentExt(ctype, name string) (string, bool) {
	mt, _, err := mime.ParseMediaType(ctype)
	if err != nil {
		return "", false
	}
	exts, ok := safeTypes[strings.ToLower(mt)]
	if !ok {
		return "", false
	}
	got := strings.ToLower(filepath.Ext(name))
	for _, e := range exts {
		if got == e {
			return exts[0], true
		}
	}
	return "", false
}

// Radice dedicata degli allegati aperti (Piano 15, Task 16b, richiesta di Andrea
// 2026-11-23): una cartella privata dell'utente (0700), NON il /tmp condiviso
// (revisione indipendente, punto 8): $XDG_RUNTIME_DIR/gmail-tui/open se esiste,
// altrimenti os.UserCacheDir()/gmail-tui/open. Contiene una cartella per sessione
// (s-<casuale>, 0700) con i file generati. All'uscita (CleanupAttachments) si
// rimuove SOLO la cartella della sessione e la radice se vuota; a ogni avvio
// (SweepAttachments) si tolgono le sessioni s-* piu vecchie di 24 h, anche nella
// vecchia radice <tmp>/gmail-tui-att delle versioni precedenti. Mai nulla fuori
// dalla radice, mai symlink seguiti.
const (
	attachRootName   = "gmail-tui-att"
	attachSessPrefix = "s-"
	attachMaxAge     = 24 * time.Hour
)

// attachRoot restituisce la radice; variabile per i test (radice iniettabile).
var attachRoot = defaultAttachRoot

// defaultAttachRoot sceglie la radice privata: XDG_RUNTIME_DIR (cartella per-utente
// del sistema; solo se assoluta e esistente), poi os.UserCacheDir(). Se nessuna e
// disponibile, una cartella per-utente nel tmp (uid nel nome, comunque 0700
// verificata da ensureAttachRoot).
func defaultAttachRoot() string {
	if rt := os.Getenv("XDG_RUNTIME_DIR"); filepath.IsAbs(rt) {
		if st, err := os.Stat(rt); err == nil && st.IsDir() {
			return filepath.Join(rt, "gmail-tui", "open")
		}
	}
	if c, err := os.UserCacheDir(); err == nil {
		return filepath.Join(c, "gmail-tui", "open")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("gmail-tui-%d", os.Getuid()), "open")
}

// legacyAttachRoot e la radice usata prima della revisione (tmp condiviso): si
// ripulisce all'avvio, non si scrive piu.
func legacyAttachRoot() string { return filepath.Join(os.TempDir(), attachRootName) }

// attachSession e la cartella di sessione corrente (creata alla prima apertura) con
// la radice a cui appartiene: cambiando radice (test) se ne crea una nuova.
var attachSession struct {
	mu        sync.Mutex
	root, dir string
}

// ensureAttachRoot crea la radice 0700 se manca e rifiuta ogni cosa che non sia una
// cartella vera (un symlink porterebbe le scritture altrove).
func ensureAttachRoot(root string) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	st, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if !st.IsDir() { // Lstat: un symlink non e una cartella
		return fmt.Errorf("%s non e una cartella", root)
	}
	if st.Mode().Perm() != 0o700 {
		return os.Chmod(root, 0o700) // fallisce se non e nostra: allegati non aperti
	}
	return nil
}

// sessionDir restituisce la cartella della sessione, creandola se serve.
func sessionDir() (string, error) {
	attachSession.mu.Lock()
	defer attachSession.mu.Unlock()
	root := attachRoot()
	if attachSession.dir != "" && attachSession.root == root {
		if _, err := os.Lstat(attachSession.dir); err == nil {
			return attachSession.dir, nil
		}
	}
	if err := ensureAttachRoot(root); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(root, attachSessPrefix) // 0700
	if err != nil {
		return "", err
	}
	attachSession.root, attachSession.dir = root, dir
	return dir, nil
}

// CleanupAttachments rimuove la cartella degli allegati aperti in questa sessione
// (e la radice, se vuota). Il main la chiama all'uscita dell'app; non fa nulla se
// nessun allegato e stato aperto.
func CleanupAttachments() {
	attachSession.mu.Lock()
	defer attachSession.mu.Unlock()
	root, dir := attachSession.root, attachSession.dir
	attachSession.root, attachSession.dir = "", ""
	if dir == "" || filepath.Dir(dir) != root { // mai fuori dalla radice
		return
	}
	_ = os.RemoveAll(dir)
	_ = os.Remove(root) // solo se vuota
}

// SweepAttachments toglie dalla radice le cartelle di sessione s-* piu vecchie di
// 24 h (residui di sessioni chiuse male). Il main la chiama all'avvio. Ignora
// symlink, file e cartelle con altri nomi.
func SweepAttachments() {
	now := time.Now()
	sweepAttachments(attachRoot(), now, attachMaxAge)
	if legacy := legacyAttachRoot(); legacy != attachRoot() {
		sweepAttachments(legacy, now, attachMaxAge)
	}
}

func sweepAttachments(root string, now time.Time, maxAge time.Duration) {
	ents, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range ents {
		if !e.IsDir() || e.Type()&os.ModeSymlink != 0 || !strings.HasPrefix(e.Name(), attachSessPrefix) {
			continue
		}
		info, err := e.Info()
		if err != nil || now.Sub(info.ModTime()) <= maxAge {
			continue
		}
		_ = os.RemoveAll(filepath.Join(root, e.Name()))
	}
}

// attSeq numera i file generati (att-<n><ext>).
var attSeq atomic.Int64

// attachmentOpened e l'esito di openAttachment.
type attachmentOpened struct{ err error }

// openAttachment salva l'allegato in una cartella temporanea 0700 con nome generato
// e lo apre col comando di sistema (Piano 15, Task 16). Il tipo e gia stato
// controllato dal chiamante (safeAttachmentExt): qui si riceve l'estensione.
func openAttachment(st mail.MailStore, id msg.MessageID, part int, ext string) tea.Cmd {
	return func() tea.Msg {
		dir, err := sessionDir() // radice dedicata, 0700 (Piano 15, Task 16b)
		if err != nil {
			return attachmentOpened{err: err}
		}
		path := filepath.Join(dir, fmt.Sprintf("att-%d%s", attSeq.Add(1), ext))
		if err := st.SaveAttachment(context.Background(), id, part, path); err != nil {
			_ = os.Remove(path)
			return attachmentOpened{err: err}
		}
		return attachmentOpened{err: openFile(path)}
	}
}

// clickAttachment gestisce thread.AttachmentClickMsg: scarta i messaggi stantii e gli
// indici fuori range; un tipo non in allowlist da un toast e non scrive nulla.
func (m Model) clickAttachment(ev thread.AttachmentClickMsg) (Model, tea.Cmd) {
	cur := m.thread.Msg()
	if cur == nil || m.thread.Ref().ID != ev.ID || ev.Index < 0 || ev.Index >= len(cur.Attachments) {
		return m, nil
	}
	a := cur.Attachments[ev.Index]
	ext, ok := safeAttachmentExt(a.ContentType, a.Filename)
	if !ok {
		cmd := m.toast.Push(toast.Info, "tipo non aperto: usa :save")
		return m, cmd
	}
	return m, openAttachment(m.store, ev.ID, a.PartIndex, ext)
}

// linkAsk e la conferma in attesa per un link o bottone con Warn (dominio del testo
// diverso da quello reale): nulla si apre finche l'utente non preme s.
type linkAsk struct{ url, shown string }

// askLink apre la conferma nella barra per un link con Warn (Piano 15, Task 16).
func (m Model) askLink(url, shown string) Model {
	m.linkAsk = &linkAsk{url: url, shown: shown}
	return m
}

// text e il testo della barra di conferma: l'host reale e il dominio mostrato.
func (a linkAsk) text() string {
	reale := thread.LinkLabel(a.url)
	shown := a.shown
	if h, ok := richtext.ShownHost(shown); ok {
		shown = h
	}
	return fmt.Sprintf("il link porta a %s, non a %s: apri? s/esc", reale, shown)
}

// openLinkCmd apre url nel browser; l'esito e un thread.LinkOpenedMsg (toast come i
// link del corpo).
func openLinkCmd(url string) tea.Cmd {
	return func() tea.Msg {
		return thread.LinkOpenedMsg{Target: thread.LinkLabel(url), Err: openURL(url)}
	}
}
