#!/usr/bin/env bash
# Builds HassTray.app for Apple Silicon and Intel.
#
#   scripts/build-mac.sh [version] [--install]
#
# --install copies the app to /Applications and starts it.
# Needs Go and the Xcode command line tools (xcode-select --install).
set -euo pipefail
cd "$(dirname "$0")/.."

if ! command -v go >/dev/null; then
  echo "Go is not installed. Get it with: brew install go" >&2
  exit 1
fi

VERSION="0.1.0"
INSTALL=0
for arg in "$@"; do
  case "$arg" in
    --install) INSTALL=1 ;;
    *) VERSION="$arg" ;;
  esac
done

APP="build/bin/HassTray.app"
export CGO_ENABLED=1
export MACOSX_DEPLOYMENT_TARGET=12.0
export CGO_CFLAGS="-mmacosx-version-min=12.0"
export CGO_LDFLAGS="-mmacosx-version-min=12.0"

go mod tidy
go test ./...

rm -rf build/bin
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"

for arch in arm64 amd64; do
  GOOS=darwin GOARCH="$arch" go build -tags production -trimpath \
    -ldflags "-s -w -X main.Version=$VERSION" -o "build/bin/hasstray-$arch" .
done
lipo -create -output "$APP/Contents/MacOS/HassTray" build/bin/hasstray-arm64 build/bin/hasstray-amd64
rm build/bin/hasstray-arm64 build/bin/hasstray-amd64

sed "s/__VERSION__/$VERSION/g" build/Info.plist > "$APP/Contents/Info.plist"

# appicon.png -> icon.icns
ICONSET="build/bin/icon.iconset"
mkdir -p "$ICONSET"
for size in 16 32 128 256 512; do
  sips -z "$size" "$size" build/appicon.png --out "$ICONSET/icon_${size}x${size}.png" >/dev/null
  sips -z "$((size * 2))" "$((size * 2))" build/appicon.png --out "$ICONSET/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/icon.icns"
rm -rf "$ICONSET"

codesign --force --deep --sign - "$APP"
echo "Built $APP ($VERSION)"

if [ "$INSTALL" = 1 ]; then
  pkill -x HassTray 2>/dev/null || true
  rm -rf /Applications/HassTray.app
  cp -R "$APP" /Applications/
  open /Applications/HassTray.app
  echo "Installed /Applications/HassTray.app"
fi
