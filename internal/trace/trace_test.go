package trace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// parseLines decodifica il JSONL emesso dal tracciatore.
func parseLines(t *testing.T, b []byte) []Event {
	t.Helper()
	var out []Event
	for _, ln := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if ln == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(ln), &e); err != nil {
			t.Fatalf("riga non JSON %q: %v", ln, err)
		}
		out = append(out, e)
	}
	return out
}

func TestDisabilitatoNonEmetteNulla(t *testing.T) {
	Uninstall()
	if Enabled() {
		t.Fatal("senza installazione Enabled deve essere false")
	}
	tm := Begin()
	tm.End(Event{K: "store", Op: "X"}) // nessun panic, nessun effetto
	Emit(Event{K: "phase", Op: "x"})
	if ctx, sp := WithSpan(context.Background(), "X"); sp != nil || SpanFrom(ctx) != nil {
		t.Fatal("senza traccia WithSpan non deve creare span")
	}
}

func TestEmitScriveUnaRigaJSONPerEvento(t *testing.T) {
	var buf bytes.Buffer
	Install(New(&buf, "v-test"))
	defer Uninstall()

	Emit(Event{K: "phase", Op: "first_view"})
	tm := Begin()
	time.Sleep(2 * time.Millisecond)
	tm.End(Event{K: "store", Op: "ListConversations", N: 50, Role: "inbox"})

	ev := parseLines(t, buf.Bytes())
	if len(ev) != 3 {
		t.Fatalf("attesi meta + 2 eventi, ho %d: %s", len(ev), buf.String())
	}
	if ev[0].K != "meta" || ev[0].Version != "v-test" || ev[0].OS != runtime.GOOS {
		t.Errorf("meta sbagliata: %+v", ev[0])
	}
	if ev[2].Op != "ListConversations" || ev[2].N != 50 || ev[2].Role != "inbox" {
		t.Errorf("evento store sbagliato: %+v", ev[2])
	}
	if ev[2].Dur < 2000 {
		t.Errorf("durata attesa >= 2000 µs, ho %d", ev[2].Dur)
	}
	if ev[2].T < ev[1].T {
		t.Errorf("t deve crescere: %d < %d", ev[2].T, ev[1].T)
	}
}

func TestSpanAccumulaAttesaGate(t *testing.T) {
	var buf bytes.Buffer
	Install(New(&buf, ""))
	defer Uninstall()
	ctx, sp := WithSpan(context.Background(), "X")
	if sp == nil || SpanFrom(ctx) != sp {
		t.Fatal("span non nel contesto")
	}
	sp.AddWait(3 * time.Millisecond)
	sp.AddWait(2 * time.Millisecond)
	if got := sp.Wait(); got != 5*time.Millisecond {
		t.Errorf("attesa = %v, attesi 5ms", got)
	}
}

func TestStartSoloConVariabile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("HOME", dir)
	Uninstall()

	tr, err := Start(func(string) string { return "" }, "v")
	if err != nil || tr != nil || Enabled() {
		t.Fatalf("senza variabile: tr=%v err=%v enabled=%v", tr, err, Enabled())
	}
	m, _ := filepath.Glob(filepath.Join(dir, "*", "gmail-tui", "trace-*"))
	m2, _ := filepath.Glob(filepath.Join(dir, "gmail-tui", "trace-*"))
	if len(m)+len(m2) != 0 {
		t.Fatalf("file di traccia senza variabile: %v %v", m, m2)
	}

	tr, err = Start(func(k string) string {
		if k == EnvVar {
			return "1"
		}
		return ""
	}, "v")
	if err != nil || tr == nil || !Enabled() {
		t.Fatalf("con variabile: tr=%v err=%v", tr, err)
	}
	defer Uninstall()
	p := tr.Path()
	if !strings.HasPrefix(filepath.Base(p), "trace-") || !strings.HasSuffix(p, ".jsonl") {
		t.Errorf("nome file inatteso: %s", p)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Errorf("permessi %v, attesi 0600", st.Mode().Perm())
	}
}

func TestStopStampaIlPercorsoSuStderr(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("HOME", dir)
	tr, err := Start(func(string) string { return "1" }, "v")
	if err != nil || tr == nil {
		t.Fatal(err)
	}
	var errw bytes.Buffer
	Stop(&errw)
	if !strings.Contains(errw.String(), tr.Path()) {
		t.Errorf("stderr %q non contiene il percorso %q", errw.String(), tr.Path())
	}
	if Enabled() {
		t.Error("dopo Stop la traccia deve essere disinstallata")
	}
	var none bytes.Buffer
	Stop(&none) // senza traccia: niente output
	if none.Len() != 0 {
		t.Errorf("Stop senza traccia ha scritto %q", none.String())
	}
}

// Piano 16, Task 7 (D16-T14): gli eventi cache portano solo Op, Mode, Role,
// conteggi e durate: nessun altro campo.
func TestEventiCacheSoloCampiAmmessi(t *testing.T) {
	var buf bytes.Buffer
	Install(New(&buf, ""))
	defer Uninstall()

	CacheLookup(OpSnapBody, ModeHit, "inbox", 1, 1234)
	CacheLookup(OpSnapList, ModeMiss, "label", 0, 0)
	SnapshotAge(OpSnapList, 3*time.Second)

	var cache []map[string]any
	for _, ln := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(ln), &m); err != nil {
			t.Fatal(err)
		}
		if m["k"] == "cache" {
			cache = append(cache, m)
		}
	}
	if len(cache) != 3 {
		t.Fatalf("attesi 3 eventi cache, ho %d: %s", len(cache), buf.String())
	}
	allowed := map[string]bool{"t": true, "k": true, "op": true, "dur_us": true, "n": true, "bytes": true, "role": true, "mode": true}
	for _, m := range cache {
		for k := range m {
			if !allowed[k] {
				t.Errorf("campo %q non ammesso nell'evento cache: %v", k, m)
			}
		}
	}
	if m := cache[0]; m["op"] != OpSnapBody || m["mode"] != ModeHit || m["role"] != "inbox" || m["n"] != float64(1) || m["bytes"] != float64(1234) {
		t.Errorf("hit sbagliato: %v", m)
	}
	if m := cache[1]; m["mode"] != ModeMiss || m["role"] != "label" {
		t.Errorf("miss sbagliato: %v", m)
	}
	if m := cache[2]; m["op"] != OpSnapList || m["mode"] != ModeAge || m["dur_us"] != float64(3_000_000) {
		t.Errorf("eta sbagliata: %v", m)
	}
}

func TestErroreNonEntraMaiNellaTraccia(t *testing.T) {
	var buf bytes.Buffer
	Install(New(&buf, ""))
	defer Uninstall()
	err := errors.New("login fallito per mario@example.com")
	Emit(Event{K: "store", Op: "Connect", Err: err != nil}) // solo il booleano
	if strings.Contains(buf.String(), "mario") {
		t.Fatal("il testo dell'errore e finito nella traccia")
	}
}
