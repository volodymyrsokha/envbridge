<div align="center">

# 🌉 envbridge

**Team `.env` management with no middle server.**

Your server *is* the secret store — envbridge syncs encrypted env files between
your team and your own infrastructure over the SSH access you already have.

[![CI](https://github.com/volodymyrsokha/envbridge/actions/workflows/ci.yml/badge.svg)](https://github.com/volodymyrsokha/envbridge/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/volodymyrsokha/envbridge?color=blue)](https://github.com/volodymyrsokha/envbridge/releases/latest)
[![Go Version](https://img.shields.io/github/go-mod/go-version/volodymyrsokha/envbridge)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-lightgrey.svg)](LICENSE)

<img src="demo/demo.gif" alt="envbridge demo: push, hand-edit detection, and adoption" width="900">

</div>

No SaaS subscription. No self-hosted dashboard to babysit. No daemon listening on a port.
Just a single binary, your SSH keys, and [age](https://age-encryption.org) encryption.

---

## Why envbridge

Every small team ends up in the same place: env files live on the server, one
person SSHes in and edits them by hand, and everyone else asks *"can you send
me the latest staging env?"* in Slack.

The existing fixes all add infrastructure:

| | Doppler / Infisical Cloud | Self-hosted Vault | SOPS in git | **envbridge** |
|---|---|---|---|---|
| Your secrets live | on their servers | on a service you operate | in git history, forever | on **your** server, where they already are |
| Setup | account + agent | deploy + maintain a service | key ceremony + deploy hooks | `envbridge init` |
| Server needs | internet + agent | database, upgrades, backups | git + hooks | **sshd** (already there) |

envbridge keeps the model your team already trusts — *the server is the source
of truth* — and upgrades the workflow around it: encryption at rest, drift
detection, atomic writes, backups, and a CLI that tells you what's going on.

## How it works

```
  laptop                        your server
 ┌─────────────────┐  SSH/SFTP ┌──────────────────────────────┐
 │ .env.production ◄───────────► /var/lib/envbridge/<project>/ │
 │  (plaintext,    │  pull/push │   production.env.age  🔒     │
 │   gitignored)   │            │   backups/ locks/ manifests  │
 └─────────────────┘            │              │ materialize   │
                                │              ▼               │
                                │   /srv/app/.env (plaintext,  │
                                │    what your app reads)      │
                                └──────────────────────────────┘
```

- Canonical env files live on your server, **encrypted with age** to your team's public keys.
- envbridge **materializes** a plaintext `.env` next to your app on every push — your app never knows envbridge exists.
- Devs `pull` / `push` / `diff` over plain SSH, reusing `~/.ssh/config` aliases, ssh-agent, and jump hosts.
- The team roster is a list of **public keys committed to your repo** — adding a teammate is a pull request.
- If someone SSHes in and hand-edits the live file at 2 a.m., envbridge **notices and offers to adopt** the change instead of clobbering it.

## Install

**One-liner** (macOS / Linux, amd64 / arm64 — verifies checksums):

```console
$ curl -fsSL https://raw.githubusercontent.com/volodymyrsokha/envbridge/main/install.sh | sh
```

**With Go:**

```console
$ go install github.com/volodymyrsokha/envbridge/cmd/envbridge@latest
```

**Manually:** grab an archive from the [releases page](https://github.com/volodymyrsokha/envbridge/releases), untar, and put `envbridge` on your `PATH`.

Servers need nothing installed — envbridge talks plain SFTP. (The optional `edit --local` workflow on a server is the same single binary.)

## Quick start

```console
$ envbridge keygen               # your age keypair, once per machine
$ envbridge init                 # wizard: environments, hosts, store setup
✓ Created store on prod-1:/var/lib/envbridge/acme-api
✓ production: imported existing /srv/acme/.env (14 keys, encrypted)
✓ .envbridge.yaml written — commit it so your team can join
```

The daily loop:

```console
$ envbridge pull production      # fetch + decrypt to .env.production
$ envbridge diff production      # what differs, values masked
$ envbridge edit staging         # decrypt → $EDITOR → re-encrypt → push
$ envbridge push production      # encrypt, atomic swap, backup, materialize
```

Teammates join in three commands:

```console
they:  envbridge init            # prints their public key + instructions
you:   envbridge team add --name "Anna" --email anna@team.dev --key age1…
you:   envbridge team sync       # re-encrypts every env for the new roster
```

## Commands

| Command | What it does |
|---|---|
| `init` | Bootstrap a project (wizard, imports existing server envs) or join one |
| `pull [env…]` | Fetch, decrypt, and write the local env file |
| `push [env…]` | Encrypt and upload — conflict detection, lock, backup, atomic swap |
| `diff [env]` | Semantic key-level diff, secret values masked by default |
| `status` | Sync state of every environment at a glance |
| `edit <env>` | Decrypt → `$EDITOR` → re-encrypt → push, in one step (works on the server too) |
| `team add/remove/sync` | Manage recipients; `sync` re-encrypts for the current roster |
| `keygen` | Generate your age identity |

Every command supports `--no-color` (and the `NO_COLOR` env var); `status` speaks `--json` for scripting.

## Safety model

envbridge is built around four hashes and refuses to lose data:

- **Teammate pushed since your last pull?** Your `push` stops: *pull, review, push again.*
- **Someone hand-edited the live file on the server?** `push` refuses to clobber it; `pull --adopt` makes the hotfix canonical.
- **You have local changes?** `pull` shows a masked diff and asks before overwriting.
- Every push **backs up the previous version** and swaps files **atomically** — your app never sees a half-written `.env`.

## The trust model, stated plainly

- **Encryption at rest protects** the store, its backups, server snapshots, and anything that leaks the blob files — useless without a team member's private key.
- **It does not protect against a teammate with SSH access** to the box: the app needs a plaintext `.env` to run, and anyone who can read that file can read your secrets. That's true of every tool that ends in a plaintext env file — envbridge just refuses to pretend otherwise.
- **Removing a teammate** = revoke their SSH access + `envbridge team remove` + rotate any secrets they held. Cryptography can't un-share what someone already saw.

Plaintext touches disk in exactly three places — your local env file, the server's materialized file, and the tempfile during `edit` — all created `0600` and written atomically. Everything else is decrypted in memory only. Host keys are verified against `known_hosts`; envbridge never skips host verification.

## Status

**Working pre-release**, validated against real servers. The full command
surface is covered by tests, including integration tests that run every sync
scenario against an in-process SSH server: conflicts, hand-edit adoption, lock
contention, trust-on-first-use, and team re-encryption. Architecture lives in
[DESIGN.md](DESIGN.md).

Until a security review, don't point it at secrets you can't rotate.

## License

[MIT](LICENSE)
