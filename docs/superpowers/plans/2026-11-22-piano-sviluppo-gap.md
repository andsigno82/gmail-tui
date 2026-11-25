# gmail-tui — Piano di sviluppo dei gap (dopo il Piano 7)

> Roadmap interna dei gap rimasti dopo il Piano 7 (`feat/piano-7`). Ogni blocco qui sotto diventa un piano numerato in `docs/superpowers/plans/` (formato dei Piani 1-7) solo quando viene avviato.

**Fonti dei gap:** analisi del 2026-11-22, decisioni D1-D2 del Piano 7, review indipendenti e `/code-review` del Piano 7 (rilievi di gravità bassa rimandati).

**Vincoli che valgono per tutti i piani:** `MailStore` resta l'unico contratto della UI; niente CGO; TDD senza rete; ogni nuova dipendenza è una decisione esplicita di Andrea e passa `govulncheck`.

## Decisioni (Andrea, 2026-11-22)

| ID | Decisione | Scelta |
|---|---|---|
| D7.1 | Indirizzi IDN e `a@b` | Punycode con `x/net/idna` + dominio con punto obbligatorio (test che usano `a@b` da aggiornare) |
| D8-1 | Integrazione X-GM-EXT-1 | Patch in-tree: `third_party/go-imap` + `replace` |
| D9-1 | Markdown → HTML | Sì, goldmark (da validare con `govulncheck`) |
| D9-2 | Fonti dei contatti | Entrambe: indirizzi già visti + comando esterno opzionale stile `address-book-cmd` |
| D10 | Annulla per archivia/cestino | Sì |
| D11-1 | Notifiche desktop | Comando configurabile (hook stile `mail-received` di aerc) |
| D11-2 | Push fuori da INBOX | No: resta solo INBOX |
| D13 | Backend Gmail API | Solo su domanda reale (resta in "Later") |
| D14 | Server MCP per agenti | Solo lettura: nessun invio, nessuna bozza, nessuna modifica (archivia, label, cestino), per evitare che venga usato per spam o abusi. Ordine e dipendenza (dopo il Piano 8 o prima) da confermare |

Restano scelte tecniche da chiudere dentro i singoli piani: D8-2 (formato degli ID Gmail `uint64`) e il tasto della selezione multipla (Piano 10).

## Quadro

| Piano | Tema | Valore | Costo | Dipende da |
|---|---|---|---|---|
| 7.1 | Rifiniture rapide (dalle review) | medio | basso | Piano 7 mergiato |
| 8 | Estensioni Gmail: conversazioni, ricerca nativa, label per messaggio | alto | alto | — |
| 9 | Scrittura: bozze, markdown → HTML, contatti | alto | medio | — |
| 10 | Smistamento: selezione multipla, annulla, rimozione label | medio | medio | 8 per la rimozione label |
| 11 | Notifiche desktop | medio | basso | — |
| 12 | Sincronizzazione incrementale e prestazioni | medio | medio | — |
| 13 | Multi-account; backend Gmail API solo su domanda (D13) | alto per chi non può usare App Password | alto | 12 consigliato |

---

## Piano 7.1 — Rifiniture rapide

Rilievi bassi delle review del Piano 7, ognuno da poche righe:

- **CLI `send`:** spezza per virgola prima di validare, quindi `"Rossi, Mario <m@x.it>"` viene rifiutato. Usare `net/mail.ParseAddressList` per ogni campo.
- **`doctor`:** aggiungere il controllo delle capability Gmail usate da cestino e cartelle (MOVE, UIDPLUS, SPECIAL-USE, LIST-EXTENDED), con esito esplicito.
- **Domini IDN** (`x@café.it`): convertire in punycode con `golang.org/x/net/idna`, già presente come dipendenza indiretta, già presente come dipendenza indiretta (D7.1).
- **`a@b` senza punto nel dominio:** oggi accettato, Gmail lo rifiuta. Richiedere il punto (D7.1); aggiornare i test esistenti che usano `a@b`.
- **`#` su righe del Cestino trovate con una ricerca da un'altra cartella:** il controllo "già nel cestino" usa la cartella mostrata. Il backend rifiuta comunque; allineare il controllo UI alla mailbox della riga, come già fatto per `a` e `l`.
- **Round IDLE inutile dopo il MOVE di Trash:** solo costo; filtrare l'EXPUNGE causato da un'op propria.

## Piano 8 — Estensioni Gmail (X-GM-EXT-1)

**Obiettivo:** conversazioni vere (`X-GM-THRID`), ricerca con la sintassi Gmail completa lato server (`X-GM-RAW`), label di ogni messaggio (`X-GM-LABELS`).

**Perché serve una patch:** go-imap v2 upstream non supporta X-GM-EXT-1, non permette comandi grezzi, e il decoder di FETCH va in errore sugli attributi sconosciuti.

**Modello di riferimento:** il progetto atrium tiene una copia in-tree di go-imap con circa 200 righe di patch, agganciata con `replace`; il fork `excavador/go-imap` (`v2.0.0-beta.8.gmext.1`) fa lo stesso come modulo separato.

**Cosa sblocca:**
- vista thread con tutti i messaggi della conversazione, comprese le risposte dell'utente in Inviati (ricerca per `X-GM-THRID` in `[Gmail]/Tutti i messaggi`);
- ricerca `/` con gli operatori Gmail completi (`older_than:`, `in:`, `-label:`…), passati come stringa `X-GM-RAW`;
- label visibili in lista e rimozione label da qualunque cartella (prerequisito del Piano 10);
- `MessageRef.ThreadLen` finalmente valorizzato.

**Impatto sul contratto:** `Query` acquista un campo per la query grezza; `MailStore` un metodo per leggere una conversazione. Il Mock va esteso.

**Test:** il memserver non conosce X-GM-*. Servono test a livello di protocollo con un server finto a risposte registrate (le forme delle risposte documentate dal fork riadafridishibly, verificate su account reale).

**Scelto (D8-1):** patch in-tree in `third_party/go-imap` con `replace` in `go.mod`. Va documentata la procedura per riallinearla a una nuova versione di go-imap.

**Da chiudere nel piano:**
- **D8-2:** gli ID Gmail sono `uint64` (alcuni superano 2^63): tenerli fuori da `MessageID` o estendere il formato `MAILBOX:UID`.

## Piano 9 — Scrittura

**Consegnato** (branch `feat/piano-9`): bozze (Esc salva, Ctrl+D scarta, ripresa da Bozze), annulla invio (5 s, `u`), rubrica locale e comando esterno `address_book_cmd` per A/Cc/Ccn. Nota D9-2: il campo Ccn è stato aggiunto al composer (decisione del coordinatore), quindi Ccn è coperto anche dai suggerimenti. Verifica reale su Gmail: vedi `docs/smoke-checklist.md`, sezione Piano 9.

- **Bozze:** `Ctrl+S` salva o "posponi" la bozza con APPEND nella mailbox `\Drafts` (ruolo SPECIAL-USE, già letto da `ListMailboxes`); dalla cartella Bozze `r`/invio la riapre nel composer. Modello: `:postpone`/`:recall` di aerc. Nuovo metodo `MailStore.SaveDraft`.
- **Markdown → HTML:** consegnato nel Piano 7.2 (`multipart/alternative` con GFM, `gmail-tui send --markdown`, goldmark per D9-1). Non più in questo piano.
- **Autocompletamento contatti** nei campi A/Cc/Ccn. Due fonti possibili: gli indirizzi già visti nelle ENVELOPE e nella cache (zero configurazione), più un comando esterno opzionale come `address-book-cmd` di aerc (es. khard), entrambi (D9-2).
- **Annulla invio** (ritardo di qualche secondo come Gmail web): opzionale, da valutare con D10.

## Piano 10 — Smistamento

**Consegnato** (branch `feat/piano-10`): selezione con `x`/`X`/`esc`, `a`/`#`/`s`/`v`/`l` su tutta la selezione, annulla di archivia e cestino con `u` (10 s), pannello label con stato misto su Gmail. Decisioni in `docs/superpowers/plans/2026-11-23-piano-10-smistamento.md`. Verifica reale su Gmail: `docs/smoke-checklist.md`, sezione Piano 10.

- **Selezione multipla:** marcare più righe e applicare `a`/`#`/`s`/`v`/`l` a tutte. Modello: `:mark` di aerc (con modalità visuale) e il tagging di neomutt. Tasto da scegliere: `x` (Gmail web) o spazio, oggi usato per scorrere nel thread.
- **Annulla** l'ultima `a`/`#`: Gmail web mostra "Annulla" per qualche secondo. Con UIDPLUS il MOVE restituisce i nuovi UID (`MoveData` in go-imap), quindi l'operazione inversa è un MOVE di ritorno preciso. Nuovo stato "ultima op annullabile" nell'app.
- **Rimozione label** dalla vista di qualunque cartella: con `X-GM-LABELS` (Piano 8) è una STORE; senza, oggi funziona solo dalla cartella della label stessa.
- Annulla incluso (D10).

## Piano 11 — Notifiche desktop

**Stato:** implementato sul branch `feat/piano-11` (piano: `docs/superpowers/plans/2026-11-22-piano-11-notifiche.md`). Smoke reale su account di Andrea: `docs/smoke-checklist.md`, sezione Piano 11.

- **Notifiche alla posta nuova** con un comando configurabile in `config.toml`, eseguito a ogni `ChangeNewMail` con mittente e oggetto in variabili d'ambiente, come l'hook `mail-received` di aerc (D11-1). Zero dipendenze: l'utente sceglie `notify-send`, `terminal-notifier`, ecc. Il comando gira fuori dal loop UI, con timeout, e i dati del messaggio passano solo come variabili d'ambiente, mai interpolati in una shell (il contenuto delle email non è fidato).
- **Push fuori da INBOX:** non si fa (D11-2). Le altre cartelle si aggiornano dopo ogni operazione, come oggi.

## Piano 12 — Sincronizzazione incrementale e prestazioni

**Stato (24 novembre 2026): diviso in due.** **12a**, ritocchi e misura, in
`docs/superpowers/plans/2026-11-23-piano-12a-ritocchi-e-misura.md`, implementato:
codifica MIME delle parti di testo, `Connect` con contesto (TLS provato), ruolo di
`\Important`, niente righe fantasma, traccia per passo IMAP. La voce "Trash su più
cartelle" è chiusa senza codice: l'app già gestisce l'errore a metà, una
conversazione alla volta. **12b**, CONDSTORE, prestazioni della lista e ricerca
paginata: piano proprio, scritto dopo una traccia reale presa con 12a.

- **CONDSTORE** (`CHANGEDSINCE`, `HIGHESTMODSEQ`, supportati da go-imap v2): rileggere solo i messaggi cambiati invece di rilistare fino a 500 righe a ogni refresh. Elimina il tetto 500 e il compromesso D5 (coda tenuta dopo stella/letto).
- **Paginazione della ricerca:** oggi `Search` fa FETCH di tutti i risultati. Paginarla come la lista (o con ESEARCH, se Gmail lo annuncia).
- **Trash su più cartelle:** oggi può fermarsi a metà lasciando spostato solo il primo gruppo. Riportare quali messaggi sono stati spostati invece di un errore generico.
- **`[Gmail]/Importanti`** (`\Important`) appariva come label normale: mappato come ruolo nel 12a.
- **Codifica MIME del corpo:** aggiungere `Content-Transfer-Encoding` e il limite di 998 caratteri per riga (RFC 5322) a `text/plain` e `text/html`. Prima del 12a le parti di testo non dichiaravano CTE (`internal/mail/smtp/mime.go`), quindi valeva il default MIME 7bit, e nessun limite di riga era applicato. Corretto nel 12a.

## Piano 13 — Account

- **Multi-account:** profili in config, voci keychain e cache per account, selettore account. Modello: account multipli di aerc (`accounts.conf`) e himalaya.
- **Backend Gmail API** con OAuth device flow, dietro `MailStore`, per gli account dove le App Password non sono disponibili (account Workspace con policy restrittive). Richiede un progetto Google Cloud e una verifica dell'app per gli scope Gmail: costo organizzativo, non solo tecnico. Si fa solo se emerge una domanda reale (D13).

## Ordine proposto

1. **7.1** subito dopo il merge del Piano 7 (piccolo, chiude i rilievi delle review).
2. **8** (già deciso come piano separato): il più alto valore, e sblocca parte del 10.
3. **9** e **11** in parallelo se si vuole: toccano parti diverse (composer contro watch).
4. **10**, poi **12**.
5. **13** solo se c'è domanda reale (utenti senza App Password).

## Piano 14 — Server MCP in sola lettura (D14)

- Sottocomando `gmail-tui mcp` (stdio) sopra `MailStore`: elenco cartelle, ricerca, lettura di messaggi e conversazioni. Nessun tool che invia, salva bozze o modifica la casella: il vincolo è nel server, perché una App Password non si può restringere alla sola lettura.
- Difese contro la prompt injection (modello: server MCP Gmail ufficiale di Google e `ni-c/imap-mcp`): corpo dei messaggi racchiuso in marcatori con un valore casuale diverso a ogni chiamata, contenuto dichiarato come non fidato, password mai esposta all'agente (resta nel portachiavi).
- SDK: `github.com/modelcontextprotocol/go-sdk` (ufficiale, v1.8.0 stabile al 2026-11): nuova dipendenza da approvare.
- Più utile dopo il Piano 8: la ricerca `X-GM-RAW` e le conversazioni rendono l'esplorazione da parte di un agente molto più efficace.

## Piano 15 — Beautify (rendering, cartelle, finestre, splash)

**Consegnato** (branch `feat/piano-15`, Task 1-18 e 16b): blocchi HTML con bottoni, tabelle, citazioni ed elenchi; link con controllo di dominio (`⚠` e conferma); allegati cliccabili per PDF, immagini e testo; cartelle annidate con contatori dei non letti; finestre con ombra a retino; barra di stato a segmenti; splash all'avvio. Decisioni in `docs/superpowers/plans/2026-11-23-piano-15-beautify.md`. Ancora aperti: Task 19 (misure e revisione indipendente) e lo smoke su Ghostty in `docs/smoke-checklist.md`, sezione Piano 15.

## Fonti

- [atrium #73 — patch Gmail per go-imap v2](https://github.com/jeiang/atrium/issues/73), [atrium #103 — copia in-tree](https://github.com/jeiang/atrium/issues/103)
- [riadafridishibly/go-imap #12](https://github.com/riadafridishibly/go-imap/issues/12), [#19](https://github.com/riadafridishibly/go-imap/issues/19)
- [excavador/mail-mcp #4 — fork gmext](https://github.com/excavador/mail-mcp/pull/4)
- [aerc-config(5): hooks, address-book-cmd](https://www.mankier.com/5/aerc-config), [aerc(1): :mark, :postpone, :recall](https://manpages.debian.org/bookworm/aerc/aerc.1.en.html), [CHANGELOG aerc](https://github.com/rjarry/aerc/blob/master/CHANGELOG.md)
