// Feed Watch → refresh automatico e indicatore di connessione (Task 8): la
// radice apre il feed all'Init (startWatch), arma un loop waitForChange e al
// cambio rilista la query corrente IN SILENZIO riusando refreshRefs (Task 7).
//
// Contratto MailStore.Watch (store.go): un canale per chiamata, chiuso al
// ctx.Done, eventi best-effort (bufferizzato: se il consumer è lento gli
// eventi si perdono — la UI farà re-list); ChangeDisconnected arriva prima
// della riconnessione. waitForChange fa select anche su ctx.Done: copre il
// caso-contratto (canale chiuso dallo store) E gli store che non chiudono il
// canale all'annullamento (il Mock condivide un canale e non lo chiude mai).
//
// Semantica degli eventi:
//   - ChangeNewMail: feedback "nuova posta in arrivo" (non silenzioso) +
//     re-list silenzioso — il feedback deve restare visibile anche dopo il
//     risultato, come il feedback delle op (Task 7);
//   - ChangeFlags: SOLO re-list silenzioso, nessun feedback (il cambiamento
//     è visibile in lista; un feedback rischierebbe di coprire quello di
//     un'op dell'utente appena conclusa, che di solito È la causa del flag);
//   - ChangeDisconnected: indicatore ⟳ in status bar + re-list; l'indicatore
//     resta finché un msgsLoaded riuscito non lo sostituisce (la
//     riconnessione è andata a buon fine e la lista è fresca) — silent al
//     risultato, il msgsLoaded successivo spegne l'indicatore (model.go).
//
// I re-list silenziosi NON si accatastano: requestRefresh (ops.go) coalesce
// — con un refresh in flight marca dirty e il follow-up parte al risultato.
package app

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
)

const (
	// reconnectIndicator è l'indicatore di connessione in status bar durante
	// una disconnessione del feed (fino al primo msgsLoaded riuscito).
	reconnectIndicator = "⟳ riconnessione…"
	// newMailText è il feedback della nuova posta (ChangeNewMail).
	newMailText = "nuova posta in arrivo"
)

// startWatch apre il feed Watch fuori dal loop UI: il contratto permette alla
// chiamata di attendere ops in flight nel gate (la baseline gira in un round
// del gate, come le op) — mai bloccare il loop. Risultato: watchStarted.
func startWatch(st mail.MailStore, ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		ch, err := st.Watch(ctx)
		return watchStarted{ch: ch, err: err}
	}
}

// connectCmd chiama Store.Connect fuori dal loop UI con un budget di 30s
// (stesso valore del wizard): l'esito torna come connResult e decide se
// partire con load/Watch (bugfix 2026-11-22).
func connectCmd(st mail.MailStore, ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		c, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		return connResult{err: st.Connect(c)}
	}
}

// waitForChange è il cmd che blocca su una lettura del canale del feed
// (pattern classico bubbletea): al cambio risponde changeEvt; canale chiuso
// o ctx annullato → nil Msg, che il runtime scarta — il loop finisce senza
// ri-armarsi e nessun goroutine resta bloccato sul feed.
func waitForChange(ctx context.Context, ch <-chan mail.Change) tea.Cmd {
	return func() tea.Msg {
		select {
		case c, ok := <-ch:
			if !ok {
				return nil // canale chiuso (ctx.Done secondo contratto): loop finito
			}
			return changeEvt{c: c}
		case <-ctx.Done():
			return nil // ctx del programma annullato: loop finito
		}
	}
}

// handleChange consuma un evento del feed: re-list silenzioso della query
// corrente (coalescato, mai due in volo) + ri-armo del loop; il feedback
// dipende dal tipo di evento (vedi doc del package-file).
func (m Model) handleChange(ev changeEvt) (Model, tea.Cmd) {
	rearm := waitForChange(m.ctx, m.watchCh)
	switch ev.c.Kind {
	case mail.ChangeNewMail:
		m.status = statusMsg{text: newMailText}
		// Piano 7, fix code-review 3: requestRefresh muta m: prima del return.
		follow := m.requestRefresh(true)
		return m, tea.Batch(follow, rearm)
	case mail.ChangeFlags:
		// nessun feedback: solo la lista si aggiorna (prima pagina, D5)
		follow := m.requestRefresh(false) // Piano 7, fix code-review 3: prima del return
		return m, tea.Batch(follow, rearm)
	case mail.ChangeDisconnected:
		m.status = statusMsg{text: reconnectIndicator}
		follow := m.requestRefresh(true) // Piano 7, fix code-review 3: prima del return
		return m, tea.Batch(follow, rearm)
	default:
		// tipo ignoto (feed futuro): il loop resta vivo, nessun re-list
		return m, rearm
	}
}
