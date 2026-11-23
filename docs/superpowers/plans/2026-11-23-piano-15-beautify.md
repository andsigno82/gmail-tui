# gmail-tui — Piano 15: beautify con lipgloss (HTML ricco, cornici, stato) — Implementation Plan

> **For agentic workers:** branch `feat/piano-15` (da `e1cdb5e`). Leggere `AGENTS.md`. TDD vero: per ogni task il test scritto prima va visto rosso (RED riportato nel report), poi verde. Commit per task, checkbox `- [ ]`. Gate dopo ogni task: `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...`, `golangci-lint run ./...` a 0 issues. Le decisioni D15-01…D15-13 sono di Andrea: non riaprirle. Le decisioni D15-T* sono prese in questo piano e si possono contestare nel review. Le domande Q15-* richiedono Andrea prima del Task indicato.

**Goal:** rendere la TUI leggibile e curata senza cambiare cosa mostra. Due fronti. (A) Contenuto: l'HTML dei messaggi diventa un modello a blocchi (paragrafi, link con testo e href, bottoni, tabelle, elenchi, immagini, citazioni) renderizzato con lipgloss: bottoni a riquadro con host di destinazione, tabelle con `lipgloss/table`, citazioni con barra colorata annidata, elenchi con `lipgloss/list`, intestazione a scheda con badge, allegati come chip cliccabili, link con controllo di dominio. (B) Cornici: selettore cartelle come albero con contatori dei non letti, finestre sovrapposte con ombra, barra di stato a segmenti, sfumature solo su separatori, bordi e titolo del pannello.

**Stack:** nessuna dipendenza nuova. `charm.land/lipgloss/v2 v2.1.1` (già diretta), `golang.org/x/net v0.63.0` (già diretta: `html`, `html/atom`, `publicsuffix`, `idna`). Nessun CGO. `github.com/k3a/html2text` resta per `BodyText` (MCP e fallback testo semplice).

## Stato attuale (verificato sul codice)

| Punto | Fatto | Riferimento |
|---|---|---|
| Corpo HTML | `Parse` mette `BodyText` = html2text (solo testo, ancore perse). `BodyHTML` è l'HTML originale, popolato se il MIME ha una parte HTML | `internal/msg/parse.go` (regola corpo) |
| Cache | `json.Marshal` dell'intero `msg.Message`: `BodyHTML` è già in cache, lo schema non cambia se il modello a blocchi si ricava a render time | `internal/mail/cache/cache.go:94`, `schemaVersion = 3` (riga 29) |
| Render corpo | `renderConversation` passa `BodyText` a `bodyRows` (a capo + link) | `internal/ui/thread/conversation.go:146-174`, `internal/ui/thread/links.go` (`bodyRows` riga 273) |
| Mappa clic | `linkSpan{x0,x1,url}` per riga viewport, `LinkAt(x,y)`, `LinkClick`, OSC 8 con id per link (`osc8Open`) | `internal/ui/thread/links.go` (righe 33-38, 244-268, 290-325) |
| Layout thread | `headerLines = 5` usato da `LinkAt` per l'offset delle righe | `internal/ui/thread/thread.go:29`, `renderHeader` riga 309, `renderAttachments` riga 329 |
| Overlay | `overlayToasts` già usa `lipgloss.NewCompositor` | `internal/app/model.go:1681-1694` |
| Barra di stato | `statusBar` testo + `renderBar` (colori solo per conferme modali) | `internal/app/model.go:1766` |
| Cartelle | selettore `m` = `bubbles/list` piatta con `folderName` (ruoli tradotti) | `internal/app/folders.go:29-90` |
| Contratto mailbox | `mail.Mailbox{Name, Role}`: manca il delimitatore IMAP (serve per l'albero) | `internal/mail/store.go` (struct `Mailbox`), `internal/mail/imap/mailboxes.go:34` |
| Tema | 14 slot: fg, bg, muted, border, primary, accent, error, success, warning, unread, starred, quoted, help, selection | `internal/theme/theme.go:44-57` |
| Contrasto | test tabellare esistente: `TestContrastoStiliConSfondo` su tutti gli stili con sfondo | `internal/ui/styles/styles_test.go:180` |
| MCP | legge `BodyText` con `Sanitize` (testo semplice): non deve cambiare | `internal/mcpserver/tools.go:490,535`, `internal/mcpserver/untrusted.go:46` |

## API lipgloss v2.1.1 verificate nel module cache

Percorso: `/Users/andrea/go/pkg/mod/charm.land/lipgloss/v2@v2.1.1/`.

| API | Dove | Uso nel piano |
|---|---|---|
| `Style.Hyperlink(link, params...)` | `set.go:820` | OSC 8 su bottoni e chip (`osc8Open` di `links.go` resta per il testo) |
| `Style.UnderlineStyle(Underline)`, `Style.UnderlineColor(color.Color)` | `set.go:223`, `set.go:234` | link tratteggiati, link ondulati in colore errore |
| `UnderlineCurly`, `UnderlineDashed`, `UnderlineDotted` | `style.go:117-131` | tipi di sottolineatura (vedi Q15-3: supporto del terminale) |
| `Style.BorderForegroundBlend(...)`, `BorderForegroundBlendOffset(int)` | `set.go:628`, `set.go:655` | sfumatura sui bordi delle finestre (D15-13) |
| `Blend1D(steps int, stops ...color.Color) []color.Color` | `blending.go:18` | sfumature primary→accent per separatori e titolo |
| `NewLayer(content, layers...)`, `Layer.X/Y/Z/ID`, `NewCompositor(layers...)` | `layer.go:22`, `:206` | finestre sovrapposte e ombra (D15-11) |
| `Lighten(c, pct)`, `Darken(c, pct)`, `Alpha(c, a)` | `color.go:340`, `:323`, `:287` | ombra e colori di livello della citazione derivati dal tema (D15-03, D15-11) |
| `table.New()`, `.Width(w)`, `.FitContent()`, `.BorderStyle`, `.StyleFunc` | `table/table.go:81`, `:258`, `:270` | tabelle (D15-02). `Width` è "best effort" con ridimensionamento colonne: il piano aggiunge la soglia di ricaduta |
| `list.New(items...)`, `list.Enumerator`, `list.Indenter`, `list.Bullet/Dash/Arabic/Alphabet/Roman` | `list/list.go:50`, `list/enumerator.go` | elenchi (D15-04) |
| `tree.New`/`tree.Root`/`tree.Child`, `tree.RoundedEnumerator` | `tree/tree.go`, `tree/enumerator.go:36` | albero cartelle (D15-09) |

API di x/net verificate: `publicsuffix.EffectiveTLDPlusOne(domain) (string, error)` in `golang.org/x/net@v0.63.0/publicsuffix/list.go:173`. Serve anche `idna` per la normalizzazione (D15-T6).

## Decisioni

### Decisioni di Andrea (non riaprirle)

| ID | Voce | Realizzazione nel piano |
|---|---|---|
| D15-01 | Bottoni come riquadri (bordo arrotondato, padding, colori del tema), host accanto (`→ amazon.it`), cliccabili e OSC 8 | Task 7 (renderer), Task 4 (rilevazione), Task 14 (clic) |
| D15-02 | Tabelle con `lipgloss/table`, ricaduta su testo se troppo larghe | Task 8 (soglia in D15-T8) |
| D15-03 | Citazioni con barra laterale colorata, livelli annidati con colori derivati dal tema | Task 8 (`quote.go`), Task 9 (colori derivati) |
| D15-04 | Elenchi `<ul>`/`<ol>` con `lipgloss/list` | Task 8 (`list.go`) |
| D15-05 | Intestazione a scheda con badge delle iniziali (colore dal tema come le label) | Task 14 (`thread.go`), colori Task 9 |
| D15-06 | Allegati come chip cliccabili: clic = apri, `:save` resta | Task 7 (`attachment.go`), Task 16 (apertura, Q15-1) |
| D15-07 | Link sottolineati tratteggiati; ondulati in colore errore se il testo mostra un dominio diverso da quello reale (confronto per dominio registrabile, `publicsuffix`) | Task 6 (`link.go`) |
| D15-08 | Immagini come chip con il testo alternativo; nessuna richiesta di rete | Task 6 (`image.go`), Task 4 (parser) |
| D15-09 | Selettore `m` come albero di cartelle e label annidate (`lipgloss/tree`) | Task 10 (`foldertree`), Task 15 (integrazione) |
| D15-10 | Contatori dei non letti per cartella: STATUS UNSEEN, nuovo metodo di LETTURA del contratto; MCP invariato | Task 2 (contratto), Task 11 (IMAP), Task 15 |
| D15-11 | Finestre sovrapposte con ombra per menu `,`, pannello label e selettore cartelle (compositor, ombra dal tema) | Task 12 (`overlay`), Task 16 (uso) |
| D15-12 | Barra di stato a segmenti in stile powerline sobrio | Task 13 (`statusline`), Task 16 (uso), Q15-2 |
| D15-13 | Sfumature sobrie primary→accent solo su separatori, bordi delle finestre e titolo del pannello, mai sul testo | Task 12, Task 13, Task 16 |

### Decisioni tecniche del piano (D15-T*)

| ID | Scelta | Motivazione |
|---|---|---|
| D15-T1 | Modello a blocchi in `internal/msg/richtext`, costruito da `BodyHTML` con `golang.org/x/net/html` (tokenizer, nessun CSS eseguito, nessun `<style>` applicato). Il corpo dei messaggi solo testo usa lo stesso modello (`FromText`). | Un solo modello per tutti i renderer. `BodyText` (html2text) resta per MCP, ricerca e fallback: il server MCP non importa né `richtext` né `render`. |
| D15-T2 | Il modello non si salva in cache: si ricava da `BodyHTML` (già in cache) e si memorizza in un cache di rendering in memoria per `(MessageID, larghezza, tema, variante)`, con dimensione massima fissa (16 voci). | Nessun cambio di `schemaVersion`. Il cache è un puntatore creato in `New` (gotcha dei value receiver di bubbletea, AGENTS.md). |
| D15-T3 | Contratto renderer: `func(b richtext.Block, c render.Ctx) render.Out`. `render.Out` = righe già stilizzate + zone cliccabili per riga (`[][]render.Zone`). `render.Zone` generalizza `linkSpan` (x0, x1, kind, url, id). `LinkAt(x,y)` resta con la stessa firma. | Un'unica mappa clic per link, bottoni, chip. Stesso contratto di `links.go`, quindi nessun cambio al test di `LinkAt`. |
| D15-T4 | Limiti di input prima del parsing: HTML ≤ 512 KiB (oltre: si parsano i primi 512 KiB e il corpo mostra "[messaggio troncato: N KiB]"); ≤ 5 000 nodi; annidamento ≤ 16 livelli; tabelle ≤ 200 righe × 20 colonne (oltre: testo). | Email grandi non bloccano la UI. Valori da tarare con misure reali (Task 19). |
| D15-T5 | Rendering fuori dal loop: `thread` lancia un `tea.Cmd` che ritorna `RenderedMsg{ID, Width, Gen, Out}`; i risultati stantii (gen diverso o messaggio chiuso) sono scartati, come da AGENTS.md. Finché non arriva, si mostra il testo semplice (`BodyText`) con spinner. | UI non bloccata; il fallback testo rende la transizione sicura. |
| D15-T6 | Phishing (D15-07): il testo del link è considerato un URL/dominio se parsa come URL o è un dominio nudo. Entrambi i lati si normalizzano con `idna.Lookup` (ASCII) e poi `publicsuffix.EffectiveTLDPlusOne`. Se il dominio registrabile non si calcola (IP, suffisso pubblico), si confronta l'host esatto. Mismatch = zona con `Warn` + marcatore testuale `⚠` (mai solo colore). | Il colore non basta (accessibilità e terminali senza sottolineatura colorata, Q15-3). Normalizzazione IDN contro le omografie. |
| D15-T7 | Euristiche bottone (D15-01), solo su attributi HTML e sull'attributo `style` inline, mai su CSS: `a` con `href` e (`background`/`background-color` in `style`) e (`padding` o `border-radius` in `style`); `td` con `bgcolor` che contiene esattamente un link; `role="button"`; `class` che contiene il token `btn` o `button`. Ogni regola ha un test su fixture anonimizzate. | Le regole sono quelle richieste da Andrea; il perimetro (solo attributi) evita di eseguire CSS. |
| D15-T8 | Tabelle (D15-02): `table.New().Width(w).FitContent()` con `w` = larghezza disponibile. Ricaduta su testo ("Intestazione: valore" per riga) se la somma delle larghezze minime di colonna (parola più lunga per colonna) supera la larghezza disponibile. | La soglia è verificabile; `Width` di lipgloss da solo comprimerebbe a parole spezzate. |
| D15-T9 | Il delimitatore IMAP entra in `mail.Mailbox` (`Delimiter string`), riempito da `ListMailboxes` (LIST). L'albero si costruisce splittando il nome sul delimitatore. Gmail usa `/`. | Senza delimitatore l'albero è impossibile senza euristiche sul nome. |
| D15-T10 | Contatori (D15-10): nuovo metodo `UnreadCounts(ctx) (map[string]int, error)` su `MailStore`, implementato con STATUS (UNSEEN) per ogni mailbox in un solo round del gate. Non esposto al server MCP (il Reader resta com'è). Si chiama all'apertura e dopo ogni `Change` (ChangeNewMail, ChangeFlags). | Il conteggio è un dato di lettura, non di scrittura; nessun EXPUNGE. |
| D15-T11 | Ombra (D15-11): un `Layer` di ombra con colore `Darken(tema.Bg, …)` spostato di (+2, +1) sotto la finestra. Le celle del terminale non hanno alpha: niente trasparenza vera. Verifica in Task 12 (spike) che un `Layer` con celle di spazio stilizzate copra il contenuto sottostante. | Lipgloss offre `Alpha` ma il terminale non compone alpha: l'ombra è una cella colorata. |
| D15-T12 | Sfumature (D15-13) con `Blend1D(n, primary, accent)` solo su: caratteri separatori, `BorderForegroundBlend` delle finestre, titolo del pannello. Mai sul testo del corpo o delle righe. | Richiesta di Andrea; il testo resta su slot con contrasto garantito. |
| D15-T13 | Sanitizzazione (AGENTS.md: contenuto non fidato): in testi e URL si tolgono controlli, spazi, caratteri bidi (U+202A–U+202E, U+2066–U+2069, U+200E/U+200F) e zero-width (U+200B–U+200D, U+FEFF). Gli href si accettano solo se `http`, `https`, `mailto` (come `linkTarget`). Nessuna richiesta di rete, `src` delle immagini mai usato. | Un link con bidi o zero-width nasconde il dominio reale (il controllo D15-07 lo deve vedere). |
| D15-T14 | Allegati: il chip mostra nome (sanificato, ≤ 40 colonne), tipo e dimensione. Clic = apertura (Task 16) solo se il tipo è in allowlist (Q15-1); altrimenti toast "tipo non aperto: usa :save". Il file si scrive in una cartella temporanea `0700` con nome generato, mai eseguito. | Su macOS `open` esegue `.app`, `.command`, `.pkg`: il tipo non si decide dal nome. |

## Global Constraints

- `mail.MailStore` resta l'unico contratto della UI. `internal/app` e `internal/ui` non importano `go-imap` né `internal/mail/imap`.
- Nuovi metodi di `MailStore`: solo `UnreadCounts` (lettura) e il campo `Mailbox.Delimiter`. Nessun metodo di scrittura nuovo. `internal/mcpserver` non importa `internal/msg/richtext` né `internal/ui/*` (test di isolamento, Task 17).
- UI solo con slot del tema (14 in `internal/theme`). I colori derivati (`Lighten`, `Darken`, `Blend1D`, `Alpha`) partono da uno slot. Ogni stile con sfondo passa `TestContrastoStiliConSfondo` (WCAG ≥ 4.5:1) su ogni tema, dark e light. Le sfumature mai sul testo.
- Nessun segnale solo colore: link ondulato = colore + `⚠` nel testo; bottone = bordo + testo; badge = iniziali.
- Contenuto email non fidato: nessuna richiesta di rete (le immagini NON si scaricano), nessun CSS eseguito, sanificazione di testi e URL (D15-T13), limiti (D15-T4). Test su fixture anonimizzate in `internal/msg/richtext/testdata/html/` (nessun messaggio reale, nessun indirizzo reale).
- Mappa clic: ogni nuovo elemento cliccabile passa da `render.Zone` e da `LinkAt`/`LinkClick` (`internal/ui/thread/links.go`). Nessuna seconda mappa. Mouse disattivato con modali aperte e guard sugli esiti stantii: preservati (AGENTS.md).
- Layout del thread: `headerLines` cambia solo nel Task 14, con il test di `LinkAt` aggiornato nello stesso commit.
- Commenti e doc del codice in italiano con riferimento "Piano 15, Task N". Stringhe utente in italiano. README/CHANGELOG in inglese.
- Gotcha: value receiver su bubbletea (`Model` value); `g`/`G` e `f`/`b` occupati (`internal/app/keys.go`).
- Nessun CGO. `CGO_ENABLED=0` su linux, darwin, windows. `govulncheck ./...` pulito (x/net è già in uso).
- Ops IMAP nuove (UNSEEN, LIST con delimitatore) passano dal gate (`idleGate`), come le altre.

## File Structure

Nuovi:

| File | Ruolo | Task |
|---|---|---|
| `internal/msg/richtext/model.go` | tipi del modello a blocchi, limiti, firme `FromHTML`/`FromText` (corpo a return vuoto) | 1 |
| `internal/msg/richtext/html.go`, `html_test.go` | parser HTML → blocchi (x/net/html) | 4 |
| `internal/msg/richtext/button.go`, `button_test.go` | euristiche bottone (D15-T7) | 4 |
| `internal/msg/richtext/sanitize.go`, `sanitize_test.go` | sanificazione testi e URL (D15-T13) | 5 |
| `internal/msg/richtext/text.go`, `text_test.go` | testo semplice → blocchi (citazioni `>`, elenchi `-`/`1.`) | 5 |
| `internal/msg/richtext/testdata/html/*.html` | fixture anonimizzate | 4 |
| `internal/ui/render/render.go`, `zone.go`, `zone_test.go` | `Ctx`, `Out`, `Zone`, `Stack`/`Indent` | 3 |
| `internal/ui/render/link.go`, `link_test.go` | link con controllo di dominio (D15-07) | 6 |
| `internal/ui/render/image.go`, `image_test.go` | immagine come chip con alt | 6 |
| `internal/ui/render/button.go`, `button_test.go` | bottone a riquadro + host | 7 |
| `internal/ui/render/attachment.go`, `attachment_test.go` | chip allegato | 7 |
| `internal/ui/render/table.go`, `table_test.go` | tabelle e ricaduta (D15-T8) | 8 |
| `internal/ui/render/quote.go`, `quote_test.go` | citazioni annidate | 8 |
| `internal/ui/render/list.go`, `list_test.go` | elenchi | 8 |
| `internal/ui/foldertree/tree.go`, `counts.go`, `*_test.go` | albero cartelle e badge dei non letti | 10 |
| `internal/mail/imap/unseen.go`, `unseen_test.go` | `UnreadCounts` (STATUS UNSEEN) | 11 (stub in 2) |
| `internal/ui/overlay/overlay.go`, `overlay_test.go` | finestra + ombra (compositor) | 12 |
| `internal/ui/statusline/statusline.go`, `statusline_test.go` | segmenti della barra di stato | 13 |
| `internal/ui/thread/render_cache.go`, `render_cache_test.go` | cache di rendering per larghezza (D15-T2, T5) | 14 |
| `internal/app/attachment_open.go`, `attachment_open_test.go` | apertura allegato sicura (D15-T14) | 16 |
| `internal/app/unread.go`, `unread_test.go` | ricarica contatori su Change | 15 |

Modificati:

| File | Modifica | Task |
|---|---|---|
| `internal/mail/store.go` | `Mailbox.Delimiter`; metodo `UnreadCounts` | 2 |
| `internal/mail/mock.go` | stub e dati di `UnreadCounts`, `Delimiter` nei mailbox mock | 2, 17 |
| `internal/mail/imap/unseen.go` | stub che ritorna errore non implementato (firma) | 2 |
| `internal/mail/imap/mailboxes.go` | `Delimiter` da LIST | 11 |
| `internal/ui/styles/styles.go` | nuovi campi (solo nomi e zero value) poi valori derivati | 2, 9 |
| `internal/ui/styles/styles_test.go` | estensione di `TestContrastoStiliConSfondo` | 9 |
| `internal/ui/thread/links.go` | `linkSpan` → `render.Zone`; `LinkAt` invariato come firma | 14 |
| `internal/ui/thread/conversation.go` | corpo da `richtext` + `render`, cache, `RenderedMsg` | 14 |
| `internal/ui/thread/thread.go` | scheda intestazione con badge, chip allegati, `headerLines` | 14 |
| `internal/app/folders.go` | selettore `m` su `foldertree`, contatori | 15 |
| `internal/app/model.go` | `RenderedMsg` e `UnreadCounts` nel dispatch; mouse su `Zone` Button/Attachment; `statusBar` su `statusline`; overlay | 15, 16 |
| `internal/app/settings.go`, `labels_picker.go`, `folders.go` | finestre su `overlay` | 16 |
| `internal/app/demo.go` | messaggio demo con HTML ricco (fixture), contatori demo | 17 |
| `internal/mcpserver/*_test.go` | test di isolamento dei renderer | 17 |
| `README.md`, `CHANGELOG.md`, `docs/smoke-checklist.md`, `AGENTS.md` | documentazione | 18 |

## Waves

Ogni wave gira in parallelo solo tra task con file disgiunti (colonna "File"). Il Wave 1 fissa le interfacce: nessun agente parallelo parte prima che i suoi contratti siano committati.

| Wave | Task | File (ownership esclusiva) | Dipende da |
|---|---|---|---|
| 1a | 1 (modello), 2 (contratti mail e stili) | `internal/msg/richtext/model.go`; `internal/mail/*`, `internal/ui/styles/styles.go` (solo campi) | — |
| 1b | 3 (contratto render) | `internal/ui/render/render.go`, `zone.go` | 1, 2 |
| 2a | 4 (HTML), 5 (testo e sanificazione) | `internal/msg/richtext/*` (un agente per tutto il package) | 1 |
| 2b | 6 (link, immagini), 7 (bottoni, allegati), 8 (tabelle, citazioni, elenchi) | `{link,image}.go`; `{button,attachment}.go`; `{table,quote,list}.go` (un agente per gruppo di file) | 3 |
| 2c | 9 (stili e contrasto) | `internal/ui/styles/*` | 2 |
| 2d | 10 (albero cartelle) | `internal/ui/foldertree/*` | 2 |
| 2e | 11 (IMAP delimitatore e UNSEEN) | `internal/mail/imap/{mailboxes,unseen}.go` | 2 |
| 2f | 12 (overlay), 13 (statusline) | `internal/ui/overlay/*`; `internal/ui/statusline/*` | 2 (colori: 9) |
| 3a | 14 (thread) | `internal/ui/thread/*` | 3, 4, 5, 6, 7, 8, 9 |
| 3b | 15 (cartelle e contatori in app) | `internal/app/folders.go`, `unread.go`, hook in `model.go` | 10, 11, 14 |
| 3c | 16 (finestre, statusline, clic, apertura allegati) | `internal/app/{model,settings,labels_picker,folders,attachment_open}.go` | 12, 13, 14, 15 |
| 4a | 17 (demo, Mock, isolamento MCP) | `internal/app/demo.go`, `internal/mail/mock.go`, `internal/mcpserver/*_test.go` | 16 |
| 4b | 18 (documentazione) | `README.md`, `CHANGELOG.md`, `docs/smoke-checklist.md`, `AGENTS.md` | 16 |
| 4c | 19 (gate, misure, revisione) | nessun file di produzione | 17, 18 |

Nota: `internal/app/model.go` è il file più conteso. Wave 3b e 3c sono sequenziali (un solo agente); nessun altro task lo tocca.

## Task 1: modello a blocchi (`internal/msg/richtext`)

**Produces:** `model.go` con i tipi (nessuna logica):

- `type Kind uint8` con `KindParagraph`, `KindQuote`, `KindList`, `KindTable`, `KindButton`, `KindImage`, `KindRule`, `KindPre`.
- `type Run struct { Text, Href string }`: `Href == ""` = testo; altrimenti link (testo + href sanificato).
- `type Block struct { Kind Kind; Runs []Run; Children []Block; Ordered bool; Start int; Rows [][]Cell; Host string; Alt string; Depth int; Warn bool }`, con `type Cell struct { Runs []Run; Header bool }` (i campi usati solo dal Kind che li richiede sono indicati nei commenti).
- Costanti dei limiti (D15-T4): `MaxInputBytes = 512 << 10`, `MaxNodes = 5000`, `MaxDepth = 16`, `MaxTableRows = 200`, `MaxTableCols = 20`.
- Firme: `func FromHTML(html string) (blocks []Block, truncated bool)` e `func FromText(text string) []Block` con corpo `return nil, false` / `return nil` (sostituiti nei Task 4 e 5).

**Files:** `internal/msg/richtext/model.go` (nuovo).

**Test:** `model_test.go`: il valore zero di `Block` ha `Kind == KindParagraph` senza run (invariante documentata); le costanti dei limiti sono quelle di D15-T4 (test che le fissa). RED: il package non compila prima del file.

- [x] Scrivere `model_test.go` (RED).
- [x] Scrivere `model.go` con tipi, costanti e firme.
- [x] Gate.
- [x] Commit `feat(richtext): modello a blocchi (Piano 15, Task 1)`.

## Task 2: contratti mail e stili (solo firme)

**Produces:**

- `mail.Mailbox` guadagna `Delimiter string` (commento: IMAP LIST, "/" per Gmail; vuoto se non noto).
- `MailStore` guadagna `UnreadCounts(ctx context.Context) (map[string]int, error)`: chiave = `Mailbox.Name`, valore = messaggi non letti; mailbox assenti dalla mappa = non calcolate.
- `internal/mail/imap/unseen.go`: `func (s *Store) UnreadCounts(ctx context.Context) (map[string]int, error)` che ritorna `errors.New("non implementato")`. Implementazione nel Task 11.
- `internal/mail/mock.go`: stub che ritorna mappa vuota (dati di demo nel Task 17).
- `internal/ui/styles/styles.go`: campi nuovi di `Styles` (solo dichiarazione, zero value): `LinkWarn`, `LinkDashed`, `Button`, `ButtonFocus`, `ButtonHost`, `ChipAttach`, `ChipImage`, `QuoteBar []lipgloss.Style` (un elemento per livello), `TableBorder`, `TableHeader`, `TableCell`, `BadgeAvatar []lipgloss.Style`, `FolderCount`, `FolderTree`, `OverlayBorder`, `OverlayShadow`, `StatusSegment []lipgloss.Style`, `StatusSep`. Il nome dei campi è il contratto per i Task 6-16 (`Divider` esiste già: non duplicare).

**Files:** `internal/mail/store.go`, `internal/mail/mock.go`, `internal/mail/imap/unseen.go` (nuovo, stub), `internal/ui/styles/styles.go` (solo campi).

**Test:** `go build ./...` e `go vet ./...` verdi; l'assertion a compile time che Mock e imap.Store implementano `MailStore` resta verde. Nessun test di comportamento.

- [x] Modificare `store.go` (campo e metodo con commento in italiano e riferimento Piano 15, Task 2).
- [x] Stub in `mock.go` e `imap/unseen.go`.
- [x] Campi in `styles.go` (senza valori: la valorizzazione è Task 9).
- [x] Gate (incluso `grep -rn "go-imap" internal --include="*.go" -l | grep -v internal/mail/imap` vuoto).
- [x] Commit `feat(mail,styles): contratti del Piano 15 (Task 2)`.

## Task 3: contratto del renderer (`internal/ui/render`)

**Produces:** `render.go` e `zone.go`:

- `type Ctx struct { Width int; St styles.Styles; Depth int }` (larghezza disponibile in colonne, stili del tema, livello di annidamento).
- `type ZoneKind uint8`: `ZoneLink`, `ZoneButton`, `ZoneAttachment`, `ZoneImage`.
- `type Zone struct { X0, X1 int; Kind ZoneKind; URL string; Index int; ID string; Warn bool }` (`ID` = id OSC 8 comune; `Index` = indice allegato).
- `type Out struct { Lines []string; Zones [][]Zone }`, con `len(Zones) == len(Lines)`.
- `func Stack(outs ...Out) Out` (concatena righe e zone) e `func Indent(o Out, pad int) Out` (sposta le zone di `pad` colonne e prefissa le righe).
- Firme dei renderer (corpo vuoto, riempiti nei Task 6-8): `Paragraph(b richtext.Block, c Ctx) Out`, `Link`, `Image`, `Button`, `Attachment`, `Table`, `Quote`, `List`, più `Dispatch(b richtext.Block, c Ctx) Out` che fa lo switch per `Kind`.

**Files:** `internal/ui/render/render.go`, `zone.go`, `zone_test.go` (nuovi).

**Test:** `zone_test.go`: `Stack` concatena righe e zone; `Indent` sposta le `X0/X1` di `pad` e mantiene `len(Zones)==len(Lines)`. RED: test prima dell'implementazione di `Stack`/`Indent`.

- [x] Test RED in `zone_test.go`.
- [x] `render.go` e `zone.go`: tipi, `Stack`, `Indent`, firme con corpo vuoto.
- [x] Gate.
- [x] Commit `feat(render): contratto dei renderer (Piano 15, Task 3)`.

## Task 4: parser HTML → blocchi e bottoni (`internal/msg/richtext`)

**Files:** `html.go`, `html_test.go`, `button.go`, `button_test.go`, `testdata/html/*.html` (nuovi).

**Produces:**

- `FromHTML`: tokenizer `golang.org/x/net/html`; ignora `script`, `style`, `head`, `title`, `noscript`, `iframe`, `object`, `embed`, `svg`, `form`; `img` → `KindImage` con solo `Alt` (il `src` non si legge né si usa); `hr` → `KindRule`; `p`/`div`/`br`/`h1-h6` → paragrafi; `blockquote` → `KindQuote` annidata (Depth); `ul`/`ol` → `KindList` (`Ordered`, `Start`); `table` → `KindTable` (righe e celle; `th` = Header; tabelle nidificate appiattite in testo nella cella); `pre` → `KindPre`; `a[href]` → Run con Href sanificato (`sanitize.go`, Task 5: il Task 4 può usare la funzione con il corpo del Task 5 o test dipendenti dal Task 5 per i link).
- Limiti D15-T4: input troncato a `MaxInputBytes` con `truncated = true`; nodi oltre `MaxNodes` ignorati; profondità oltre `MaxDepth` appiattita.
- `button.go`: `isButton(n *html.Node) bool` con le regole di D15-T7; un bottone diventa `KindButton` (Runs = testo, Href, Host) e non è più un link di paragrafo.
- Fixture anonimizzate (`testdata/html/`): `newsletter.html` (tabelle, bottoni `a` con style), `table_td_bgcolor.html` (td con bgcolor + un link), `role_button.html`, `class_btn.html`, `inline_link.html`, `quote_nested.html`, `img_alt.html`, `huge.html` (oltre il limite). Domini di esempio `example.com`, `example.org`.

**Test (RED prima di ogni regola):** una fixture per regola D15-T7; un `a` senza style resta link di paragrafo; `td` con bgcolor e due link NON è bottone; `class="btndark-theme"` (token parziale) NON è bottone; `script`/`style` non producono testo; limiti (troncamento, `truncated`); nessun `src` finisce nel modello.

- [ ] Fixture e test RED.
- [ ] Implementazione `html.go`, `button.go`.
- [ ] Gate.
- [ ] Commit `feat(richtext): parser HTML a blocchi e bottoni (Piano 15, Task 4)`.

## Task 5: testo semplice e sanificazione (`internal/msg/richtext`)

**Files:** `sanitize.go`, `sanitize_test.go`, `text.go`, `text_test.go` (nuovi).

**Produces:**

- `sanitizeText(s string) string` e `sanitizeURL(s string) (string, bool)`: tolgono controlli (tranne `\n` nel testo), spazi nell'URL, caratteri bidi e zero-width (D15-T13); `sanitizeURL` accetta solo `http`, `https`, `mailto` con host/opaque non vuoto e ≤ 2048 byte (stessa logica di `linkTarget` in `internal/ui/thread/links.go`: il Task 14 fa puntare `linkTarget` a questa funzione, senza duplicarla).
- `FromText(text string) []Block`: righe che iniziano con `>` → `KindQuote` (Depth = numero di `>` iniziali, massimo `MaxDepth`); `- `/`* ` → `KindList` non ordinata; `1. ` → ordinata; `http(s)://` e `mailto:` dentro il testo → Run con Href (regex come `urlRE` in `links.go`, punteggiatura finale tolta come `trimURL`); il resto → `KindParagraph`.

**Test:** RED con: `U+202E` nel mezzo di un URL rimosso; `U+200B` in un dominio rimosso; `javascript:` rifiutato; `data:` rifiutato; URL > 2048 rifiutato; `>>` annidato → Depth 2; `1.` → ordinata con Start 1; punteggiatura finale fuori dal link.

- [ ] Test RED.
- [ ] Implementazione.
- [ ] Gate.
- [ ] Commit `feat(richtext): testo semplice e sanificazione (Piano 15, Task 5)`.

## Task 6: link con controllo di dominio e immagini (`internal/ui/render`)

**Files:** `link.go`, `link_test.go`, `image.go`, `image_test.go` (nuovi).

**Produces:**

- `link.go`: `Link(text, href string, c Ctx) Out` (testo sottolineato tratteggiato `UnderlineDashed` con `UnderlineColor` = slot accent, D15-07). Se `text` è un URL o un dominio nudo e il dominio registrabile (`EffectiveTLDPlusOne` su entrambi i lati dopo `idna.Lookup`; fallback: host esatto) differisce da quello di `href`: `Warn = true`, sottolineatura ondulata (`UnderlineCurly`) con `UnderlineColor` = slot error, e marcatore `⚠` nel testo (D15-T6). Zona OSC 8 con `Hyperlink(href)`.
- `image.go`: `Image(alt string, c Ctx) Out` → chip testuale `[img: alt]` (nessuna emoji; Q15-4). Alt vuoto → `[img]`. Nessuna zona cliccabile. Alt troncato a 80 colonne.

**Test:** tabella di casi di phishing: `amazon.it` → `https://www.amazon.it/x` (nessun warn); `amazon.it` → `https://amazon.evil.example/` (warn); `paypal.com` → `https://paypal.com.evil.example` (warn); stesso dominio in unicode e in `xn--` (nessun warn); IP (fallback host esatto); suffisso pubblico (`co.uk`) → fallback; testo non dominio (`clicca qui`) → nessun warn; ogni warn ha `⚠` nel testo e `UnderlineCurly`. Immagine: nessuna `Zone`, alt troncato, alt vuoto.

- [ ] Test RED (tabella phishing).
- [ ] Implementazione.
- [ ] Gate.
- [ ] Commit `feat(render): link con controllo di dominio e immagini (Piano 15, Task 6)`.

## Task 7: bottoni a riquadro e chip allegati (`internal/ui/render`)

**Files:** `button.go`, `button_test.go`, `attachment.go`, `attachment_test.go` (nuovi).

**Produces:**

- `Button(b richtext.Block, c Ctx) Out`: riquadro `lipgloss.RoundedBorder()` con padding 1×0, testo in `St.Button`, a destra (dopo uno spazio) `→ host` in `St.ButtonHost` (D15-01). Larghezza = min(larghezza testo + padding, `c.Width`); testo troncato con `…`. Ogni riga del riquadro ha una zona `ZoneButton` con `URL` e OSC 8 (stesso id per le righe). Se il link è `Warn` (Task 6) → riga `⚠` nel riquadro.
- `Attachment(name, ctype string, size int64, index int, c Ctx) Out`: chip `▣ nome · tipo · dimensione` in `St.ChipAttach`, una riga, una zona `ZoneAttachment` con `Index`. Nome sanificato (Task 5) e troncato a 40 colonne.

**Test:** riquadro di 3 righe con bordo arrotondato (`╭╮╰╯`); host visibile accanto; larghezza stretta → troncamento con `…`; zona per riga con `X0/X1` corretti (`lipgloss.Width`, non `len`); chip: nome con bidi rimosso, zona con `Index`.

- [ ] Test RED.
- [ ] Implementazione.
- [ ] Gate.
- [ ] Commit `feat(render): bottoni e chip allegati (Piano 15, Task 7)`.

## Task 8: tabelle, citazioni, elenchi (`internal/ui/render`)

**Files:** `table.go`, `table_test.go`, `quote.go`, `quote_test.go`, `list.go`, `list_test.go` (nuovi).

**Produces:**

- `Table(b richtext.Block, c Ctx) Out`: calcola la larghezza minima per colonna (parola più lunga); se la somma supera `c.Width` → ricaduta testo ("Intestazione: valore" per riga, D15-T8); altrimenti `lipgloss/table` con `Width(c.Width).FitContent()`, `BorderStyle` da `St.TableBorder`, `StyleFunc` con `St.TableHeader`/`St.TableCell`. Le zone dei link nelle celle si ricalcolano dal testo già reso (a capo compresi), non da indici logici.
- `Quote(b richtext.Block, c Ctx) Out`: barra verticale `▎` in `St.QuoteBar[Depth % len]` (colori derivati dal tema, D15-03), testo in `St.Quoted`; a capo sulla larghezza meno la barra; annidamento via `Indent` ricorsivo.
- `List(b richtext.Block, c Ctx) Out`: `lipgloss/list` con enumerator `list.Dash` (non ordinata) o `list.Arabic` (ordinata, da `Start`), indentazione fissa; ogni item è un paragrafo.

**Test:** tabella stretta → ricaduta (nessun `│` nel risultato); tabella larga → bordi presenti e larghezza ≤ `c.Width`; tabella con link → zona sulla colonna giusta anche dopo a capo; citazione Depth 3 → tre barre con colori diversi (`QuoteBar` a cicli); elenco ordinato con Start 3 → "3."; zone sempre `len(Zones)==len(Lines)`.

- [x] Test RED.
- [x] Implementazione.
- [x] Gate.
- [x] Commit `feat(render): tabelle, citazioni ed elenchi (Piano 15, Task 8)`.

## Task 9: stili derivati e contrasto (`internal/ui/styles`)

**Files:** `styles.go` (valori dei campi del Task 2), `styles_test.go` (estensione).

**Produces:** valorizzazione dei campi con colori dagli slot (`Accent`, `Primary`, `Error`, `Success`, `Muted`, `Quoted`, `Border`, `Selection`, `Fg`, `Bg`) e derivati (`Lighten`, `Darken`, `Blend1D`). Ogni stile con `Background` passa `onColor` come gli altri. `QuoteBar`: `Blend1D` da `Quoted` a `Primary` in 4 passi. `BadgeAvatar`: un colore per iniziale dal tema come le label (`labelIndex`), con `onColor`. `OverlayShadow`: `Darken(Bg, 0.35)`. `StatusSegment`: alternanza `Selection`/`Border` con `onColor`. Sfumature (`OverlayBorder`, titolo) da `Blend1D(primary, accent)`: nessuna sfumatura su stili di testo.

**Test:** estensione di `TestContrastoStiliConSfondo` (`styles_test.go:180`) a tutti i nuovi stili con sfondo, su tutti i temi embedded, dark e light, WCAG ≥ 4.5:1; le sfumature non compaiono in `Button`, `ChipAttach`, `Link*`; `QuoteBar` ha 4 elementi.

- [ ] Test RED (estensione tabellare).
- [ ] Valori e derivati.
- [ ] Gate.
- [ ] Commit `feat(styles): colori derivati e contrasto dei nuovi stili (Piano 15, Task 9)`.

## Task 10: albero cartelle (`internal/ui/foldertree`)

**Files:** `tree.go`, `counts.go`, `tree_test.go`, `counts_test.go` (nuovi).

**Produces:**

- `Build(boxes []mail.Mailbox, name func(mail.Mailbox) string) *tree.Tree` (`charm.land/lipgloss/v2/tree`): split del nome sul `Delimiter` (vuoto = nessuna gerarchia, D15-T9); nodi intermedi non selezionabili (solo etichetta, `St.FolderTree`); foglie con il nome. La funzione `folderName` resta in `internal/app`: si passa come argomento per evitare il ciclo di import.
- `Render(t, cursor, counts, st) (string, []Row)`: riga per riga, conteggio `(N)` in `St.FolderCount` se `N > 0` e la cartella rientra nell'insieme di Q15-4 (default INBOX e label).
- `Rows` restituisce l'elenco lineare delle foglie selezionabili con indice, così `j`/`k`/`invio` in `app` restano invariati.

**Test:** `Lavoro/Clienti` e `Lavoro/Fornitori` → un nodo `Lavoro` non selezionabile con due foglie; delimitatore vuoto → piatta; contatore 0 → nessun `(0)`; cursore sulla foglia giusta; ordine (INBOX, speciali, label alfabetiche) mantenuto.

- [ ] Test RED.
- [ ] Implementazione.
- [ ] Gate.
- [ ] Commit `feat(foldertree): albero cartelle e label annidate (Piano 15, Task 10)`.

## Task 11: IMAP, delimitatore e UNSEEN (`internal/mail/imap`)

**Files:** `mailboxes.go` (campo `Delimiter` dal LIST), `unseen.go` (sostituisce lo stub del Task 2), `unseen_test.go`, un caso nuovo in `mailboxes_test.go`.

**Produces:**

- `ListMailboxes` riempie `Delimiter` dal delimitatore del LIST. Prima di scrivere: verificare l'API di go-imap v2.0.0-beta.9 sul sorgente (`third_party/go-imap` o module cache), come da AGENTS.md.
- `UnreadCounts`: un round del gate (`idleGate`), per ogni mailbox listata (escluse `\Noselect`) `STATUS <name> (UNSEEN)`; mappa `Name → UNSEEN`. Errore su una mailbox: si salta quella (best-effort); errore di connessione: si restituisce. Niente SELECT (STATUS non cambia la mailbox selezionata).

**Test:** con `imapmemserver`: due mailbox con messaggi letti/non letti → conteggi attesi; mailbox vuota → 0; `\Noselect` esclusa; `Delimiter` = `/` per una label annidata; la mailbox selezionata non cambia. RED prima dell'implementazione.

- [ ] Test RED.
- [ ] Implementazione.
- [ ] Gate (+ grep della regola 1 di AGENTS.md).
- [ ] Commit `feat(imap): delimitatore e UNSEEN per cartella (Piano 15, Task 11)`.

## Task 12: finestre sovrapposte con ombra (`internal/ui/overlay`)

**Files:** `overlay.go`, `overlay_test.go` (nuovi).

**Produces:** `Window(content, title string, st styles.Styles, w, h int) string`: riquadro `RoundedBorder()`, bordo con `BorderForegroundBlend` (D15-13), sfumatura anche sul titolo. `Overlay(base, win string, x, y int, st styles.Styles) string`: `lipgloss.NewCompositor` con `NewLayer(base)`, `NewLayer(shadow).X(x+2).Y(y+1).Z(1)`, `NewLayer(win).X(x).Y(y).Z(2)`. Usi: menu `,`, pannello label, selettore cartelle (Task 16).

**Test (spike, prima di adottare):** un `Layer` con celle di spazio stilizzate copre le celle sottostanti nel render del compositor; l'ombra è visibile fuori dalla finestra; la finestra è sopra il contenuto. Se lo spike fallisce, il piano si ferma e la Q15-5 va posta ad Andrea.

- [ ] Spike RED.
- [ ] Implementazione.
- [ ] Gate.
- [ ] Commit `feat(overlay): finestre con ombra (Piano 15, Task 12)`.

## Task 13: barra di stato a segmenti (`internal/ui/statusline`)

**Files:** `statusline.go`, `statusline_test.go` (nuovi).

**Produces:** `Line(segs []Segment, width int, st styles.Styles) string` con `type Segment struct { Text string; Kind SegKind }` (`SegMode`, `SegFolder`, `SegStatus`, `SegHelp`). Separatori: default senza glifo (cambio di sfondo, D15-T12); con glifo `""` (U+E0B0) solo se Q15-2 è confermata. Il testo dei segmenti usa `onColor`. Se la somma è più larga di `width`, il segmento help si tronca per primo.

**Test:** segmenti in ordine; contrasto leggibile (Task 9); troncamento dal segmento help; larghezza esatta `width` (`lipgloss.Width`); nessun segmento vuoto renderizzato.

- [ ] Test RED.
- [ ] Implementazione.
- [ ] Gate.
- [ ] Commit `feat(statusline): barra di stato a segmenti (Piano 15, Task 13)`.

## Task 14: thread con i renderer (`internal/ui/thread`)

**Files:** `conversation.go`, `links.go`, `thread.go`, `render_cache.go` (nuovo), `render_cache_test.go` (nuovo), e i test di thread e links che cambiano.

**Produces:**

- Corpo: `renderConversation` usa `richtext.FromHTML(BodyHTML)` se presente, altrimenti `richtext.FromText(BodyText)`; poi `render.Dispatch` per ogni blocco; `Stack` dei risultati; `linkRows [][]render.Zone`.
- `render_cache.go`: `type renderCache struct` con mappa e ordine di inserimento (16 voci), chiave `(MessageID, width, theme, variant)`. Puntatore creato in `New`/`NewLazy` (D15-T2).
- Rendering asincrono (D15-T5): `RenderedMsg{ID, Width, Gen, Out}` da un `tea.Cmd`; `Gen` confrontato con il corrente; fallback testo finché non arriva.
- `links.go`: `linkSpan` → `render.Zone`; `LinkAt(x,y)` e `LinkClick` con la stessa firma; `linkTarget` delegato a `richtext.sanitizeURL` (Task 5). `LinkAt` restituisce anche `ZoneButton` e `ZoneAttachment` col loro `Kind`; `LinkClick` avvia la Cmd del browser solo per `ZoneLink`/`ZoneButton`; per `ZoneAttachment` emette `AttachmentRequested{Index}` (gestito nel Task 16).
- `thread.go`: scheda intestazione (D15-05): badge con iniziali in `St.BadgeAvatar`, nome e indirizzo; `headerLines` aggiornato nello stesso commit del test di `LinkAt`. Chip allegati da `render.Attachment` (D15-06).

**Test:** messaggio HTML fixture → zone corrette per link, bottone, allegato; `LinkAt` su bottone e chip; test esistenti di `links_test.go` verdi (salvo `headerLines`); cache: stessa chiave non ricalcola (contatore), larghezza diversa ricalcola, la 17ª voce espelle la più vecchia; `RenderedMsg` con `Gen` vecchio scartato; HTML oltre limite (`huge.html`) mostra `[messaggio troncato]` senza bloccare.

- [ ] Test RED (fixture e cache).
- [ ] Implementazione.
- [ ] Gate (incluso `TestContrastoStiliConSfondo`).
- [ ] Commit `feat(thread): corpo a blocchi, scheda e chip (Piano 15, Task 14)`.

## Task 15: cartelle e contatori in app (`internal/app`)

**Files:** `internal/app/folders.go`, `internal/app/unread.go` (nuovo), `unread_test.go` (nuovo), hook minimi in `model.go`.

**Produces:**

- Selettore `m` su `foldertree` (`Build`, `Render`, `Rows`): `j`/`k`/`invio`/`esc` invariati.
- `unread.go`: `unreadLoaded{counts map[string]int, err error, gen int}` da un `tea.Cmd` (`UnreadCounts`); richiesto all'apertura e su `Change` `ChangeNewMail`/`ChangeFlags`; scarto con gen (stantii).
- Hook in `model.go` (solo punti di dispatch): `Update` gestisce `unreadLoaded`.

**Test:** `unread_test.go` con `mail.Mock`: i contatori arrivano nel selettore; un `Change` ricarica; risultato stantio scartato; errore: il selettore resta senza contatori (nessun toast).

- [ ] Test RED.
- [ ] Implementazione.
- [ ] Gate.
- [ ] Commit `feat(app): albero cartelle e contatori non letti (Piano 15, Task 15)`.

## Task 16: finestre, statusline, clic su bottoni e allegati (`internal/app`)

**Files:** `model.go` (mouse su zone, `statusBar`, overlay), `settings.go`, `labels_picker.go`, `folders.go` (overlay), `attachment_open.go` e `attachment_open_test.go` (nuovi).

**Produces:**

- Finestre: `,` (settings), pannello label e selettore `m` renderizzati con `overlay.Window`/`overlay.Overlay` (D15-11). Mouse gating invariato.
- `statusBar` su `statusline.Line` con segmenti: modo, cartella, stato, help (D15-12). Le barre colorate restano per le conferme modali.
- Clic: `ZoneButton` e `ZoneLink` via `LinkClick` (browser, invariato); `ZoneAttachment` → `AttachmentRequested` → `openAttachment`.
- `attachment_open.go`: `openAttachment(st, id, index, name, ctype) tea.Cmd`: se il tipo è in allowlist (Q15-1; default: `application/pdf`, `image/png`, `image/jpeg`, `image/gif`, `text/plain`), `SaveAttachment` in `os.MkdirTemp` (0700) con nome generato (`att-<n><ext>` dall'allowlist), poi apertura con il comando di sistema (come `browserCmd`, senza shell); altrimenti toast "tipo non aperto: usa :save". Variabile per i test (come `openLink`).

**Test:** clic su chip → `AttachmentRequested` con l'indice giusto; tipo non in allowlist → toast, nessuna scrittura (store mock che conta le chiamate); tipo in allowlist → file in dir 0700, nome generato (nessun nome dal messaggio nel path), apertura chiamata con il path (variabile sostituita nel test); `:save` invariato; `,`, label e selettore: overlay visibile, `esc` chiude, mouse ignorato con modale aperto; statusline: non regressione dei messaggi di stato.

- [ ] Test RED.
- [ ] Implementazione.
- [ ] Gate.
- [ ] Commit `feat(app): finestre, statusline e allegati cliccabili (Piano 15, Task 16)`.

## Task 17: demo, Mock e isolamento MCP

**Files:** `internal/app/demo.go`, `internal/mail/mock.go`, `internal/mcpserver/*_test.go`.

**Produces:**

- Demo: un messaggio con HTML ricco (fixture anonimizzata, nessun dominio reale), contatori demo (`UnreadCounts`), cartelle annidate demo (`Delimiter` "/").
- Mock: `UnreadCounts` con dati, `Delimiter` nei mailbox.
- Test di isolamento MCP: `internal/mcpserver` non importa `internal/msg/richtext` né `internal/ui/*` (test che legge `go list -deps`); `Reader` invariato (nessun `UnreadCounts`).

**Test:** demo: il messaggio demo ha almeno un bottone, una tabella, una citazione annidata, un link con warn; MCP: `tools` restituisce ancora `BodyText` (test esistente verde); isolamento verde.

- [ ] Test RED.
- [ ] Implementazione.
- [ ] Gate.
- [ ] Commit `feat(demo): contenuto ricco e contatori demo (Piano 15, Task 17)`.

## Task 18: documentazione

**Files:** `README.md`, `CHANGELOG.md`, `docs/smoke-checklist.md`, `AGENTS.md` (README e CHANGELOG in inglese; AGENTS.md in italiano).

**Produces:** README: sezione "Rendering" (bottoni, tabelle, link con controllo di dominio, allegati cliccabili, cartelle annidate con contatori); CHANGELOG: voce Piano 15; smoke: link ondulato su un dominio reale diverso (Andrea), apertura allegato su macOS (Terminal, iTerm2), sottolineature tratteggiate e ondulate sui terminali di Andrea (Q15-3); AGENTS.md: la regola "UI solo slot" con le derivazioni ammesse (D15-T11/T12) e la regola "richtext/render non importati da mcpserver".

**Test:** nessun test di codice; controllo di privacy: nessun indirizzo reale o nome di persona nelle fixture e nei doc (grep).

- [ ] Doc.
- [ ] Gate.
- [ ] Commit `docs: Piano 15 (beautify con lipgloss)`.

## Task 19: gate finale, misure e revisione

**Files:** nessun file di produzione (eventuali fix in branch separato di review).

**Produces:**

- Misure: tempo di `FromHTML` + `render.Dispatch` su `huge.html` e su tre messaggi reali **redatti** di Andrea (solo nel report, nessuna fixture reale nel repo). Soglia da fissare in questo task (proposta: < 50 ms per 512 KiB; oltre, async).
- Gate: `go build`, `go vet`, `go test -race -count=1 ./...`, `golangci-lint run ./...` a 0 issues, `govulncheck ./...`, build `CGO_ENABLED=0` linux/darwin/windows.
- Revisione indipendente: un agente diverso sul diff di `internal/msg/richtext` (sanitizzazione, limiti) e su `internal/app/attachment_open.go` (apertura file).

**Test:** gate verdi; misure registrate.

- [ ] Misure.
- [ ] Gate.
- [ ] Revisione e fix.

## Domande per Andrea

**Risposte (2026-11-23):** Q15-1 solo tipi sicuri (PDF, immagini, testo; il resto → toast "usa :save"), come il default proposto. Q15-2 nessun glifo speciale (nessun campo di config). Q15-3 terminale principale di Andrea: Ghostty (undercurl e colore supportati; il marcatore `⚠` resta comunque). Q15-4 confermato dal coordinatore: contatori su INBOX e label, icone solo testo. Q15-6 confermato: 512 KiB con troncamento dichiarato. Q15-5 resta aperta finché lo spike del Task 12 non dà esito.

- **Q15-1: apertura degli allegati.** Default proposto (D15-T14): aprire solo PDF, immagini e testo; tutto il resto (`.app`, `.command`, `.pkg`, Office con macro) → toast "usa :save". Alternativa: aprire tutto con conferma. Quale?
- **Q15-2: separatori powerline.** Il glifo `U+E0B0` richiede un font Powerline/Nerd. Default: separatori senza glifo (cambio di sfondo). Opzione `statusline_glyphs = "powerline"` in config: si aggiunge un campo di configurazione. Confermare?
- **Q15-3: sottolineature.** Undercurl e colore della sottolineatura dipendono dal terminale (Apple Terminal non le supporta: da verificare). Default: il link ondulato ha sempre anche il marcatore `⚠` nel testo. Quali terminali usa Andrea per lo smoke?
- **Q15-4: contatori e icone.** Contatori su INBOX e label (come Gmail), non su Bozze, Inviati, Spam, Cestino, Tutti i messaggi. Confermare. Icone: solo testo ASCII/Unicode base, nessuna emoji (default).
- **Q15-5: ombra e compositor.** Lo spike del Task 12 verifica che i layer opachi coprano il sottostante. Se non copre, serve un'altra strategia (ridisegno della base): la scelta è sua.
- **Q15-6: limite HTML.** 512 KiB e troncamento con avviso (D15-T4). Confermare o alzare.

## Rischi e mitigazioni

| Rischio | Mitigazione |
|---|---|
| HTML reale con tabelle di layout annidate illeggibili | Limiti di profondità e ricaduta a testo (D15-T4, D15-T8); fixture di newsletter |
| Link ondulato assente sul terminale | Marcatore `⚠` nel testo (D15-T6); smoke sui terminali di Andrea (Q15-3) |
| Falsi positivi del controllo di dominio (CDN, tracking) | Confronto per dominio registrabile, non per host; test tabellare; il warn non blocca il clic |
| Heuristica bottone sbagliata | Regole di D15-T7 testate su fixture; il fallback è un link normale |
| Sfumature su terminali a 256 colori | `Blend1D` degrada al colore più vicino; nessuna informazione dipende dalla sfumatura |
| Cache di rendering cresce | 16 voci fisse (D15-T2); test di espulsione |
| Render lento su email grandi | Limiti D15-T4 e rendering async D15-T5 con scarto stantii |
| Apertura allegato eseguibile su macOS | Allowlist per tipo, nome generato, cartella 0700, nessuna shell (D15-T14, Q15-1) |
| Conflitti su `internal/app/model.go` | Wave 3b e 3c sequenziali, un solo agente sul file |
| `headerLines` cambia e `LinkAt` sfalsa i clic | Aggiornamento nello stesso commit del test di `LinkAt` (Task 14) |
| Contatori UNSEEN lenti su molte label | Un solo round del gate, ricalcolo solo su Change e apertura; misura in Task 19 |
| Ombra non coperta dal compositor | Spike di Task 12 prima di adottare la finestra (Q15-5) |

## Revisione indipendente

Dopo il Task 16: review da un agente diverso sul diff di `internal/msg/richtext` (sanitizzazione, limiti, euristiche bottone) e su `internal/app/attachment_open.go` (apertura file, allowlist). Esito e fix nel report di Task 19.
