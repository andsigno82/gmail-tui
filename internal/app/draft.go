// Bozze (Piano 9, Task 5-7, D9-A..E): esc salva il composer nella cartella
// Bozze, ctrl+d scarta con conferma, invio su una riga di Bozze riprende la
// bozza, il risalvataggio sostituisce la precedente e l'invio la toglie da
// Bozze. Ogni operazione sul server gira in tea.Cmd; il MIME è smtp.BuildDraft.
package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/cmd"
	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/mail/smtp"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/composer"
	"github.com/andsigno82/gmail-tui/internal/ui/toast"
)

// draftSaved è l'esito di SaveDraft (Piano 9, Task 5): ok → composer chiuso
// con toast; errore → composer aperto con la bozza intatta.
// id è la copia in Bozze anche con err (rimozione della precedente fallita,
// Piano 9, review F): la bozza del composer punta alla copia nuova.
type draftSaved struct {
	id  msg.MessageID
	err error
}

// draftDeleted è l'esito di DeleteDraft: discard → toast "bozza scartata" se
// riuscita; errore → toast d'errore in ogni caso.
type draftDeleted struct {
	err     error
	discard bool
}

// resumeLoaded è il risultato del fetch per riprendere una bozza da Bozze
// (Piano 9, Task 6). Come composeLoaded porta il ref richiesto.
type resumeLoaded struct {
	ref msg.MessageRef
	m   *msg.Message
	err error
}

// saveDraftCmd costruisce il MIME della bozza (smtp.BuildDraft, con Bcc) e la
// salva fuori dal loop UI. replace sono le bozze da sostituire: lo store le
// rimuove DOPO l'APPEND e rifiuta quelle che non stanno in Bozze.
func saveDraftCmd(st mail.MailStore, out msg.Outgoing, replace []msg.MessageID) tea.Cmd {
	return func() tea.Msg {
		raw, err := smtp.BuildDraft(out, time.Now())
		if err != nil {
			return draftSaved{err: err}
		}
		ctx, cancel := context.WithTimeout(context.Background(), cmd.SendTimeout)
		defer cancel()
		id, err := st.SaveDraft(ctx, raw, replace)
		return draftSaved{id: id, err: err}
	}
}

// deleteDraftCmd toglie una bozza da Bozze (per UID, validato dallo store).
func deleteDraftCmd(st mail.MailStore, id msg.MessageID, discard bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), cmd.SendTimeout)
		defer cancel()
		return draftDeleted{err: st.DeleteDraft(ctx, []msg.MessageID{id}), discard: discard}
	}
}

// fetchForResume legge il messaggio della bozza fuori dal loop UI.
func fetchForResume(st mail.MailStore, ref msg.MessageRef) tea.Cmd {
	return func() tea.Msg {
		full, err := st.FetchMessage(context.Background(), ref.ID)
		return resumeLoaded{ref: ref, m: full, err: err}
	}
}

// handleComposerCancel gestisce esc (Piano 9, D9-A e D9-3). Bozza vuota (nessun
// destinatario, oggetto, allegato, corpo): il composer si chiude e, se la bozza
// era già in Bozze, la si toglie (una bozza svuotata non va conservata). Bozza
// non vuota: salvataggio in Bozze; il composer resta aperto fino all'esito
// (draftBusy). Un risalvataggio sostituisce la bozza ripresa (D9-B).
func (m *Model) handleComposerCancel() tea.Cmd {
	if m.draftBusy {
		return nil
	}
	if m.composer.Empty() {
		id, keep := m.draftID, m.draftKeep
		removeTmp(m.draftTmp)
		m.closeComposer()
		if id != "" && !keep {
			return deleteDraftCmd(m.store, id, false)
		}
		return nil
	}
	m.draftBusy = true
	var replace []msg.MessageID
	if m.draftID != "" && !m.draftKeep {
		replace = []msg.MessageID{m.draftID}
	}
	return saveDraftCmd(m.store, m.composer.Draft(), replace)
}

// requestDiscard gestisce ctrl+d (Piano 9, D9-C): una bozza vuota senza copia
// salvata si chiude subito; altrimenti si apre la conferma "scartare la bozza?".
func (m Model) requestDiscard() (Model, tea.Cmd) {
	if m.composer.Empty() && m.draftID == "" {
		m.closeComposer()
		return m, nil
	}
	m.discardOpen = true
	return m, nil
}

// discardDraft è `s` nel modale di scarto: chiude il composer e, se la bozza è
// già in Bozze, la rimuove da lì (stessa rimozione per UID del risalvataggio).
func (m *Model) discardDraft() tea.Cmd {
	id := m.draftID
	removeTmp(m.draftTmp)
	m.closeComposer()
	if id != "" {
		return deleteDraftCmd(m.store, id, true)
	}
	cmd := m.toast.Push(toast.Info, "bozza scartata")
	return cmd
}

// draftAttachments è l'esito del download degli allegati di una bozza ripresa
// (fix review): dir è la directory privata con i file (vuota se fallisce).
type draftAttachments struct {
	ref   msg.MessageRef
	m     *msg.Message
	dir   string
	paths []string
	err   error
}

// downloadDraftCmd scarica gli allegati della bozza in una directory privata
// (os.MkdirTemp: 0700), un sottodirectory per allegato così i nomi originali
// restano. Un download che fallisce elimina tutto e riporta l'errore.
func downloadDraftCmd(st mail.MailStore, ref msg.MessageRef, full *msg.Message) tea.Cmd {
	return func() tea.Msg {
		dir, err := os.MkdirTemp("", "gmail-tui-bozza-")
		if err != nil {
			return draftAttachments{ref: ref, m: full, err: fmt.Errorf("directory temporanea: %w", err)}
		}
		ctx, cancel := context.WithTimeout(context.Background(), cmd.SendTimeout)
		defer cancel()
		var paths []string
		for i, a := range full.Attachments {
			sub := filepath.Join(dir, strconv.Itoa(i))
			p := filepath.Join(sub, safeFilename(a.Filename, i))
			if err := os.Mkdir(sub, 0o700); err != nil {
				removeTmp(dir)
				return draftAttachments{ref: ref, m: full, err: fmt.Errorf("directory allegato: %w", err)}
			}
			if err := st.SaveAttachment(ctx, ref.ID, a.PartIndex, p); err != nil {
				removeTmp(dir)
				return draftAttachments{ref: ref, m: full, err: fmt.Errorf("allegato %q: %w", a.Filename, err)}
			}
			paths = append(paths, p)
		}
		return draftAttachments{ref: ref, m: full, dir: dir, paths: paths}
	}
}

// safeFilename è il nome del file dell'allegato: solo il nome base (niente
// percorsi dal messaggio); senza nome utile ne serve uno generico.
func safeFilename(name string, i int) string {
	b := filepath.Base(name)
	if b == "" || b == "." || b == string(filepath.Separator) {
		return fmt.Sprintf("allegato-%d", i+1)
	}
	return b
}

// removeTmp elimina la directory degli allegati di una bozza ripresa ("" = nulla).
func removeTmp(dir string) {
	if dir != "" {
		_ = os.RemoveAll(dir)
	}
}

// openResumed apre il composer con una bozza di Bozze: destinatari (con il nome),
// oggetto, corpo (firma tolta), allegati scaricati. keep: la bozza è ripresa senza
// allegati, quindi il risalvataggio non sostituisce l'originale (nessuna perdita).
func (m *Model) openResumed(ref msg.MessageRef, full *msg.Message, paths []string, dir string, keep bool) {
	body := full.BodyText
	if m.signature != "" {
		body = strings.TrimSuffix(body, "\n\n--\n"+m.signature)
	}
	m.openComposer(composer.ComposerConfig{
		To:          strings.Join(displayOr(full.ToDisplay, full.To), ", "),
		Cc:          strings.Join(displayOr(full.CcDisplay, full.Cc), ", "),
		Bcc:         strings.Join(displayOr(full.BccDisplay, full.Bcc), ", "),
		Subject:     full.Ref.Subject,
		Body:        body,
		Attachments: paths,
	})
	m.draftID = ref.ID
	m.draftTmp = dir
	m.draftKeep = keep
}

// displayOr usa la forma "Nome <indirizzo>" se c'è, altrimenti gli indirizzi nudi
// (messaggi costruiti senza msg.Parse, es. nel Mock).
func displayOr(display, bare []string) []string {
	if len(display) > 0 {
		return display
	}
	return bare
}
