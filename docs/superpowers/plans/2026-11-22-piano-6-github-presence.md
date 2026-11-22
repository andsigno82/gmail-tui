# gmail-tui — Piano 6: presenza GitHub open-source (estensione Piano 5) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Il pacchetto completo per la presenza GitHub open-source: **pagina di presentazione** (GitHub Pages, statica) deployata automaticamente, più i file standard mancanti (SECURITY, CODE_OF_CONDUCT, CHANGELOG, FUNDING) e i link nel README. Il repo esiste ancora solo localmente: tutto usa i placeholder `github.com/USER/gmail-tui` coerenti con Piano 5.

**Architecture:** Landing page = **un singolo `site/index.html` self-contained** (CSS inline, zero framework — NIH banale) deployato su GitHub Pages via workflow moderno (`actions/upload-pages-artifact` + `actions/deploy-pages`, source "GitHub Actions" da impostare in Settings al push — passo manuale documentato). Il sito NON serve da `docs/` (evita di esporre specs/piani interni): cartella dedicata `site/`. Contenuti derivati SOLO da verità già verificate (GIF esistente, temi TOML, keymap reale).

**Tech Stack:** HTML/CSS statico · GitHub Actions Pages · vercel/ncc no — solo actions ufficiali

## Global Constraints

- Placeholder `github.com/USER/gmail-tui` e `https://USER.github.io/gmail-tui/` ovunque (coerenza P5)
- Zero dipendenze/temi: la pagina usa i COLORI del tema dracula/default come palette statica (hex presi dai TOML — dati, non codice)
- Nessuna modifica a codice Go runtime (solo README link + file nuovi)
- Gates sempre verdi: `go vet && go test -race && golangci-lint 0 issues`
- Verifica visiva della pagina OBBLIGATORIA con il browser Playwright (screenshot) prima del commit
- Lingua: inglese (pagina pubblica)

## File Structure

```
site/index.html                        (Task 1: landing page self-contained)
.github/workflows/pages.yml            (Task 2: deploy Pages da site/)
SECURITY.md                            (Task 2)
CODE_OF_CONDUCT.md                     (Task 2)
CHANGELOG.md                           (Task 2: v0.1.0)
.github/FUNDING.yml                    (Task 2: commentato/placeholder)
README.md                              (Task 1-2: sezione Site + link ai nuovi file)
docs/superpowers/plans/...piano-5...md (Task 3: nota collegamento al Piano 6)
```

---

### Task 1: landing page (site/index.html)

**Files:** Create `site/index.html`; modify `README.md` (riga "🌐 **[Landing page](https://USER.github.io/gmail-tui/)** — screenshots, themes showcase" vicino al badge).

**Contenuto (single-file, inline CSS, dark theme dracula-inspired):**
- Hero: `gmail-tui` + tagline "A Gmail client for your terminal — built with Charm." + GIF `docs/demo.gif` — ATTENZIONE PATH: da una pagina servita in `/gmail-tui/`, i path relativi devono funzionare sia in locale che su Pages: usare `../docs/demo.gif`? NO — su Pages da artifact la root è la cartella site/. Decisione: copiare `docs/demo.gif` → `site/demo.gif` nel Task 1 (asset duplicato voluto, il tape lo rigenera; nota nel file) e referenziare `demo.gif` relativo
- Sezione features: 6 card (Inbox & threads, Compose/reply/forward, Search, Labels & archive, Attachments, 12 themes) — testo breve derivato dal README (già verificato)
- Themes showcase: 12 strisce con nome + swatch (hex dai TOML: fg/bg/primary/accent per ognuno — leggere i 12 file e trascrivere; dati statici, drift accettato con commento "generated from internal/theme/themes/*.toml at v0.1.0")
- Quickstart: 3 comandi (install placeholder, config env o login, run) + link "Full README"
- Keymap mini-table (8 righe principali)
- Footer: MIT · link repo placeholder · link ROADMAP/CONTRIBUTING
- Responsive minimo (max-width container, flex wrap)

**Verifica obbligatoria:** aprire `site/index.html` col browser Playwright (`mcp__playwright__browser_navigate` su file:// o server locale python -m http.server) + screenshot → verificare rendering (hero, GIF visibile, swatch leggibili) PRIMA di committare. Includere gli screenshot nel report (path).

- [ ] Steps: write → playwright verify + screenshot → README link → `go vet && go build` (sanity) → Commit `docs(site): landing page for GitHub Pages`

### Task 2: workflow Pages + file standard + README links

**Files:** Create `.github/workflows/pages.yml`, `SECURITY.md`, `CODE_OF_CONDUCT.md`, `CHANGELOG.md`, `.github/FUNDING.yml`; modify `README.md` (link a SECURITY/CONTRIBUTING/CHANGELOG).

- **pages.yml**: on push to main (paths: site/**) + workflow_dispatch; permissions pages:write + id-token:write; steps: checkout → upload-pages-artifact (path: site) → deploy-pages. Commento in testa: "Settings → Pages → Source: GitHub Actions da attivare una volta dopo il push"
- **SECURITY.md**: versione supportata (v0.1.x), come riportare (GitHub Security Advisories / email placeholder), scope (credential handling: password mai loggate/config; keychain CLI), disclosure timeline
- **CODE_OF_CONDUCT.md**: Contributor Covenant 2.1 standard (testo canonico breve + link versione completa)
- **CHANGELOG.md**: formato Keep a Changelog; entry [0.1.0] con Added/Changed/Security riassunti dai commit P1-P5 (feat/fix — ~15 voci significative, non 54)
- **FUNDING.yml**: `github: USER` commentato con nota (decommentare al push se si vuole)
- README: badge/links section — aggiungere link a SECURITY e CHANGELOG

- [ ] Steps: write → YAML validati (python yaml) → `go build && go vet` sanity → Commit `docs: pages workflow, security policy, code of conduct, changelog`

### Task 3: chiusura + handoff aggiornato

**Files:** modify `docs/superpowers/plans/2026-11-22-piano-5-community.md` (nota: estensione in Piano 6), plan P6 checkboxes; **verifica finale globale**.

- Nota di collegamento nel piano 5: "Presenza GitHub estesa in Piano 6 (site + pages + security/COC/changelog)"
- P6 "Verifica finale": tick con annotazioni oneste (pages.yml NON eseguibile senza repo → validato YAML + azioni ufficiali; pagina verificata con screenshot Playwright)
- Gates finali globali: `go vet ./... && go test -race ./... && golangci-lint run ./...` + lista passi manuali aggiornata (aggiungere: Settings→Pages→Source GitHub Actions; decommentare FUNDING se voluto; set description/topics/social preview nel repo)
- Commit `docs: close piano 6 (github presence), update handoff`

## Verifica finale del piano

- [ ] site/index.html render verificato con screenshot Playwright (hero/GIF/swatch/footer)
- [ ] pages.yml validato (azioni ufficiali, permissions corrette)
- [ ] CHANGELOG/SECURITY/COC coerenti coi fatti del progetto
- [ ] Gates Go verdi; nessun segreto; placeholder consistenti
- [ ] Handoff utente aggiornato (Pages setup + metadata repo)

## Dopo il piano

Tutto il resto resta ai passi manuali dell'utente (repo, push, tag, release, tap, Pages source, metadata social). Roadmap v1.1 invariata.
