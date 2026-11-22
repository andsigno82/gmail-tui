# gmail-tui — Piano 5: Community & Release — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Il pacchetto community per il rilascio open-source: README con quickstart e attribuzione pop, checklist smoke manuale, CONTRIBUTING + templates, goreleaser + release workflow, tape VHS per la GIF, roadmap. Pronto al push/tag v0.1.0 (tag e push sono passi manuali dell'utente — il remote/org non è ancora noto).

**Architecture:** Solo docs/CI: nessun codice runtime tranne piccole rettifiche ai testi utente (`--help`, messaggi). Il module path resta `gmail-tui` locale: il README usa il placeholder `github.com/USER/gmail-tui` con istruzioni di rename (`go mod edit -module` + sostituzione import) al push — decisione 2026-11-22 (nome confermato `gmail-tui`, org da definire).

**Tech Stack:** markdown · GitHub Actions · goreleaser · VHS (tape file; generazione GIF opzionale se vhs installato)

## Global Constraints

- Go 1.26, MIT, golangci-lint 0 issues, `go test -race` verde (sempre, anche se i task sono docs)
- Lingua README/CONTRIBUTING: inglese (community); i file doc interni restano italiano
- Attribuzione pop obbligatoria nel README (composer adattato, MIT)
- Nota utente da P4: `gmail-tui login` riscrive la config (name/signature azzerati) — da documentare in README (Troubleshooting/Notes)
- GIF: se `vhs` non è installato, committare solo `docs/demo.tape` + istruzioni `vhs docs/demo.tape` (placeholder img nel README); se installato, generare anche la GIF
- NESSUN push/tag da qui: solo file pronti

## File Structure

```
README.md                                    (Task 1)
docs/smoke-checklist.md                      (Task 1)
CONTRIBUTING.md                              (Task 2)
.github/ISSUE_TEMPLATE/bug_report.md         (Task 2)
.github/ISSUE_TEMPLATE/feature_request.md    (Task 2)
.github/PULL_REQUEST_TEMPLATE.md             (Task 2)
.goreleaser.yaml                             (Task 3)
.github/workflows/release.yml                (Task 3)
docs/demo.tape                               (Task 4)
ROADMAP.md                                   (Task 4)
```

---

### Task 1: README + smoke checklist

**Files:** Create `README.md`, `docs/smoke-checklist.md`.

**Contenuti README (inglese, conciso):**
- Badge CI (placeholder URL), logo/title, one-liner: "A Gmail client for your terminal, built with Charm."
- Screenshot/GIF placeholder (`docs/demo.gif` + istruzione rigenerazione)
- Quickstart 3 passi: brew (placeholder tap) / go install placeholder / build from source; requisiti (2FA + App Password, link myaccount.google.com/apppasswords); configurazione (env o `gmail-tui login` wizard)
- Keymap table (dalla spec: j/k, enter, esc/q, r/f/c, /, a, s, v, l, o, :, ?, mouse)
- Temi: elenco 12 + `:theme` runtime + `--theme` flag
- Comandi CLI: version/themes/doctor/fetch/send/login
- Attribution: composer adattato da charmbracelet/pop (MIT)
- Notes: login riscrive name/signature; IMAP=Gmail preset; md resta text/plain (v1.1 roadmap link)
- Roadmap breve (v1.1: md→HTML multipart, multi-account?, Gmail API backend opzionale)
- License MIT

**docs/smoke-checklist.md (la checklist manuale richiesta dalla spec, inglese):**
- Setup (App Password, env), launch, demo skip, connessione reale
- Sessione: inbox load, open thread, markread auto, r/f/c composer, send (demo Mock e reale), :save/:open, :theme persist, search (from:/is:unread/text), label l, archive/star, IDLE push (seconda mail da web), reconnect (toggle network), doctor output, resize min-size
- Ogni voce: [ ] con risultato atteso

- [ ] Steps: verifica ortografia/coerenza con le feature reali (leggere help/test), commit `docs: README, smoke checklist, roadmap pointers`

### Task 2: CONTRIBUTING + templates

**Files:** Create `CONTRIBUTING.md`, `.github/ISSUE_TEMPLATE/{bug_report,feature_request}.md`, `.github/PULL_REQUEST_TEMPLATE.md`.

- CONTRIBUTING: setup (Go 1.26, make-like comandi go), stile (TDD richiesto, golangci-lint 0 issues, -race), come aggiungere un tema (un TOML + test tabellare — contributor-friendly dalla spec), convenzioni commit (conventional), PR process, code of conduct lite (link Contributor Covenant)
- Templates: bug (versione, OS/terminale, passi, output doctor), feature (problema/use-case), PR (checklist: test, lint, docs, conventional commit)

- [ ] Steps: commit `docs: contributing guide and issue/PR templates`

### Task 3: goreleaser + release workflow

**Files:** Create `.goreleaser.yaml`, `.github/workflows/release.yml`.

- goreleaser: builds cmd/gmail-tui per linux/mac/windows (amd64+arm64 dove applicabile), CGO_ENABLED=0, ldflags `-X main.version={{.Version}}`, checksum, changelog grouped (feat/fix/docs)
- release.yml: on tag push `v*`, goreleaser-action con GITHUB_TOKEN, permissions contents:write
- NOTA: senza repo/org il file usa nomi repo neutri (project_name: gmail-tui); il brew tap section resta commentata con istruzioni (creare homebrew-gmail-tui tap dopo il push)
- Verifica locale: `goreleaser check` se goreleaser installato (brew install goreleaser; se assente: installa via brew — tool standard CI; se l'installazione fallisce, valida YAML + documentalo)

- [ ] Steps: commit `ci: goreleaser config and release workflow for v0.1.0`

### Task 4: VHS tape + roadmap + chiusura

**Files:** Create `docs/demo.tape`, `ROADMAP.md`.

- demo.tape (VHS): Output docs/demo.gif, FontSize 16, Theme Catppuccin Mocha (VHS theme), 1200x700, comandi: build+run demo (env senza password → wizard → s skip), navigazione j/k, enter thread, :theme dracula, c composer → subject → invio, :save, q. Sequenza con Type/Sleep realistica
- Se `vhs` disponibile (brew install vhs): generare docs/demo.gif e referenziarla nel README; altrimenti README mostra placeholder testuale + comando di generazione
- ROADMAP.md: v1.0 (fatto: elenco feature), v1.1 (md→HTML multipart send, CLI send-only completamento, multi-account valutazione), future (Gmail API backend via MailStore — l'astrazione esiste)
- Chiusura: tick "Verifica finale" piano 5; `go build ./... && go vet ./... && go test -race ./... && golangci-lint run ./...` finali
- Commit `docs: vhs demo tape, roadmap, close piano 5`

## Verifica finale del piano

- [x] Tutti i file presenti e coerenti (niente placeholder rotti tranne USER/URL repo) — verificato 2026-11-22 (Task 4): README.md, docs/smoke-checklist.md, CONTRIBUTING.md, .github/ISSUE_TEMPLATE/{bug_report,feature_request}.md, .github/PULL_REQUEST_TEMPLATE.md, .goreleaser.yaml, .github/workflows/release.yml, docs/demo.tape, ROADMAP.md tutti presenti; gli unici placeholder residui sono `USER`/URL repo (mandato del piano — org non ancora deciso); docs/demo.gif è un asset REALE generato con vhs 0.12.1 (547 KB, 31.3 s, 1200×700, tema Catppuccin Mocha) — il nastro isola HOME in `$HOME/.local/share/vhs-home` (wiped a ogni run, verificato: config scritta solo lì, config reale intatta)
- [x] `go test -race ./... && golangci-lint run ./...` verdi — verificato 2026-11-22: `go build ./...` OK, `go vet ./...` OK, `go test -race -count=1 ./...` ok su 16 package, `golangci-lint run ./...` (v2.15.0) → **0 issues**
- [x] README quickstart riproducibile da clone fresco (seguire letteralmente i passi in HOME separata) — percorso build-from-source seguito letteralmente (copia fresca del tree in /tmp/qs/gmail-tui, `go build ./cmd/gmail-tui`, run con `HOME=/tmp/qs/home` isolata via pty/expect): wizard "Benvenuto in gmail-tui" → `s` → lista seeded ("report spese" presente) + banner "demo mode — set GMAIL_APP_PASSWORD" → `q` pulito; nessun file scritto nella HOME isolata (lo skip non scrive config). I percorsi `brew install USER/tap/...` e `go install github.com/USER/...` NON sono riproducibili ora per natura: repo GitHub e modulo non ancora pubblicati (placeholder USER, documentato nel README stesso — diventano funzionanti al push, vedi handoff)
- [x] goreleaser check valido (o validazione YAML documentata) — verificato nel Task 3 (2026-11-22, goreleaser 2.20.3): config validata **con un remote temporaneo** (il check v2 auto-detecta owner/repo dal git remote e senza remote fallisce per motivi ambientali, non di config); workaround documentato in `.superpowers/sdd/task-p5-3-report.md`; la config non è cambiata da allora; ldflags `-X main.version` verificato con smoke su binario
- [x] Handoff utente: lista dei passi manuali (crea repo GitHub → rename module → push → tag v0.1.0 → release → tap brew → Pages source) — lista completa: (1) crea il repo GitHub e pusha il branch (remote ancora assente per scelta); (2) rinomina il modulo: `go mod edit -module github.com/<utente>/gmail-tui` + sostituzione import (istruzioni nel README) + `go mod tidy && go build ./...`; (3) sostituisci `USER` nel README (badge, install, go install) e nel footer di `.goreleaser.yaml`; (4) push e tag `v0.1.0` (`git tag v0.1.0 && git push origin v0.1.0`) → la release workflow pubblica con goreleaser; (5) dopo il release: crea il repo `homebrew-gmail-tui` tap, decommenta la sezione brew in `.goreleaser.yaml` (serve un PAT `TAP_GITHUB_TOKEN`), ri-valida con `goreleaser check`; (6) verifica il CI verde sul repo (badge README); (7) dopo il push su main: Settings → Pages → Build and deployment → Source: "GitHub Actions" (necessario perché il workflow Pages pubblichi la landing page — aggiunto 2026-11-22, Piano 6); (8) opzionale: decommentare `github: USER` in `.github/FUNDING.yml` (sostituendo USER) se si vuole il pulsante Sponsor — aggiunto 2026-11-22, Piano 6. Nota follow-up opzionale (Piano 6): allineare i pin checkout/setup-go di `ci.yml`/`release.yml` ai major correnti (checkout@v7 per evidenza API del 2026-11-22; setup-go v5+ da verificare al momento)

## Dopo il piano

Fase "audit di completamento" (controller): confronto finale spec ↔ app + handoff manuale all'utente.

**Nota 2026-11-22:** presenza GitHub estesa in Piano 6 (site + pages + security/COC/changelog) — vedi `docs/superpowers/plans/2026-11-22-piano-6-github-presence.md`.
