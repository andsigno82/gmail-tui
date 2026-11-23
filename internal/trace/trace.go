// Package trace e la traccia OPZIONALE dei tempi (Piano 16): attiva solo con
// GMAIL_TUI_TRACE=1, scrive un file JSONL (una riga per evento) in
// os.UserCacheDir()/gmail-tui/trace-<data-ora>.jsonl con permessi 0600. Serve a
// misurare sull'account reale dove va il tempo (store, gate IMAP, Update/View,
// rendering del corpo); `gmail-tui trace <file>` ne stampa il riepilogo.
//
// PRIVACY (vincolo assoluto): la traccia contiene SOLO nomi di operazione, durate,
// conteggi e dimensioni in byte. Mai indirizzi, oggetti, nomi, corpi, nomi di
// label o cartelle utente (solo i ruoli), URL, password, ne il testo degli errori
// (solo un booleano). Chi aggiunge un campo a Event deve rispettarlo.
//
// Senza la variabile nulla e installato: Enabled e un solo caricamento atomico e
// nessuno dei wrapper (store, modello) viene montato.
package trace

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// EnvVar e la variabile che attiva la traccia (valore "1").
const EnvVar = "GMAIL_TUI_TRACE"

// Event e una riga del file. Tutti i tempi sono in microsecondi; i campi a zero
// si omettono. K distingue il tipo: meta | store | msg | phase | body.
//
//   - store: Op = metodo di MailStore, Dur totale, Wait attesa del gate IMAP
//     (esecuzione = Dur - Wait), N conteggio (ref/righe/id), Bytes corpo, Role.
//   - msg:   Op = tipo del messaggio Bubble Tea, Key = nome del tasto, Screen,
//     Upd durata di Update, View durata di View, ViewBytes byte della stringa resa.
//   - phase: Op = fase d'avvio; T e il tempo dall'avvio.
//   - body:  Op = FromHTML | render | build, Dur, Bytes = byte del sorgente,
//     N = blocchi, Mode = sync | async.
//   - cache: Piano 16, Task 7 (D16-T14). Op = snap_list | snap_thread | snap_body
//     | fetch_body; Mode = hit | miss | epoch (scartata per R3) | age (eta
//     dell'istantanea mostrata, in Dur); N = righe/ref; Bytes = byte del corpo;
//     Role = solo il ruolo della cartella. Mai ID, nomi, oggetti o indirizzi.
//     Si emettono con CacheLookup e SnapshotAge.
//
// T e l'istante di FINE dell'evento, in microsecondi dall'avvio della traccia.
type Event struct {
	T         int64  `json:"t"`
	K         string `json:"k"`
	Op        string `json:"op,omitempty"`
	Dur       int64  `json:"dur_us,omitempty"`
	Wait      int64  `json:"wait_us,omitempty"`
	Upd       int64  `json:"upd_us,omitempty"`
	View      int64  `json:"view_us,omitempty"`
	N         int    `json:"n,omitempty"`
	Bytes     int    `json:"bytes,omitempty"`
	ViewBytes int    `json:"view_bytes,omitempty"`
	Role      string `json:"role,omitempty"`
	Mode      string `json:"mode,omitempty"`
	Key       string `json:"key,omitempty"`
	Screen    string `json:"screen,omitempty"`
	Err       bool   `json:"err,omitempty"`

	// Solo per K == "meta" (prima riga del file).
	Start   string `json:"start,omitempty"`
	OS      string `json:"os,omitempty"`
	Arch    string `json:"arch,omitempty"`
	Term    string `json:"term,omitempty"`
	Version string `json:"version,omitempty"`
}

// Tracer scrive gli eventi su un io.Writer (serializzato da un mutex).
type Tracer struct {
	mu    sync.Mutex
	w     io.Writer
	start time.Time
	path  string
	dead  bool // errore di scrittura: la traccia si spegne, mai un errore alla TUI
}

// cur e la traccia installata (nil = disattiva).
var cur atomic.Pointer[Tracer]

// New crea un tracciatore su w e vi scrive la riga meta. Se w e un *os.File il
// suo nome diventa Path.
func New(w io.Writer, version string) *Tracer {
	t := &Tracer{w: w, start: time.Now()}
	if f, ok := w.(*os.File); ok {
		t.path = f.Name()
	}
	t.write(Event{
		K:       "meta",
		Start:   t.start.Format(time.RFC3339),
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
		Term:    os.Getenv("TERM"), // nome del terminale (es. xterm-ghostty): non e un dato personale
		Version: version,
	})
	return t
}

// Path e il file della traccia (vuoto se non e su file).
func (t *Tracer) Path() string { return t.path }

func (t *Tracer) write(e Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.dead {
		return
	}
	e.T = time.Since(t.start).Microseconds()
	b, err := json.Marshal(e)
	if err == nil {
		_, err = t.w.Write(append(b, '\n'))
	}
	if err != nil {
		t.dead = true
	}
}

// Install rende t la traccia globale; Uninstall la toglie.
func Install(t *Tracer) { cur.Store(t) }

// Uninstall disinstalla la traccia senza chiuderla.
func Uninstall() { cur.Store(nil) }

// Enabled dice se una traccia e installata: un solo caricamento atomico.
func Enabled() bool { return cur.Load() != nil }

// Emit scrive un evento (no-op senza traccia).
func Emit(e Event) {
	if t := cur.Load(); t != nil {
		t.write(e)
	}
}

// Start attiva la traccia se getenv(EnvVar) == "1": crea il file con permessi
// 0600 in os.UserCacheDir()/gmail-tui/trace-<data-ora>.jsonl, la installa e la
// ritorna. Senza la variabile ritorna (nil, nil) senza toccare il disco.
func Start(getenv func(string) string, version string) (*Tracer, error) {
	if getenv(EnvVar) != "1" {
		return nil, nil
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	dir = filepath.Join(dir, "gmail-tui")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	base := "trace-" + time.Now().Format("20060102-150405")
	var f *os.File
	for i := 0; i < 100; i++ {
		name := base + ".jsonl"
		if i > 0 {
			name = fmt.Sprintf("%s-%d.jsonl", base, i)
		}
		f, err = os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil || !os.IsExist(err) {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	t := New(f, version)
	Install(t)
	return t, nil
}

// Stop disinstalla e chiude la traccia, stampando il percorso su errw. Senza
// traccia installata non fa nulla.
func Stop(errw io.Writer) {
	t := cur.Swap(nil)
	if t == nil {
		return
	}
	t.mu.Lock()
	t.dead = true
	t.mu.Unlock()
	if c, ok := t.w.(io.Closer); ok {
		_ = c.Close()
	}
	if t.path != "" {
		fmt.Fprintf(errw, "traccia dei tempi: %s\n", t.path)
	}
}

// Operazioni e modalita della cache (Piano 16, Task 7, D16-T14): campi Op e Mode
// dell'evento K == "cache".
const (
	OpSnapList   = "snap_list"   // istantanea di lista (CachedConversations)
	OpSnapThread = "snap_thread" // istantanea di thread (CachedThread)
	OpSnapBody   = "snap_body"   // corpo in cache (CachedMessage)
	OpFetchBody  = "fetch_body"  // lettura cache-first del corpo (store IMAP, Task 8)

	ModeHit   = "hit"
	ModeMiss  = "miss"
	ModeEpoch = "epoch" // scartata per la regola d'epoca (R3)
	ModeAge   = "age"   // eta dell'istantanea mostrata (Dur)
)

// CacheLookup emette l'esito di una lettura dalla cache: op e uno degli Op
// sopra, mode hit | miss | epoch, role solo il ruolo della cartella ("" se non
// si applica), n righe/ref, bytes byte del corpo. Piano 16, Task 7.
func CacheLookup(op, mode, role string, n, bytes int) {
	Emit(Event{K: "cache", Op: op, Mode: mode, Role: role, N: n, Bytes: bytes})
}

// SnapshotAge emette l'eta dell'istantanea mostrata a video (Mode age, Dur = eta).
// Piano 16, Task 7.
func SnapshotAge(op string, age time.Duration) {
	Emit(Event{K: "cache", Op: op, Mode: ModeAge, Dur: age.Microseconds()})
}

// Timer misura una durata: con la traccia spenta e il valore zero e non chiama
// time.Now.
type Timer struct{ t0 time.Time }

// Begin avvia un Timer (zero se la traccia e spenta).
func Begin() Timer {
	if !Enabled() {
		return Timer{}
	}
	return Timer{t0: time.Now()}
}

// Elapsed e il tempo trascorso (0 con la traccia spenta).
func (t Timer) Elapsed() time.Duration {
	if t.t0.IsZero() {
		return 0
	}
	return time.Since(t.t0)
}

// End emette e con Dur = tempo trascorso dal Begin (no-op con la traccia spenta).
func (t Timer) End(e Event) {
	if t.t0.IsZero() {
		return
	}
	e.Dur = time.Since(t.t0).Microseconds()
	Emit(e)
}

// Span accumula, per UNA chiamata allo store, l'attesa del gate IMAP: il wrapper
// lo mette nel ctx, lo store lo riempie in acquire (cfr. imap.Store.acquire).
type Span struct{ wait atomic.Int64 } // nanosecondi

type spanKey struct{}

// WithSpan crea uno Span nel ctx; con la traccia spenta ritorna (ctx, nil).
func WithSpan(ctx context.Context) (context.Context, *Span) {
	if !Enabled() {
		return ctx, nil
	}
	sp := &Span{}
	return context.WithValue(ctx, spanKey{}, sp), sp
}

// SpanFrom recupera lo Span del ctx (nil se assente).
func SpanFrom(ctx context.Context) *Span {
	sp, _ := ctx.Value(spanKey{}).(*Span)
	return sp
}

// AddWait somma un'attesa del gate (nil-safe).
func (s *Span) AddWait(d time.Duration) {
	if s != nil {
		s.wait.Add(int64(d))
	}
}

// Wait e l'attesa accumulata (nil-safe).
func (s *Span) Wait() time.Duration {
	if s == nil {
		return 0
	}
	return time.Duration(s.wait.Load())
}
