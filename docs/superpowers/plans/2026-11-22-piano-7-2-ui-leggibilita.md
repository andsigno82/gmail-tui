# gmail-tui — Piano 7.2: leggibilità dei temi, menu impostazioni, due colonne, markdown → HTML — Implementation Plan

> **For agentic workers:** branch `feat/piano-7-2` (da `feat/piano-7-1`). Leggere `AGENTS.md`. TDD, un commit per task, checkbox `- [ ]`. Tutte le decisioni sotto sono di Andrea (2026-11-22): non reinterpretarle.

**Goal:** (1) testo leggibile su ogni barra colorata in tutti i temi; (2) menu impostazioni comodo con anteprima immediata; (3) vista a due colonne con lista a sinistra e anteprima grande a destra; (4) i messaggi scritti in markdown arrivano formattati, non come markdown grezzo.

**Problemi verificati:**
- In `internal/ui/styles/styles.go` le barre colorate usano `fg` del tema come testo: `StatusOK`, `StatusErr`, `StatusBar` (e `DemoBanner` con `bg`). Contrasto misurato (WCAG) su tutti i 12 temi, dark e light: tra 1.0:1 e 3.5:1. La soglia WCAG AA per il testo è 4.5:1.
- `buildMIME` (`internal/mail/smtp/mime.go`) invia il corpo solo come `text/plain`, mentre il composer dichiara markdown: il destinatario vede `**grassetto**`.

## Decisioni (Andrea, 2026-11-22)

| ID | Decisione | Scelta |
|---|---|---|
| U1 | Contrasto | Colore del testo calcolato: per ogni sfondo colorato si sceglie tra `fg`/`bg` del tema il primo con contrasto ≥ 4.5:1, altrimenti nero o bianco (il migliore). Nessun TOML da modificare. Verificato in fase di piano: tutti i temi arrivano ≥ 4.9:1 |
| U2 | Apertura impostazioni | Tasto `,`, modale con ↑↓ per la voce, ←→ per il valore, anteprima immediata; invio salva, esc annulla e ripristina |
| U3 | Voci | Tema, Layout lettura, Segna come letto in anteprima, Nome, Firma |
| U4 | Layout | Una colonna / Due colonne (lista a sinistra, anteprima a destra). Niente "anteprima sotto" |
| U5 | Segna letto in anteprima | Valori: subito / dopo 2 secondi / mai. Default: dopo 2 secondi sullo stesso messaggio |
| U6 | Focus | `Tab` sposta il focus tra lista e anteprima; con focus sull'anteprima j/k/pgup/pgdn la scorrono; `esc` o `Tab` tornano alla lista; invio apre il thread a schermo intero come oggi |
| U7 | Default layout | Una colonna (comportamento attuale invariato) |
| U8 | Proporzione | Fissa: lista 35%, anteprima 65% |
| U9 | Dove si converte il markdown | Composer sempre; `gmail-tui send` solo con il nuovo flag `--markdown` |
| U10 | Sintassi | GFM via goldmark (tabelle, barrato, link automatici, liste di attività); HTML grezzo escapato (default sicuro di goldmark) |

## Riferimenti

- Riquadro di lettura di Gmail: "No split / Right of inbox / Below inbox", anche dalle Quick settings (support.google.com/mail/answer/9499937). Qui si adottano solo "No split" e "Right".
- Menu opzioni con modifica immediata dei valori: btop.
- Colori del testo sugli sfondi d'accento: pattern "on-color" di Material Design, qui calcolato invece che dichiarato.

## Global Constraints

- Una sola dipendenza nuova: `github.com/yuin/goldmark` (decisione D9-1 del piano dei gap, anticipata). `govulncheck` pulito dopo l'aggiunta.
- UI solo con slot tema (il colore calcolato in U1 deriva comunque dal tema o da nero/bianco).
- Gotcha AGENTS.md: value receiver, mouse disattivato con modali aperte, scarto dei risultati stantii, `g`/`G` occupati.
- Commenti e stringhe in italiano con `Piano 7.2, Task N`; README/CHANGELOG in inglese.
- Gate: build, vet, `go test -race`, golangci-lint 0, `govulncheck`, `CGO_ENABLED=0` linux/darwin/windows.

## Waves

```
Wave 1 (parallelo, file disgiunti)
  T1 contrasto (internal/ui/styles)      T2 config (internal/config, wizard)      T5 markdown → HTML (internal/mail/smtp, internal/cmd/send)
Wave 2 (sequenziale, internal/app)
  T3 menu impostazioni (dip. T1 per l'anteprima tema, T2 per i campi) → T4 due colonne (dip. T2, T3)
Wave 3
  T6 documentazione → T7 verifica finale
```

---

### Task 1: contrasto automatico del testo sulle barre colorate

**Files:** `internal/ui/styles/styles.go` (+test).

- Funzione `onColor(bg, fg, base string) string` per ciascuna variante (dark e light separatamente, perché `compat.AdaptiveColor` ha due lati): restituisce il primo tra `fg` e `base` (lo slot `bg` del tema) con rapporto di contrasto WCAG ≥ 4.5 rispetto a `bg`, altrimenti `#000000` o `#ffffff`, quello con rapporto maggiore. Luminanza relativa WCAG 2.x.
- Applicata a ogni stile con `Background`: `ListSelected`, `StatusOK`, `StatusErr`, `StatusBar`, `DemoBanner` e a quelli aggiunti nei task successivi.
- **Test tabellare** (come `TestEveryEmbeddedThemeValid`): per ogni tema embedded × ogni stile con sfondo × variante dark/light → contrasto ≥ 4.5. Un tema nuovo illeggibile fa fallire il test.
- Test unitari della funzione di contrasto su coppie note (nero/bianco = 21, stesso colore = 1).

- [ ] RED → impl → GREEN → Commit `fix(styles): readable text on colored bars (WCAG AA auto contrast)`

### Task 2: nuovi campi di configurazione

**Files:** `internal/config/config.go` (+test), `internal/ui/wizard/wizard.go` (+test).

- `Config` aggiunge `Layout string` (`"single"` | `"split"`, vuoto = `single`) e `PreviewMarkRead string` (`"immediate"` | `"delay"` | `"never"`, vuoto = `delay`), con chiavi TOML `layout` e `preview_mark_read`. `Validate` rifiuta valori sconosciuti. File esistenti senza queste chiavi si caricano con i default.
- `gmail-tui login` oggi riscrive la config con solo indirizzo e tema `default` (`wizard.go`, `config.Config{Address: ..., Theme: "default"}`), cancellando nome e firma. Con nome e firma ora modificabili dal menu, il login deve **conservare** nome, firma, tema, layout e preview dalla config esistente, aggiornando solo l'indirizzo. Rimuovere la limitazione dalle Notes del README (Task 6).

- [ ] RED → impl → GREEN → Commit `feat(config): layout and preview mark-read settings; login keeps existing settings`

### Task 3: menu impostazioni (`,`)

**Files:** create `internal/app/settings.go` (+test); modify `model.go`, `keys.go` (+`keys_test.go`).

- `,` apre la modale da inbox e thread (non dal composer né da altre modali). Voci (U3), nell'ordine: Tema, Layout lettura, Segna come letto in anteprima, Nome, Firma.
- ↑↓/j/k scelgono la voce; ←→/h/l cambiano il valore delle voci a scelta. Tema: anteprima immediata (re-styling come fa oggi `:theme`). Layout: si applica subito.
- Nome: campo di testo su una riga. Firma: invio sulla voce apre un'area di testo multiriga (bubbles textarea), `esc` torna alla lista delle voci.
- Invio sulla lista (fuori dall'editing) salva tutto in config (`cfgPath`, come la persistenza di `:theme` in `ops.go`) e chiude; `esc` annulla: ripristina tema e layout precedenti e chiude. Errore di salvataggio → status d'errore, valori applicati a runtime.
- Le modifiche a nome e firma valgono dal prossimo messaggio composto (aggiornare l'identità usata da `SetIdentity`).
- Mouse disattivato con la modale aperta; `ctrl+c` esce sempre; `,` in `routedKeys`, help breve e overlay `?` aggiornati.
- Stili della modale solo da slot tema; l'eventuale riga selezionata con sfondo usa il colore calcolato di Task 1.
- `:theme` resta funzionante (compatibilità).

**Test:** apertura/chiusura; ←→ sul tema cambia gli stili subito; esc ripristina il tema; invio salva in un config temporaneo e il file contiene i valori; nome e firma aggiornano la bozza successiva; tasti globali (`a`, `#`…) e mouse ignorati con la modale aperta.

- [ ] RED → impl → GREEN → Commit `feat(app): settings menu (,) with live preview`

### Task 4: vista a due colonne

**Files:** create `internal/app/split.go` (+test); modify `model.go`, `internal/ui/inbox/inbox.go` (+test se cambia il rendering delle righe).

- Attiva quando `layout = split` **e** larghezza ≥ 120 colonne; sotto, una colonna automaticamente (nessun messaggio d'errore; il minimo resta 80×24).
- Larghezze: lista `floor(35%)`, anteprima il resto meno il separatore (U8). Altezza come oggi (ultima riga = status bar). Separatore verticale con slot `border`.
- L'anteprima riusa `thread.Model` (header, corpo, quote, allegati) dimensionato sulla colonna destra.
- **Caricamento:** a ogni cambio del messaggio sotto il cursore parte un timer di 150 ms con un token (ID + contatore); allo scadere, se il cursore è ancora lì, `FetchMessage` (cache-first). Risultati con token diverso scartati (stesso pattern anti-stantio di `fetchMsg`). Lista vuota → anteprima vuota con testo attenuato.
- **Segna letto (U5):** `immediate` = al caricamento dell'anteprima; `delay` = timer di 2 s con token, `MarkRead` solo se il messaggio è ancora quello in anteprima ed è non letto; `never` = nulla. Invio (thread a schermo intero) mantiene il comportamento attuale.
- **Focus (U6):** `Tab` alterna lista/anteprima; con focus sull'anteprima j/k/pgup/pgdn/space/d/u scorrono l'anteprima, `esc`/`Tab` tornano alla lista. Le azioni (`r f a s v l # o :`) agiscono sul messaggio in anteprima con entrambi i focus. Indicatore visivo del focus (bordo/separatore con slot `primary` sulla colonna attiva).
- **Mouse:** clic e rotella sulla colonna sinistra come oggi sulla lista; rotella sulla destra scorre l'anteprima; doppio clic apre il thread a schermo intero.
- Righe della lista a 35%: verificare che il delegate dell'inbox tronchi mittente/oggetto senza rompere l'allineamento della data; adattare se serve.
- Cambio cartella, ricerca, archivia/cestino: l'anteprima segue il nuovo messaggio sotto il cursore; con lista vuota si svuota.

**Test:** attivazione per layout e larghezza (119 vs 120); proporzione 35/65; anteprima caricata dopo il timer e non prima; risultato stantio scartato dopo movimento rapido; mark-read per i tre valori (orologio/timer iniettabili, niente sleep reali); Tab/esc; azioni sul messaggio in anteprima; rotella a destra; resize da 140 a 100 colonne torna a una colonna.

- [ ] RED → impl → GREEN → Commit `feat(app): two-column layout with message preview`

### Task 5: markdown → HTML in uscita

**Files:** `internal/mail/smtp/mime.go` (+test), `internal/msg/message.go` (campo nuovo in `Outgoing`), `internal/app` (punto in cui si costruisce l'Outgoing dal composer), `internal/cmd/send.go` (+test), `cmd/gmail-tui/main.go` (flag), `go.mod`.

- `msg.Outgoing` aggiunge `Markdown bool`. Il composer lo imposta sempre (U9); `gmail-tui send` solo con `--markdown`.
- Con `Markdown`: `buildMIME` produce `multipart/alternative` con `text/plain` (il markdown sorgente, leggibile così com'è) e `text/html` (goldmark con estensione GFM, senza `html.WithUnsafe`: l'HTML scritto a mano viene escapato). Con allegati: `multipart/mixed` che contiene prima il `multipart/alternative`, poi gli allegati. Senza `Markdown`: comportamento attuale invariato.
- HTML minimo: documento con `<meta charset="utf-8">`, nessuno stile esterno né script.
- La firma (`-- \n` + firma) fa parte del corpo markdown: verificare che resti su righe separate nell'HTML (riga `-- ` seguita da a capo; usare l'estensione di line break o convertire la firma come blocco a parte).
- Le citazioni della risposta (`> `) diventano `<blockquote>`.

**Test:** struttura MIME (boundary annidati, Content-Type delle parti) con e senza allegati; HTML contiene `<strong>`, `<table>` per una tabella GFM, `<blockquote>` per le citazioni; `<script>` scritto dall'utente risulta escapato; senza `Markdown` il messaggio è identico a prima (test esistenti verdi); CLI `--markdown` imposta il campo.

- [ ] RED → impl → GREEN → `govulncheck` → Commit `feat(smtp): send markdown as multipart/alternative with HTML (goldmark, GFM)`

### Task 6: documentazione

- README: keymap (`,`, `Tab` nelle due colonne), sezione impostazioni, layout a due colonne (≥ 120 colonne), markdown inviato come HTML, `--markdown` nella tabella CLI; togliere dalle Notes "login rewrites the config" e "Markdown bodies are sent as text/plain".
- `site/index.html`: keymap e feature card se presenti.
- CHANGELOG [Unreleased]; ROADMAP (markdown → HTML consegnato, togliere da "planned"); `docs/smoke-checklist.md` (contrasto in un tema chiaro e uno scuro, menu `,`, due colonne, email markdown ricevuta formattata in Gmail web); AGENTS.md se emergono gotcha nuovi; piano dei gap (il markdown esce dal Piano 9).

- [ ] Commit `docs: settings menu, two-column layout, markdown sending, readable themes`

### Task 7: verifica finale

- [ ] Gate completi + check go-imap + `govulncheck`
- [ ] Review indipendente per wave + `/code-review` sul branch, rilievi corretti
- [ ] Prova manuale in demo (tmux, HOME temporanea): contrasto in `catppuccin-latte` e `one-dark`, menu `,` con anteprima tema ed esc che ripristina, due colonne a 140 colonne e ritorno a una colonna a 100
- [ ] Smoke reale (Andrea): email markdown ricevuta formattata

## Rischi e mitigazioni

| Rischio | Mitigazione |
|---|---|
| Il terminale non rileva lo sfondo e sceglie la variante sbagliata del tema | Il contrasto è garantito su entrambe le varianti (test su dark e light) |
| Troppi fetch IMAP scorrendo la lista in due colonne | Attesa di 150 ms con token; cache-first |
| Messaggi segnati letti solo attraversandoli | Default "dopo 2 s" sullo stesso messaggio, con token |
| HTML generato da input non fidato (es. testo incollato) | goldmark senza `WithUnsafe`: HTML grezzo escapato; test con `<script>` |
| Regressione dell'invio senza markdown (CLI, test SMTP) | `Markdown=false` lascia `buildMIME` identico; test esistenti invariati |
| Conflitti in `internal/app` | Wave 2 sequenziale, un solo implementer |
