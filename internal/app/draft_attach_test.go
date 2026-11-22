package app

import (
	netmail "net/mail"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/composer"
	"github.com/andsigno82/gmail-tui/internal/ui/toast"
)

// Ripresa di una bozza con allegati (fix review): gli allegati si scaricano in
// una directory privata e tornano nel composer come file locali; il risalvataggio
// e l'invio li conservano. Se un download fallisce la bozza originale non si tocca.

// attachedDraftMock prepara un Mock con una bozza in Bozze con un allegato.
// withRaw=false: il contenuto dell'allegato manca (download che fallisce).
func attachedDraftMock(t *testing.T, withRaw bool) (Model, *mail.Mock, msg.MessageRef) {
	t.Helper()
	bozza := draftMsg(1, []string{"anna@example.dev"}, "ciao", "testo")
	bozza.Attachments = []msg.Attachment{{PartIndex: 0, Filename: "report.pdf", ContentType: "application/pdf"}}
	m, mk := composerModel(t, bozza)
	if withRaw {
		mk.Raw = map[msg.MessageID]map[int][]byte{bozza.Ref.ID: {0: []byte("PDF-DATA")}}
	}
	mm, _ := step(m, teaSize())
	mm.folder = mail.Mailbox{Name: "[Gmail]/Drafts", Role: "drafts"}
	return mm, mk, bozza.Ref
}

// resumeFlow apre la bozza come farebbe l'utente (invio sulla riga di Bozze) e
// segue le cmd successive (fetch, download degli allegati) fino al composer.
func resumeFlow(t *testing.T, m Model, ref msg.MessageRef) Model {
	t.Helper()
	m, cmd := step(m, inboxSelected(ref))
	cmds := []tea.Cmd{cmd}
	for i := 0; i < 4 && len(cmds) > 0; i++ {
		var next []tea.Cmd
		for _, c := range cmds {
			if c == nil {
				continue
			}
			for _, ms := range runCmd(c) {
				var nc tea.Cmd
				m, nc = step(m, ms)
				if nc != nil {
					next = append(next, nc)
				}
			}
		}
		cmds = next
	}
	return m
}

// TestResumeDownloadsAttachments: la ripresa scarica l'allegato in una directory
// privata e il composer lo riceve come file locale con il nome originale.
func TestResumeDownloadsAttachments(t *testing.T) {
	m, _, ref := attachedDraftMock(t, true)
	mm := resumeFlow(t, m, ref)
	if mm.currentView() != viewComposer {
		t.Fatalf("vista = %v, voglio composer", mm.currentView())
	}
	atts := mm.composer.Draft().Attachments
	if len(atts) != 1 || filepath.Base(atts[0]) != "report.pdf" {
		t.Fatalf("allegati nel composer = %v, voglio report.pdf", atts)
	}
	data, err := os.ReadFile(atts[0])
	if err != nil || string(data) != "PDF-DATA" {
		t.Errorf("contenuto allegato = %q (err %v), voglio PDF-DATA", data, err)
	}
	if mm.draftTmp == "" {
		t.Error("directory temporanea degli allegati non registrata")
	}
	if strings.Contains(toastText(mm), "allegati non ripresi") {
		t.Errorf("avviso di perdita ancora presente: %q", toastText(mm))
	}
	mm.discardDraftAndRemoveTmp(t)
}

// discardDraftAndRemoveTmp è un helper di pulizia dei test: rimuove la directory.
func (m Model) discardDraftAndRemoveTmp(t *testing.T) {
	t.Helper()
	if m.draftTmp != "" {
		_ = os.RemoveAll(m.draftTmp)
	}
}

// TestResaveKeepsAttachments: risalvare la bozza ripresa sostituisce la copia e
// l'allegato resta nella nuova bozza.
func TestResaveKeepsAttachments(t *testing.T) {
	m, mk, ref := attachedDraftMock(t, true)
	mm := resumeFlow(t, m, ref)
	tmp := mm.draftTmp
	mm, cmd := step(mm, pressKey("esc"))
	mm = drainOp(t, mm, cmd)
	ids := inDrafts(mk)
	if len(ids) != 1 || ids[0] == ref.ID {
		t.Fatalf("bozze in Bozze = %v, voglio una sola copia nuova", ids)
	}
	var saved *msg.Message
	for _, mm := range mk.Msgs {
		if mm.Ref.ID == ids[0] {
			saved = mm
		}
	}
	if saved == nil || len(saved.Attachments) != 1 || saved.Attachments[0].Filename != "report.pdf" {
		t.Errorf("bozza risalvata senza l'allegato: %+v", saved)
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Errorf("directory temporanea ancora presente dopo il risalvataggio: %v", err)
	}
}

// TestResumeDownloadFailureKeepsOriginal: se il download fallisce → toast d'errore,
// il composer si apre e il risalvataggio NON sostituisce la bozza originale.
func TestResumeDownloadFailureKeepsOriginal(t *testing.T) {
	m, mk, ref := attachedDraftMock(t, false)
	mm := resumeFlow(t, m, ref)
	if !toastIs(mm, toast.Error) {
		t.Errorf("toast = %q, voglio errore del download", toastText(mm))
	}
	mm, cmd := step(mm, pressKey("esc"))
	drainOp(t, mm, cmd)
	if got := inDrafts(mk); len(got) != 2 {
		t.Errorf("bozze in Bozze = %v, voglio l'originale e la copia (doppione, nessuna perdita)", got)
	}
}

// TestDiscardRemovesAttachmentDir: lo scarto della bozza ripresa rimuove la directory.
func TestDiscardRemovesAttachmentDir(t *testing.T) {
	m, _, ref := attachedDraftMock(t, true)
	mm := resumeFlow(t, m, ref)
	tmp := mm.draftTmp
	mm, _ = step(mm, pressKey("ctrl+d"))
	mm, cmd := step(mm, pressKey("s"))
	mm = drainOp(t, mm, cmd)
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Errorf("directory temporanea ancora presente dopo lo scarto: %v", err)
	}
}

// TestRecipientNamesSurviveResume: il nome dei destinatari sopravvive al giro
// bozza → Bozze → ripresa ("Nome <indirizzo>").
func TestRecipientNamesSurviveResume(t *testing.T) {
	m, mk := composerModel(t)
	mm, _ := step(m, teaSize())
	mm = draftComposer(mm, composer.ComposerConfig{To: "Anna Rossi <anna@x.dev>", Subject: "s", Body: "c"})
	mm, cmd := step(mm, pressKey("esc"))
	mm = drainOp(t, mm, cmd)
	ids := inDrafts(mk)
	if len(ids) != 1 {
		t.Fatalf("bozze = %v, voglio 1", ids)
	}
	mm.folder = mail.Mailbox{Name: "[Gmail]/Drafts", Role: "drafts"}
	var ref msg.MessageRef
	for _, mm := range mk.Msgs {
		if mm.Ref.ID == ids[0] {
			ref = mm.Ref
		}
	}
	mm = resumeFlow(t, mm, ref)
	// Il composer serializza i destinatari in RFC 5322 (il nome può tornare quotato):
	// si verifica nome e indirizzo, non la stringa.
	to := mm.composer.Draft().To
	if len(to) != 1 {
		t.Fatalf("destinatari ripresi = %v, voglio 1", to)
	}
	a, err := netmail.ParseAddress(to[0])
	if err != nil || a.Name != "Anna Rossi" || a.Address != "anna@x.dev" {
		t.Errorf("destinatario ripreso = %q (nome %q, indirizzo %q), voglio Anna Rossi <anna@x.dev>", to[0], a.Name, a.Address)
	}
}

// TestSendAfterFailedDownloadKeepsOriginal: la bozza ripresa senza allegati (download
// fallito) non si toglie da Bozze all'invio riuscito: l'allegato esiste solo lì,
// e un toast lo dice.
func TestSendAfterFailedDownloadKeepsOriginal(t *testing.T) {
	m, mk, ref := attachedDraftMock(t, false)
	mm := resumeFlow(t, m, ref)
	mm = sendRequested(t, mm)
	mm, _ = step(mm, pressKey("s"))
	mm, done := flushUndo(t, mm)
	mm, _ = step(mm, done)
	if !done.draftKept {
		t.Error("opDone senza draftKept: la bozza originale verrebbe rimossa")
	}
	if got := inDrafts(mk); len(got) != 1 || got[0] != ref.ID {
		t.Errorf("bozze in Bozze = %v, voglio l'originale (%s) ancora presente", got, ref.ID)
	}
	if !strings.Contains(toastText(mm), "conservata") {
		t.Errorf("toast = %q, voglio l'avviso della bozza originale conservata", toastText(mm))
	}
}
