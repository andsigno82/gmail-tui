# Patch Gmail su go-imap v2 (third_party/go-imap)

Copia in-tree di `github.com/emersion/go-imap/v2` versione **v2.0.0-beta.9**, presa dal module cache.
Agganciata al progetto con `replace` in `go.mod`. Decisione D8-1 del Piano 8 (`docs/superpowers/plans/2026-11-22-piano-8-estensioni-gmail.md`).

La copia resta identica a upstream salvo i file elencati nella tabella delle patch. Ogni patch ha il task che la introduce.

## Patch

| Task | File | Patch |
|---|---|---|
| 1 | (nessuno) | Copia iniziale, nessuna modifica |

## Procedura di riallineamento a una nuova versione upstream

1. Scaricare la nuova versione in una directory temporanea (`go mod download github.com/emersion/go-imap/v2@<tag>`) e copiarla in `third_party/go-imap.new`.
2. Confrontare la copia attuale con la vecchia versione upstream (`diff -ru <vecchia upstream> third_party/go-imap`): le differenze sono solo le patch elencate sopra.
3. Sostituire `third_party/go-imap` con la nuova upstream e riapplicare le patch, file per file, dalla tabella.
4. Aggiornare la riga "versione" in questo file.
5. Eseguire `cd third_party/go-imap && go test ./...`, poi `go test -race -count=1 ./...` dalla radice e `govulncheck ./...`.
6. Verificare che `go.mod` non richieda dipendenze nuove (`GOPROXY=off go build ./...`).

I test della copia si lanciano dalla sua directory: `third_party/go-imap` è un modulo separato, quindi `go test ./third_party/...` dalla radice fallisce con "setup failed".
