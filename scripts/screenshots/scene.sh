# Runs inside unshare -rmnu (see record.sh): sets up the network and the
# fake services, then runs the Playwright scripts. $1 is the work dir.
set -e
W=$1
ip link set lo up
ip link add dummy0 type dummy
ip addr add 10.20.0.5/24 dev dummy0
ip link set dummy0 up
hostname web-01
mount -t tmpfs tmpfs /srv
mkdir -p /srv/bin
for b in node python3 postgres redis-server nginx sshd; do cp "$W/fakesrv" /srv/bin/$b; done
mkdir -p /srv/dev/storefront/node_modules/next/dist/bin /srv/dev/admin-ui/node_modules/vite/bin \
  /srv/dev/docs-site/node_modules/@docusaurus/core/bin /srv/dev/billing-api/.venv/bin
touch /srv/dev/storefront/node_modules/next/dist/bin/next /srv/dev/admin-ui/node_modules/vite/bin/vite.js \
  /srv/dev/docs-site/node_modules/@docusaurus/core/bin/docusaurus.mjs /srv/dev/billing-api/.venv/bin/uvicorn
printf '%s' '{"name":"storefront","dependencies":{"next":"15.5.0","react":"19.1.0"}}' > /srv/dev/storefront/package.json
printf '%s' '{"name":"admin-ui","devDependencies":{"vite":"7.1.0"}}' > /srv/dev/admin-ui/package.json
printf '%s' '{"name":"docs-site","dependencies":{"@docusaurus/core":"3.8.0"}}' > /srv/dev/docs-site/package.json
printf '[project]\nname = "billing-api"\ndependencies = ["fastapi>=0.115", "uvicorn"]\n' > /srv/dev/billing-api/pyproject.toml

# Services run from /, as daemons do; dev servers from their project.
cd /
SERVE_PORT=5432 SERVE_REPLY=48000 /srv/bin/postgres -D /var/lib/postgresql/17/main >/dev/null 2>&1 &
SERVE_PORT=6379 SERVE_REPLY=600 /srv/bin/redis-server 127.0.0.1:6379 >/dev/null 2>&1 &
SERVE_PORT=22 SERVE_BIND=0.0.0.0 /srv/bin/sshd -D >/dev/null 2>&1 &
sleep 0.5
(cd /srv/dev/billing-api && SERVE_PORT=8000 SERVE_REPLY=26000 SERVE_CONNECT=127.0.0.1:5432:900:0.15,127.0.0.1:6379:120:0.25 \
  /srv/bin/python3 .venv/bin/uvicorn app.main:app --port 8000 >/dev/null 2>&1 &)
sleep 0.3
(cd /srv/dev/storefront && SERVE_PORT=3000 SERVE_REPLY=64000 SERVE_CONNECT=127.0.0.1:8000:1800:0.12,127.0.0.1:6379:90:0.3 \
  /srv/bin/node node_modules/next/dist/bin/next dev >/dev/null 2>&1 &)
(cd /srv/dev/admin-ui && SERVE_PORT=5173 SERVE_REPLY=9000 SERVE_CONNECT=127.0.0.1:8000:600:0.4 \
  /srv/bin/node node_modules/vite/bin/vite.js >/dev/null 2>&1 &)
(cd /srv/dev/docs-site && SERVE_PORT=3001 SERVE_REPLY=4000 \
  /srv/bin/node node_modules/@docusaurus/core/bin/docusaurus.mjs start >/dev/null 2>&1 &)
sleep 0.3
SERVE_PORT=80 SERVE_BIND=0.0.0.0 SERVE_WORKERS=4 SERVE_REPLY=70000 SERVE_CONNECT=127.0.0.1:3000:2400:0.1 \
  /srv/bin/nginx -g "daemon off;" >/dev/null 2>&1 &
sleep 1

cd "$W"
"$W/devserver" -hide-self -frontend "$FRONTEND" >"$W/devserver.log" 2>&1 &
sleep 6
python3 "$W/desktop.py" "$W"
python3 "$W/web.py" "$W"
