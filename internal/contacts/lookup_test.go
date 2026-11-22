package contacts

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Piano 9, Task 10: comando esterno dei contatti (es. khard), senza shell.
// Il binario di test rilancia sé stesso come helper (TestHelperProcess).

// helperArgs è la riga di comando dell'helper: "--" separa gli argomenti veri.
func helperArgs(mode string, args ...string) []string {
	return append([]string{os.Args[0], "-test.run=TestHelperProcess", "--", mode}, args...)
}

// TestHelperProcess non è un test: è il processo figlio dei test di Lookup.
// mode "echo": una riga "q@x.dev\t<arg>" per ogni argomento dopo il modo;
// "fail": stderr e exit 3; "sleep": attende molto (per il timeout).
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	for i, a := range args {
		if a == "--" {
			args = args[i+1:]
			break
		}
	}
	switch args[0] {
	case "echo":
		for _, a := range args[1:] {
			fmt.Printf("q@x.dev\t%s\n", a)
		}
	case "fail":
		fmt.Fprint(os.Stderr, "khard: nessun contatto")
		os.Exit(3)
	case "sleep":
		time.Sleep(10 * time.Second)
	case "flood":
		// Piano 9, review C: output molto più grande del tetto (40 righe da ~2 KB).
		name := strings.Repeat("x", 2000)
		for i := 0; i < 40; i++ {
			fmt.Printf("q%d@x.dev\t%s\n", i, name)
		}
	case "ctrl":
		// Piano 9, review C: nome con ESC, C0, bidi override; indirizzo con controllo.
		fmt.Print("q@x.dev\t\x1b[31mEve\u202e\x07 Rossi\n")
		fmt.Print("bad\x1b@x.dev\tNome\n")
	case "spawngrand":
		// Piano 9, review C: un nipote eredita stdout e lo tiene aperto dopo la morte del padre.
		g := exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--", "sleep")
		g.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
		g.Stdout = os.Stdout
		g.Stderr = os.Stderr
		_ = g.Start()
		time.Sleep(10 * time.Second)
	case "env":
		// Piano 9, review B: riporta lo stato delle variabili sensibili nell'ambiente del figlio.
		for _, k := range []string{"GMAIL_APP_PASSWORD", "GMAIL_TUI_FROM"} {
			v, ok := os.LookupEnv(k)
			if !ok {
				v = "ABSENT"
			}
			fmt.Printf("q@x.dev\t%s=%s\n", k, v)
		}
	}
	os.Exit(0)
}

// helperEnv abilita il modo helper nel figlio.
func helperEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GO_WANT_HELPER_PROCESS", "1")
}

// TestLookupPassesQueryAsArgument: la query arriva come UN argomento intatto
// (spazi e ';' compresi): nessuna shell la interpreta.
func TestLookupPassesQueryAsArgument(t *testing.T) {
	helperEnv(t)
	query := "anna rossi; rm -rf /"
	argv := helperArgs("echo", "%s")
	got, err := Lookup(context.Background(), argv, query)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if len(got) != 1 || got[0].Name != query {
		t.Errorf("argomento ricevuto = %+v, voglio la query intatta %q", got, query)
	}
}

// TestLookupAppendsQueryWithoutPlaceholder: senza %s la query è l'ultimo argomento.
func TestLookupAppendsQueryWithoutPlaceholder(t *testing.T) {
	helperEnv(t)
	got, err := Lookup(context.Background(), helperArgs("echo", "--fisso"), "anna")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if len(got) != 2 || got[1].Name != "anna" {
		t.Errorf("risultati = %+v, voglio l'ultimo argomento = anna", got)
	}
}

// TestLookupParsesTabSeparated: "indirizzo<TAB>nome" (nome opzionale); righe senza
// indirizzo valido scartate.
func TestLookupParsesTabSeparated(t *testing.T) {
	helperEnv(t)
	// L'helper "echo" stampa "q@x.dev\t<arg>": con %s la query è il nome.
	got, err := Lookup(context.Background(), helperArgs("echo", "%s"), "Anna Rossi")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if len(got) != 1 || got[0].Addr != "q@x.dev" || got[0].Name != "Anna Rossi" {
		t.Errorf("parsing = %+v", got)
	}
	res := parseOutput("niente indirizzo\nsenza tab\nok@x.dev\tOk\n")
	if len(res) != 1 || res[0].Addr != "ok@x.dev" {
		t.Errorf("righe non valide non scartate: %+v", res)
	}
}

// TestLookupTimeoutKillsProcess: il timeout del ctx uccide il comando e ritorna errore.
func TestLookupTimeoutKillsProcess(t *testing.T) {
	helperEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := Lookup(ctx, helperArgs("sleep"), "anna"); err == nil {
		t.Error("timeout: err = nil, voglio errore")
	}
}

// TestLookupNonZeroExitIsError: exit diverso da zero → errore con lo stderr.
func TestLookupNonZeroExitIsError(t *testing.T) {
	helperEnv(t)
	_, err := Lookup(context.Background(), helperArgs("fail"), "anna")
	if err == nil {
		t.Fatal("exit 3: err = nil, voglio errore")
	}
	if !strings.Contains(err.Error(), "nessun contatto") {
		t.Errorf("errore = %v, voglio lo stderr del comando", err)
	}
}

// TestLookupEmptyArgvIsError: nessun comando configurato → errore esplicito.
func TestLookupEmptyArgvIsError(t *testing.T) {
	if _, err := Lookup(context.Background(), nil, "anna"); err == nil {
		t.Error("argv vuoto: err = nil")
	}
}

// TestLookupChildEnvHidesSecrets (Piano 9, review B): il comando esterno non
// eredita la password App né le variabili GMAIL_TUI_* del processo.
func TestLookupChildEnvHidesSecrets(t *testing.T) {
	helperEnv(t)
	t.Setenv("GMAIL_APP_PASSWORD", "s3cret-app-pass")
	t.Setenv("GMAIL_TUI_FROM", "anna@x.dev")
	got, err := Lookup(context.Background(), helperArgs("env"), "anna")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("righe = %d, voglio 2", len(got))
	}
	for _, s := range got {
		if strings.Contains(s.Name, "s3cret") || strings.Contains(s.Name, "anna@x.dev") {
			t.Errorf("segreto nell'ambiente del figlio: %q", s.Name)
		}
		if !strings.HasSuffix(s.Name, "=ABSENT") {
			t.Errorf("variabile ereditata dal figlio: %q, voglio =ABSENT", s.Name)
		}
	}
}

// TestLookupStdoutIsCapped (Piano 9, review C): l'output è letto con un tetto
// (64 KB): le righe oltre il tetto non entrano nei suggerimenti.
func TestLookupStdoutIsCapped(t *testing.T) {
	helperEnv(t)
	got, err := Lookup(context.Background(), helperArgs("flood"), "x")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if len(got) == 0 || len(got) >= 40 {
		t.Errorf("suggerimenti = %d, voglio un sottoinsieme non vuoto di 40 (tetto stdout)", len(got))
	}
}

// TestLookupSanitizesNameAndDropsControlAddress (Piano 9, review C): nome senza
// ESC, C0 e controlli bidi; indirizzo con controlli scartato.
func TestLookupSanitizesNameAndDropsControlAddress(t *testing.T) {
	helperEnv(t)
	got, err := Lookup(context.Background(), helperArgs("ctrl"), "x")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("suggerimenti = %d, voglio 1 (l'indirizzo con ESC scartato)", len(got))
	}
	if got[0].Name != "Eve Rossi" {
		t.Errorf("nome = %q, voglio senza controlli", got[0].Name)
	}
	for _, r := range got[0].Name {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) || (r >= 0x202a && r <= 0x202e) {
			t.Errorf("nome con controllo U+%04X: %q", r, got[0].Name)
		}
	}
}

// TestLookupTimeoutKillsProcessGroup (Piano 9, review C): allo scadere del
// timeout muore anche il nipote che tiene aperto lo stdout: Lookup torna subito.
func TestLookupTimeoutKillsProcessGroup(t *testing.T) {
	helperEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := Lookup(ctx, helperArgs("spawngrand"), "anna"); err == nil {
		t.Error("timeout: err = nil, voglio errore")
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("Lookup ha impiegato %v con un nipote vivo, voglio < 3s (kill del gruppo)", d)
	}
}
