package keyring

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// Il pacchetto si testa su due livelli:
//  1. runner finto (runnerStub): scelta backend, costruzione argomenti e
//     path d'errore — girano su QUALSIASI OS;
//  2. keychain reale (TestRealKeychainRoundtrip): solo su darwin con
//     `security` disponibile, service/user dedicati ai test e t.Cleanup.

// --- doppio: runner finto ---

// callRecord memorizza una chiamata al runner per le asserzioni.
type callRecord struct {
	name  string
	args  []string
	stdin []byte
}

// runnerStub registra le chiamate e risponde con l'esito scriptato;
// senza result ogni chiamata è "inattesa" (errore esplicito).
type runnerStub struct {
	calls  []callRecord
	result func(c callRecord, nth int) ([]byte, error)
}

func (s *runnerStub) run(name string, args []string, stdin []byte) ([]byte, error) {
	c := callRecord{name: name, args: args, stdin: stdin}
	s.calls = append(s.calls, c)
	if s.result == nil {
		return nil, fmt.Errorf("runnerStub: chiamata inattesa: %s %v", name, args)
	}
	return s.result(c, len(s.calls))
}

// errSecurityNotFound simula l'exit 44 di security (item assente,
// "not found" su stderr).
func errSecurityNotFound() error {
	return &cmdError{
		name:   "security",
		args:   []string{"find-generic-password", "-s", "svc", "-a", "u", "-w"},
		code:   44,
		stderr: "security: SecKeychainSearchCopyNext: The specified item could not be found in the keychain.",
	}
}

// assertCall verifica comando e argomenti di una chiamata registrata.
func assertCall(t *testing.T, got callRecord, wantName string, wantArgs []string) {
	t.Helper()
	if got.name != wantName {
		t.Fatalf("comando: atteso %q, avuto %q", wantName, got.name)
	}
	if !reflect.DeepEqual(got.args, wantArgs) {
		t.Fatalf("args:\n attesi %v\n  avuti %v", wantArgs, got.args)
	}
}

// --- doppio: Store finto per Resolve ---

// fakeStore è il doppio di Store per i test di Resolve.
type fakeStore struct {
	getSecret  string
	getErr     error
	getCalled  bool
	getService string
	getUser    string
}

var _ Store = (*fakeStore)(nil)

func (f *fakeStore) Get(service, user string) (string, error) {
	f.getCalled, f.getService, f.getUser = true, service, user
	return f.getSecret, f.getErr
}

func (f *fakeStore) Set(service, user, secret string) error { return nil }

func (f *fakeStore) Delete(service, user string) error { return nil }

// --- scelta backend ---

func TestNewStoreBackendSelection(t *testing.T) {
	darwin, err := newStore("darwin", execRunner)
	if err != nil {
		t.Fatalf("darwin: %v", err)
	}
	if _, ok := darwin.(*darwinStore); !ok {
		t.Fatalf("darwin: atteso *darwinStore, avuto %T", darwin)
	}
	linux, err := newStore("linux", execRunner)
	if err != nil {
		t.Fatalf("linux: %v", err)
	}
	if _, ok := linux.(*linuxStore); !ok {
		t.Fatalf("linux: atteso *linuxStore, avuto %T", linux)
	}
	_, err = newStore("windows", execRunner)
	if err == nil {
		t.Fatal("windows: atteso errore, avuto nil")
	}
	if !strings.Contains(err.Error(), "keychain non supportata") ||
		!strings.Contains(err.Error(), "GMAIL_APP_PASSWORD") {
		t.Fatalf("windows: messaggio errato: %v", err)
	}
}

func TestNewUsesRuntimeGOOS(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skipf("GOOS %q non supportato dal pacchetto", runtime.GOOS)
	}
	kr, err := New()
	if err != nil {
		t.Fatalf("New su %s: %v", runtime.GOOS, err)
	}
	if kr == nil {
		t.Fatal("New: store nil")
	}
}

// --- backend darwin (security) ---

func TestDarwinGetArgsAndSecret(t *testing.T) {
	st := &runnerStub{result: func(callRecord, int) ([]byte, error) {
		return []byte("pw-123\n"), nil
	}}
	kr := &darwinStore{run: st.run}
	got, err := kr.Get("gmail-tui", "mario")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "pw-123" {
		t.Fatalf("segreto: atteso %q, avuto %q", "pw-123", got)
	}
	if len(st.calls) != 1 {
		t.Fatalf("attesa 1 chiamata, avuto %d", len(st.calls))
	}
	assertCall(t, st.calls[0], "security",
		[]string{"find-generic-password", "-s", "gmail-tui", "-a", "mario", "-w"})
	if st.calls[0].stdin != nil {
		t.Fatal("Get non deve usare stdin")
	}
}

func TestDarwinGetNotFoundMapsToErrNotFound(t *testing.T) {
	st := &runnerStub{result: func(callRecord, int) ([]byte, error) {
		return nil, errSecurityNotFound()
	}}
	kr := &darwinStore{run: st.run}
	if _, err := kr.Get("svc", "u"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("atteso ErrNotFound, avuto %v", err)
	}
}

func TestDarwinGetOtherErrorWrappedWithCommand(t *testing.T) {
	st := &runnerStub{result: func(c callRecord, _ int) ([]byte, error) {
		return nil, &cmdError{name: "security", args: c.args, code: 51, stderr: "User interaction is not allowed."}
	}}
	kr := &darwinStore{run: st.run}
	_, err := kr.Get("svc", "u")
	if err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("errore generico non deve essere ErrNotFound: %v", err)
	}
	if !strings.Contains(err.Error(), "security") || !strings.Contains(err.Error(), "exit 51") {
		t.Fatalf("errore non wrapped col comando: %v", err)
	}
}

func TestDarwinSetDeleteThenAdd(t *testing.T) {
	// 1a chiamata: delete → item assente (exit 44, ignorato); 2a: add → ok.
	st := &runnerStub{result: func(_ callRecord, nth int) ([]byte, error) {
		if nth == 1 {
			return nil, errSecurityNotFound()
		}
		return nil, nil
	}}
	kr := &darwinStore{run: st.run}
	if err := kr.Set("gmail-tui", "mario", "s3cr3t"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if len(st.calls) != 2 {
		t.Fatalf("attese 2 chiamate (delete+add), avuto %d", len(st.calls))
	}
	assertCall(t, st.calls[0], "security",
		[]string{"delete-generic-password", "-s", "gmail-tui", "-a", "mario"})
	assertCall(t, st.calls[1], "security",
		[]string{"add-generic-password", "-s", "gmail-tui", "-a", "mario", "-w", "s3cr3t"})
	if st.calls[1].stdin != nil {
		t.Fatal("add: stdin non atteso (il segreto passa per -w)")
	}
}

func TestDarwinSetOverwritesExisting(t *testing.T) {
	// delete trova l'item (esito ok) → add comunque eseguito.
	st := &runnerStub{result: func(callRecord, int) ([]byte, error) { return nil, nil }}
	kr := &darwinStore{run: st.run}
	if err := kr.Set("svc", "u", "pw2"); err != nil {
		t.Fatalf("Set overwrite: %v", err)
	}
	if len(st.calls) != 2 {
		t.Fatalf("attese 2 chiamate (delete+add), avuto %d", len(st.calls))
	}
}

func TestDarwinSetDeleteErrorAborts(t *testing.T) {
	// delete fallisce con errore diverso da not-found → Set fallisce, add non parte.
	st := &runnerStub{result: func(c callRecord, nth int) ([]byte, error) {
		if nth == 1 {
			return nil, &cmdError{name: "security", args: c.args, code: 51, stderr: "boom"}
		}
		return nil, nil
	}}
	kr := &darwinStore{run: st.run}
	if err := kr.Set("svc", "u", "pw"); err == nil {
		t.Fatal("delete fallito deve bloccare Set")
	}
	if len(st.calls) != 1 {
		t.Fatalf("add non deve partire: %d chiamate", len(st.calls))
	}
}

func TestDarwinDeleteArgsAndNotFound(t *testing.T) {
	t.Run("args", func(t *testing.T) {
		st := &runnerStub{result: func(callRecord, int) ([]byte, error) { return nil, nil }}
		kr := &darwinStore{run: st.run}
		if err := kr.Delete("gmail-tui", "mario"); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		assertCall(t, st.calls[0], "security",
			[]string{"delete-generic-password", "-s", "gmail-tui", "-a", "mario"})
	})
	t.Run("not found", func(t *testing.T) {
		st := &runnerStub{result: func(callRecord, int) ([]byte, error) {
			return nil, errSecurityNotFound()
		}}
		kr := &darwinStore{run: st.run}
		if err := kr.Delete("svc", "u"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("atteso ErrNotFound, avuto %v", err)
		}
	})
}

// --- backend linux (secret-tool) ---

func TestLinuxGetArgsAndSecret(t *testing.T) {
	st := &runnerStub{result: func(callRecord, int) ([]byte, error) {
		return []byte("pw-xyz\n"), nil
	}}
	kr := &linuxStore{run: st.run}
	got, err := kr.Get("gmail-tui", "mario")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "pw-xyz" {
		t.Fatalf("segreto: atteso %q, avuto %q", "pw-xyz", got)
	}
	assertCall(t, st.calls[0], "secret-tool",
		[]string{"lookup", "service", "gmail-tui", "username", "mario"})
	if st.calls[0].stdin != nil {
		t.Fatal("lookup non deve usare stdin")
	}
}

func TestLinuxGetEmptyStdoutIsNotFound(t *testing.T) {
	// secret-tool lookup senza risultato: stdout vuoto, exit 0.
	st := &runnerStub{result: func(callRecord, int) ([]byte, error) {
		return []byte{}, nil
	}}
	kr := &linuxStore{run: st.run}
	if _, err := kr.Get("svc", "u"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stdout vuoto deve dare ErrNotFound, avuto %v", err)
	}
}

func TestLinuxGetOtherErrorWrapped(t *testing.T) {
	st := &runnerStub{result: func(c callRecord, _ int) ([]byte, error) {
		return nil, &cmdError{name: "secret-tool", args: c.args, code: 1, stderr: "No secret service found"}
	}}
	kr := &linuxStore{run: st.run}
	_, err := kr.Get("svc", "u")
	if err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("errore generico non deve essere ErrNotFound: %v", err)
	}
	if !strings.Contains(err.Error(), "secret-tool") {
		t.Fatalf("errore non wrapped col comando: %v", err)
	}
}

func TestLinuxSetArgsAndStdin(t *testing.T) {
	st := &runnerStub{result: func(callRecord, int) ([]byte, error) { return nil, nil }}
	kr := &linuxStore{run: st.run}
	if err := kr.Set("gmail-tui", "mario", "s3cr3t"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	assertCall(t, st.calls[0], "secret-tool",
		[]string{"store", "--label=gmail-tui", "service", "gmail-tui", "username", "mario"})
	if string(st.calls[0].stdin) != "s3cr3t" {
		t.Fatalf("stdin: atteso %q, avuto %q", "s3cr3t", st.calls[0].stdin)
	}
}

func TestLinuxDeleteArgs(t *testing.T) {
	st := &runnerStub{result: func(callRecord, int) ([]byte, error) { return nil, nil }}
	kr := &linuxStore{run: st.run}
	if err := kr.Delete("gmail-tui", "mario"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	assertCall(t, st.calls[0], "secret-tool",
		[]string{"clear", "service", "gmail-tui", "username", "mario"})
}

// --- runner di default (exec) ---

func TestExecRunnerStdout(t *testing.T) {
	out, err := execRunner("echo", []string{"ciao"}, nil)
	if err != nil {
		t.Fatalf("echo: %v", err)
	}
	if strings.TrimSpace(string(out)) != "ciao" {
		t.Fatalf("stdout: atteso %q, avuto %q", "ciao", out)
	}
}

func TestExecRunnerStdin(t *testing.T) {
	out, err := execRunner("cat", nil, []byte("segreto-stdin"))
	if err != nil {
		t.Fatalf("cat: %v", err)
	}
	if string(out) != "segreto-stdin" {
		t.Fatalf("stdin non propagato: %q", out)
	}
}

func TestExecRunnerExitCode(t *testing.T) {
	_, err := execRunner("false", nil, nil)
	var ce *cmdError
	if !errors.As(err, &ce) {
		t.Fatalf("atteso *cmdError, avuto %v (%T)", err, err)
	}
	if ce.code != 1 {
		t.Fatalf("exit code: atteso 1, avuto %d", ce.code)
	}
}

func TestCmdErrorMasksSecretInMessage(t *testing.T) {
	// su darwin il segreto è un argomento (-w): non deve finire nei log/errori.
	e := &cmdError{
		name:   "security",
		args:   []string{"add-generic-password", "-s", "gmail-tui", "-a", "mario", "-w", "s3cr3t"},
		code:   45,
		stderr: "The specified item already exists.",
	}
	msg := e.Error()
	if strings.Contains(msg, "s3cr3t") {
		t.Fatalf("il segreto non deve finire nel messaggio: %s", msg)
	}
	if !strings.Contains(msg, "****") || !strings.Contains(msg, "exit 45") {
		t.Fatalf("messaggio incompleto/mascherato male: %s", msg)
	}
}

// --- Resolve: env → keychain → assente ---

func TestResolveEnvWins(t *testing.T) {
	fake := &fakeStore{getSecret: "dal-keychain"}
	pw, src, err := Resolve(func(string) string { return "dalla-env" }, fake, "mario")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if pw != "dalla-env" || src != "env" {
		t.Fatalf("atteso (%q, %q), avuto (%q, %q)", "dalla-env", "env", pw, src)
	}
	if fake.getCalled {
		t.Fatal("la env presente non deve interrogare il keychain")
	}
}

func TestResolveFromKeychain(t *testing.T) {
	fake := &fakeStore{getSecret: "dal-keychain"}
	pw, src, err := Resolve(func(string) string { return "" }, fake, "mario")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if pw != "dal-keychain" || src != "keychain" {
		t.Fatalf("atteso (%q, %q), avuto (%q, %q)", "dal-keychain", "keychain", pw, src)
	}
	if fake.getService != ServiceName || fake.getUser != "mario" {
		t.Fatalf("Get chiamato con service=%q user=%q", fake.getService, fake.getUser)
	}
}

func TestResolveAbsent(t *testing.T) {
	fake := &fakeStore{getErr: ErrNotFound}
	pw, src, err := Resolve(func(string) string { return "" }, fake, "mario")
	if err != nil {
		t.Fatalf("assenza non è un errore: %v", err)
	}
	if pw != "" || src != "" {
		t.Fatalf("atteso (\"\", \"\"), avuto (%q, %q)", pw, src)
	}
}

func TestResolvePropagatesKeychainError(t *testing.T) {
	want := errors.New("keychain bloccato")
	fake := &fakeStore{getErr: want}
	_, _, err := Resolve(func(string) string { return "" }, fake, "mario")
	if !errors.Is(err, want) {
		t.Fatalf("errore keychain deve propagarsi, avuto %v", err)
	}
}

func TestResolveQueriesEnvVarName(t *testing.T) {
	var asked string
	_, _, _ = Resolve(func(k string) string { asked = k; return "" }, &fakeStore{getErr: ErrNotFound}, "mario")
	if asked != "GMAIL_APP_PASSWORD" {
		t.Fatalf("getenv chiamato con %q, atteso %q", asked, "GMAIL_APP_PASSWORD")
	}
}

// --- keychain reale (darwin) ---

// TestRealKeychainRoundtrip esercita `security` contro il login keychain
// REALE con service/user dedicati ai test; skip su non-darwin o senza
// binario. Pulizia garantita da t.Cleanup.
func TestRealKeychainRoundtrip(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("keychain reale: solo su darwin")
	}
	if _, err := exec.LookPath("security"); err != nil {
		t.Skip("security non disponibile")
	}
	kr, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	const service = "gmail-tui-test"
	user := fmt.Sprintf("test-user-%d", os.Getpid())
	t.Cleanup(func() { _ = kr.Delete(service, user) })
	_ = kr.Delete(service, user) // stato pulito (l'assenza è ignorata)

	// assente → ErrNotFound
	if _, err := kr.Get(service, user); !errors.Is(err, ErrNotFound) {
		t.Fatalf("prima del Set: atteso ErrNotFound, avuto %v", err)
	}

	// Set → Get restituisce il segreto
	if err := kr.Set(service, user, "segreto-test-1"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := kr.Get(service, user)
	if err != nil {
		t.Fatalf("Get dopo Set: %v", err)
	}
	if got != "segreto-test-1" {
		t.Fatalf("roundtrip: atteso %q, avuto %q", "segreto-test-1", got)
	}

	// overwrite: Set → Get nuovo valore
	if err := kr.Set(service, user, "segreto-test-2"); err != nil {
		t.Fatalf("Set overwrite: %v", err)
	}
	if got, err = kr.Get(service, user); err != nil || got != "segreto-test-2" {
		t.Fatalf("dopo overwrite: avuto (%q, %v)", got, err)
	}

	// Delete → Get torna ErrNotFound
	if err := kr.Delete(service, user); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := kr.Get(service, user); !errors.Is(err, ErrNotFound) {
		t.Fatalf("dopo Delete: atteso ErrNotFound, avuto %v", err)
	}
}
