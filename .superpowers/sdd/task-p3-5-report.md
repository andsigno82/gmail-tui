# Task P3-5 report — thread view: lettura messaggio

Commit: `9776eb2` `feat(thread): message reading view with headers, quotes, attachments` — branch `piano-3-tui`.

## Files

- `internal/ui/thread/thread.go` (NEW) — vista lettura su bubbles/viewport v2. API: `New(ref msg.MessageRef, full *msg.Message, st styles.Styles) Model`, `Update(tea.Msg) (Model, tea.Cmd)`, `View() string`, `BackMsg{}`, `SetSize(w,h)` (pointer), `MarkRead()` (pointer), più accessori `Ref()`, `Loaded()`, `YOffset()` (seam test). Messaggi di protocollo: `OpenRequested{Path, Err}` (risultato della preparazione di `o`) e `OpenedMsg{Err}` (callback di exec) + `OpenCmd(path) tea.Cmd` (tea.ExecProcess). Header pre-renderizzato in New (coppie chiave/valore Da/A/Oggetto/Data: label in `ThreadHeaderKey`, valori in `ThreadHeader` — il padding(0,1) dello stile fa da spazio), corpo pre-stilizzato riga-per-riga (righe `^>` in `Quoted`, il resto plain) PRIMA di `vp.SetContent`, allegati in coda `📎 nome (size umana)` stilizzati con `ListDate` (solo elenco; save/open = Piano 4). `humanSize`: B/KiB/MiB/GiB binari, 1 decimale con ".0" tagliato. Data header assoluta italiana (`20 nov 2026 10:00`, zero-date → "—"). `SetSize` dà alla viewport `h - 5 (header+blank) - (1+len(atts))` righe così View() rende ESATTAMENTE `h` righe e la status bar radice resta sull'ultima riga.
- `internal/ui/thread/thread_test.go` (NEW) — 14 test: header key/value con assert sull'ANSI degli stili (non sul testo nudo), quote dimming (`Quoted.Render("> …")` presente, assente su righe normali), j/k con clamp al fondo, rotella (+3/-3, delta default), View esattamente `h` righe con e senza allegati, righe allegato con size umane e in coda, no-📎 senza allegati, `humanSize` tabellare (9 casi), BackMsg su esc e q, `o` → `OpenRequested` con tmpfile `.html` (TMPDIR reindirizzato a `t.TempDir`: suffisso, esistenza e contenuto == BodyHTML; il browser NON è eseguito), `o` senza HTML → nil, `browserCmd` open/xdg-open con skip `exec.LookPath` se assente, MarkRead muta la copia locale del ref, `New(ref, nil, st)` difensivo non panica.
- `internal/app/model.go` — integrato: campo `thread thread.Model`; `inbox.SelectedMsg` → `pendingRef` + viewThread + `pending++` + `tea.Batch(fetchMsg(store, id), spinner.Tick)`; **placeholder `_ = ev.m` RIMOSSO**: `msgLoaded` → `thread.New(pendingRef, m, styles)` + `SetSize` se la size è nota + **auto MarkRead** se `m.Ref.Unread` (copia locale del ref mutata subito via `thread.MarkRead()`, cmd `markRead` → `opDone{"letta"}` — errore solo in status, non fatale); `thread.BackMsg` → viewInbox senza re-list; `thread.OpenRequested` → `thread.OpenCmd(path)` (o status isErr); `thread.OpenedMsg` → status ("aperto nel browser" / errore); esc/q in viewThread DELEGATI al modello thread (che risponde con BackMsg), le altre pressioni e il mouse (click+wheel) delegati in thread view; `WindowSizeMsg` → `thread.SetSize(w, h-1)`; `body()` viewThread → "Caricamento messaggio…" se `!thread.Loaded()`, altrimenti `thread.View()`; nuove cmd `fetchMsg` e `markRead` (context.Background() come loadRefs, fino al ctx del programma Task 7/8).
- `internal/app/model_test.go` — ESTESO: 7 test nuovi (fetch→thread vera con placeholder durante il fetch, auto markread con assert su cmd/opDone/Mock/status, msgLoaded err → status isErr + no thread, BackMsg preserva la lista e il cursore, OpenRequested → exec cmd, OpenedMsg → status, wheel nel thread scorre la viewport) + 2 riscritti per l'evoluzione del wiring (vedi TDD) + helper `newTestModelMock`, `fullMessage`, `runCmd` (srotola ricorsivamente `tea.BatchMsg` come fa il runtime). I 19 test Task 3+4 non toccati passano intatti.
- `internal/msg/message.go` / `parse.go` / `parse_test.go` — DEVIAZIONE documentata (vedi sotto): nuovo campo `Message.BodyHTML` popolato da `Parse` nel ramo solo-HTML (l'HTML originale, NON la conversione html2text); test `TestParseBodyHTML` (presente con HTML-only, vuoto con text/plain).
- `docs/superpowers/plans/2026-11-21-piano-3-tui.md` — checkbox Task 5 spuntata.

## TDD evidence

1. **RED 1 (msg)** — `TestParseBodyHTML` scritto prima: `go test ./internal/msg/` → build failed `Message has no field or method BodyHTML`. **GREEN 1**: campo + una riga in `Parse`; verde (nessun test Piano 2 rotto: il campo è additivo).
2. **RED 2 (thread)** — `thread_test.go` scritto prima: build failed `undefined: Model / New / humanSize / browserCmd`. **GREEN 2**: implementato `thread.go`; **1 failure reale di comportamento**: `browserCmd.Path` = `/usr/bin/open` non `"open"` — da Go 1.19 `exec.Command` risolve il binario nel PATH assoluto alla costruzione; fix nel TEST (assert su `filepath.Base`), l'implementazione era corretta. 14/14 verde.
3. **RED 3 (app)** — test aggiornati/nuovi: build failed su `mm.thread` / `thread.BackMsg`. **GREEN 3**: 3 failure reali di comportamento, tutti e tre di harness — `SelectedMsg` ora emette `tea.Batch(fetchMsg, spinner.Tick)` e i test steppavano `cmd()` (BatchMsg) direttamente come Msg: il runtime srotola il Batch, il test no. Fix: helper `runCmd` che srotola `tea.BatchMsg` ricorsivamente (pattern già usato a mano da `TestInitEmitsLoadAndSpinner`). Verde al giro dopo.
4. **Verifica finale**: `gofmt -l` pulito, `go vet ./...` pulito, `go test -race -count=1 ./...` → ok su 12 package, `golangci-lint run ./...` → **0 issues** (binario `/Users/andrea/go/bin/golangci-lint`).
5. **Smoke visuale** (go run scratch, rimosso): 24 righe esatte a `SetSize(80,24)`; header con chiavi muted e valori bold-primary su righe separate; righe `>` in italic muted dentro il corpo; corpo paddato a larghezza piena; allegati in coda con 📎 e size umane.

## Note bubbles/viewport v2 (per Task 6-9) — verificate su charm.land/bubbles/v2@v2.2.2 nel module cache

1. **Mouse BUILT-IN (a differenza di bubbles/list)**: `viewport.Model.MouseWheelEnabled` è `true` di DEFAULT (`setInitialValues`), `MouseWheelDelta` default 3; `updateAsModel` gestisce `tea.MouseWheelMsg` per up/down/left/right, con `ModShift` che converte verticale→orizzontale per terminali che non inviano shift. NESSUN handler custom necessario (a differenza dell'inbox dove list v2.2.2 non ha mouse). `Update` restituisce SEMPRE cmd nil; i click non sono gestiti (nessun caso `MouseClickMsg`).
2. **Keymap di default**: `j/down` ScrollDown(1), `k/up` ScrollUp(1), `f/space/pgdown` PageDown, `b/pgup` PageUp, `d/ctrl+d` HalfPageDown, `u/ctrl+u` HalfPageUp, `h/left` `l/right` scroll orizzontale. `Update(KeyPressMsg)` ricevitore VALORE → in thread: `m.vp, cmd = m.vp.Update(ev)` statement form.
3. **SetContent/SetContentLines**: split su `\n`, normalizzazione CRLF; contenuto con UNA riga vuota → `lines = nil`. Con SoftWrap=true e Width()==0 `calculateLine` divide per maxWidth zero → nessun panic ma numeri spazzati: garantire sempre `SetSize` prima dello scroll (nel flusso reale WindowSizeMsg arriva all'avvio; nei test si dimensiona sempre).
4. **View() string**: rende ESATTAMENTE `height` righe (pad via lipgloss Height) e padda ogni riga alla larghezza; `""` se width==0 o height==0. Il pre-styling (quote) sopravvive al View: le righe sono ANSI-aware misurate con `ansi.StringWidth` — a patto che non scattino soft-wrap su righe più larghe del viewport (i test usano righe corte).
5. **tea.ExecProcess esiste in v2** (`exec.go:50`): `ExecProcess(c *exec.Cmd, fn ExecCallback) Cmd`; il callback riceve l'errore del processo; la TUI viene sospesa/ripristinata dal runtime (clear view + release/restore terminal). Non è componibile dentro un'altra Cmd (restituisce una Cmd, non un Msg) → il tmpfile write DEVE avvenire in una cmd separata che produce un Msg di handoff (qui `OpenRequested`), e la exec vera in un secondo step (`thread.OpenCmd`) — è la ragione del design a due messaggi.
6. viewport pad ogni riga alla larghezza → le asserzioni substring su righe stilizzate funzionano solo per il PREFISSO stilizzato (i pad spazi sono fuori dal reset ANSI).

## Scelte di design

- **`o` a due fasi (Update puro)**: `o` → cmd che scrive il tmpfile (I/O locale istantaneo, comunque fuori dal loop Update) → `OpenRequested{Path, Err}` → radice → `thread.OpenCmd` (ExecProcess) → `OpenedMsg{Err}` → status. Motivazioni: (a) ExecProcess non è annidabile in una Cmd, (b) il percorso del tmpfile resta assertabile nei test senza eseguire il browser, (c) gli errori di preparazione ed esecuzione finiscono entrambi in status bar (vincolo globale "errori visibili"). Test: `TMPDIR=t.TempDir()`, skip esplicito via `exec.LookPath` se il comando manca (darwin ha `open`).
- **Auto MarkRead ottimistico lato UI**: la copia locale del ref passa a "letta" AL MOMENTO del msgLoaded (non a opDone): deterministico nei test e coerente col vincolo "nessun re-list al markread" — la lista si aggiornerà al back o via Watch (Task 8), documentato anche nel commento di codice. L'errore della cmd è solo feedback in status (`opDone{letta, err}`), non fatale, e non tocca la vista.
- **Layout ad altezza esatta**: `SetSize` scala la viewport di header(5)+allegati(1+N) → la thread view rende esattamente `h` righe come fa l'inbox, e il padding esistente della radice (Task 3) non cambia. Senza allegati la sezione 📎 sparisce del tutto (nessuna riga vuota orfana).
- **Pre-styling del corpo PRIMA del viewport** (come da piano): il viewport non aggiunge stili (nessuno `StyleLineFunc` impostato), le righe citate arrivano già in `Quoted`; soft-wrap attivo (`SoftWrap=true`) perché in lettura email le righe lunghe devono andare a capo, non scorrere orizzontalmente.
- **Data header assoluta** (`20 nov 2026 10:00`, mesi italiani, fuso del messaggio, "—" se zero): in lettura serve l'informazione completa che `inbox.RelativeDate` (relativa, senza anno) non dà; mesiIT duplicato in package (12 stringhe) per non creare dipendenza thread→inbox tra due viste foglia.
- **Delegazione esc/q nel thread al modello** (invece dell'intercetto radice di Task 3): rende `BackMsg` il protocollo vero del contratto (come richiesto dal piano) e lascia alla radice solo il consumo del messaggio; search resta intercettata direttamente (Task 6 la collegherà).
- **`Cc` NON renderizzato** nell'header: il piano elenca esplicitamente Da/A/Oggetto/Data; l'informazione resta nel modello (aggiungerla è una riga).

## Self-review

- Conformità piano: sezione Task 5 seguita (New(ref, full, st) su viewport, Update, BackMsg su esc/q, header key/value Da/A/Oggetto/Data, corpo scrollabile, quote `^>` attenuate line-per-line pre-stilate, allegati in coda solo-elenco, fetch su selezione → msgLoaded → thread.New, auto MarkRead in cmd concatenata, `o` con tmpfile HTML + open/xdg-open e skip test se comando assente). Deviazioni: (a) ricevitori puntatori per SetSize/MarkRead (API bubbles v2 muta lo stato — stesso argomento di Task 4), (b) `View() string` per la sub-vista, la radice resta l'unica `tea.NewView` (v2), (c) **campo `msg.Message.BodyHTML` nuovo** — il piano non lo elencava ma senza l'HTML originale `o` sarebbe stato un tempfile di testo travestito; enabler minimo, additivo, testato in msg.
- Stili SOLO da `styles.Styles` (ThreadHeader/ThreadHeaderKey/Quoted/ListDate); nessun hex, nessun import theme/lipgloss-color in thread (lipgloss non importato affatto); nessun import imap; MailStore toccato solo dalle cmd (fetchMsg/markRead/loadRefs).
- Task 3/4 intatti: i 19 test precedenti passano così come sono; le sole scritture sono i 2 test il cui comportamento osservato è LEGITTIMAMENTE cambiato dal nuovo wiring (esc/q ora passano per BackMsg; SelectedMsg ora fetcha) — nomi e intento preservati, commenti in test spiegano l'evoluzione.
- Gates: `go vet ./...` 0, `go test -race -count=1 ./...` verde su 12 package, `golangci-lint run ./...` **0 issues**, gofmt pulito. Conformance grep: nessun "imap" in internal/app|ui.
- Conteggio: thread 14 test, app 24, msg 6 (1 nuovo).

## Concerns

1. **`Message.BodyHTML` nel payload normalizzato** (deviazione dal file structure del piano): è la superficie giusta per Piano 4 (save/open allegati, composer) ma tocca il contratto dati di Piano 2 — se il piano vuole restare strict, l'alternativa è un wrapper UI-only, qui giudicato peggiore (doppio fetch o campo opaco).
2. **`_ = ev.c` (changeEvt) resta placeholder** — previsto: consumo vero in Task 8.
3. **Doppio tick dello spinner quando le op si accavallano** (Batch(fetch, Tick) su op in flight già tickate): solo una accelerazione cosmetica dei frame, la convenzione era già fissata dal report Task 3; Task 9 può centralizzare.
4. **Viewport con Width==0 e SoftWrap=true** prima del primo WindowSizeMsg: nessun panic (View early-return, righe vuote) ma offset calcolati male se si scorresse prima della size; il runtime reale non può (WindowSizeMsg arriva all'avvio), i test dimensionano sempre. Se Task 9 porterà la schermata min-size, valutare un guard `m.width==0` in thread.Update.
5. **TMPFILE non ripulito** dopo l'apertura nel browser (lasciato a OS tmpwatch): delibero — il browser può riaprirlo/ricaricarlo; se si vuole pulizia esplicita, il posto è il callback `OpenedMsg` (Piano 4/polish).
6. **`o` premuto più volte** apre N browser e N tmpfile: comportamento accettabile (intenzione utente), non deduplicato.
7. **`markRead` usa `context.Background()`** come tutte le cmd finora: quando Task 7/8 porteranno il ctx del programma, aggiornare fetchMsg/markRead/loadRefs insieme.

## Fix: BodyHTML multipart/alternative

Commit: `24e2998` `fix(msg): populate BodyHTML for any HTML part; o gate on BodyHTML (+ nil guard)` — branch `piano-3-tui`. Review finding (Important, adottato dal controller): con multipart/alternative (text+html, la forma dominante della posta reale) lo switch di `Parse` faceva vincere il ramo text/plain → HasHTML=false E BodyHTML="" → il tasto `o` non funzionava mai sulla posta HTML vera (solo sulle HTML-only).

### Cosa è cambiato

- `internal/msg/parse.go` — DOPO lo switch esistente (regola corpo e HasHTML invariate: text/plain vince, HasHTML=true solo per messaggi solo-HTML) blocco incondizionato: `if strings.TrimSpace(env.HTML) != "" { m.BodyHTML = env.HTML }` — BodyHTML porta l'HTML originale per QUALSIASI messaggio con parte HTML, anche quando il corpo mostrato è testuale. Doc comment di `Parse` aggiornato.
- `internal/msg/message.go` — doc comment di `BodyHTML`: "contiene l'HTML originale se presente (anche per messaggi con parte testuale); HasHTML resta true solo per messaggi SOLO HTML".
- `internal/ui/thread/thread.go` — gate di `o` in `openInBrowser`: da `!HasHTML || BodyHTML == ""` a `m.msg.BodyHTML == ""` (nil-guard su `m.msg` resta). Un alternative ora apre il browser.
- `internal/app/model.go` — due minor della review, stesso commit: (a) nil guard `ev.m == nil` nell'handler `msgLoaded` prima di `thread.New`/`MarkRead` (contratto MailStore senza (nil, nil), ma senza guardia sarebbe deref di puntatore nullo → status isErr "risposta vuota dallo store"); (b) commento overpromising del MarkRead corretto: "il refresh lista avviene via Watch/nuova ricerca, il back preserva la lista" (prima: "si aggiornerà al back o via Watch" — il back NON aggiorna la lista). Stesso allineamento ai commenti gemelli con lo stesso errore fattuale in `thread.go` (doc di MarkRead) e `thread_test.go` (doc di TestMarkReadMutatesRef).

### Test

- `internal/msg/parse_test.go` — fixture `multipartALTMail` (multipart/alternative: parte text/plain + parte text/html con link) + `TestParseMultipartAlternative`: BodyText dalla parte testuale (senza markup), **HasHTML=false (semantica invariata)**, BodyHTML contiene `href="https://example.net/sconto"`. Doc di `TestParseBodyHTML` aggiornata ("senza nessuna parte HTML resta vuoto").
- `internal/ui/thread/thread_test.go` — `TestOAlternativeEmitsOpenRequested`: `o` su messaggio con BodyText+BodyHTML e HasHTML=false → OpenRequested con tmpfile `.html` e contenuto == BodyHTML (prima: cmd nil, nessun effetto). `TestONoHTMLNilCmd` resta verde (testMsg non ha BodyHTML).

### Proof: pinned test invariato

`TestFetchMessageMultipart` (internal/mail/imap/fetch_test.go:117, assert Piano 2 HasHTML=false su multipart con text/plain) passa IDENTICO senza alcuna modifica al file: il messaggio del test non ha parte HTML → env.HTML vuoto → il nuovo blocco non tocca nulla (BodyText dal text/plain, HasHTML=false come prima). Anche `TestParseBodyHTML` e `TestParseHTMLFallback` passano intatti, e `TestOEmitsOpenRequested` (HTML-only) continua a valicare il nuovo gate.

### Gates

- `go test ./... -race -count=1` → ok su 12 package (imap incluso).
- `go vet ./...` → pulito; `golangci-lint run ./...` → **0 issues**; `gofmt -l` → vuoto.

### Note

- BodyHTML per gli alternative duplica il payload in memoria (testo convertito + HTML originale): già vero per gli HTML-only (BodyText convertito + BodyHTML originale), stesso trade-off accettato al Task 5.
- Il ramo solo-HTML dello switch assegna ancora BodyHTML prima del nuovo blocco (riassegnazione identica, nessun comportamento doppio): switch lasciato intatto per minimizzare il diff sulla logica pinata.
