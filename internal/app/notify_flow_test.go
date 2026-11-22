package app

// Piano 11, Task 4: flusso delle notifiche nel Model. Il comando è iniettato
// (notifyRun): nessun processo reale, il runner registra argv ed env.

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/theme"
)

// notifyRec registra le esecuzioni del comando di notifica.
type notifyRec struct {
	argvs [][]string
	envs  [][]string
	err   error
}

func (r *notifyRec) run(_ context.Context, argv, env []string) error {
	r.argvs = append(r.argvs, append([]string(nil), argv...))
	r.envs = append(r.envs, append([]string(nil), env...))
	return r.err
}

// notifyLoaded è il modello consolidato (primo load INBOX riuscito, quindi armato)
// con il comando configurato e il runner registrato.
func notifyLoaded(tb testing.TB, argv []string, rec *notifyRec) Model {
	tb.Helper()
	m := opsModel(tb, &mail.Mock{})
	m.SetNotify(argv)
	m.notifyRun = rec.run
	return m
}

// bareNotify è un modello senza alcun load: il primo caricamento non è avvenuto.
func bareNotify(tb testing.TB, argv []string, rec *notifyRec) Model {
	tb.Helper()
	th, err := theme.Load("default")
	if err != nil {
		tb.Fatalf("tema default: %v", err)
	}
	m := New(&mail.Mock{}, noopSender{}, th)
	m, _ = step(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m.SetNotify(argv)
	m.notifyRun = rec.run
	return m
}

// newMailChange costruisce un ChangeNewMail con n ID e le intestazioni date
// (Refs dal più recente, come il Watch).
func newMailChange(n int, refs ...msg.MessageRef) mail.Change {
	c := mail.Change{Kind: mail.ChangeNewMail, Refs: refs, Unread: n}
	for i := 0; i < n; i++ {
		c.IDs = append(c.IDs, msg.NewID("INBOX", uint32(100+i)))
	}
	return c
}

// feedChange invia un evento del feed e fa girare il cmd con il ctx annullato (il
// rearm di waitForChange non blocca sul canale nil), riapplicando i notifyResult.
func feedChange(tb testing.TB, m Model, c mail.Change) Model {
	tb.Helper()
	mm, cmd := step(m, changeEvt{c: c})
	for _, ms := range drainCmd(tb, mm, cmd) {
		if r, ok := ms.(notifyResult); ok {
			mm, _ = step(mm, r)
		}
	}
	return mm
}

// Prima del primo load riuscito gli eventi non notificano (D11-8).
func TestNotifyWaitsForFirstLoad(t *testing.T) {
	var rec notifyRec
	m := bareNotify(t, []string{"notify-cmd"}, &rec)
	feedChange(t, m, newMailChange(1, msg.MessageRef{From: "Ada", Subject: "Ciao"}))
	if len(rec.argvs) != 0 {
		t.Fatalf("prima del primo load: %d esecuzioni, voglio 0", len(rec.argvs))
	}
}

// Un evento con due messaggi = una sola esecuzione; COUNT=2; FROM/SUBJECT del
// più recente; argv invariato (nessun dato del messaggio in argv).
func TestNotifyRunsOnceWithCountAndNewestSender(t *testing.T) {
	var rec notifyRec
	argv := []string{"notify-cmd", "--flag"}
	m := notifyLoaded(t, argv, &rec)
	feedChange(t, m, newMailChange(2,
		msg.MessageRef{From: "Ada <ada@example.org>", Subject: "Nuovo"},
		msg.MessageRef{From: "Bob <bob@example.org>", Subject: "Vecchio"},
	))
	if len(rec.argvs) != 1 {
		t.Fatalf("esecuzioni = %d, voglio 1 per evento", len(rec.argvs))
	}
	if !slices.Equal(rec.argvs[0], argv) {
		t.Fatalf("argv = %q, voglio %q", rec.argvs[0], argv)
	}
	want := map[string]string{
		"GMAIL_TUI_COUNT":   "2",
		"GMAIL_TUI_FROM":    "Ada <ada@example.org>",
		"GMAIL_TUI_SUBJECT": "Nuovo",
		"GMAIL_TUI_FOLDER":  "INBOX",
	}
	for k, v := range want {
		if got, _ := envValue(rec.envs[0], k); got != v {
			t.Errorf("%s = %q, voglio %q", k, got, v)
		}
	}
}

// Piano 11, review punto 3: nessun non letto tra i nuovi → nessuna notifica.
func TestNotifySkippedWhenNoUnread(t *testing.T) {
	var rec notifyRec
	m := notifyLoaded(t, []string{"notify-cmd"}, &rec)
	c := newMailChange(1, msg.MessageRef{From: "Ada", Subject: "Letta"})
	c.Unread = 0
	feedChange(t, m, c)
	if len(rec.argvs) != 0 {
		t.Fatalf("nessun non letto: %d esecuzioni, voglio 0", len(rec.argvs))
	}
}

// Piano 11, review punto 6: solo il load della INBOX arma le notifiche; una
// cartella diversa (es. Lavoro) non arma.
func TestNotifyArmedOnlyByInboxLoad(t *testing.T) {
	var rec notifyRec
	m := bareNotify(t, []string{"notify-cmd"}, &rec)
	m.folder = mail.Mailbox{Name: "Lavoro"}
	m, _ = step(m, msgsLoaded{refs: opsRefs(t), mailbox: "Lavoro"})
	feedChange(t, m, newMailChange(1, msg.MessageRef{From: "Ada", Subject: "Ciao"}))
	if len(rec.argvs) != 0 {
		t.Fatalf("load di una cartella diversa da INBOX ha armato le notifiche: %d esecuzioni", len(rec.argvs))
	}
}

// Piano 11, review punto 7: con un processo in corso gli eventi successivi si
// accumulano in un unico riepilogo, lanciato quando il precedente termina.
func TestNotifyBusyAccumulatesIntoOneSummary(t *testing.T) {
	var rec notifyRec
	m := notifyLoaded(t, []string{"notify-cmd"}, &rec)
	m, first := m.notifyFor(newMailChange(1, msg.MessageRef{From: "Ada", Subject: "Uno"}))
	if first == nil {
		t.Fatal("primo evento: atteso un comando")
	}
	m, second := m.notifyFor(newMailChange(2,
		msg.MessageRef{From: "Bob", Subject: "Due"},
		msg.MessageRef{From: "Carla", Subject: "Tre"},
	))
	if second != nil {
		t.Fatal("con un processo in corso non deve partirne un altro")
	}
	m, flush := step(m, first())
	for _, ms := range runCmd(flush) {
		m, _ = step(m, ms)
	}
	if len(rec.argvs) != 2 {
		t.Fatalf("esecuzioni = %d, voglio 2 (il primo e il riepilogo)", len(rec.argvs))
	}
	if v, _ := envValue(rec.envs[1], "GMAIL_TUI_COUNT"); v != "2" {
		t.Errorf("riepilogo COUNT = %q, voglio 2 (i messaggi accumulati)", v)
	}
	if v, _ := envValue(rec.envs[1], "GMAIL_TUI_FROM"); v != "Bob" {
		t.Errorf("riepilogo FROM = %q, voglio il mittente più recente \"Bob\"", v)
	}
}

// Flag e disconnessione non eseguono il comando (solo la posta nuova).
func TestNotifySkippedForFlagsAndDisconnected(t *testing.T) {
	var rec notifyRec
	m := notifyLoaded(t, []string{"notify-cmd"}, &rec)
	feedChange(t, m, mail.Change{Kind: mail.ChangeFlags, IDs: []msg.MessageID{msg.NewID("INBOX", 1)}})
	feedChange(t, m, mail.Change{Kind: mail.ChangeDisconnected})
	if len(rec.argvs) != 0 {
		t.Fatalf("flag/disconnessione: %d esecuzioni, voglio 0", len(rec.argvs))
	}
}

// Senza comando configurato non c'è esecuzione.
func TestNotifyDisabledWithoutCommand(t *testing.T) {
	var rec notifyRec
	m := notifyLoaded(t, nil, &rec)
	feedChange(t, m, newMailChange(1, msg.MessageRef{From: "Ada", Subject: "Ciao"}))
	if len(rec.argvs) != 0 {
		t.Fatalf("senza comando: %d esecuzioni, voglio 0", len(rec.argvs))
	}
}

// Errore del comando: un toast d'errore per sessione, non uno per evento.
func TestNotifyErrorToastShownOnce(t *testing.T) {
	rec := notifyRec{err: errors.New("exit status 1")}
	m := notifyLoaded(t, []string{"notify-cmd"}, &rec)
	m = feedChange(t, m, newMailChange(1, msg.MessageRef{From: "Ada", Subject: "Uno"}))
	m = feedChange(t, m, newMailChange(1, msg.MessageRef{From: "Ada", Subject: "Due"}))
	if len(rec.argvs) != 2 {
		t.Fatalf("esecuzioni = %d, voglio 2 (ogni evento esegue)", len(rec.argvs))
	}
	n := 0
	for _, txt := range m.toast.Texts() {
		if strings.Contains(txt, "notifica desktop") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("toast d'errore del comando = %d, voglio 1", n)
	}
}
