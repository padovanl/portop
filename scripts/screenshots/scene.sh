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
for b in node python3 postgres redis-server nginx sshd syncthing transmission-gtk php ruby bash; do cp "$W/fakesrv" /srv/bin/$b; done
# A terminal for Restart and Start to find (never run here).
mkdir -p /srv/term && printf '#!/bin/sh\nexit 0\n' > /srv/term/xterm && chmod +x /srv/term/xterm
mkdir -p /srv/dev/storefront/node_modules/next/dist/bin /srv/dev/admin-ui/node_modules/vite/bin \
  /srv/dev/docs-site/node_modules/@docusaurus/core/bin /srv/dev/billing-api/.venv/bin
touch /srv/dev/storefront/node_modules/next/dist/bin/next /srv/dev/admin-ui/node_modules/vite/bin/vite.js \
  /srv/dev/docs-site/node_modules/@docusaurus/core/bin/docusaurus.mjs /srv/dev/billing-api/.venv/bin/uvicorn
printf '%s' '{"name":"storefront","dependencies":{"next":"15.5.0","react":"19.1.0"}}' > /srv/dev/storefront/package.json
printf '%s' '{"name":"admin-ui","devDependencies":{"vite":"7.1.0"}}' > /srv/dev/admin-ui/package.json
printf '%s' '{"name":"docs-site","dependencies":{"@docusaurus/core":"3.8.0"}}' > /srv/dev/docs-site/package.json
# Simple, made-up favicons, so the cards show project icons.
mkdir -p /srv/dev/storefront/public /srv/dev/admin-ui/public /srv/dev/docs-site/static
printf '%s' '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="8" fill="#f97316"/><path d="M9 12h14l-1.5 13h-11z" fill="#fff"/><path d="M12 12a4 4 0 0 1 8 0" stroke="#fff" stroke-width="2" fill="none"/></svg>' > /srv/dev/storefront/public/favicon.svg
printf '%s' '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="8" fill="#0ea5e9"/><rect x="7" y="8" width="18" height="16" rx="2" fill="none" stroke="#fff" stroke-width="2"/><path d="M7 13h18M12 13v11" stroke="#fff" stroke-width="2"/></svg>' > /srv/dev/admin-ui/public/favicon.svg
printf '%s' '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="8" fill="#16a34a"/><path d="M9 8h9a5 5 0 0 1 5 5v11H14a5 5 0 0 1-5-5z" fill="#fff"/></svg>' > /srv/dev/docs-site/static/favicon.svg
mkdir -p /srv/dev/shop-stack
cat > /srv/dev/shop-stack/compose.yaml <<'YAML'
services:
  web:
    image: nginx:1.29
    ports: ["80:80"]
  db:
    image: postgres:17
    ports: ["127.0.0.1:5433:5432"]
  cache:
    image: redis:8
    ports: ["6379:6379"]
YAML
printf '[project]\nname = "billing-api"\ndependencies = ["fastapi>=0.115", "uvicorn"]\n' > /srv/dev/billing-api/pyproject.toml

# Services run from /, as daemons do; dev servers from their project.
cd /
# A database on every interface: the exposure warning's case.
SERVE_PORT=5432 SERVE_BIND=0.0.0.0 SERVE_REPLY=48000 /srv/bin/postgres -D /var/lib/postgresql/17/main >/dev/null 2>&1 &
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
# Scripts with no project: the language comes from the interpreter, even
# for a #! script whose process is named after itself.
mkdir -p /srv/dev/blog /srv/dev/scripts
printf '<?php echo "ok";\n' > /srv/dev/blog/index.php
printf 'require "socket"\n' > /srv/dev/scripts/webhook.rb
printf '#!/srv/bin/bash\n# answers health checks\n' > /srv/dev/scripts/healthcheck && chmod +x /srv/dev/scripts/healthcheck
(cd /srv/dev/blog && SERVE_PORT=8081 SERVE_REPLY=12000 /srv/bin/php -S 127.0.0.1:8081 >/dev/null 2>&1 &)
(cd /srv/dev/scripts && SERVE_PORT=4567 SERVE_REPLY=800 /srv/bin/ruby webhook.rb >/dev/null 2>&1 &)
(cd /srv/dev/scripts && SERVE_PORT=9100 SERVE_REPLY=300 ./healthcheck >/dev/null 2>&1 &)
# Two real desktop apps, found through their .desktop entries and icons
# in the scene's own home folder.
mkdir -p /srv/home/.local/share/applications /srv/home/.local/share/icons/hicolor/scalable/apps
cp "$W"/icons/*.svg /srv/home/.local/share/icons/hicolor/scalable/apps/ 2>/dev/null || true
printf '[Desktop Entry]\nType=Application\nName=Syncthing\nExec=syncthing serve\nIcon=syncthing\n' > /srv/home/.local/share/applications/syncthing-ui.desktop
printf '[Desktop Entry]\nType=Application\nName=Transmission\nExec=transmission-gtk %%U\nIcon=transmission\n' > /srv/home/.local/share/applications/transmission-gtk.desktop
SERVE_PORT=8384 SERVE_REPLY=3000 /srv/bin/syncthing serve --no-browser >/dev/null 2>&1 &
SERVE_PORT=51413 SERVE_BIND=0.0.0.0 SERVE_REPLY=16000 /srv/bin/transmission-gtk >/dev/null 2>&1 &
sleep 1

cd "$W"
HOME=/srv/home PATH="/srv/term:$PATH" "$W/devserver" -hide-self -shell-preview -frontend "$FRONTEND" >"$W/devserver.log" 2>&1 &
sleep 6
python3 "$W/desktop.py" "$W"
python3 "$W/web.py" "$W"
