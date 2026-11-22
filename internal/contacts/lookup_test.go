package contacts

import (
	"context"
	"fmt"
	"os"
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
