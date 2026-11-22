// Composer (Piano 4, Task 3): builders reply/forward/new e wiring della vista
// composer nel radice. I builders trasformano un messaggio (o la config) nella
// ComposerConfig precompilata; l'apertura vera (openComposer) aggiunge l'
// identità dell'utente (mittente/firma via SetIdentity) e sostituisce la vista
// corrente. `r`/`f` nell'inbox girano fetch-then-compose (cache-first via
// store.FetchMessage); nel thread usano il messaggio già caricato.
// Piano 4, Task 4: flusso di invio — SendRequestedMsg apre la conferma modale
// ("Invio a …" nella status bar); `s` consegna la bozza al Sender fuori dal
// loop UI (budget di timeout condiviso col sottocomando send), l'esito torna
// come opDone{"inviato"} (ritorno inbox + refresh) o opDone errore (composer
// resta aperto con la bozza, riprova possibile).
package app

import (
	"context"
	"fmt"
	"mime"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/cmd"
	"github.com/andsigno82/gmail-tui/internal/config"
	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/composer"
	"github.com/andsigno82/gmail-tui/internal/ui/thread"
)

// composeLoaded è il risultato del fetch per r/f nell'inbox
// (fetch-then-compose): il messaggio completo e la direzione richiesta
// (forward=false → reply). Come msgLoaded porta il ref richiesto: il handler
// scarta i risultati stanti se l'utente ha cambiato vista nel frattempo.
type composeLoaded struct {
	ref     msg.MessageRef
	m       *msg.Message
	err     error
	forward bool
}

// replyFor costruisce la config precompilata della risposta: To = mittente
// del messaggio, Subject = "Re: <subject>" (idempotente: un oggetto già
// "Re: …", qualsiasi capitalizzazione, resta intatto), corpo citato con
// header "Il <data>, <from> ha scritto:" e ogni riga prefissata "> ".
// Il ref del MESSAGGIO COMPLETO è la fonte veritiera (mittente/oggetto/data):
// quello di lista può essere una riga appena ri-listata — vince il suo Ref.
// REQUISITO: full deve essere non-nil (i call site lo garantiscono: il handler
// di composeLoaded guarda ev.m != nil, il thread passa il messaggio caricato
// dopo aver controllato Loaded()) — il corpo citato legge full.BodyText.
func replyFor(ref msg.MessageRef, full *msg.Message) composer.ComposerConfig {
	if full != nil && full.Ref.From != "" {
		ref = full.Ref
	}
	return composer.ComposerConfig{
		To:         ref.From,
		Subject:    reSubject(ref.Subject),
		QuotedBody: quoteHeader(ref) + "\n" + quoteLines(full.BodyText),
	}
}

// forwardFor costruisce la config precompilata dell'inoltro: NESSUN
// destinatario (il forward non indovina il destinatario), Subject =
// "Fwd: <subject>" (idempotente), stesso corpo citato del reply + blocco con
// gli allegati ORIGINALI INDICATI. LIMITAZIONE v1 documentata: gli allegati
// NON sono ri-allegati (il composer allega solo file locali) — il blocco li
// elenca perché l'utente sappia cosa manca.
// REQUISITO: full deve essere non-nil (vedi replyFor) — il corpo citato legge
// full.BodyText.
func forwardFor(ref msg.MessageRef, full *msg.Message) composer.ComposerConfig {
	if full != nil && full.Ref.From != "" {
		ref = full.Ref
	}
	cfg := composer.ComposerConfig{
		Subject:    fwdSubject(ref.Subject),
		QuotedBody: quoteHeader(ref) + "\n" + quoteLines(full.BodyText),
	}
	if names := attachmentNames(full); names != "" {
		cfg.QuotedBody += "\n\n[allegati originali NON ri-allegati in v1: " + names + "]"
	}
	return cfg
}

// newMessage costruisce il composer per un nuovo messaggio: mittente e firma
// dalla config, tutti i campi di contenuto vuoti.
func newMessage(cfg config.Config) composer.ComposerConfig {
	return composer.ComposerConfig{From: cfg.Address, Signature: cfg.Signature}
}

// reSubject prefissa "Re: " se l'oggetto non lo contiene già (idempotente
// anche su "re:"/"RE:" — il confronto è case-insensitive).
func reSubject(s string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(s)), "re:") {
		return s
	}
	return "Re: " + s
}

// fwdSubject prefissa "Fwd: " se l'oggetto non lo contiene già.
func fwdSubject(s string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(s)), "fwd:") {
		return s
	}
	return "Fwd: " + s
}

// quoteHeader è la riga di citazione: "Il 20 nov 2026 10:00, anna@x.dev ha
// scritto:" (stesso formato data dell'header del thread, via FormatDate).
func quoteHeader(ref msg.MessageRef) string {
	return fmt.Sprintf("Il %s, %s ha scritto:", thread.FormatDate(ref.Date), ref.From)
}

// quoteLines prefissa "> " a ogni riga del corpo (le righe vuote diventano
// ">"): il quoting è letterale per riga, anche su righe già citate (che
// diventano "> > …"), come da piano.
func quoteLines(body string) string {
	lines := strings.Split(body, "\n")
	for i, ln := range lines {
		if ln == "" {
			lines[i] = ">"
		} else {
			lines[i] = "> " + ln
		}
	}
	return strings.Join(lines, "\n")
}

// attachmentNames elenca i nomi file degli allegati (per il blocco del
// forward), virgola-spazio separati.
func attachmentNames(full *msg.Message) string {
	if full == nil {
		return ""
	}
	names := make([]string, 0, len(full.Attachments))
	for _, a := range full.Attachments {
		names = append(names, a.Filename)
	}
	return strings.Join(names, ", ")
}

// openComposer apre la vista composer per la config data: mittente e firma
// sono integrati dalla identity del radice SOLO se la config non li porta
// già (i builders reply/forward li lasciano vuoti, newMessage li passa).
// La vista di provenienza è custodita: il back del composer torna lì.
func (m *Model) openComposer(cfg composer.ComposerConfig) {
	if cfg.From == "" {
		cfg.From = m.from
	}
	if cfg.Signature == "" {
		cfg.Signature = m.signature
	}
	m.composer = composer.New(cfg, m.styles)
	m.composeBack = m.view
	m.discardOpen = false
	m.draft = msg.Outgoing{}
	m.draftID = "" // nuova bozza: nessuna copia in Bozze (Piano 9)
	m.draftBusy = false
	m.view = viewComposer
	if m.width > 0 {
		m.composer.SetSize(m.width, max(0, m.height-1))
	}
}

// closeComposer chiude il composer e torna alla vista di provenienza.
func (m *Model) closeComposer() {
	m.composer = composer.Model{}
	m.discardOpen = false
	m.draftID = ""
	m.draftBusy = false
	m.view = m.composeBack
}

// startCompose gestisce r/f: nell'inbox fetch-then-compose (spinner finché
// composeLoaded); nel thread il messaggio è già caricato — composer
// immediato. Senza ref (lista vuota) o senza msg caricato: no-op silenzioso.
func (m Model) startCompose(forward bool) (Model, tea.Cmd) {
	switch m.view {
	case viewInbox:
		ref, ok := m.currentRef()
		if !ok {
			return m, nil
		}
		m.pending++
		return m, tea.Batch(fetchForCompose(m.store, ref, forward), m.spinner.Tick)
	case viewThread:
		if !m.thread.Loaded() {
			return m, nil
		}
		ref := m.thread.Ref()
		cfg := replyFor(ref, m.thread.Msg())
		if forward {
			cfg = forwardFor(ref, m.thread.Msg())
		}
		m.openComposer(cfg)
		return m, nil
	}
	return m, nil
}

// fetchForCompose esegue store.FetchMessage fuori dal loop UI (cache-first
// nel backend): il risultato composeLoaded porta ref, messaggio e direzione.
func fetchForCompose(st mail.MailStore, ref msg.MessageRef, forward bool) tea.Cmd {
	return func() tea.Msg {
		full, err := st.FetchMessage(context.Background(), ref.ID)
		return composeLoaded{ref: ref, m: full, err: err, forward: forward}
	}
}

// --- invio (Piano 4, Task 4) --------------------------------------------------

// sendConfirmText compone il testo della conferma pre-invio (status bar, stile
// StatusErr): "Invio a <to> — <oggetto> — <n> allegati — s conferma · esc
// annulla". I destinatari sono A, Cc e Ccn in quest'ordine (join virgole, voci
// vuote omesse: Piano 7, fix review 7, una bozza solo in Cc è valida); i
// segmenti vuoti (oggetto vuoto, nessun allegato) sono omessi. Con un solo
// allegato il conteggio è al singolare ("1 allegato").
func sendConfirmText(out msg.Outgoing) string {
	// Piano 7.1, fix code-review: il nome non ASCII è in RFC 2047 nella bozza
	// (header); qui si decodifica per la status bar, leggibile.
	var dec mime.WordDecoder
	var rcpt []string
	for _, group := range [][]string{out.To, out.Cc, out.Bcc} {
		for _, a := range group {
			if a = strings.TrimSpace(a); a != "" {
				if d, err := dec.DecodeHeader(a); err == nil {
					a = d
				}
				rcpt = append(rcpt, a)
			}
		}
	}
	to := strings.Join(rcpt, ", ")
	if to == "" {
		to = "(nessun destinatario)"
	}
	segs := []string{"Invio a " + to}
	if out.Subject != "" {
		segs = append(segs, out.Subject)
	}
	if n := len(out.Attachments); n == 1 {
		segs = append(segs, "1 allegato")
	} else if n > 1 {
		segs = append(segs, fmt.Sprintf("%d allegati", n))
	}
	return strings.Join(append(segs, "s conferma · esc annulla"), " — ")
}

// sendCmd consegna l'invio in attesa al Sender fuori dal loop UI (Piano 4, Task 4;
// Piano 9, Task 7 e 9): l'esito torna come opDone. Errore → toast d'errore e il
// composer si riapre con la bozza (retry). Successo → refresh e, se la bozza era
// ripresa da Bozze, la si toglie da lì (D9-E).
func sendCmd(sender Sender, st mail.MailStore, p *pendingSend) tea.Cmd {
	return func() tea.Msg { return deliverSend(sender, st, p) }
}

// deliverSend esegue l'invio con il budget SendTimeout (stessa semantica
// anti-hang del sottocomando send) e poi rimuove la bozza ripresa. Un errore di
// rimozione non annulla l'invio: torna in draftErr (D9-E). Usata da sendCmd e dal
// flush in uscita (quitCmd).
func deliverSend(sender Sender, st mail.MailStore, p *pendingSend) opDone {
	ctx, cancel := context.WithTimeout(context.Background(), cmd.SendTimeout)
	defer cancel()
	if err := sender.Send(ctx, p.out); err != nil {
		return opDone{label: "invio", err: err, send: true, retry: p}
	}
	res := opDone{label: "inviato", refresh: true, send: true, observed: addresses(p.out.To, p.out.Cc, p.out.Bcc)}
	if p.draftID != "" {
		dctx, dcancel := context.WithTimeout(context.Background(), cmd.SendTimeout)
		defer dcancel()
		res.draftErr = st.DeleteDraft(dctx, []msg.MessageID{p.draftID})
	}
	return res
}
