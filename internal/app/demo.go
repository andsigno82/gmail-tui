// Demo seed (Piano 4, Task 5): app.DemoStore() è la factory della modalità
// demo — un Mock con 5-6 messaggi realistici (mix unread/starred, un allegato
// fittizio con Raw popolato così `:save` funziona). Usata dal wizard-skip e
// dal path demo senza password del main: chi clona il repo vede l'app viva
// (lista, lettura, ricerca, stella, invio registrato in Mock.Sent).
package app

import (
	"time"

	"gmail-tui/internal/mail"
	"gmail-tui/internal/msg"
)

// DemoStore costruisce il Mock della demo con il seed realistico. Le date sono
// relative a "ora" (il relativo della lista le rende naturali) e decrescenti:
// il Mock ordina per data desc alla ListMessages.
func DemoStore() *mail.Mock {
	now := time.Now()
	mk := &mail.Mock{Raw: map[msg.MessageID]map[int][]byte{}}
	mk.Msgs = []*msg.Message{
		{
			Ref: msg.MessageRef{
				ID:      msg.NewID("INBOX", 1),
				From:    "team@gmail-tui.dev",
				Subject: "Benvenuto in gmail-tui",
				Date:    now.Add(-5 * time.Minute),
				Unread:  true,
			},
			To:       []string{"tu@example.com"},
			BodyText: "Ciao!\n\nQuesta è la **modalità demo**: i messaggi sono finti e vivono in memoria.\n\nProva a:\n\n- premere `j`/`k` per muoverti nella lista\n- aprire un messaggio con `invio`\n- cercare con `/`\n- scrivere con `c` (l'invio viene registrato, non spedito)\n\nBuon divertimento!\n\n— il team gmail-tui",
			BodyHTML: "<p>Ciao! Questa è la <b>modalità demo</b>: i messaggi sono finti e vivono in memoria.</p><ul><li>premere j/k per muoverti</li><li>aprire un messaggio con invio</li></ul><p>Buon divertimento! — il team gmail-tui</p>",
			HasHTML:  true,
		},
		{
			Ref: msg.MessageRef{
				ID:      msg.NewID("INBOX", 2),
				From:    "anna@example.dev",
				Subject: "Re: design review",
				Date:    now.Add(-2 * time.Hour),
				Starred: true,
			},
			To:       []string{"tu@example.com"},
			BodyText: "Ottimo lavoro sulla vista thread!\n\nUn solo appunto: il padding dell'header è di 2 colonne ma il mockup ne chiede 3.\n\nIl 21 nov 2026, tu@example.com ha scritto:\n> Ho caricato la prima bozza della vista thread, dateci un'occhiata.",
		},
		{
			Ref: msg.MessageRef{
				ID:      msg.NewID("INBOX", 3),
				From:    "mario@budgeting.io",
				Subject: "Fwd: report spese Q4",
				Date:    now.Add(-26 * time.Hour),
				Unread:  true,
				Starred: true,
			},
			To:       []string{"tu@example.com"},
			BodyText: "Ti inoltro il report delle spese del trimestre, i numeri sono nel PDF allegato.\n\nIn sintesi: -12% rispetto al Q3, soprattutto grazie alla migrazione della infrastruttura.\n\nMario",
			Attachments: []msg.Attachment{
				{PartIndex: 1, Filename: "report-spese-q4.pdf", ContentType: "application/pdf", Size: 153600},
			},
		},
		{
			Ref: msg.MessageRef{
				ID:      msg.NewID("INBOX", 4),
				From:    "newsletter@golangweekly.dev",
				Subject: "Go Weekly: novità dalla community",
				Date:    now.Add(-3 * 24 * time.Hour),
			},
			To:       []string{"tu@example.com"},
			BodyText: "In questo numero:\n\n- il nuovo client IMAP in puro Go\n- pattern per le tea.Cmd nei progetti Bubble Tea\n- unprofilatore di goroutine via browser\n\nLeggi il numero completo online.",
		},
		{
			Ref: msg.MessageRef{
				ID:      msg.NewID("INBOX", 5),
				From:    "hr@example.dev",
				Subject: "Promemoria: formazione sicurezza",
				Date:    now.Add(-4 * 24 * time.Hour),
				Unread:  true,
			},
			To:       []string{"tu@example.com"},
			BodyText: "Gentile collega,\n\nricordiamo che la formazione sulla sicurezza va completata entro fine mese.\n\nIl corso dura 40 minuti ed è disponibile sulla intranet.\n\nHR",
		},
		{
			Ref: msg.MessageRef{
				ID:      msg.NewID("INBOX", 6),
				From:    "luca@example.dev",
				Subject: "Re: pranzo venerdì?",
				Date:    now.Add(-5 * 24 * time.Hour),
			},
			To:       []string{"tu@example.com"},
			BodyText: "Per me venerdì va bene, confermo il posto delle scorse volte.\n\nIl 17 nov 2026, tu@example.com ha scritto:\n> Ci vediamo venerdì per il solito pranzo?",
		},
	}
	// Allegato fittizio con contenuto: `:save 1` sul messaggio 3 scrive un
	// PDF (intestazione minima, non un file vero) — sufficiente alla demo.
	mk.Raw[msg.NewID("INBOX", 3)] = map[int][]byte{
		1: []byte("%PDF-1.4\n% gmail-tui: allegato fittizio della modalità demo\n1 0 obj\n<< /Type /Catalog >>\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%EOF\n"),
	}
	return mk
}
