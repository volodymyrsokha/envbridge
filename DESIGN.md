# envbridge — Architecture

This document is the source of truth for envbridge's design. The elevator pitch and user-facing docs live in [README.md](README.md).

## 1. Overview

envbridge is a single static Go binary acting as a smart SSH/SFTP client for an encrypted env-file store on the team's own server. There is no daemon and no third-party service.

All state lives in four places:

| Location | Contents | Committed? |
|---|---|---|
| `.envbridge.yaml` (repo root) | environments, hosts, team recipients | yes |
| `~/.config/envbridge/` | user's age identity, personal settings | no (per user) |
| `.envbridge/` (repo, gitignored) | `state.json` — per-env base snapshots for drift detection | no (per clone) |
| `/var/lib/envbridge/<project>/` (server) | encrypted blobs, manifests, locks, backups | n/a |

The core abstraction is the `store.Store` interface with two v1 implementations: `sftpStore` (dev machines, over SSH) and `localStore` (the same binary run on the server, used by `envbridge edit` there and by every integration test). A v2 daemon or any other transport is a third implementation — nothing above the interface changes.

## 2. Configuration

### 2.1 Committed: `.envbridge.yaml`

```yaml
version: 1
project: acme-api
store: /var/lib/envbridge/acme-api

environments:
  production:
    host: prod-1                       # ~/.ssh/config alias (user@host:port also accepted)
    materialize: /srv/acme/.env        # plaintext path the app reads on the server
    local: .env.production             # what `pull` writes locally (gitignored)
  staging:
    host: staging.acme.internal
    materialize: /srv/acme-staging/.env
    local: .env.staging

recipients:
  - name: Volodymyr Sokha
    email: volodymyr.sokha@mathema.me
    key: age1qy8p7ev3jh0v5m2c...
  - name: Dana (devops)
    email: dana@acme.dev
    key: age1r3w9k2m8f4x7n1q...
```

Design points:

- **Recipients are committed.** Only public keys — adding a teammate is a reviewable PR, and every clone carries the roster needed to re-encrypt.
- `host` is deliberately an ssh alias so users, ports, and jump hosts stay in `~/.ssh/config` where they already work.
- `local` paths and `.envbridge/` are appended to `.gitignore` by `init`.
- `team add` edits this file through yaml.v3's Node API so the user's formatting and comments survive.

### 2.2 User-local: `~/.config/envbridge/` (via `os.UserConfigDir`)

```
config.yaml     # optional: { identity: <path>, editor: "code --wait" }
identity.txt    # age X25519 identity, 0600, standard age file format
```

`identity.txt` is a plain age identity file (inspectable with age tooling). `ENVBRIDGE_IDENTITY` overrides the path for CI and servers. v1 identities are unencrypted-at-0600 — the same posture as a passphrase-less `~/.ssh/id_ed25519`, which sits next to it and is the more powerful credential anyway. A passphrase option on `keygen` is a v1.1 item.

### 2.3 Per-clone state: `.envbridge/state.json`

```json
{
  "version": 1,
  "envs": {
    "production": {
      "base_blob_sha256": "9f2c…",
      "base_plaintext_sha256": "a11d…",
      "pulled_at": "2026-07-23T10:02:11Z"
    }
  }
}
```

This is the merge base: the hashes of the encrypted blob and its plaintext as of this clone's last pull/push. It is what makes drift detection three-way (§5).

### 2.4 Server store

```
/var/lib/envbridge/acme-api/
├── production.env.age
├── production.manifest.json
├── staging.env.age
├── staging.manifest.json
├── locks/production.lock              # exists only while a push/edit is in flight
└── backups/production/2026-07-23T10-02-11Z.env.age   # last 10 kept
```

```json
{
  "version": 1,
  "blob_sha256": "9f2c…",
  "plaintext_sha256": "a11d…",
  "materialize_path": "/srv/acme/.env",
  "recipients_fingerprint": "sha256 of sorted recipient keys",
  "updated_by": "volodymyr.sokha@mathema.me",
  "updated_at": "2026-07-23T10:02:11Z",
  "tool_version": "0.1.0"
}
```

Manifests are per-env so concurrent pushes to different envs never contend. `plaintext_sha256` is the hand-edit tripwire. Note: a plaintext hash lets a store reader confirm a guessed file content — acceptable, since the plaintext itself sits on the same box at `materialize_path`.

## 3. Package layout

```
cmd/envbridge/main.go        thin shim → internal/cli.Execute()
internal/
  cli/                       cobra commands; thin — parse flags, call ops, render via ui
  envfile/                   lossless .env parser/writer (see §4)
  envdiff/                   semantic key-level diff + value masking
  agecrypt/                  filippo.io/age wrapper: Encrypt/Decrypt/GenerateIdentity/LoadIdentity
  store/                     Store interface, manifest schema, localStore, sftpStore, ops.go
  sshx/                      ssh_config resolution, agent auth, known_hosts, ProxyJump dialing
  config/                    project/local config load-validate-save, git-style discovery
  state/                     .envbridge/state.json
  ui/                        lipgloss styles, huh prompts/spinners, error rendering, tables
  version/                   set via -ldflags
```

Everything is `internal/` in v1 — no public API commitment. `envfile` is kept free of envbridge-specific types so it can later be extracted as a standalone library.

### Dependencies

| Concern | Library | Why |
|---|---|---|
| CLI | spf13/cobra | the gh/kubectl standard; completions, nested commands |
| Styling | charmbracelet/lipgloss | colors/tables; degrades on non-TTY |
| Prompts | charmbracelet/huh (+ huh/spinner) | wizard-quality forms without a bubbletea event loop |
| SSH | golang.org/x/crypto/ssh (+ agent, knownhosts) | canonical |
| SFTP | github.com/pkg/sftp | O_EXCL opens (locks), rename, chmod |
| ssh_config | github.com/kevinburke/ssh_config | HostName/User/Port/IdentityFile/ProxyJump resolution |
| Encryption | filippo.io/age (library) | first-party Go; no age binary required on any machine |
| YAML | gopkg.in/yaml.v3 | Node API → comment-preserving `team add` |
| Test SSH | github.com/gliderlabs/ssh | in-process SSH+SFTP server; CI without Docker |

## 4. The envfile package

The single most load-bearing piece. Existing parsers (godotenv, go-envparse) have read-only semantics — writing through them destroys comments, ordering, blank lines, and quoting style. envbridge round-trips files a devops has hand-curated for years, so:

- Line-based AST: `File` holds `[]Line`; a `Line` is `Blank | Comment | Entry | Malformed`. `Entry` records key, value, quote style, `export` prefix, and inline comment.
- `Render()` guarantees **byte-identical round-trip** for any input (`Malformed` lines are preserved verbatim; commands refuse to *push* files containing them unless `--force`).
- Mutations (`Set`, `Unset`) touch only the affected entry's value bytes; everything else is untouched.
- Verified by table-driven round-trip tests and a Go fuzz test asserting `Render(Parse(x)) == x`.

## 5. Drift and conflict model

Four hashes produce three independent checks:

| Check | Comparison | Meaning | Surfaced |
|---|---|---|---|
| **L** | sha(local file) vs `base_plaintext_sha256` | unpushed local edits | `status` "● N uncommitted"; `pull` confirms before overwrite |
| **R** | `manifest.blob_sha256` vs `base_blob_sha256` | teammate pushed since your pull | `push` aborts: "pull, review, re-push" |
| **H** | sha(materialized file) vs `manifest.plaintext_sha256` | hand-edit on the server | `status` "⚠ hand-edited"; `push`/`pull` stop and offer adoption |

**Adoption** (`pull --adopt`, also offered interactively when H trips): fetch the materialized plaintext, show a masked diff against the decrypted blob, confirm, then encrypt the hand-edited content as the new canonical blob (lock → backup → swap → manifest). A 2 a.m. hotfix becomes a first-class change; it is never silently overwritten, and pushing over it without `--adopt`/`--force` always aborts.

**L+R together** is a true conflict. v1 does not auto-merge: `push` aborts with guidance to `diff` and `pull`. A key-level three-way merge is a natural v1.1 — `state.json` provides the base and the AST is key-indexed, so no additional state is needed.

## 6. Core flows

All remote flows share one pipeline in `store/ops.go`; commands differ in direction and rendering.

### push (the careful one)

1. Parse local file — refuse with line-numbered errors on malformed lines.
2. Acquire lock: `OpenFile(locks/<env>.lock, O_CREATE|O_EXCL)` with `{who, host, at}`; stale after 10 min with interactive takeover. Released on defer and SIGINT.
3. CAS check **R**, hand-edit check **H** (both abort with guidance; `--force` overrides loudly).
4. Encrypt to current recipients. age output is non-deterministic — CAS always compares remembered hashes, never re-encryptions.
5. Upload to `<env>.env.age.tmp-<rand>` → back up current blob → rename over → prune backups to 10.
6. Materialize plaintext: temp file in the same directory, `0600`, rename — the app never sees a half-written file.
7. Write manifest, unlock, update `state.json`.

### pull

Download manifest + blob → H check first → decrypt (friendly error if you're not a recipient) → if local edits exist (L), masked diff + confirm → atomic local write, `0600` → update state.

### edit

Auto-selects `localStore` when the resolved host is this machine (or `--local`) — this is the devops replacement for vim-on-the-server. Lock → decrypt to a `0600` tempfile in a private temp dir → `$VISUAL`/`$EDITOR` → re-parse (offer re-edit on syntax error; no changes → no push) → masked diff + confirm → push pipeline steps 4–7 → zero-overwrite and delete the tempfile on every exit path.

### init (bimodal)

- **Join** (`.envbridge.yaml` exists): ensure identity (offer keygen inline), print public key with literal PR instructions, update `.gitignore`, verify SSH connectivity per env.
- **Bootstrap** (wizard): project + environments via huh forms → identity + self as first recipient → create store dirs on each host (permission errors print the exact `sudo mkdir/chown` to run) → **import existing materialized files as the initial blobs** — the devops' hand-edited files become the store with zero retyping → write config + gitignore.

### team

- `add`: validate the key parses as X25519, insert via yaml Node surgery, prompt to sync.
- `sync`: per env — lock → decrypt (runner must be an existing recipient) → re-encrypt to current roster → swap + backup + manifest (`recipients_fingerprint` updated; `status` flags "recipients out of date" until synced).
- `remove`: same, plus the honest warning: re-encryption stops *future* reads only — revoke SSH and rotate secrets the person held.

## 7. Security posture

Plaintext touches disk in exactly three places, all `0600` + atomic rename:

1. the dev-machine `local` file (the product's purpose),
2. the server `materialize` path (the app requires it),
3. the `edit` tempfile (scrubbed and removed on all exit paths).

Everything else — `diff`, `status`, `team sync`, pre-push checks — decrypts in memory only.

Host keys are verified against `known_hosts` with an explicit trust-on-first-use prompt; `InsecureIgnoreHostKey` is never used. Locks are advisory, carry holder identity, and go stale after 10 minutes. Identity files are enforced `0600` on load.

**Trust model** (also in README): encryption-at-rest protects blobs, backups, and snapshots — not against a teammate with SSH access to the box, who can read the materialized file directly. Offboarding = revoke SSH + `team remove` + rotate.

## 8. Known risks

- **ssh_config fidelity.** v1 dials natively: agent → identity files, recursive ProxyJump, known_hosts. `ProxyCommand`, `Match`, and `ControlMaster` are *not* interpreted — envbridge names the unsupported directive in its error. A fallback transport that tunnels stdio through the system `ssh` binary is reserved as a fast-follow, contained entirely inside `sshx`.
- **Materialize ownership.** Atomic rename requires write permission on the *directory*, and the app user must retain read access. v1 verifies after materializing and fails loudly with the exact fix; `materialize_mode: "0640"` plus documented group setup covers app-runs-as-different-user. No sudo escalation attempts.

## 9. Roadmap

| Milestone | Deliverable | Status |
|---|---|---|
| M1 | `envfile` parser/writer + fuzz round-trip | ✅ done |
| M2 | `envdiff` + masking; `agecrypt` + `keygen` | ✅ done |
| M3 | config + state + `Store` + `localStore` + `edit --local` | ✅ done |
| M4 | `sshx` + `sftpStore` + pull/push/status/diff (integration-tested on in-process gliderlabs/ssh) | ✅ done |
| M5 | `init` wizard (incl. server import) + `team` | ✅ done |
| M6 | CI, goreleaser, NO_COLOR, `version` | ✅ done — animated spinners/huh forms and brew tap publication remain |

### v2 anticipation (frozen now, built later)

- `store.Store` is the only seam commands use — a daemon, HTTPS store, or ssh-exec transport is a new implementation, not a refactor.
- `backups/` with sortable timestamped blobs is proto-history; `History()` joins the interface later without breaking either implementation. No logic may assume exactly one blob per env.
- `state.json` base snapshots + the key-indexed AST are exactly the inputs for a v1.1 three-way merge.

```go
type Store interface {
    ReadManifest(ctx context.Context, env string) (*Manifest, error)
    ReadBlob(ctx context.Context, env string) ([]byte, error)
    WriteBlob(ctx context.Context, env string, blob []byte, m *Manifest) error
    ReadMaterialized(ctx context.Context, env string) ([]byte, error)
    WriteMaterialized(ctx context.Context, env string, plaintext []byte) error
    Lock(ctx context.Context, env string, info LockInfo) (unlock func() error, err error)
    Init(ctx context.Context) error
}
```
