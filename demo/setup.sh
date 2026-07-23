#!/bin/sh
# Builds the self-contained environment demo/demo.tape records against:
# a local SSH+SFTP server playing "prod-1", a fake $HOME with the ssh alias
# and known_hosts pre-trusted, and a seeded acme-api project.
set -eu

ROOT=/tmp/envbridge-demo
REPO=$(cd "$(dirname "$0")/.." && pwd)

if [ -f "$ROOT/server.pid" ]; then
  kill "$(cat "$ROOT/server.pid")" 2>/dev/null || true
fi
rm -rf "$ROOT"
mkdir -p "$ROOT/bin" "$ROOT/home/.ssh" "$ROOT/project"
chmod 700 "$ROOT/home/.ssh"

cd "$REPO"
go build -o "$ROOT/bin/envbridge" ./cmd/envbridge
go build -o "$ROOT/bin/demo-server" ./demo/server

"$ROOT/bin/demo-server" -addr 127.0.0.1:2222 -dir "$ROOT/home" >/dev/null 2>&1 &
echo $! > "$ROOT/server.pid"
sleep 1
ssh-keyscan -p 2222 127.0.0.1 > "$ROOT/home/.ssh/known_hosts" 2>/dev/null

cat > "$ROOT/home/.ssh/config" <<'EOF'
Host prod-1
  HostName 127.0.0.1
  Port 2222
EOF

export ENVBRIDGE_IDENTITY="$ROOT/home/identity.txt"
"$ROOT/bin/envbridge" keygen >/dev/null
PUB=$(grep 'public key:' "$ENVBRIDGE_IDENTITY" | awk '{print $4}')

cat > "$ROOT/project/.envbridge.yaml" <<EOF
version: 1
project: acme-api
store: ~/store
environments:
  production:
    host: prod-1
    materialize: ~/app/.env
    local: .env.production
recipients:
  - name: Vlad
    email: vlad@acme.dev
    key: $PUB
EOF

cat > "$ROOT/project/.env.production" <<'EOF'
# acme-api — production
DATABASE_URL=postgres://acme:s3cr3t@db.internal:5432/acme
REDIS_URL=redis://cache.internal:6379
STRIPE_SECRET_KEY=sk_demo_51HxQ9LkXo2vRwPmT8uY4dGnA
SESSION_SECRET=9f8e7d6c5b4a39281706f5e4d3c2b1a0
API_TIMEOUT=30
EOF

cd "$ROOT/project"
HOME="$ROOT/home" ENVBRIDGE_USER=dana@devops "$ROOT/bin/envbridge" push production >/dev/null 2>&1

cat > "$ROOT/env.sh" <<EOF
export HOME="$ROOT/home"
export ENVBRIDGE_IDENTITY="$ROOT/home/identity.txt"
export ENVBRIDGE_USER=vlad@laptop
export PATH="$ROOT/bin:\$PATH"
export PS1='$ '
cd "$ROOT/project"
EOF

echo "demo env ready — run: source $ROOT/env.sh"
