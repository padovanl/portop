#!/bin/sh
# Build an unsigned Debian source/binary package from this working tree.
set -eu
umask 022

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
for tool in dpkg-parsechangelog dpkg-buildpackage tar xz install; do
    command -v "$tool" >/dev/null 2>&1 || {
        echo "Missing required Debian build tool: $tool" >&2
        exit 1
    }
done

version=$(dpkg-parsechangelog -l"$repo_dir/debian/changelog" -SVersion)
upstream=${version%-*}
epoch=$(dpkg-parsechangelog -l"$repo_dir/debian/changelog" -STimestamp)
output_dir=${PORTOP_DEBIAN_OUT:-"$repo_dir/dist/debian"}
mkdir -p "$output_dir"
output_dir=$(CDPATH= cd -- "$output_dir" && pwd)
work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT HUP INT TERM
source_dir="$work_dir/portop-$upstream"
mkdir "$source_dir"

# Use an explicit list so local configuration and build outputs never enter
# the source archive. Normalize permissions from Windows/WSL checkouts.
tar -C "$repo_dir" -cf "$work_dir/checkout.tar" \
    cmd internal cockpit docs e2e scripts .github \
    go.mod go.sum Makefile README.md CONTRIBUTING.md LICENSE \
    .gitattributes .gitignore .goreleaser.yaml install.sh
tar -C "$source_dir" -xf "$work_dir/checkout.tar"
find "$source_dir" -type d -exec chmod 755 {} +
find "$source_dir" -type f -exec chmod 644 {} +
find "$source_dir/scripts" -type f \( -name '*.sh' -o -name '*.py' \) -exec chmod 755 {} +
chmod 755 "$source_dir/install.sh"
tar --sort=name --mtime="@$epoch" --owner=0 --group=0 --numeric-owner \
    -C "$work_dir" -cJf "$work_dir/portop_$upstream.orig.tar.xz" "portop-$upstream"
cp -R "$repo_dir/debian" "$source_dir/debian"
find "$source_dir/debian" -type d -exec chmod 755 {} +
find "$source_dir/debian" -type f -exec chmod 644 {} +
chmod 755 "$source_dir/debian/rules" "$source_dir/debian/tests/smoke"

(cd "$source_dir" && dpkg-buildpackage -us -uc "$@")
for artifact in "$work_dir"/*.deb "$work_dir"/*.ddeb "$work_dir"/*.dsc \
    "$work_dir"/*.tar.xz "$work_dir"/*.buildinfo "$work_dir"/*.changes; do
    [ ! -f "$artifact" ] || install -m 644 "$artifact" "$output_dir/"
done
echo "Debian artifacts: $output_dir"
