package contacts

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Piano 9, Task 8: rubrica locale da mittenti e destinatari.

var t0 = time.Date(2026, 11, 22, 10, 0, 0, 0, time.UTC)

// TestObserveOrdersByFrequencyThenRecency: Count desc, poi Last desc, poi Addr asc.
func TestObserveOrdersByFrequencyThenRecency(t *testing.T) {
	b, err := Open("")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	must(t, b.Observe(t0, true, "Anna <anna@x.dev>"))
	must(t, b.Observe(t0.Add(time.Hour), true, "bob@x.dev"))
	must(t, b.Observe(t0.Add(2*time.Hour), true, "bob@x.dev"))
	must(t, b.Observe(t0.Add(3*time.Hour), true, "carla@x.dev"))
	got := b.Suggest("x.dev", 10)
	want := []string{"bob@x.dev", "carla@x.dev", "anna@x.dev"}
	if len(got) != len(want) {
		t.Fatalf("Suggest = %v, voglio %v", got, want)
	}
	for i := range want {
		if got[i].Addr != want[i] {
			t.Errorf("posizione %d = %q, voglio %q", i, got[i].Addr, want[i])
		}
	}
}

// TestObserveWithoutBumpKeepsCount: i mittenti da lista non incrementano.
func TestObserveWithoutBumpKeepsCount(t *testing.T) {
	b, _ := Open("")
	must(t, b.Observe(t0, true, "anna@x.dev"))
	must(t, b.Observe(t0.Add(time.Hour), false, "anna@x.dev", "nuovo@x.dev"))
	got := b.Suggest("anna", 10)
	if len(got) != 1 || got[0].Count != 1 {
		t.Errorf("anna dopo observe senza bump = %+v, voglio Count 1", got)
	}
	if n := b.Suggest("nuovo", 10); len(n) != 1 {
		t.Errorf("indirizzo nuovo senza bump non aggiunto: %v", n)
	}
}

// TestSuggestMatchesNameAndAddress: sottostringa case-insensitive su indirizzo o nome.
func TestSuggestMatchesNameAndAddress(t *testing.T) {
	b, _ := Open("")
	must(t, b.Observe(t0, true, "Mario Rossi <m.rossi@studio.it>"))
	must(t, b.Observe(t0, true, "luca@other.dev"))
	if got := b.Suggest("ROSSI", 5); len(got) != 1 || got[0].Name != "Mario Rossi" {
		t.Errorf("ricerca per nome = %+v", got)
	}
	if got := b.Suggest("other", 5); len(got) != 1 || got[0].Addr != "luca@other.dev" {
		t.Errorf("ricerca per indirizzo = %+v", got)
	}
	if got := b.Suggest("", 5); len(got) != 0 {
		t.Errorf("query vuota: %v, voglio nessun suggerimento", got)
	}
	if got := b.Suggest("anna", 0); len(got) != 0 {
		t.Errorf("nessuna corrispondenza: %v", got)
	}
}

// TestFileIs0600AndRoundTrips: il file è 0600 e si ricarica identico.
func TestFileIs0600AndRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "contacts.json")
	b, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	must(t, b.Observe(t0, true, "Anna <anna@x.dev>"))
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("file non scritto: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("permessi = %v, voglio 0600", perm)
	}
	again, err := Open(path)
	if err != nil {
		t.Fatalf("riapertura: %v", err)
	}
	got := again.Suggest("anna", 5)
	if len(got) != 1 || got[0].Name != "Anna" || got[0].Count != 1 || !got[0].Last.Equal(t0) {
		t.Errorf("dopo il ricaricamento = %+v", got)
	}
}

// TestCorruptFileIsAnError: un file illeggibile è un errore, non una rubrica vuota.
func TestCorruptFileIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contacts.json")
	must(t, os.WriteFile(path, []byte("{non json"), 0o600))
	if _, err := Open(path); err == nil {
		t.Error("Open su file corrotto: err = nil, voglio errore")
	}
}

// TestMemoryBookWritesNothing: path "" resta in memoria, nessun file.
func TestMemoryBookWritesNothing(t *testing.T) {
	dir := t.TempDir()
	prev, _ := os.Getwd()
	must(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(prev) })
	b, err := Open("")
	if err != nil {
		t.Fatalf("Open(\"\"): %v", err)
	}
	must(t, b.Observe(t0, true, "anna@x.dev"))
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("rubrica in memoria ha scritto %d file", len(entries))
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
