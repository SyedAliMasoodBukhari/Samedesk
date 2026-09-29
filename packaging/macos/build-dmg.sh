#!/bin/sh
# Builds dist/SameDesk-<version>.dmg: a universal (Apple Silicon + Intel)
# SameDesk.app next to a shortcut to Applications.
# Usage: packaging/macos/build-dmg.sh <version> <arm64 binary> <amd64 binary>
set -eu
VERSION=$1 ARM=$2 X64=$3
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

APP="$WORK/stage/SameDesk.app"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
lipo -create -output "$APP/Contents/MacOS/samedesk" "$ARM" "$X64"
sed "s/@VERSION@/$VERSION/g" "$ROOT/packaging/macos/Info.plist" > "$APP/Contents/Info.plist"
cp "$ROOT/packaging/icons/samedesk.icns" "$APP/Contents/Resources/"
cp "$ROOT/LICENSE" "$ROOT/NOTICE" "$APP/Contents/Resources/"

# Ad-hoc signature: enough for Apple Silicon to run it. A Developer ID signature
# and notarisation (so Gatekeeper doesn't ask) need an Apple developer account;
# set SIGN_IDENTITY to use one.
codesign --force --options runtime --timestamp=none --sign "${SIGN_IDENTITY:--}" "$APP"
codesign --verify --strict "$APP"

ln -s /Applications "$WORK/stage/Applications"
mkdir -p "$ROOT/dist"
OUT="$ROOT/dist/SameDesk-$VERSION.dmg"
rm -f "$OUT"
hdiutil create -quiet -volname "SameDesk" -srcfolder "$WORK/stage" -fs HFS+ -format UDZO -imagekey zlib-level=9 "$OUT"
echo "$OUT"
