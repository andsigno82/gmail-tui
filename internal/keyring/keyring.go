// Package keyring salva e recupera la Gmail app password nel keychain di
// sistema tramite le CLI native, senza CGO né dipendenze Go aggiuntive:
//
//   - darwin: `security` (add/find/delete-generic-password)
//   - linux:  `secret-tool` (store/lookup/clear, libsecret)
//
// Ordine di risoluzione della password adottato dall'app (vedi Resolve):
// env GMAIL_APP_PASSWORD → keychain → assente (il chiamante decide:
// wizard nel Piano 4, istruzione export nell'uso manuale).
package keyring

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// ServiceName è il service con cui gmail-tui registra la app password nel
// keychain (identico per tutti i backend).
const ServiceName = "gmail-tui"

// ErrNotFound indica che il keychain non contiene il segreto richiesto.
// Mapping per backend:
//   - darwin (security): exit code 44, "not found" su stderr
//   - linux (secret-tool lookup): stdout vuoto con exit 0
var ErrNotFound = errors.New("segreto non presente nel keychain")

// Store è il keychain astratto. I backend reali usano le CLI di sistema; i
// test usano un runner finto per coprire su qualsiasi OS scelta backend,
// costruzione argomenti e path d'errore.
type Store interface {
	Get(service, user string) (string, error)
	Set(service, user, secret string) error
	Delete(service, user string) error
}

// runner esegue un comando esterno e restituisce stdout; stdin è passato al
// processo (secret-tool store legge il segreto da lì, security usa -w come
// argomento). Iniettabile nei test.
type runner func(name string, args []string, stdin []byte) ([]byte, error)

// execRunner è il runner di default: exec.Command con stdin opzionale.
// Gli errori exec sono wrapped in *cmdError (comando, exit code, stderr).
func execRunner(name string, args []string, stdin []byte) ([]byte, error) {
	cmd := exec.Command(name, args...)
	if len(stdin) > 0 {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		code := -1
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		}
		return out, &cmdError{name: name, args: args, code: code, stderr: stderr.String()}
	}
	return out, nil
}

// cmdError arricchisce l'errore exec con comando, exit code e stderr. Il
// messaggio maschera il valore dopo "-w": su darwin il segreto è un
// argomento e non deve finire in log o messaggi d'errore.
type cmdError struct {
	name   string
	args   []string
	code   int
	stderr string
}

func (e *cmdError) Error() string {
	return fmt.Sprintf("%s %s: exit %d: %s", e.name, maskSecretArgs(e.args), e.code, strings.TrimSpace(e.stderr))
}

// maskSecretArgs sostituisce con "****" il valore che segue "-w".
func maskSecretArgs(args []string) string {
	masked := make([]string, len(args))
	copy(masked, args)
	for i, a := range masked {
		if a == "-w" && i+1 < len(masked) {
			masked[i+1] = "****"
			break
		}
	}
	return strings.Join(masked, " ")
}

// isExitCode riporta true se err è un fallimento del runner con quel codice
// d'uscita (44 = "item not found" per security).
func isExitCode(err error, code int) bool {
	var ce *cmdError
	if errors.As(err, &ce) {
		return ce.code == code
	}
	var ee *exec.ExitError
	return errors.As(err, &ee) && ee.ExitCode() == code
}

// New restituisce lo Store per il keychain del sistema corrente:
// darwin → security, linux → secret-tool; su altri OS restituisce l'errore
// "keychain non supportata; usa GMAIL_APP_PASSWORD".
//
// L'ordine di risoluzione della password dell'app è documentato su Resolve
// (env GMAIL_APP_PASSWORD → keychain → assente): usare Resolve per applicarlo.
func New() (Store, error) {
	return newStore(runtime.GOOS, execRunner)
}

// newStore separa la scelta del backend dalla GOOS di runtime, così i test
// coprono entrambi i backend (con runner finto) su qualsiasi OS.
func newStore(goos string, run runner) (Store, error) {
	switch goos {
	case "darwin":
		return &darwinStore{run: run}, nil
	case "linux":
		return &linuxStore{run: run}, nil
	default:
		return nil, errors.New("keychain non supportata; usa GMAIL_APP_PASSWORD")
	}
}

// darwinStore usa /usr/bin/security sul login keychain.
type darwinStore struct{ run runner }

func (s *darwinStore) Get(service, user string) (string, error) {
	out, err := s.run("security", []string{"find-generic-password", "-s", service, "-a", user, "-w"}, nil)
	if err != nil {
		if isExitCode(err, 44) { // "not found" su stderr, exit 44
			return "", ErrNotFound
		}
		return "", err
	}
	return strings.TrimSuffix(string(out), "\n"), nil
}

// Set sovrascrive: add-generic-password fallisce (exit 45) se l'item esiste
// già, quindi delete-then-add rende Set idempotente.
func (s *darwinStore) Set(service, user, secret string) error {
	if err := s.Delete(service, user); err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	_, err := s.run("security", []string{"add-generic-password", "-s", service, "-a", user, "-w", secret}, nil)
	return err
}

func (s *darwinStore) Delete(service, user string) error {
	_, err := s.run("security", []string{"delete-generic-password", "-s", service, "-a", user}, nil)
	if err != nil && isExitCode(err, 44) {
		return ErrNotFound
	}
	return err
}

// linuxStore usa secret-tool (libsecret); il segreto passa su stdin.
type linuxStore struct{ run runner }

func (s *linuxStore) Get(service, user string) (string, error) {
	out, err := s.run("secret-tool", []string{"lookup", "service", service, "username", user}, nil)
	if err != nil {
		return "", err
	}
	secret := strings.TrimSuffix(string(out), "\n")
	if secret == "" { // lookup senza risultato: stdout vuoto, exit 0
		return "", ErrNotFound
	}
	return secret, nil
}

func (s *linuxStore) Set(service, user, secret string) error {
	args := []string{"store", "--label=" + service, "service", service, "username", user}
	_, err := s.run("secret-tool", args, []byte(secret))
	return err
}

func (s *linuxStore) Delete(service, user string) error {
	_, err := s.run("secret-tool", []string{"clear", "service", service, "username", user}, nil)
	return err
}

// Resolve applica l'ordine di risoluzione della password dell'app:
//
//  1. env GMAIL_APP_PASSWORD (via getenv) → (segreto, "env", nil)
//  2. keychain ServiceName/user (via kr.Get) → (segreto, "keychain", nil)
//  3. nessuna delle due → ("", "", nil): la decisione spetta al chiamante
//     (wizard nel Piano 4, istruzione export nell'uso manuale).
//
// Un errore del keychain diverso da ErrNotFound viene propagato: resta al
// chiamante decidere se degradare ad "assente".
func Resolve(getenv func(string) string, kr Store, user string) (secret, source string, err error) {
	if pw := getenv("GMAIL_APP_PASSWORD"); pw != "" {
		return pw, "env", nil
	}
	pw, err := kr.Get(ServiceName, user)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", "", nil
		}
		return "", "", err
	}
	return pw, "keychain", nil
}
