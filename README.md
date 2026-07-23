# envbridge

**Team `.env` management with no middle server.** Your server *is* the secret store — envbridge syncs encrypted env files between your team and your own infrastructure over the SSH access you already have.

```
$ envbridge status
  ENV          LOCAL            SERVER           LAST PUSH
  production   ✓ in sync        ✓ clean          dana · 2d ago
  staging      ● 3 uncommitted  ⚠ hand-edited    you · 4h ago
```

No SaaS subscription. No self-hosted dashboard to babysit. No daemon listening on a port. Just a single binary, your SSH keys, and [age](https://age-encryption.org) encryption.

## Why

Every small team ends up in the same place: env files live on the server, one person SSHes in and edits them by hand, and everyone else asks "can you send me the latest staging env?" in Slack.

The existing fixes all add infrastructure:

| | Doppler / Infisical Cloud | Self-hosted Infisical / Vault | SOPS in git | **envbridge** |
|---|---|---|---|---|
| Your secrets live | on their servers | on a service you now operate | in git history, forever | on **your** server, where they already are |
| Setup | account + agent | deploy + maintain a service | key ceremony + deploy hooks | `envbridge init` |
| Server needs | internet + agent | database, upgrades, backups | git + hooks | **sshd** (already there) |

envbridge keeps the model your team already trusts — *the server is the source of truth* — and upgrades the workflow around it: encryption at rest, drift detection, atomic writes, backups, and a CLI that tells you what's going on.

## How it works

- Canonical env files live on your server, encrypted with age to your team's public keys (`/var/lib/envbridge/<project>/production.env.age`).
- envbridge **materializes** a plaintext `.env` next to your app on every push — your app never knows envbridge exists.
- Devs `pull`/`push`/`diff` over plain SSH/SFTP, reusing `~/.ssh/config` aliases, ssh-agent, and jump hosts.
- The team roster is a list of age public keys in `.envbridge.yaml`, committed to your repo — adding a teammate is a pull request.
- If someone SSHes in and hand-edits the live file at 2 a.m., envbridge notices (hash mismatch) and offers to **adopt** the change instead of clobbering it.

## Quick start

```console
$ envbridge init
? Project name … acme-api
? Environment name … production
? SSH host (from ~/.ssh/config) … prod-1
? Where does the app read its .env? … /srv/acme/.env

✓ Generated your age identity (~/.config/envbridge/identity.txt)
✓ Created store on prod-1:/var/lib/envbridge/acme-api
✓ Imported existing /srv/acme/.env → production (encrypted, 14 keys)
✓ Wrote .envbridge.yaml — commit it so your team can join
```

Then the daily loop:

```console
$ envbridge pull production        # fetch + decrypt to .env.production
$ envbridge diff production        # what differs, values masked
$ envbridge edit staging           # $EDITOR round-trip: decrypt → edit → confirm → push
$ envbridge push production        # encrypt, atomic swap, backup, materialize
```

Teammates join with two commands:

```console
$ envbridge init                   # detects .envbridge.yaml → prints your public key
$ # open a PR adding your key to recipients, then anyone on the team runs:
$ envbridge team sync              # re-encrypts every env for the new roster
```

## Commands

| Command | What it does |
|---|---|
| `init` | Bootstrap a project (wizard) or join an existing one |
| `pull [env…]` | Fetch, decrypt, and write the local env file |
| `push [env…]` | Encrypt and upload — with conflict detection, lock, backup, atomic swap |
| `diff [env]` | Semantic key-level diff, secret values masked by default |
| `status` | Sync state of every environment at a glance |
| `edit <env>` | Decrypt → `$EDITOR` → re-encrypt → push, in one step (works on the server too) |
| `team add/remove/sync` | Manage recipients; `sync` re-encrypts for the current roster |
| `keygen` | Generate your age identity |

## The trust model, stated plainly

envbridge is honest about what encryption buys you:

- **Encryption at rest protects** the store, its backups, server snapshots, and anything that leaks the blob files — those are useless without a team member's private key.
- **It does not protect against a teammate with SSH access** to the box: the app needs a plaintext `.env` to run, and anyone who can read that file can read your secrets. That's true of every tool that ends in a plaintext env file — envbridge just refuses to pretend otherwise.
- **Removing a teammate** = revoke their SSH access + `envbridge team remove` + rotate any secrets they held. Cryptography can't un-share what someone already saw.

Plaintext touches disk in exactly three places — your local env file, the server's materialized file, and the tempfile during `edit` — all created `0600` and written atomically. Everything else is decrypted in memory only. Host keys are verified against `known_hosts`; envbridge never skips host verification.

## Install

```console
$ go install github.com/vladimirsokha/envbridge/cmd/envbridge@latest
```

Homebrew tap and prebuilt binaries are planned for the first tagged release.

## Status

Early development — the design is settled ([DESIGN.md](DESIGN.md)), the surface above is being built. Not yet ready for production secrets.

## License

MIT
