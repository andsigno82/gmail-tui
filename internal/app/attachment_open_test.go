package app

// Piano 15, Task 16 (D15-T14, Q15-1): clic su un chip allegato. Si apre solo un
// tipo in allowlist (Content-Type E estensione coerenti); il file va in una
// cartella temporanea 0700 con nome generato, mai col nome del messaggio.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/thread"
	"github.com/andsigno82/gmail-tui/internal/ui/toast"
)

// saveCounter conta le SaveAttachment (e ricorda i percorsi) senza toccare il resto.
type saveCounter struct {
	mail.MailStore
	mu    sync.Mutex
	paths []string
}

func (s *saveCounter) SaveAttachment(ctx context.Context, id msg.MessageID, part int, path string) error {
	s.mu.Lock()
	s.paths = append(s.paths, path)
	s.mu.Unlock()
	return s.MailStore.SaveAttachment(ctx, id, part, path)
}

func (s *saveCounter) saved() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.paths...)
}

// stubOpenFile sostituisce l'apertura del file con il comando di sistema.
func stubOpenFile(t *testing.T) *[]string {
	t.Helper()
	var opened []string
	orig := openFile
	openFile = func(p string) error { opened = append(opened, p); return nil }
	t.Cleanup(func() { openFile = orig })
	return &opened
}

// attModel apre il thread del primo messaggio con un solo allegato, dato.
func attModel(t *testing.T, att msg.Attachment) (Model, *saveCounter, msg.MessageRef) {
	t.Helper()
	ref := testRefs(t)[0]
	full := fullMessage(ref)
	full.Attachments = []msg.Attachment{att}
	m, mock := newTestModelMock(t, full)
	mock.Raw = map[msg.MessageID]map[int][]byte{ref.ID: {att.PartIndex: []byte("contenuto")}}
	sc := &saveCounter{MailStore: mock}
	m.store = sc
	return openThread(t, m, ref), sc, ref
}

func clickAttachment(t *testing.T, m Model, ref msg.MessageRef) Model {
	t.Helper()
	mm, cmd := step(m, thread.AttachmentClickMsg{ID: ref.ID, Index: 0})
	if cmd == nil {
		t.Fatal("AttachmentClickMsg: nessuna Cmd")
	}
	for _, ms := range runCmd(cmd) {
		mm, _ = step(mm, ms)
	}
	return mm
}

func TestAllegatoInAllowlistApreFileTemporaneo(t *testing.T) {
	opened := stubOpenFile(t)
	m, sc, ref := attModel(t, msg.Attachment{PartIndex: 0, Filename: "../../segreto.PDF", ContentType: "application/pdf", Size: 9})
	mm := clickAttachment(t, m, ref)

	if len(*opened) != 1 {
		t.Fatalf("aperture = %q, voglio 1", *opened)
	}
	p := (*opened)[0]
	if got := sc.saved(); len(got) != 1 || got[0] != p {
		t.Errorf("salvato in %q, aperto %q", got, p)
	}
	if base := filepath.Base(p); !strings.HasPrefix(base, "att-") || filepath.Ext(base) != ".pdf" {
		t.Errorf("nome %q, voglio att-<n>.pdf generato", base)
	}
	if strings.Contains(p, "segreto") {
		t.Errorf("il nome del messaggio e finito nel percorso: %q", p)
	}
	st, err := os.Stat(filepath.Dir(p))
	if err != nil {
		t.Fatal(err)
	}
	if perm := st.Mode().Perm(); perm != 0o700 {
		t.Errorf("cartella temporanea %v, voglio 0700", perm)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(p)) })
	if data, err := os.ReadFile(p); err != nil || string(data) != "contenuto" {
		t.Errorf("contenuto file = %q, %v", data, err)
	}
	assertToast(t, mm, toast.Success, "allegato aperto")
}

func TestAllegatoFuoriAllowlistNonScrivePiuNulla(t *testing.T) {
	cases := []struct{ name, ctype string }{
		{"app.command", "text/plain"},              // estensione non coerente col tipo
		{"setup.pkg", "application/octet-stream"},  // tipo non in lista
		{"foto.png", "application/pdf"},            // tipo e estensione discordi
		{"senza_estensione", "application/pdf"},    // estensione assente
		{"x.app", "application/x-apple-diskimage"}, // tipo pericoloso
		{"pagina.html", "text/html"},               // html non si apre
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			opened := stubOpenFile(t)
			m, sc, ref := attModel(t, msg.Attachment{PartIndex: 0, Filename: c.name, ContentType: c.ctype, Size: 9})
			// La Cmd e solo il timer di scadenza del toast (dorme davvero): non si esegue.
			mm, _ := step(m, thread.AttachmentClickMsg{ID: ref.ID, Index: 0})
			if len(sc.saved()) != 0 || len(*opened) != 0 {
				t.Errorf("scritture=%q aperture=%q, voglio nessuna", sc.saved(), *opened)
			}
			assertToast(t, mm, toast.Info, "tipo non aperto: usa :save")
		})
	}
}

// Un messaggio stantio (ID diverso dal selezionato) o un indice fuori range non fanno nulla.
func TestAllegatoStantioOFuoriRangeIgnorato(t *testing.T) {
	opened := stubOpenFile(t)
	m, sc, ref := attModel(t, msg.Attachment{PartIndex: 0, Filename: "a.pdf", ContentType: "application/pdf"})
	for _, c := range []thread.AttachmentClickMsg{
		{ID: msg.NewID("INBOX", 99), Index: 0},
		{ID: ref.ID, Index: 5},
		{ID: ref.ID, Index: -1},
	} {
		mm, cmd := step(m, c)
		if cmd != nil {
			t.Errorf("%+v: Cmd inattesa", c)
		}
		_ = mm
	}
	if len(sc.saved()) != 0 || len(*opened) != 0 {
		t.Errorf("scritture=%q aperture=%q", sc.saved(), *opened)
	}
}

// L'estensione si decide dall'allowlist, non dal nome del messaggio.
func TestAllowlistEstensioneCoerente(t *testing.T) {
	cases := []struct {
		ctype, name, ext string
		ok               bool
	}{
		{"application/pdf", "a.pdf", ".pdf", true},
		{"application/pdf", "A.PDF", ".pdf", true},
		{"image/png", "a.png", ".png", true},
		{"image/jpeg", "a.jpg", ".jpg", true},
		{"image/jpeg", "a.jpeg", ".jpg", true},
		{"image/gif", "a.gif", ".gif", true},
		{"text/plain", "a.txt", ".txt", true},
		{"text/plain; charset=utf-8", "a.txt", ".txt", true},
		{"IMAGE/PNG", "a.png", ".png", true},
		{"text/plain", "a.command", "", false},
		{"image/png", "a.pdf", "", false},
		{"application/zip", "a.zip", "", false},
		{"application/pdf", "", "", false},
	}
	for _, c := range cases {
		ext, ok := safeAttachmentExt(c.ctype, c.name)
		if ok != c.ok || ext != c.ext {
			t.Errorf("safeAttachmentExt(%q,%q) = (%q,%v), voglio (%q,%v)", c.ctype, c.name, ext, ok, c.ext, c.ok)
		}
	}
}
