# gmail-tui: stato dei lavori

23 novembre 2026 · Andrea Signorile

Tutti i piani da 7 a 16 (escluso il 12) sono in `main` e pubblicati su GitHub. Restano gli smoke test sull'account reale, il Piano 12 e alcuni rischi noti da verificare.

## Fatto

Ogni piano ha seguito lo stesso percorso: piano TDD scritto prima in `docs/superpowers/plans/`, poi implementazione con revisione indipendente, merge in `main` e push. L'unica release pubblicata resta `v0.1.0` (22 novembre): tutto quello che segue è in `main` ma non ancora in una release.

| Data | Piano | Cosa è cambiato | Commit su main |
| --- | --- | --- | --- |
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

## Da fare: smoke test sull'account reale

Nessun piano dall'8 in poi è stato provato sul tuo account Gmail: i test girano su server finti in memoria. I passi sono in `docs/smoke-checklist.md`, una sezione per piano.

- [ ] **Piano 8, conversazioni**: thread raggruppati, ricerca con sintassi Gmail, label applicate e tolte
- [ ] **Piano 9, scrittura**: bozza salvata con flag `\Draft`, Ccn assente dalle intestazioni anche nelle bozze, annulla invio
- [ ] **Piano 10, smistamento**: selezione multipla, annulla dell'archivia che non crea una label "\Inbox"
- [ ] **Piano 11, notifiche desktop**: comando eseguito all'arrivo di posta nuova
- [ ] **Piano 14, server MCP**: un agente legge cartelle e messaggi, nessuno strumento di scrittura disponibile
- [ ] **Piano 15, rendering su Ghostty**: sottolineature dei link, conferma ⚠ sui link a rischio, albero cartelle, splash
- [ ] **Piano 16, cache**: seconda apertura con lista immediata, toast di rifiuto durante l'aggiornamento, Connect fallito in sola lettura
- [ ] **Piano 16, dopo `R` o dopo il wizard**: archivia e cestina restano rifiutati finché non torna la lista fresca (fino a circa 77 s). Va deciso se è accettabile
- [ ] **Piano 16, ricerca o cambio cartella prima di Connect**: compare il toast "store non connesso". Va deciso se il messaggio va bene
- [ ] **Traccia dei tempi**: un avvio con `GMAIL_TUI_TRACE=1` per confrontare con la misura di prima (prima lista a 106 s)

## Da fare: Piano 12 e passi manuali

Il Piano 12 è l'ultimo del piano dei gap (`docs/superpowers/plans/2026-11-22-piano-sviluppo-gap.md`) e non è ancora scritto in dettaglio. Contiene:

1. Sincronizzazione incrementale con CONDSTORE, invece di rileggere la lista intera
2. Connect con contesto (`ctx`) dentro il package IMAP, per interromperlo in modo pulito
3. Codifica dei messaggi in uscita con righe oltre 998 caratteri (CTE)
4. Righe fantasma nella lista quando la conversione in riferimento (`toRef`) fallisce
5. Ricerca paginata, oggi limitata alla prima pagina di risultati

Altri lavori aperti:

- [ ] **Release**: tutto quello che c'è dopo `v0.1.0` è solo in `main`. Serve un tag (per esempio `v0.2.0`) per pubblicare i binari
- [ ] **ROADMAP.md è indietro**: elenca ancora come "planned" bozze, azioni multiple e notifiche, che sono già fatte; mancano MCP, beautify e cache
- [ ] **Social preview di GitHub**: l'immagine è in `.superpowers/sdd/social-preview.png`, da caricare a mano in Settings → General (non esiste un'API)
- [ ] **Tap Homebrew** (facoltativo): serve il repo `homebrew-gmail-tui` e il token `TAP_GITHUB_TOKEN`, poi si attiva la sezione in `.goreleaser.yaml`
- [ ] **Lentezza di Gmail**: la traccia ha mostrato Connect a 29 s e liste fino a 77 s. La cache la nasconde ma non la risolve: vale la pena controllare le impostazioni IMAP e di sicurezza dell'account

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

Un Connect lento su Gmail resta il limite principale: la cache rende l'avvio immediato, ma le azioni aspettano comunque il server.
