package thread

import (
	"testing"
	"time"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Indagine prestazioni (dopo il Piano 15): aprire un messaggio non deve rendere il
// corpo due volte (prima alla larghezza di default, poi alla larghezza vera dopo
// SetSize). NewLazyCachedSized parte gia dalla misura finale.

func sizedFixture() (msg.MessageRef, []msg.MessageRef, map[msg.MessageID]*msg.Message) {
	ref := msg.MessageRef{ID: msg.NewID("INBOX", 1), From: "a@x.dev", Subject: "s", Date: time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)}
	full := &msg.Message{Ref: ref, To: []string{"b@x.dev"}, BodyHTML: "<p>" + perfLorem + "</p>", BodyText: "testo"}
	return ref, []msg.MessageRef{ref}, map[msg.MessageID]*msg.Message{ref.ID: full}
}

const perfLorem = "Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua."

func TestNewLazyCachedSizedRendeUnaVoltaAllaMisuraFinale(t *testing.T) {
	ref, refs, bodies := sizedFixture()
	rc := NewRenderCache()
	m := NewLazyCachedSized(ref, refs, bodies, testStyles(t), rc, 120, 30)
	if len(rc.m) != 1 {
		t.Fatalf("voci in cache = %d, voglio 1 (nessun rendering alla larghezza di default)", len(rc.m))
	}
	for k := range rc.m {
		if k.width != 120 {
			t.Errorf("larghezza della voce = %d, voglio 120", k.width)
		}
	}
	if w, h := m.Size(); w != 120 || h != 30 {
		t.Errorf("Size = %dx%d, voglio 120x30", w, h)
	}
}

// La vista e identica a quella di NewLazyCached seguita da SetSize.
func TestNewLazyCachedSizedEquivaleASetSize(t *testing.T) {
	ref, refs, bodies := sizedFixture()
	st := testStyles(t)
	ref2 := NewLazyCached(ref, refs, bodies, st, NewRenderCache())
	ref2.SetSize(100, 25)
	got := NewLazyCachedSized(ref, refs, bodies, st, NewRenderCache(), 100, 25)
	if got.View() != ref2.View() {
		t.Errorf("vista diversa:\n--- sized\n%s\n--- SetSize\n%s", got.View(), ref2.View())
	}
}

// Senza misura nota (0x0) si comporta come NewLazyCached.
func TestNewLazyCachedSizedSenzaMisura(t *testing.T) {
	ref, refs, bodies := sizedFixture()
	st := testStyles(t)
	got := NewLazyCachedSized(ref, refs, bodies, st, NewRenderCache(), 0, 0)
	want := NewLazyCached(ref, refs, bodies, st, NewRenderCache())
	if got.View() != want.View() {
		t.Errorf("vista diversa senza misura")
	}
}
