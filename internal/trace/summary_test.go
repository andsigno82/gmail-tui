package trace

import (
	"encoding/json"
	"strings"
	"testing"
)

// jsonl serializza eventi come il file di traccia.
func jsonl(evs ...Event) string {
	var sb strings.Builder
	for _, e := range evs {
		b, _ := json.Marshal(e)
		sb.Write(b)
		sb.WriteByte('\n')
	}
	return sb.String()
}

func TestPercentile(t *testing.T) {
	vals := []int64{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	if got := median(vals); got != 55 {
		t.Errorf("mediana = %d, attesa 55", got)
	}
	if got := percentile(vals, 95); got != 100 {
		t.Errorf("p95 = %d, atteso 100", got)
	}
	if got := percentile(nil, 95); got != 0 {
		t.Errorf("p95 vuoto = %d", got)
	}
}

func TestSummarizeRiepilogo(t *testing.T) {
	in := jsonl(
		Event{K: "meta", Start: "2026-11-23T10:00:00Z", OS: "darwin", Arch: "arm64", Term: "xterm-ghostty", Version: "dev"},
		Event{T: 500_000, K: "store", Op: "Connect", Dur: 480_000},
		Event{T: 1_500_000, K: "store", Op: "ListConversations", Dur: 900_000, Wait: 100_000, N: 50, Role: "inbox"},
		Event{T: 2_500_000, K: "store", Op: "ListConversations", Dur: 700_000, Wait: 300_000, N: 50, Role: "inbox"},
		Event{T: 1_600_000, K: "phase", Op: "first_view"},
		Event{T: 2_600_000, K: "phase", Op: "first_list_view"},
		Event{T: 3_000_000, K: "msg", Op: "tea.KeyPressMsg", Key: "down", Screen: "inbox", Upd: 2_000, View: 80_000, ViewBytes: 20_000},
		Event{T: 3_100_000, K: "msg", Op: "tea.KeyPressMsg", Key: "down", Screen: "inbox", Upd: 1_000, View: 4_000, ViewBytes: 10_000},
		Event{T: 3_200_000, K: "body", Op: "FromHTML", Dur: 12_000, Bytes: 40_000, Mode: "async"},
	)
	var out strings.Builder
	if err := Summarize(strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{
		"darwin/arm64", "xterm-ghostty",
		"ListConversations", "inbox",
		"attesa gate",
		"tea.KeyPressMsg down",
		"Frame piu lenti",
		"Byte medi per View",
		"FromHTML", "async",
		"Avvio", "first_list_view",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("riepilogo senza %q:\n%s", want, s)
		}
	}
	// ListConversations: durate 900 e 700 ms => mediana 800 ms, max 900 ms.
	if !strings.Contains(s, "800.0") || !strings.Contains(s, "900.0") {
		t.Errorf("mediana/max di ListConversations attesi 800.0/900.0:\n%s", s)
	}
	// Il frame piu lento (2 ms + 80 ms) sta in cima.
	if i, j := strings.Index(s, "82.0"), strings.Index(s, "5.0 ms"); i < 0 || (j >= 0 && j < i) {
		t.Errorf("frame piu lento non in cima:\n%s", s)
	}
	// Byte medi per View: (20000+10000)/2 = 15000 B = 14.6 KiB.
	if !strings.Contains(s, "14.6") {
		t.Errorf("byte medi per View attesi 14.6 KiB:\n%s", s)
	}
	// Un file senza passi non ha la tabella dei passi (Piano 12a, Task 4).
	if strings.Contains(s, "Passi IMAP") {
		t.Errorf("file senza eventi step con la tabella dei passi:\n%s", s)
	}
}

func TestSummarizeToleraRigheRotteEFileVuoto(t *testing.T) {
	var out strings.Builder
	if err := Summarize(strings.NewReader(""), &out); err == nil {
		t.Error("file vuoto: atteso errore")
	}
	out.Reset()
	in := "non json\n" + jsonl(Event{T: 1, K: "store", Op: "Search", Dur: 1000})
	if err := Summarize(strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "righe non valide: 1") || !strings.Contains(out.String(), "Search") {
		t.Errorf("riepilogo inatteso:\n%s", out.String())
	}
}

// Piano 16, Task 7: la sezione Cache conta hit e miss per tipo, le scartate per
// epoca e mostra l'eta mediana dell'istantanea.
func TestSummarizeSezioneCache(t *testing.T) {
	in := jsonl(
		Event{T: 1_000, K: "cache", Op: OpSnapList, Mode: ModeHit, Role: "inbox", N: 50},
		Event{T: 2_000, K: "cache", Op: OpSnapList, Mode: ModeHit, Role: "inbox", N: 50},
		Event{T: 3_000, K: "cache", Op: OpSnapList, Mode: ModeHit, Role: "inbox", N: 50},
		Event{T: 4_000, K: "cache", Op: OpSnapList, Mode: ModeMiss, Role: "label"},
		Event{T: 5_000, K: "cache", Op: OpSnapList, Mode: ModeEpoch, Role: "inbox"},
		Event{T: 6_000, K: "cache", Op: OpSnapList, Mode: ModeAge, Dur: 2_000_000},
		Event{T: 7_000, K: "cache", Op: OpSnapList, Mode: ModeAge, Dur: 4_000_000},
		Event{T: 8_000, K: "cache", Op: OpSnapList, Mode: ModeAge, Dur: 6_000_000},
		Event{T: 9_000, K: "cache", Op: OpSnapBody, Mode: ModeHit, N: 1, Bytes: 1000},
		Event{T: 10_000, K: "cache", Op: OpSnapBody, Mode: ModeHit, N: 1, Bytes: 1000},
	)
	var out strings.Builder
	if err := Summarize(strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{"Cache", "snap_list", "snap_body", "75.0%", "100.0%", "4.00 s"} {
		if !strings.Contains(s, want) {
			t.Errorf("riepilogo senza %q:\n%s", want, s)
		}
	}
}

func TestSummarizeFrameLentiLimitatiADieci(t *testing.T) {
	var evs []Event
	for i := 0; i < 30; i++ {
		evs = append(evs, Event{T: int64(i) * 1000, K: "msg", Op: "app.msgsLoaded", Upd: int64(i+1) * 1000})
	}
	var out strings.Builder
	if err := Summarize(strings.NewReader(jsonl(evs...)), &out); err != nil {
		t.Fatal(err)
	}
	// 1 riga nella tabella per tipo + 10 nei frame piu lenti.
	if n := strings.Count(out.String(), "app.msgsLoaded"); n != 11 {
		t.Errorf("occorrenze di app.msgsLoaded = %d, attese 11:\n%s", n, out.String())
	}
	if !strings.Contains(out.String(), "30.0 ms") || strings.Contains(out.String(), " 5.0 ms") {
		t.Errorf("i frame lenti devono andare da 30 a 21 ms:\n%s", out.String())
	}
}

// righeCon ritorna le righe del riepilogo che iniziano con prefisso.
func righeCon(s, prefisso string) []string {
	var out []string
	for riga := range strings.SplitSeq(s, "\n") {
		if strings.HasPrefix(riga, prefisso) {
			out = append(out, riga)
		}
	}
	return out
}

// Piano 12a, Task 4: la tabella dei passi IMAP, una riga per operazione, passo e
// ruolo; i passi del Connect finiscono nella cronologia dell'avvio.
func TestSummarizeSezionePassi(t *testing.T) {
	in := jsonl(
		Event{K: "meta", Start: "2026-11-23T10:00:00Z", OS: "darwin", Arch: "arm64", Version: "dev"},
		Event{T: 100_000, K: "step", Op: "Connect", Step: "tcp", Dur: 100_000},
		Event{T: 250_000, K: "step", Op: "Connect", Step: "tls", Dur: 150_000},
		Event{T: 1_250_000, K: "step", Op: "Connect", Step: "login", Dur: 1_000_000},
		Event{T: 1_300_000, K: "store", Op: "Connect", Dur: 1_300_000},
		Event{T: 1_600_000, K: "step", Op: "ListConversations", Step: "select", Role: "inbox", Dur: 900_000},
		Event{T: 2_100_000, K: "step", Op: "ListConversations", Step: "search", Role: "inbox", Dur: 500_000},
		Event{T: 2_900_000, K: "step", Op: "ListConversations", Step: "select", Role: "all", Dur: 800_000},
	)
	var out strings.Builder
	if err := Summarize(strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "Passi IMAP") {
		t.Errorf("riepilogo senza la tabella dei passi:\n%s", s)
	}
	// Una riga per operazione, passo e ruolo: due "select" (inbox e all).
	if got := righeCon(s, "ListConversations select"); len(got) != 2 {
		t.Errorf("righe di \"ListConversations select\" = %d, attese 2:\n%s", len(got), s)
	}
	if got := righeCon(s, "ListConversations search"); len(got) != 1 || !strings.Contains(got[0], "inbox") || !strings.Contains(got[0], "500.0") {
		t.Errorf("riga di search attesa con inbox e 500.0:\n%s", s)
	}
	// Cronologia dell'avvio: i passi del Connect, in ordine di tempo.
	i, j, k := strings.Index(s, "Connect tcp"), strings.Index(s, "Connect tls"), strings.Index(s, "Connect login")
	if i < 0 || j < 0 || k < 0 || i >= j || j >= k {
		t.Errorf("i passi del Connect non sono in ordine nell'avvio:\n%s", s)
	}
}
