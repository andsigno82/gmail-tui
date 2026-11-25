package imap

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/trace"
)

// Piano 16 (traccia): con uno Span nel ctx, l'attesa del gate di un'op dello Store
// finisce nello Span, separata dal tempo di esecuzione. Senza traccia l'op non
// misura nulla.
func TestAttesaDelGateNelloSpan(t *testing.T) {
	trace.Install(trace.New(&bytes.Buffer{}, ""))
	defer trace.Uninstall()

	s := NewStore(Options{Addr: "127.0.0.1:1", Username: "u", Password: "p"})
	release := s.gate.acquire() // un'altra op tiene la connessione

	ctx, sp := trace.WithSpan(context.Background(), "ListConversations")
	done := make(chan struct{})
	go func() {
		// ListMessages acquisisce il gate prima di tutto: attende il rilascio.
		_, _ = s.ListMessages(ctx, msg.Query{}, mail.Page{Limit: 1})
		close(done)
	}()
	time.Sleep(30 * time.Millisecond)
	release()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ListMessages non e ripartita dopo il rilascio del gate")
	}
	if w := sp.Wait(); w < 20*time.Millisecond {
		t.Errorf("attesa del gate = %v, attesa >= 20ms", w)
	}
}

func TestSenzaTracciaAcquireNonMisura(t *testing.T) {
	trace.Uninstall()
	s := NewStore(Options{Addr: "127.0.0.1:1", Username: "u", Password: "p"})
	release := s.acquire(context.Background())
	release()
	ctx, sp := trace.WithSpan(context.Background(), "ListConversations")
	s.acquire(ctx)()
	if sp != nil || trace.SpanFrom(ctx) != nil {
		t.Fatal("senza traccia non deve esistere uno span")
	}
}
