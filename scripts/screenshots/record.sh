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
# Playwright's Chromium, and the ip command.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

(cd "$here/fakesrv" && CGO_ENABLED=0 go build -o "$work/fakesrv" .)
(cd "$repo" && go build -o "$work/portop" ./cmd/portop)
(cd "$repo/desktop" && go build -o "$work/devserver" ./cmd/devserver)
cp "$here"/*.py "$work/"

FRONTEND="$repo/desktop/frontend" unshare -rmnu sh "$here/scene.sh" "$work"

python3 - "$work" "$repo/docs/assets" <<'PY'
import sys, glob, os
from PIL import Image
work, assets = sys.argv[1], sys.argv[2]
for path in sorted(glob.glob(os.path.join(work, "*-light.png")) + glob.glob(os.path.join(work, "*-dark.png"))):
    Image.open(path).convert("RGB").save(os.path.join(assets, os.path.basename(path)), optimize=True)
    print("wrote", os.path.join("docs/assets", os.path.basename(path)))
PY
