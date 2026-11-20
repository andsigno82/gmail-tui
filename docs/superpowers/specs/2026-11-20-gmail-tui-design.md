# gmail-tui — Design Spec

Data: 2026-11-20
Stato: bozza approvata verbalmente, in attesa di revisione finale

## Obiettivo

Un client di posta Gmail **completo** per il terminale: inbox, lettura thread,
risposta, composizione, ricerca, label, allegati. Open-source (MIT), donato alla
community, con configurazione minima e first-run guidato.

**Non-goals della v1**: multi-account, OAuth/Gmail API, calendario, contatti,
modalità CLI send-only (roadmap v1.1), provider non-Gmail.

## Principi

1. **Zero NIH** ("niente soluzioni custom"): prima una component di bubbles,
   poi una libreria battle-tested, codice custom solo come colla.
2. **Mouse first-class**: rotella per scroll, click per selezionare/aprire,
   focus input con click — tutto via il supporto mouse nativo di Bubble Tea v2.
3. **UI svincolata dal protocollo**: la UI non conosce IMAP; parla solo con
   l'interfaccia `MailStore` (domani: backend Gmail API senza toccare la UI).
4. **La rete non blocca mai la UI**: ogni operazione IMAP/SMTP gira in
   goroutine e torna alla TUI come `tea.Cmd`; spinner durante le attese.

## Stack (tutto verificato attivo a nov 2026)

| Pezzo              | Scelta                                    | Note |
|--------------------|-------------------------------------------|------|
| TUI                | `charm.land/bubbletea/v2`                 | stesso stack di pop, mouse nativo |
| Componenti         | `charm.land/bubbles/v2`                   | list, viewport, textinput, textarea, filepicker, help, spinner |
| Stili              | `charm.land/lipgloss/v2`                  | |
| Lettura email      | `github.com/emersion/go-imap/v2`          | IMAP TLS, IDLE; default branch = v2, attivo |
| Invio email        | `github.com/emersion/go-smtp`             | v0.26, nov 2026 |
| Parsing MIME       | `github.com/jhillyerd/enmime`             | attivo |
| HTML→testo         | libreria esistente (k3a/html2text o jaytaylor/html2text), deciso in implementazione | zero parser custom |
| Cache locale       | `go.etcd.io/bbolt`                        | puro Go, no CGO |
| Keychain (opt.)    | `github.com/zalando/go-keyring`           | salvataggio App Password |
| Config             | TOML (`BurntSushi/toml`) in `~/.config/gmail-tui/` | |

Riferimento per il composer: **charmbracelet/pop** (MIT) — schermata di
composizione adattata con attribuzione nel README.

## Architettura

```
gmail-tui/                          modulo: github.com/<utente>/gmail-tui
                                    (path definitivo legato al nome, v. Domande aperte)
├── cmd/gmail-tui/main.go           entrypoint, parsing flag
├── internal/
│   ├── app/                        modello root Bubble Tea, routing viste,
│   │                               stato globale, riconnessione
│   ├── ui/
│   │   ├── inbox/                  lista messaggi (bubbles/list)
│   │   ├── thread/                 lettura messaggio/thread (bubbles/viewport)
│   │   ├── composer/               composizione — adattato da pop (MIT)
│   │   ├── search/                 barra di ricerca
│   │   └── styles/                 tema lipgloss condiviso
│   ├── mail/
│   │   ├── store.go                interfaccia MailStore (+ tipi Message, Query…)
│   │   ├── imap/                   implementazione IMAP (lettura, flag, label)
│   │   ├── smtp/                   implementazione invio
│   │   └── cache/                  cache bbolt di corpi e header
│   ├── config/                     caricamento/validazione config, first-run wizard
│   └── msg/                        enmime → struct interna normalizzata, HTML→testo
└── docs/
```

### Interfaccia MailStore (contratto chiave)

```go
type MailStore interface {
    Connect(ctx context.Context) error
    ListMessages(ctx context.Context, q Query, pg Page) ([]MessageRef, error)
    FetchMessage(ctx context.Context, id MessageID) (*Message, error) // cache-first
    Search(ctx context.Context, q Query) ([]MessageRef, error)        // server-side
    MarkRead(ctx context.Context, ids []MessageID, read bool) error
    Archive(ctx context.Context, ids []MessageID) error
    Star(ctx context.Context, ids []MessageID, on bool) error
    ApplyLabel(ctx context.Context, ids []MessageID, label string, on bool) error
    SaveAttachment(ctx context.Context, id MessageID, part int, path string) error
    Send(ctx context.Context, m Outgoing) error
    Watch(ctx context.Context) (<-chan Change, error) // IMAP IDLE → push
}
```

## Modello dati e sincronizzazione

- All'avvio: connessione a `imap.gmail.com:993` (TLS), conteggio UNSEEN,
  header degli ultimi 50 messaggi (batch ENVELOPE).
- Corpi **lazy**: scaricati all'apertura, salvati in cache bbolt → navigazione
  istantanea alla seconda visita.
- Invalidazione cache per UID + `UIDVALIDITY` della mailbox.
- **IMAP IDLE** per il push: nuova mail/flag cambiati → evento su `Watch` →
  la lista si aggiorna da sola.
- **Thread**: estensione IMAP `THREAD=REFERENCES` (Gmail la supporta);
  fallback: raggruppamento su `In-Reply-To`/`References`/Subject normalizzato.
- **Label Gmail via IMAP** (approccio standard mutt/aerc): `\Inbox`, `\Starred`
  = flag; label personalizzate = cartelle; "archivia" = rimozione da INBOX.
- **Ricerca**: traduzione di `from:`, `to:`, `is:unread`, `label:`, `has:attachment`,
  testo libero → `IMAP SEARCH` server-side; parser query ≈ 100 righe, testato.

## UX / TUI

Layout a lista singola + status bar, ispirato a mutt/aerc ma con lo stile Charm:

```
┌─ Gmail ────────────────────────────────────── 12 unseen ─┐
│ ▸ ● Alice Rossi        Re: design review     10:32  [2] │
│ ▸ ○ GitHub             PR #42 merged          9:15       │
│ ▸ ● Luca Bianchi       Fwd: contratto         ieri       │
├───────────────────────────────────────────────────────────┤
│ j/k naviga · ↵ apri · r rispondi · c componi · / cerca   │
└───────────────────────────────────────────────────────────┘
```

**Keymap** (vim-ish, help sempre visibile con bubbles/help):

| Tasto | Azione |
|---|---|
| `j`/`k`, ↑/↓, rotella | naviga lista |
| `↵` / doppio click | apri messaggio/thread |
| `Esc`/`q` | indietro |
| `r` / `f` | rispondi / inoltra |
| `c` | componi nuovo |
| `/` | ricerca |
| `a` | archivia |
| `s` | preferito (star) |
| `v` | segna letto/non letto |
| `l` | applica/rimuovi label |
| `o` | apri nel browser |
| `?` | help completo |

**Mouse**: rotella = scroll (lista e reader), click = seleziona, doppio click
= apri, click su input composer = focus. Tutto nativo Bubble Tea v2.

**Lettura**: viewport scrollabile; header (Da/A/Oggetto/Data) stilizzato
lipgloss; thread = messaggi impilati; quote attenuate.

**Rendering corpo** (in ordine): parte `text/plain` → conversione HTML→testo
con libreria → fallback `o` apre nel browser.

**Composer** (adattato da pop): tab tra campi, textarea markdown, filepicker
allegati, anteprima pre-invio, firma da config.

**Allegati**: elencati con dimensione; `:save` (filepicker) e `:open` (app di sistema).

## Config e sicurezza

```toml
# ~/.config/gmail-tui/config.toml
address   = "tu@gmail.com"
name      = "Andrea"
signature = "Inviato con gmail-tui"
# host imap/smtp: preset Gmail, non esposti nella config
```

- Password: `GMAIL_APP_PASSWORD` env var **o** `gmail-tui login` → keychain
  di sistema (go-keyring). Mai in chiaro nel file config.
- **First-run wizard**: niente config → la TUI guida: link a
  myaccount.google.com/apppasswords, inserimento password, test connessione,
  scrittura config. Requisito documentato: 2FA attiva.
- Nessun progetto Google Cloud, nessun OAuth: zero attrito per l'utente.

## Gestione errori

- Disconnessione IMAP → riconnessione automatica con backoff esponenziale +
  indicatore in status bar.
- Timeout per ogni operazione di rete (context).
- Errore su singolo messaggio (MIME malformato ecc.) → skip + log, mai crash.
- Terminale sotto la dimensione minima → schermata di avviso.
- Errori utente (invio fallito ecc.) → messaggio in status bar, nulla di bloccante.

## Test

- **Unitari**: parser query ricerca, HTML→testo (golden file), envelope→struct,
  caricamento config.
- **UI**: `MailStore` mock in-memory; la TUI è testabile senza rete.
- **Integrazione**: GreenMail in Docker (IMAP+SMTP locali) in CI.
- **Smoke manuale**: checklist contro Gmail reale (documento in `docs/`).
- CI GitHub Actions: `golangci-lint`, `go vet`, `go test`, build cross-platform.

## Distribuzione / community

- **MIT** (compatibile con riuso codice pop) + attribuzione pop nel README.
- README inglese con GIF registrate con Charm VHS; Quickstart in 3 comandi.
- Release con goreleaser (binari macOS/Linux/Windows) + tap Homebrew.
- `CONTRIBUTING.md`, issue/PR templates, roadmap pubblica.
- Roadmap v1.1: modalità CLI send-only (`gmail-tui send --to … < body.md`).

## Rischi e mitigazioni

| Rischio | Mitigazione |
|---|---|
| Google depreca IMAP/App Password | `MailStore` astrae il backend → backend Gmail API (OAuth) aggiungibile senza toccare la UI |
| Email HTML complesse rese male | Fallback browser con `o`; rendering solo-testo è il default |
| API churn Bubble Tea v2 | Versioni pinate in go.mod; pop stesso usa v2 in produzione |
| Rate limit IMAP Gmail | IDLE invece di polling, fetch batch, backoff |

## Domande aperte

1. **Nome definitivo** del progetto ("gmail-tui" è provvisorio; serve un nome
   proprio per la community — decidere prima del primo annuncio pubblico).
2. Scelta finale libreria HTML→testo (k3a vs jaytaylor) — spike di 1 ora in
   fase implementativa, criterio: rendering link e grassetti su email reali.
