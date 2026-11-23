package app

// Benchmark di prestazioni dell'app (indagine "app molto piu lenta dopo il Piano
// 15"): dati realistici (DemoStore + ~200 conversazioni sintetiche + una newsletter
// HTML da ~150 KB), costo di Update+View per tasto, apertura messaggio, finestre e
// toast, e conteggio delle chiamate allo store. Il file usa solo API presenti sia
// al commit 82efdd8 sia dopo il Piano 15, cosi gira identico su entrambe.

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/config"
	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/theme"
	"github.com/andsigno82/gmail-tui/internal/ui/toast"
)

// --- dati sintetici ---------------------------------------------------------

// heavyNewsletterHTML costruisce una newsletter da ~size byte: tabelle annidate
// su 4 livelli, molti link, immagini, bottoni e paragrafi con stili inline.
func heavyNewsletterHTML(size int) string {
	var b strings.Builder
	b.WriteString(`<html><head><style>body{margin:0}</style></head><body><table width="600" cellpadding="0" cellspacing="0"><tr><td>`)
	i := 0
	for b.Len() < size {
		i++
		fmt.Fprintf(&b, `<table width="100%%" cellpadding="0" cellspacing="0"><tr><td style="padding:12px"><table><tr><td><table><tr><td>`+
			`<h2 style="font-size:20px;color:#333">Titolo della sezione numero %d dalla newsletter</h2>`+
			`<p style="font-size:14px;line-height:1.5">Lorem ipsum dolor sit amet, <a href="https://example.com/a/%d?utm_source=news&utm_medium=email&utm_campaign=c%d">consectetur adipiscing</a> elit, sed do `+
			`<b>eiusmod tempor</b> incididunt ut labore et <i>dolore magna</i> aliqua. Ut enim ad minim veniam, quis nostrud <a href="https://example.com/b/%d">exercitation ullamco</a> `+
			`laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur.</p>`+
			`<img src="https://img.example.com/%d.jpg" alt="immagine %d" width="560" height="200">`+
			`<ul><li><a href="https://example.com/l1/%d">primo link</a></li><li><a href="https://example.com/l2/%d">secondo link</a></li><li>terzo punto senza link</li></ul>`+
			`<table><tr><td style="background:#0a66c2;padding:10px"><a href="https://example.com/cta/%d" style="color:#fff">Leggi di piu</a></td></tr></table>`+
			`</td></tr></table></td></tr></table></td></tr></table>`, i, i, i, i, i, i, i, i, i)
	}
	b.WriteString(`</td></tr></table></body></html>`)
	return b.String()
}

// perfStore e il Mock della demo con ~200 conversazioni sintetiche (un quinto sono
// thread di 2-4 messaggi) e la newsletter pesante in cima alla lista.
func perfStore() *mail.Mock {
	mk := DemoStore()
	now := time.Now()
	html := heavyNewsletterHTML(150 << 10)
	mk.Msgs = append(mk.Msgs, &msg.Message{
		Ref: msg.MessageRef{
			ID: msg.NewID("INBOX", 9000), From: "news@bigletter.example", Subject: "La newsletter pesante",
			Date: now.Add(-time.Second), Unread: true, ThreadID: 9000, GmailID: 9000,
		},
		To:       []string{"tu@example.com"},
		BodyText: strings.Repeat("Testo alternativo della newsletter con https://example.com/x link.\n", 100),
		BodyHTML: html, HasHTML: true,
	})
	// Newsletter media (~30 KB, sotto il limite del rendering sincrono) in seconda riga.
	mk.Msgs = append(mk.Msgs, &msg.Message{
		Ref: msg.MessageRef{
			ID: msg.NewID("INBOX", 9001), From: "promo@medium.example", Subject: "La promo media",
			Date: now.Add(-2 * time.Second), Unread: true, ThreadID: 9001, GmailID: 9001,
		},
		To:       []string{"tu@example.com"},
		BodyText: "Versione testo della promo.",
		BodyHTML: heavyNewsletterHTML(30 << 10), HasHTML: true,
	})
	uid := uint32(100)
	for i := 0; i < 200; i++ {
		members := 1
		if i%5 == 0 {
			members = 2 + i%3
		}
		for k := 0; k < members; k++ {
			uid++
			mk.Msgs = append(mk.Msgs, &msg.Message{
				Ref: msg.MessageRef{
					ID: msg.NewID("INBOX", uid), From: fmt.Sprintf("utente%d@example.dev", i%37),
					Subject: fmt.Sprintf("Oggetto della conversazione %d con un titolo abbastanza lungo", i),
					Date:    now.Add(-time.Duration(i+1)*17*time.Minute - time.Duration(k)*time.Minute),
					Unread:  i%3 == 0, Starred: i%11 == 0, ThreadID: uint64(1000 + i), GmailID: uint64(1000+i)*10 + uint64(k),
				},
				To:       []string{"tu@example.com"},
				BodyText: fmt.Sprintf("Corpo del messaggio %d/%d.\n\n%s\n> riga citata\n", i, k, strings.Repeat("riga di testo del messaggio. ", 12)),
			})
		}
	}
	return mk
}

// --- store contatore --------------------------------------------------------

type perfCountStore struct {
	mail.MailStore
	mu sync.Mutex
	n  map[string]int
}

func newPerfCountStore(st mail.MailStore) *perfCountStore {
	return &perfCountStore{MailStore: st, n: map[string]int{}}
}

func (c *perfCountStore) hit(k string) { c.mu.Lock(); c.n[k]++; c.mu.Unlock() }
func (c *perfCountStore) reset()       { c.mu.Lock(); c.n = map[string]int{}; c.mu.Unlock() }
func (c *perfCountStore) snapshot() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	keys := []string{"Connect", "ListConversations", "ThreadMessages", "FetchMessage", "UnreadCounts", "ListMailboxes", "ConversationMembers", "Search", "Watch", "MarkRead"}
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, c.n[k]))
	}
	return strings.Join(parts, " ")
}

func (c *perfCountStore) Connect(ctx context.Context) error {
	c.hit("Connect")
	return c.MailStore.Connect(ctx)
}
func (c *perfCountStore) ListConversations(ctx context.Context, q msg.Query, pg mail.Page) ([]mail.Conversation, error) {
	c.hit("ListConversations")
	return c.MailStore.ListConversations(ctx, q, pg)
}
func (c *perfCountStore) ThreadMessages(ctx context.Context, id uint64) ([]msg.MessageRef, error) {
	c.hit("ThreadMessages")
	return c.MailStore.ThreadMessages(ctx, id)
}
func (c *perfCountStore) FetchMessage(ctx context.Context, id msg.MessageID) (*msg.Message, error) {
	c.hit("FetchMessage")
	return c.MailStore.FetchMessage(ctx, id)
}
func (c *perfCountStore) ListMailboxes(ctx context.Context) ([]mail.Mailbox, error) {
	c.hit("ListMailboxes")
	return c.MailStore.ListMailboxes(ctx)
}
func (c *perfCountStore) ConversationMembers(ctx context.Context, box string, id uint64) ([]msg.MessageID, error) {
	c.hit("ConversationMembers")
	return c.MailStore.ConversationMembers(ctx, box, id)
}
func (c *perfCountStore) Search(ctx context.Context, q msg.Query) ([]msg.MessageRef, error) {
	c.hit("Search")
	return c.MailStore.Search(ctx, q)
}
func (c *perfCountStore) Watch(ctx context.Context) (<-chan mail.Change, error) {
	c.hit("Watch")
	return c.MailStore.Watch(ctx)
}
func (c *perfCountStore) MarkRead(ctx context.Context, ids []msg.MessageID, read bool) error {
	c.hit("MarkRead")
	return c.MailStore.MarkRead(ctx, ids, read)
}

// --- pilota -----------------------------------------------------------------

// meter accumula tempo e allocazioni del lavoro sul loop UI (Update + View) e il
// tempo delle Cmd (fuori dal loop). Le Cmd bloccanti (feed Watch, timer) si
// scartano dopo cmdWait.
type meter struct {
	uiNs, bgNs      int64
	uiAllocs, uiByt uint64
	skipPreview     bool // drain non consegna previewDue (lo fa il benchmark)
}

const cmdWait = 400 * time.Millisecond

func (mt *meter) ui(f func()) {
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	t := time.Now()
	f()
	d := time.Since(t)
	runtime.ReadMemStats(&b)
	mt.uiNs += int64(d)
	mt.uiAllocs += b.Mallocs - a.Mallocs
	mt.uiByt += b.TotalAlloc - a.TotalAlloc
}

func (mt *meter) report(b *testing.B) {
	n := float64(b.N)
	b.ReportMetric(float64(mt.uiNs)/n, "ui-ns/op")
	b.ReportMetric(float64(mt.uiAllocs)/n, "ui-allocs/op")
	b.ReportMetric(float64(mt.uiByt)/n, "ui-B/op")
	b.ReportMetric(float64(mt.bgNs)/n, "bg-ns/op")
}

// stepM e step misurato.
func (mt *meter) stepM(m Model, ev tea.Msg) (Model, tea.Cmd) {
	var c tea.Cmd
	mt.ui(func() { m, c = step(m, ev) })
	return m, c
}

// viewM e View misurata.
func (mt *meter) viewM(m Model) string {
	var s string
	mt.ui(func() { s = m.View().Content })
	return s
}

type cmdRes struct {
	m  tea.Msg
	ns int64
}

// collect esegue la cmd (batch in parallelo) e ritorna i msg prodotti entro
// cmdWait; accumula il tempo delle Cmd completate.
func (mt *meter) collect(c tea.Cmd) []tea.Msg {
	if c == nil {
		return nil
	}
	ch := make(chan cmdRes, 256)
	started := 0
	var launch func(c tea.Cmd)
	launch = func(c tea.Cmd) {
		if c == nil {
			return
		}
		started++
		go func() {
			t := time.Now()
			r := c()
			ch <- cmdRes{r, int64(time.Since(t))}
		}()
	}
	launch(c)
	var out []tea.Msg
	deadline := time.After(cmdWait)
	for done := 0; done < started; {
		select {
		case r := <-ch:
			done++
			if b, ok := r.m.(tea.BatchMsg); ok {
				for _, sub := range b {
					launch(sub)
				}
				continue
			}
			mt.bgNs += r.ns
			out = append(out, r.m)
		case <-deadline:
			return out
		}
	}
	return out
}

// skippable dice se il msg e rumore del runtime (tick, scadenze).
func skippable(m tea.Msg) bool {
	switch m.(type) {
	case nil, spinner.TickMsg, toast.ExpiredMsg:
		return true
	}
	return false
}

// drain consegna ricorsivamente i risultati delle Cmd (profondita max depth).
func (mt *meter) drain(m Model, c tea.Cmd, depth int) Model {
	if depth == 0 {
		return m
	}
	for _, ms := range mt.collect(c) {
		if skippable(ms) {
			continue
		}
		if _, ok := ms.(previewDue); ok && mt.skipPreview {
			continue
		}
		var next tea.Cmd
		m, next = mt.stepM(m, ms)
		m = mt.drain(m, next, depth-1)
	}
	return m
}

func perfTheme(tb testing.TB) theme.Theme {
	tb.Helper()
	th, err := theme.Load("default")
	if err != nil {
		tb.Fatalf("tema: %v", err)
	}
	return th
}

// perfModel avvia una sessione completa (Init, connect, load della lista) a w*h.
func perfModel(tb testing.TB, st mail.MailStore, w, h int, layout string) (Model, *meter) {
	tb.Helper()
	m := New(st, noopSender{}, perfTheme(tb))
	m.SetPrefs(layout, config.PreviewMarkReadNever)
	mt := &meter{skipPreview: true}
	m, _ = mt.stepM(m, tea.WindowSizeMsg{Width: w, Height: h})
	m = mt.drain(m, m.Init(), 5)
	mt.skipPreview = false
	if len(m.refs) == 0 {
		tb.Fatalf("lista vuota dopo l'avvio")
	}
	return m, mt
}

// perfModelReady e perfModel con l'anteprima gia caricata (solo layout a due colonne).
func perfModelReady(tb testing.TB, st mail.MailStore, w, h int, layout string) (Model, *meter) {
	tb.Helper()
	m, mt := perfModel(tb, st, w, h, layout)
	if layout == config.LayoutSplit {
		var c tea.Cmd
		m, c = mt.stepM(m, previewDue{tok: m.prevTok})
		m = mt.drain(m, c, 5)
		if !m.previewReady() {
			tb.Fatalf("anteprima non pronta")
		}
	}
	return m, mt
}

type dims struct{ w, h int }

var perfDims = []dims{{120, 35}, {200, 50}}

func (d dims) name() string { return fmt.Sprintf("%dx%d", d.w, d.h) }

// --- benchmark: tasti in lista (stato stazionario) ---------------------------

func benchListKey(b *testing.B, layout string, withView bool) {
	for _, d := range perfDims {
		b.Run(d.name(), func(b *testing.B) {
			m, _ := perfModelReady(b, perfStore(), d.w, d.h, layout)
			j := pressKey("j")
			k := pressKey("k")
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				key := j
				if i%2 == 1 {
					key = k
				}
				m2, _ := step(m, key)
				m = m2
				if withView {
					_ = m.View()
				}
			}
		})
	}
}

func BenchmarkListKeyUpdateView_Single(b *testing.B) { benchListKey(b, config.LayoutSingle, true) }
func BenchmarkListKeyUpdateView_Split(b *testing.B)  { benchListKey(b, config.LayoutSplit, true) }
func BenchmarkListKeyUpdateOnly_Single(b *testing.B) { benchListKey(b, config.LayoutSingle, false) }

func benchViewOnly(b *testing.B, layout string) {
	for _, d := range perfDims {
		b.Run(d.name(), func(b *testing.B) {
			m, _ := perfModelReady(b, perfStore(), d.w, d.h, layout)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = m.View()
			}
		})
	}
}

func BenchmarkViewOnlyList_Single(b *testing.B) { benchViewOnly(b, config.LayoutSingle) }
func BenchmarkViewOnlyList_Split(b *testing.B)  { benchViewOnly(b, config.LayoutSplit) }

// --- benchmark: apertura messaggio (fino al render completo) -----------------

// BenchmarkOpenHeavy_Single: da lista a thread a schermo intero sulla newsletter
// pesante (riga 0): invio, fetch, render fino a View finale. Modello nuovo (cache
// fredda) a ogni iterazione.
func BenchmarkOpenHeavy_Single(b *testing.B) {
	for _, d := range perfDims {
		b.Run(d.name(), func(b *testing.B) {
			var mt meter
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				m, _ := perfModel(b, perfStore(), d.w, d.h, config.LayoutSingle)
				b.StartTimer()
				m, c := mt.stepM(m, pressKey("enter"))
				m = mt.drain(m, c, 5)
				s := mt.viewM(m)
				if m.currentView() != viewThread || !strings.Contains(s, "newsletter") {
					b.Fatalf("thread non aperto:\n%s", s)
				}
			}
			mt.report(b)
		})
	}
}

// BenchmarkOpenMedium_Single: come OpenHeavy ma sulla newsletter da ~30 KB (seconda
// riga: sotto il limite di rendering sincrono dopo il Piano 15).
func BenchmarkOpenMedium_Single(b *testing.B) {
	for _, d := range perfDims {
		b.Run(d.name(), func(b *testing.B) {
			var mt meter
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				m, _ := perfModel(b, perfStore(), d.w, d.h, config.LayoutSingle)
				m, _ = step(m, pressKey("j"))
				b.StartTimer()
				m, c := mt.stepM(m, pressKey("enter"))
				m = mt.drain(m, c, 5)
				s := mt.viewM(m)
				if m.currentView() != viewThread || !strings.Contains(s, "promo") {
					b.Fatalf("thread non aperto:\n%s", s)
				}
			}
			mt.report(b)
		})
	}
}

// BenchmarkReopenHeavy_Single: riapertura della stessa newsletter (cache calda
// dove esiste) dopo esc.
func BenchmarkReopenHeavy_Single(b *testing.B) {
	for _, d := range perfDims {
		b.Run(d.name(), func(b *testing.B) {
			m, mt0 := perfModel(b, perfStore(), d.w, d.h, config.LayoutSingle)
			m, c := mt0.stepM(m, pressKey("enter"))
			m = mt0.drain(m, c, 5)
			var mt meter
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				m, c = mt.stepM(m, pressKey("esc"))
				m = mt.drain(m, c, 3)
				m, c = mt.stepM(m, pressKey("enter"))
				m = mt.drain(m, c, 5)
				_ = mt.viewM(m)
			}
			mt.report(b)
		})
	}
}

// BenchmarkOpenHeavy_Split: anteprima nella seconda colonna della newsletter
// pesante (riga 0 selezionata dal load): previewDue, fetch, render, View.
func BenchmarkOpenHeavy_Split(b *testing.B) {
	for _, d := range perfDims {
		b.Run(d.name(), func(b *testing.B) {
			var mt meter
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				m, _ := perfModel(b, perfStore(), d.w, d.h, config.LayoutSplit)
				b.StartTimer()
				m, c := mt.stepM(m, previewDue{tok: m.prevTok})
				m = mt.drain(m, c, 5)
				s := mt.viewM(m)
				if !m.previewReady() {
					b.Fatalf("anteprima non pronta:\n%s", s)
				}
			}
			mt.report(b)
		})
	}
}

// --- benchmark: thread a schermo intero, tasti e View ------------------------

func openedHeavyThread(tb testing.TB, d dims) Model {
	tb.Helper()
	m, mt := perfModel(tb, perfStore(), d.w, d.h, config.LayoutSingle)
	m, c := mt.stepM(m, pressKey("enter"))
	m = mt.drain(m, c, 5)
	if m.currentView() != viewThread {
		tb.Fatalf("thread non aperto")
	}
	return m
}

func BenchmarkThreadKeyUpdateView(b *testing.B) {
	for _, d := range perfDims {
		b.Run(d.name(), func(b *testing.B) {
			m := openedHeavyThread(b, d)
			down := pressKey("j")
			up := pressKey("k")
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				key := down
				if i%2 == 1 {
					key = up
				}
				m2, _ := step(m, key)
				m = m2
				_ = m.View()
			}
		})
	}
}

func BenchmarkThreadViewOnly(b *testing.B) {
	for _, d := range perfDims {
		b.Run(d.name(), func(b *testing.B) {
			m := openedHeavyThread(b, d)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = m.View()
			}
		})
	}
}

func BenchmarkSplitPreviewViewOnly(b *testing.B) {
	for _, d := range perfDims {
		b.Run(d.name(), func(b *testing.B) {
			m, mt := perfModel(b, perfStore(), d.w, d.h, config.LayoutSplit)
			m, c := mt.stepM(m, previewDue{tok: m.prevTok})
			m = mt.drain(m, c, 5)
			if !m.previewReady() {
				b.Fatalf("anteprima non pronta")
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = m.View()
			}
		})
	}
}

// --- benchmark: finestre e toast ----------------------------------------------

// BenchmarkFolderWindowKey: selettore `m` aperto e caricato, tasto giu/su + View.
func BenchmarkFolderWindowKey(b *testing.B) {
	for _, layout := range []string{config.LayoutSingle, config.LayoutSplit} {
		for _, d := range perfDims {
			b.Run(layout+"/"+d.name(), func(b *testing.B) {
				m, mt := perfModelReady(b, perfStore(), d.w, d.h, layout)
				m, c := mt.stepM(m, pressKey("m"))
				m = mt.drain(m, c, 3)
				if !m.folderOpen {
					b.Fatal("selettore non aperto")
				}
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					key := tea.KeyPressMsg{Code: tea.KeyDown}
					if i%2 == 1 {
						key = tea.KeyPressMsg{Code: tea.KeyUp}
					}
					m2, _ := step(m, key)
					m = m2
					_ = m.View()
				}
			})
		}
	}
}

// BenchmarkOpenFolders: apertura di `m` fino alla finestra completa (Update, fetch
// cartelle e contatori, View).
func BenchmarkOpenFolders(b *testing.B) {
	for _, d := range perfDims {
		b.Run(d.name(), func(b *testing.B) {
			var mt meter
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				m, _ := perfModel(b, perfStore(), d.w, d.h, config.LayoutSingle)
				b.StartTimer()
				m, c := mt.stepM(m, pressKey("m"))
				mt.ui(func() { _ = m.View() })
				m = mt.drain(m, c, 3)
				_ = mt.viewM(m)
			}
			mt.report(b)
		})
	}
}

// BenchmarkToastView: lista con un toast attivo, tasto + View.
func BenchmarkToastView(b *testing.B) {
	for _, layout := range []string{config.LayoutSingle, config.LayoutSplit} {
		for _, d := range perfDims {
			b.Run(layout+"/"+d.name(), func(b *testing.B) {
				m, _ := perfModelReady(b, perfStore(), d.w, d.h, layout)
				_ = m.toast.Push(toast.Info, "nuova posta in arrivo")
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					key := pressKey("j")
					if i%2 == 1 {
						key = pressKey("k")
					}
					m2, _ := step(m, key)
					m = m2
					_ = m.View()
				}
			})
		}
	}
}

// --- conteggio chiamate allo store -------------------------------------------

// TestPerfStoreCalls stampa (GMAIL_TUI_PERF=1 go test -run PerfStoreCalls -v) le
// chiamate allo store per scenario. Non asserisce nulla: e uno strumento di misura,
// spento di default (le Cmd bloccanti si scartano a tempo, ~secondi).
func TestPerfStoreCalls(t *testing.T) {
	if os.Getenv("GMAIL_TUI_PERF") == "" {
		t.Skip("strumento di misura: GMAIL_TUI_PERF=1 per eseguirlo")
	}
	for _, layout := range []string{config.LayoutSingle, config.LayoutSplit} {
		cs := newPerfCountStore(perfStore())
		var mt meter
		m := New(cs, noopSender{}, perfTheme(t))
		m.SetPrefs(layout, config.PreviewMarkReadNever)
		m, _ = mt.stepM(m, tea.WindowSizeMsg{Width: 200, Height: 50})
		m = mt.drain(m, m.Init(), 5)
		t.Logf("[%s] avvio -> prima lista (refs=%d): %s", layout, len(m.refs), cs.snapshot())
		cs.reset()
		for i := 0; i < 5; i++ {
			_ = m.View()
		}
		t.Logf("[%s] 5 View stazionarie: %s", layout, cs.snapshot())

		cs.reset()
		var c tea.Cmd
		if layout == config.LayoutSingle {
			m, c = mt.stepM(m, pressKey("enter"))
			m = mt.drain(m, c, 5)
			t.Logf("[%s] apertura messaggio: %s", layout, cs.snapshot())
			cs.reset()
			m, c = mt.stepM(m, pressKey("esc"))
			m = mt.drain(m, c, 5)
			t.Logf("[%s] ritorno alla lista: %s", layout, cs.snapshot())
		} else {
			m, c = mt.stepM(m, previewDue{tok: m.prevTok})
			m = mt.drain(m, c, 5)
			t.Logf("[%s] anteprima del primo messaggio: %s", layout, cs.snapshot())
			cs.reset()
			for i := 0; i < 10; i++ {
				m, c = mt.stepM(m, pressKey("j"))
				m = mt.drain(m, c, 1)
				m, c = mt.stepM(m, previewDue{tok: m.prevTok})
				m = mt.drain(m, c, 5)
			}
			t.Logf("[%s] 10 tasti j (anteprima completa a ogni passo): %s", layout, cs.snapshot())
		}
		cs.reset()
		m, c = mt.stepM(m, pressKey("m"))
		m = mt.drain(m, c, 3)
		t.Logf("[%s] apertura m: %s", layout, cs.snapshot())
		cs.reset()
		m, c = mt.stepM(m, pressKey("esc"))
		m = mt.drain(m, c, 3)
		t.Logf("[%s] chiusura m: %s", layout, cs.snapshot())
	}
}
