# Security Policy

## Supported versions

Security fixes are applied to the latest release line only.

| Version | Supported |
|---------|-----------|
| 0.1.x   | yes       |
| older   | no        |

## Reporting a vulnerability

**Please do not open a public issue for a security problem.**

Preferred: report privately via **GitHub Security Advisories** — on this repository's *Security* tab, use *Report a vulnerability*. This keeps the report confidential and lets us discuss and fix it before disclosure.

If that is not possible, contact the maintainer directly through GitHub (private message on the [andsigno82 profile](https://github.com/andsigno82) — not a public issue).

Please include: the version or commit you tested, steps to reproduce or a proof of concept, the affected component (TUI, IMAP/SMTP backend, wizard/keychain, CLI, config), and your assessment of the impact.

## Scope

What is in scope for gmail-tui:

- **Credential handling** — the Gmail App Password is resolved from the `GMAIL_APP_PASSWORD` environment variable or the **system keychain** (via the operating system's own keychain CLI); it is never written to the config file and never logged. Keychain-related errors mask the secret. A bug that leaks the password to config, logs, or the terminal is a vulnerability.
- **Transport and authentication** — the client talks only to Gmail's official endpoints (IMAP over TLS on `imap.gmail.com:993`, SMTP over STARTTLS on `smtp.gmail.com:587`) using an App Password. There are **no OAuth tokens** stored or requested, and no third-party servers or telemetry.
- **The bundled keychain integration** — gmail-tui shells out to the OS keychain CLI (e.g. `security` on macOS); bugs in how gmail-tui builds those commands or handles their output are in scope. Weaknesses of the OS keychain implementations themselves are not.

- **MCP server** — `gmail-tui mcp` exposes mail to AI agents over stdio, read-only. Email content is untrusted input for the agent: the defenses are the per-call nonce markers, the untrusted flag and a read-only tool set. A tool that writes to the mailbox, leaks the App Password, or lets email text escape its markers is a vulnerability.
- **Local cache** — message bodies, senders and subjects are stored **unencrypted** in `cache.db` in the user cache directory: the folder is mode 0700 and the file mode 0600 (set by gmail-tui on open). It holds the last 200 opened messages and the first page of each folder. The password is never written to it. The file is bound to the account address by a SHA-256 hash; the address itself is not stored, and a change of account empties the cache. Anyone who can read your user account's files can read the cached mail. A bug that makes the cache readable by other users, stores the password in it, or shows one account's mail under another account is a vulnerability. To delete the cache, quit gmail-tui and remove the `gmail-tui` folder in the user cache directory.

Out of scope: Gmail's own security, accounts without 2-Step Verification, issues that require an attacker to already control your machine or config file, and demo mode (offline, seeded data, no network).

## Disclosure timeline

- **48 hours** — acknowledgement of your report.
- **7 days** — an assessment and a decision on whether it is accepted as a security issue.
- **Fix and disclosure** — we aim to ship a patch for supported versions within 30 days and will publish a GitHub Security Advisory crediting you (unless you prefer to stay anonymous). We will keep you informed at every step.
