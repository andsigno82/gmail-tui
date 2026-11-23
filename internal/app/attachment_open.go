package app

// Apertura di allegati e link cliccati (Piano 15, Task 16; D15-T14, Q15-1).
// Un allegato si apre solo se e di un tipo sicuro: Content-Type in allowlist E
// estensione coerente (su macOS `open` esegue .app, .command, .pkg: il tipo non si
// decide dal nome da solo). Il file e scritto in una cartella temporanea 0700 con un
// nome generato (mai quello del messaggio) e aperto col comando di sistema, senza
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
	"sync/atomic"

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

// attSeq numera i file generati (att-<n><ext>).
var attSeq atomic.Int64

// attachmentOpened e l'esito di openAttachment.
type attachmentOpened struct{ err error }

// openAttachment salva l'allegato in una cartella temporanea 0700 con nome generato
// e lo apre col comando di sistema (Piano 15, Task 16). Il tipo e gia stato
// controllato dal chiamante (safeAttachmentExt): qui si riceve l'estensione.
func openAttachment(st mail.MailStore, id msg.MessageID, part int, ext string) tea.Cmd {
	return func() tea.Msg {
		dir, err := os.MkdirTemp("", "gmail-tui-att-") // 0700
		if err != nil {
			return attachmentOpened{err: err}
		}
		path := filepath.Join(dir, fmt.Sprintf("att-%d%s", attSeq.Add(1), ext))
		if err := st.SaveAttachment(context.Background(), id, part, path); err != nil {
			_ = os.RemoveAll(dir)
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
