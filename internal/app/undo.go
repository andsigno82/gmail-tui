// Annulla di archivia e cestino (Piano 10, Task 6, D10-U1/U2, D10-T6, D10-C1/C2):
// dopo un'azione riuscita il toast offre `u` per undoBulkWindow. L'ULTIMA azione
// annullabile vince: un archivia o un cestino sostituiscono l'annulla dell'invio
// (che parte comunque allo scadere), e un invio sostituisce l'annulla precedente.
// Il ritorno dell'archivia usa gli ID di All Mail dei soli messaggi archiviati (mai
// la conversazione intera); il ritorno del cestino è per gruppo (una conversazione
// alla volta), con le sue label utente, perché Gmail le perde col cestino.
package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/toast"
)

// undoBulkWindow è la finestra di annullamento di archivia e cestino (D10-U1). I test
// non la aspettano: consegnano direttamente bulkUndoDueMsg col seq dell'annulla.
var undoBulkWindow = 10 * time.Second

// undoKind distingue i due annulli.
type undoKind int

const (
	undoArchive undoKind = iota + 1
	undoTrash
)

// trashGroup è il ritorno di una conversazione spostata nel cestino: gli ID che Trash
// ha restituito, la cartella d'origine e le label utente da riapplicare.
type trashGroup struct {
	ids    []msg.MessageID
	dest   string
	labels []string
}

// bulkUndo è un'azione annullabile. archived: gli ID di All Mail da riportare in INBOX
// (undoArchive). groups: i ritorni dal cestino (undoTrash). n: conversazioni, per il
// toast d'esito. seq: identifica la finestra (timer e scadenza stantia).
type bulkUndo struct {
	seq      int
	kind     undoKind
	n        int
	archived []msg.MessageID
	groups   []trashGroup
}

// bulkUndoDueMsg è lo scadere della finestra dell'annulla; seq stantio → ignorato.
type bulkUndoDueMsg struct{ seq int }

// bulkUndoTimer arma la finestra dell'annulla.
func bulkUndoTimer(seq int) tea.Cmd {
	return tea.Tick(undoBulkWindow, func(time.Time) tea.Msg { return bulkUndoDueMsg{seq: seq} })
}

// archiveRows archivia le conversazioni delle righe (Piano 10, Task 6). PRIMA di
// Archive registra gli ID di All Mail (D10-T6): se AllMailIDs fallisce l'archivia
// avviene comunque, ma senza annulla. Restituisce gli ID usciti dalla INBOX.
func archiveRows(ctx context.Context, st mail.MailStore, rows []msg.MessageRef) ([]msg.MessageID, *bulkUndo, error) {
	ids, err := bulkIDs(ctx, st, rows)
	if err != nil {
		return nil, nil, err
	}
	var undo *bulkUndo
	if all, aerr := st.AllMailIDs(ctx, ids); aerr == nil && len(all) > 0 {
		undo = &bulkUndo{kind: undoArchive, n: len(rows), archived: all}
	}
	if err := st.Archive(ctx, ids); err != nil {
		return ids, nil, err
	}
	return ids, undo, nil
}

// trashRows sposta nel cestino le conversazioni delle righe, una per volta (D10-C2).
// Tutti gli ID sono risolti e guardati PRIMA di qualunque Trash (come bulkIDs): un
// rifiuto di guardia non sposta nulla. Il ritorno è per gruppo, con gli ID che Trash
// ha restituito; se uno dei gruppi non li ha, il ritorno non è offerto (nil).
func trashRows(ctx context.Context, st mail.MailStore, rows []msg.MessageRef) ([]msg.MessageID, *bulkUndo, error) {
	plan := make([][]msg.MessageID, len(rows))
	for i, r := range rows {
		ids, err := conversationIDs(ctx, st, r)
		if err == nil {
			err = guardBox(ids, boxOf(r))
		}
		if err != nil {
			return nil, nil, err
		}
		plan[i] = ids
	}
	var gone []msg.MessageID
	var groups []trashGroup
	restorable := true
	for i, r := range rows {
		in, err := st.Trash(ctx, plan[i])
		if err != nil {
			return gone, nil, err
		}
		gone = append(gone, plan[i]...)
		if len(in) == 0 {
			restorable = false
		}
		groups = append(groups, trashGroup{ids: in, dest: boxOf(r), labels: userLabels(r.Labels)})
	}
	if !restorable {
		return gone, nil, nil
	}
	return gone, &bulkUndo{kind: undoTrash, n: len(rows), groups: groups}, nil
}

// userLabels toglie dalle label della riga quelle di sistema (\Inbox, \Important…),
// quelle di Gmail ([Gmail]/…) e INBOX: restano le label utente.
func userLabels(labels []string) []string {
	var out []string
	for _, l := range labels {
		up := strings.ToUpper(l)
		if strings.HasPrefix(l, `\`) || strings.HasPrefix(up, "[GMAIL]/") || up == "INBOX" {
			continue
		}
		out = append(out, l)
	}
	return out
}

// undoCmd esegue l'annulla fuori dal loop UI: l'archivia torna in INBOX con \Inbox
// sugli ID di All Mail; il cestino torna nella cartella d'origine (Restore) e poi
// riapplica le label utente (ModifyLabels). L'esito è un opDone con refresh profondo.
func undoCmd(st mail.MailStore, u bulkUndo) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		var err error
		label := ""
		switch u.kind {
		case undoArchive:
			err = st.ModifyLabels(ctx, u.archived, []string{`\Inbox`}, nil)
			label = fmt.Sprintf("annullato: %d in Posta in arrivo", u.n)
		case undoTrash:
			for _, g := range u.groups {
				var back []msg.MessageID
				back, err = st.Restore(ctx, g.ids, g.dest)
				if err == nil && len(g.labels) > 0 {
					err = st.ModifyLabels(ctx, back, g.labels, nil)
				}
				if err != nil {
					break
				}
			}
			label = fmt.Sprintf("annullato: %d ripristinate", u.n)
		}
		return opDone{label: label, err: err, refresh: true, deep: true}
	}
}

// armBulkUndo rende u l'ultima azione annullabile dopo l'esito riuscito di archivia
// o cestino: il toast d'esito offre `u` per undoBulkWindow (D10-U1). Un invio in
// attesa resta inviato allo scadere: il suo `u` è sostituito (D10-U2).
func (m *Model) armBulkUndo(u *bulkUndo, label string) tea.Cmd {
	m.undoBulkSeq++
	u.seq = m.undoBulkSeq
	m.lastUndo = u
	if m.undo != nil {
		// Piano 10, review punto 1 (D10-U2): il toast dell'invio non offre più `u`: l'invio parte
		// comunque allo scadere, il suo conto alla rovescia resta visibile.
		m.undoSendOff = true
		m.toast.SetText(m.sendToast, fmt.Sprintf("invio tra %d s", m.sendSecondsLeft()))
	}
	toastCmd := m.toast.PushFor(toast.Info, label+" — u annulla", undoBulkWindow)
	return tea.Batch(toastCmd, bulkUndoTimer(u.seq))
}

// restoreBulkUndo è `u` dopo archivia o cestino: l'ultima azione torna com'era.
func (m *Model) restoreBulkUndo() tea.Cmd {
	u := m.lastUndo
	m.lastUndo = nil
	m.pending++
	return tea.Batch(undoCmd(m.store, *u), m.spinner.Tick)
}
