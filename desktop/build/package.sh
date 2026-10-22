#!/usr/bin/env bash
# Builds and packages portop desktop for the OS it runs on, into dist/.
#
#   desktop/build/package.sh 0.4.0
#
# Linux: portop-desktop deb, rpm and tar.gz (amd64; needs GTK4 and
#   WebKitGTK 6 headers, and nfpm).
# macOS: a universal portop.app, ad-hoc signed, zipped.
# Windows: portop-desktop.exe for amd64 and arm64 with icon and manifest,
#   zipped (needs the wails3 CLI for the resources).
set -euo pipefail

version=${1:?usage: package.sh VERSION}
version=${version#v}
cd "$(dirname "$0")/.."
rm -rf bin dist
mkdir -p bin dist
ldflags="-s -w -X main.version=$version"

case "$(go env GOOS)" in
linux)
  go build -trimpath -tags production -ldflags "$ldflags" -o bin/portop-desktop .
  export VERSION=$version
  nfpm package -f build/linux/nfpm.yaml -p deb -t "dist/portop-desktop_${version}_linux_amd64.deb"
  nfpm package -f build/linux/nfpm.yaml -p rpm -t "dist/portop-desktop_${version}_linux_amd64.rpm"
  here=$PWD
  tar -czf "dist/portop-desktop_${version}_linux_amd64.tar.gz" -C "$here/bin" portop-desktop \
    -C "$here/build/linux" portop-desktop.desktop portop-desktop.png -C "$here/.." LICENSE
  ;;
darwin)
  for arch in arm64 amd64; do
    carch=$arch
    [ "$arch" = amd64 ] && carch=x86_64
    CGO_ENABLED=1 GOARCH=$arch CC="clang -arch $carch" \
      go build -trimpath -tags production -ldflags "$ldflags" -o "bin/portop-desktop-$arch" .
  done
  lipo -create -output bin/portop-desktop bin/portop-desktop-arm64 bin/portop-desktop-amd64
  app=dist/portop.app
  mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
  cp bin/portop-desktop "$app/Contents/MacOS/portop-desktop"
  sed "s/@VERSION@/$version/g" build/darwin/Info.plist > "$app/Contents/Info.plist"
  iconset=bin/icon.iconset
  mkdir -p "$iconset"
  for size in 16 32 128 256 512; do
    sips -z $size $size build/appicon.png --out "$iconset/icon_${size}x${size}.png" >/dev/null
    sips -z $((size * 2)) $((size * 2)) build/appicon.png --out "$iconset/icon_${size}x${size}@2x.png" >/dev/null
  done
  iconutil -c icns -o "$app/Contents/Resources/icon.icns" "$iconset"
  # Ad-hoc signature: Apple Silicon refuses unsigned code. Without a
  # Developer ID the first launch still needs "Open" from the context menu.
  codesign --force --deep --sign - "$app"
  ditto -c -k --keepParent "$app" "dist/portop-desktop_${version}_macos_universal.zip"
  rm -rf "$app"
  ;;
windows)
  # The file version resource takes numbers only: 1.2.3-rc1 is 1.2.3.
  sed -e "s/\"file_version\": \"@VERSION@\"/\"file_version\": \"${version%%[-+]*}\"/" \
    -e "s/@VERSION@/$version/g" build/windows/info.json > bin/info.json
  for arch in amd64 arm64; do
    wails3 generate syso -arch "$arch" -icon build/windows/icon.ico \
      -manifest build/windows/portop-desktop.exe.manifest -info bin/info.json \
      -out "rsrc_windows_$arch.syso"
    mkdir -p "bin/$arch"
    GOARCH=$arch go build -trimpath -tags production -ldflags "$ldflags -H windowsgui" -o "bin/$arch/portop-desktop.exe" .
    cp ../LICENSE "bin/$arch/LICENSE.txt"
    (cd "bin/$arch" && 7z a -tzip -bso0 "../../dist/portop-desktop_${version}_windows_$arch.zip" portop-desktop.exe LICENSE.txt)
  done
  rm -f rsrc_windows_*.syso
  ;;
*)
  echo "package.sh: unsupported OS $(go env GOOS)" >&2
  exit 1
  ;;
esac
ls -l dist
