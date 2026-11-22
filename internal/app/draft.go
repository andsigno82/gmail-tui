// Bozze (Piano 9, Task 5-7, D9-A..E): esc salva il composer nella cartella
// Bozze, ctrl+d scarta con conferma, invio su una riga di Bozze riprende la
// bozza, il risalvataggio sostituisce la precedente e l'invio la toglie da
// Bozze. Ogni operazione sul server gira in tea.Cmd; il MIME è smtp.BuildDraft.
package app

import (
	"context"
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
type draftSaved struct{ err error }

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
		_, err = st.SaveDraft(ctx, raw, replace)
		return draftSaved{err: err}
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
		id := m.draftID
		m.closeComposer()
		if id != "" {
			return deleteDraftCmd(m.store, id, false)
		}
		return nil
	}
	m.draftBusy = true
	var replace []msg.MessageID
	if m.draftID != "" {
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
	m.closeComposer()
	if id != "" {
		return deleteDraftCmd(m.store, id, true)
	}
	cmd := m.toast.Push(toast.Info, "bozza scartata")
	return cmd
}

// resumeDraft riapre una bozza di Bozze nel composer (Piano 9, Task 6, D9-D):
// destinatari (A, Cc, Ccn), oggetto e corpo. La firma salvata in coda al corpo
// si toglie: la riaggiunge il composer. Gli allegati non sono ricostruibili da
// file locali: un toast d'avviso li elenca. draftID = ID della bozza ripresa.
func (m *Model) resumeDraft(ref msg.MessageRef, full *msg.Message) tea.Cmd {
	body := full.BodyText
	if m.signature != "" {
		body = strings.TrimSuffix(body, "\n\n--\n"+m.signature)
	}
	m.openComposer(composer.ComposerConfig{
		To:      strings.Join(full.To, ", "),
		Cc:      strings.Join(full.Cc, ", "),
		Bcc:     strings.Join(full.Bcc, ", "),
		Subject: full.Ref.Subject,
		Body:    body,
	})
	m.draftID = ref.ID
	if len(full.Attachments) == 0 {
		return nil
	}
	names := make([]string, 0, len(full.Attachments))
	for _, a := range full.Attachments {
		names = append(names, a.Filename)
	}
	cmd := m.toast.Push(toast.Info, "allegati non ripresi: "+strings.Join(names, ", "))
	return cmd
}
