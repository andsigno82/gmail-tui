package app

import (
	"testing"
	"time"

	"github.com/andsigno82/gmail-tui/internal/ui/composer"
)

// Piano 9, Task 12-13: suggerimenti da rubrica locale e comando esterno; la
// rubrica si alimenta dai mittenti in lista, dai destinatari dei messaggi aperti
// e dagli invii riusciti.

// TestSuggestionsFromLocalBook: digitare in A propone i contatti della rubrica locale.
func TestSuggestionsFromLocalBook(t *testing.T) {
	m, _ := composerModel(t)
	mm, _ := step(m, teaSize())
	if err := mm.book.Observe(time.Now(), true, "anna@x.dev"); err != nil {
		t.Fatalf("Observe: %v", err)
	}
	mm = draftComposer(mm, composer.ComposerConfig{})
	mm, cmd := step(mm, pressKey("a"))
	for _, ms := range runCmd(cmd) {
		mm, _ = step(mm, ms)
	}
	got := mm.composer.Suggestions()
	if len(got) != 1 || got[0].Addr != "anna@x.dev" {
		t.Errorf("suggerimenti = %+v, voglio anna@x.dev dalla rubrica locale", got)
	}
}

// TestSuggestionsMergeExternalAndDedupe: locali prima, poi il comando esterno;
// stesso indirizzo (anche in maiuscolo) una volta sola.
func TestSuggestionsMergeExternalAndDedupe(t *testing.T) {
	local := []composer.Suggestion{{Addr: "anna@x.dev", Name: "Anna"}, {Addr: "bob@y.dev"}}
	ext := []composer.Suggestion{{Addr: "ANNA@x.dev", Name: "Anna Rossi"}, {Addr: "carla@z.dev", Name: "Carla"}}
	got := mergeSuggestions(local, ext)
	want := []string{"anna@x.dev", "bob@y.dev", "carla@z.dev"}
	if len(got) != len(want) {
		t.Fatalf("merge = %+v, voglio %v", got, want)
	}
	for i := range want {
		if got[i].Addr != want[i] {
			t.Errorf("posizione %d = %q, voglio %q", i, got[i].Addr, want[i])
		}
	}
	if got[0].Name != "Anna" {
		t.Errorf("il duplicato deve tenere la versione locale: nome %q", got[0].Name)
	}
}

// TestStaleSuggestionsDiscarded: un risultato per una query superata non entra nel composer.
func TestStaleSuggestionsDiscarded(t *testing.T) {
	m, _ := composerModel(t)
	mm, _ := step(m, teaSize())
	mm = draftComposer(mm, composer.ComposerConfig{})
	mm, _ = step(mm, pressKey("a"))
	mm, _ = step(mm, pressKey("n")) // la query è ora "an"
	mm, _ = step(mm, suggestionsLoaded{
		field: composer.FieldTo, query: "a",
		items: []composer.Suggestion{{Addr: "x@x.dev"}},
	})
	if got := mm.composer.Suggestions(); len(got) != 0 {
		t.Errorf("risultato stantio accettato: %+v", got)
	}
}

// TestLoadedListFeedsBook: i mittenti della lista entrano in rubrica senza incremento.
func TestLoadedListFeedsBook(t *testing.T) {
	m, _ := newTestModelMock(t)
	mm, _ := step(m, teaSize())
	mm, _ = step(mm, msgsLoaded{refs: testRefs(t)})
	got := mm.book.Suggest("x.dev", 10)
	if len(got) != 2 {
		t.Fatalf("rubrica dopo la lista = %+v, voglio i 2 mittenti", got)
	}
	for _, s := range got {
		if s.Count != 0 {
			t.Errorf("%s Count = %d dalla lista, voglio 0 (senza incremento)", s.Addr, s.Count)
		}
	}
}

// TestOpenedMessageBumpsRecipients: aprire un messaggio incrementa i suoi destinatari.
func TestOpenedMessageBumpsRecipients(t *testing.T) {
	ref := testRefs(t)[0]
	m, _ := newTestModelMock(t, fullMessage(ref))
	mm := openThread(t, m, ref)
	got := mm.book.Suggest("bob@example", 10)
	if len(got) != 1 || got[0].Count != 1 {
		t.Errorf("destinatario del messaggio aperto = %+v, voglio bob@example.com con Count 1", got)
	}
}

// TestSentMessageBumpsRecipients: un invio riuscito incrementa i destinatari (A e Cc).
func TestSentMessageBumpsRecipients(t *testing.T) {
	rec := &senderRecorder{}
	m, _ := senderModel(t, rec)
	mm, _ := step(m, teaSize())
	mm = sendRequested(t, draftComposer(mm, composer.ComposerConfig{To: "anna@x.dev", Cc: "carlo@x.dev", Subject: "s", Body: "c"}))
	mm, _ = step(mm, pressKey("s"))
	mm, done := flushUndo(t, mm)
	mm, _ = step(mm, done)
	if got := mm.book.Suggest("anna@x.dev", 5); len(got) != 1 || got[0].Count != 1 {
		t.Errorf("destinatario inviato = %+v, voglio anna@x.dev con Count 1", got)
	}
	if got := mm.book.Suggest("carlo@x.dev", 5); len(got) != 1 {
		t.Errorf("Cc inviato non nella rubrica: %+v", got)
	}
}
