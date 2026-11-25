# gmail-tui: stato dei lavori

24 novembre 2026 · Andrea Signorile

Tutti i piani da 7 a 16 sono in `main` e pubblicati su GitHub. Il Piano 12a, ritocchi e misura, è sul branch `feat/piano-12a`, in attesa del merge. Restano gli smoke test sull'account reale, il Piano 12b e alcuni rischi noti da verificare.

## Fatto

Ogni piano ha seguito lo stesso percorso: piano TDD scritto prima in `docs/superpowers/plans/`, poi implementazione con revisione indipendente, merge in `main` e push. L'unica release pubblicata resta `v0.1.0` (22 novembre): tutto quello che segue è in `main` ma non ancora in una release.

| Data | Piano | Cosa è cambiato | Commit su main |
| --- | --- | --- | --- |
| 24 nov | 12a, ritocchi e misura | Codifica delle parti di testo in uscita, niente righe fantasma, Connect interrompibile alla scadenza (con il TLS provato dai test), Importanti tra le cartelle speciali, traccia per passo IMAP; ROADMAP allineata a `v0.1.0` | `feat/piano-12a` |
| 23 nov | 16, cache locale | Lista e conversazioni subito dall'istantanea locale, in sola lettura finché Gmail non conferma; ultimi 200 messaggi aperti in cache; Connect, wizard e doctor a 90 s | `99679a3` |
| 23 nov | Prestazioni e traccia | Meno rendering e copie nel percorso dei tasti; traccia dei tempi con `GMAIL_TUI_TRACE=1` e comando `gmail-tui trace` | `e7718ce` |
| 23 nov | 15, beautify | Bottoni, tabelle, citazioni ed elenchi resi nel terminale; avviso phishing sui link; albero cartelle con non letti; finestre con ombra; barra di stato a segmenti; splash | `6429097` |
| 23 nov | Rifiniture | Caselle di selezione attenuate, tinta delle righe spuntate, spinner centrato, mai il messaggio precedente all'apertura, link cliccabili, `v` che funziona nei due sensi | `e1cdb5e` |
| 23 nov | 10, smistamento | Selezione multipla `x`/`X`, annulla `u` per 10 s, pannello label con stato misto | `17d1bc8` |
| 23 nov | 9 e 11, scrittura e notifiche | Bozze, rubrica, annulla invio, Ccn; notifiche desktop con comando configurabile | `03ea51c` |
| 22 nov | 8 e 14, Gmail ed MCP | Conversazioni e ricerca Gmail (X-GM), label vere, go-imap patchato nel repo; server MCP `gmail-tui mcp` in sola lettura | `6900e8b` |
| 22 nov | 7.3, toast | Gli esiti diventano notifiche a scomparsa, gli errori restano fino al tasto | `4fed2d5` |
| 22 nov | 7.2, leggibilità | Contrasto WCAG su tutti i 12 temi, menu impostazioni `,`, layout a due colonne, markdown inviato come HTML | `9163dfb` |
| 22 nov | 7 e 7.1, uso quotidiano | Paginazione, cartelle, cestino, validazione destinatari e rifiniture | `f8c220c` |
| 22 nov | Base | AGENTS.md, RIEPILOGO.md, release `v0.1.0` con 5 binari, landing page | `v0.1.0` |

Oltre al codice: `AGENTS.md` per gli agenti, `RIEPILOGO.md` aggiornato a ogni piano, `docs/smoke-checklist.md` con i passi di verifica manuale per ogni piano.

## Decisioni prese

Queste scelte sono tue e restano ferme finché non le cambi tu. Il dettaglio di ognuna è nel piano indicato.

| Tema | Decisione | Dove |
| --- | --- | --- |
| Server MCP | Solo lettura: niente invio, bozze o modifiche, per evitare che venga usato per spam | Piano 14 |
| Stack | Tutto lo stack Charm in v2 (bubbletea, bubbles, lipgloss) | Piano 7.3 |
| Esiti delle azioni | Toast invece della barra colorata: info e successo 3 s, errori fino al tasto | Piano 7.3 |
| Selezione | Caselle attenuate, luminose e in grassetto quando spuntate; riga spuntata con tinta tenue | Rifiniture |
| Link a rischio | Se il testo visibile mostra un altro dominio, il link si apre solo dopo conferma | Piano 15 |
| Allegati | Si aprono solo PDF, PNG, JPEG, GIF e testo, da una cartella privata pulita all'uscita | Piano 15 |
| Cache | Corpi degli ultimi 200 messaggi aperti, scartando i meno usati (non FIFO); anche istantanee di lista e thread; nessuna cifratura | Piano 16 |
| Dati provvisori | Azioni rifiutate con toast finché la lista non è confermata; si mostra l'età dei dati | Piano 16 |
| Timeout | Connect a 90 s nell'app, nel wizard e nel doctor | Piano 16 |
| Lavoro con agenti | Opus pianifica, Sonnet per i compiti rischiosi, Haiku per quelli meccanici; il coordinatore orchestra; merge e push solo con il tuo ok | Processo |
| Piano 12 | Due piani: 12a ritocchi e misura, 12b prestazioni con una spec propria dopo una traccia reale | Piano 12a |
| Codifica in uscita | `7bit` se il testo è ASCII puro con righe fino a 998 byte, altrimenti `quoted-printable` | Piano 12a |

## Da fare: smoke test sull'account reale

Nessun piano dall'8 in poi è stato provato sul tuo account Gmail con i passi della checklist: i test girano su server finti in memoria. L'unica sessione reale finora è la traccia dei tempi del 23 novembre. I passi sono in `docs/smoke-checklist.md`, una sezione per piano.

- [ ] **Piano 8, conversazioni**: thread raggruppati, ricerca con sintassi Gmail, label applicate e tolte
- [ ] **Piano 9, scrittura**: bozza salvata con flag `\Draft`, Ccn assente dalle intestazioni anche nelle bozze, annulla invio
- [ ] **Piano 10, smistamento**: selezione multipla, annulla dell'archivia che non crea una label "\Inbox"
- [ ] **Piano 11, notifiche desktop**: comando eseguito all'arrivo di posta nuova
- [ ] **Piano 14, server MCP**: un agente legge cartelle e messaggi, nessuno strumento di scrittura disponibile
- [ ] **Piano 15, rendering su Ghostty**: sottolineature dei link, conferma ⚠ sui link a rischio, albero cartelle, splash
- [ ] **Piano 12a**: "Importanti" dopo Posta in arrivo; un messaggio con accenti e una riga lunga in "Show original"; nessuna riga vuota dopo una cancellazione da Gmail web (passi in `docs/smoke-checklist.md`)
- [ ] **Piano 12b, la misura**: una sessione con `GMAIL_TUI_TRACE=1` per confrontare con la misura di prima (prima lista a 106 s); la tabella "Passi IMAP" incollata in chat decide l'ordine del 12b
- [ ] **Piano 16, cache**: seconda apertura con lista immediata, toast di rifiuto durante l'aggiornamento, Connect fallito in sola lettura
- [ ] **Piano 16, dopo `R` o dopo il wizard**: archivia e cestina restano rifiutati finché non torna la lista fresca (fino a circa 77 s). Va deciso se è accettabile
- [ ] **Piano 16, ricerca o cambio cartella prima di Connect**: compare il toast "store non connesso". Va deciso se il messaggio va bene

## Da fare: Piano 12b

Il Piano 12 è diviso in due (decisione del 23 novembre). Il **12a**, ritocchi e misura, è implementato sul branch `feat/piano-12a`, piano in `docs/superpowers/plans/2026-11-23-piano-12a-ritocchi-e-misura.md`. Resta il **12b**, prestazioni, che avrà una spec propria dopo la traccia reale con i passi per comando:

1. Niente rilista dopo `s` o `v`: solo i flag cambiati, con CONDSTORE
2. Meno comandi nella lista: oggi ogni lista rilancia il LIST di tutte le cartelle e ogni operazione rifà la SELECT
3. La base del Watch senza la SEARCH di tutti gli UID di INBOX
4. Ricerca oltre i primi risultati
5. Contesto per le altre operazioni dello store, da interrompere in modo pulito

Altri lavori aperti:

- [ ] **Release**: tutto quello che c'è dopo `v0.1.0` è solo in `main`. Serve un tag (per esempio `v0.2.0`) per pubblicare i binari
- [ ] **Social preview di GitHub**: l'immagine è in `.superpowers/sdd/social-preview.png`, da caricare a mano in Settings → General (non esiste un'API)
- [ ] **Tap Homebrew** (facoltativo): serve il repo `homebrew-gmail-tui` e il token `TAP_GITHUB_TOKEN`, poi si attiva la sezione in `.goreleaser.yaml`
- [ ] **Lentezza di Gmail**: la traccia ha mostrato Connect a 29 s e liste fino a 77 s. La cache la nasconde ma non la risolve: la tabella "Passi IMAP" dice quale comando è lento, e vale la pena controllare anche le impostazioni IMAP e di sicurezza dell'account

## Rischi noti

Nessuno di questi è un problema di sicurezza: R1-R4 e la guardia degli ID bloccano comunque ogni azione su dati non confermati.

| Rischio | Effetto | Stato |
| --- | --- | --- |
| Azioni bloccate dopo `R` o wizard | Archivia e cestina danno errore su righe già viste, fino al ritorno della lista fresca | Da provare sul reale |
| Ricerca o cartella prima di Connect | Toast "store non connesso"; la lista provvisoria resta | Da provare sul reale |
| Riga archiviata che riappare | Se la cache non sa il GmailID, la riga torna all'avvio successivo, solo provvisoria | Cosmetico, accettato |
| Seconda istanza in parallelo | Non ottiene il lock della cache entro 1 s e parte senza istantanee | Previsto dal piano |
| `UpdateGmailCopies` | Vecchio schema di scrittura senza controllo nil, precedente al Piano 16 | Non raggiungibile oggi |
| Cache reale già allo schema 4 | Un vecchio test l'ha migrata il 23 novembre alle 14:58; dati intatti, test ora isolati | Chiuso |
| Cambio reale di UIDVALIDITY | Coperto dai test, mai visto su Gmail vero | Da osservare |
| `\r` isolato nel corpo di un messaggio in uscita | Resta com'è, come prima del Piano 12a: esce dichiarato `7bit` | Cosmetico, accettato |

Un Connect lento su Gmail resta il limite principale: la cache rende l'avvio immediato, ma le azioni aspettano comunque il server.
