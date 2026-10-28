#!/usr/bin/env bash
# Records the documentation screenshots of the desktop app and the web
# dashboard, in light and dark, without anything from this machine: the
# scene runs in its own network, mount and UTS namespaces (unshare -rmnu,
# no root needed) with host name web-01 and fake services copied from
# fakesrv under realistic names.
#
#   scripts/screenshots/record.sh            # writes docs/assets/*.png
#
# Needs Linux with unprivileged user namespaces, Go, python3 with
# Playwright's Chromium, and the ip command. With Docker, the scene also
# shows two real open source apps, Syncthing and Transmission, with the
# icons from their Ubuntu packages (cached, not kept in the repository);
# without it they get monograms.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

(cd "$here/fakesrv" && CGO_ENABLED=0 go build -o "$work/fakesrv" .)
(cd "$repo" && go build -o "$work/portop" ./cmd/portop)
(cd "$repo/desktop" && go build -o "$work/devserver" ./cmd/devserver)
cp "$here"/*.py "$work/"

icons=${XDG_CACHE_HOME:-$HOME/.cache}/portop-screenshots/icons
if [ ! -s "$icons/syncthing.svg" ] && command -v docker >/dev/null; then
  mkdir -p "$icons"
  docker run --rm -v "$icons:/out" ubuntu:24.04 sh -c '
    apt-get update -qq >/dev/null && cd /tmp && apt-get download -qq syncthing transmission-common >/dev/null &&
    for d in *.deb; do dpkg-deb -x "$d" x; done &&
    cp x/usr/share/icons/hicolor/scalable/apps/syncthing.svg x/usr/share/icons/hicolor/scalable/apps/transmission.svg /out/ &&
    chown -R '"$(id -u):$(id -g)"' /out' || echo "could not fetch the app icons; the scene uses monograms"
fi
mkdir -p "$work/icons" && cp "$icons"/*.svg "$work/icons/" 2>/dev/null || true

FRONTEND="$repo/desktop/frontend" unshare -rmnu sh "$here/scene.sh" "$work"

python3 - "$work" "$repo/docs/assets" <<'PY'
import sys, glob, os
from PIL import Image
work, assets = sys.argv[1], sys.argv[2]
for path in sorted(glob.glob(os.path.join(work, "*-light.png")) + glob.glob(os.path.join(work, "*-dark.png"))):
    Image.open(path).convert("RGB").save(os.path.join(assets, os.path.basename(path)), optimize=True)
    print("wrote", os.path.join("docs/assets", os.path.basename(path)))
PY
