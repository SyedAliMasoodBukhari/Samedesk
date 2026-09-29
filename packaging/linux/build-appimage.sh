#!/bin/sh
# Builds dist/SameDesk-<version>-<arch>.AppImage from a Linux binary. Runs on Linux
# (the release workflow); downloads appimagetool on first use.
# Usage: packaging/linux/build-appimage.sh <version> <x86_64|aarch64> <binary>
set -eu
VERSION=$1 ARCH=$2 BIN=$3
ROOT=$(cd "$(dirname "$0")/../.." && pwd)
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

TOOL="$ROOT/dist/.appimagetool"
if [ ! -x "$TOOL" ]; then
	curl -fsSL -o "$TOOL" "https://github.com/AppImage/appimagetool/releases/download/continuous/appimagetool-$(uname -m).AppImage"
	chmod +x "$TOOL"
fi

APPDIR="$WORK/SameDesk.AppDir"
mkdir -p "$APPDIR/usr/bin" "$APPDIR/usr/share/applications" "$APPDIR/usr/share/icons/hicolor/512x512/apps"
cp "$BIN" "$APPDIR/usr/bin/samedesk"
cp "$ROOT/packaging/linux/samedesk.desktop" "$APPDIR/usr/share/applications/"
cp "$ROOT/packaging/linux/samedesk.desktop" "$APPDIR/"
cp "$ROOT/packaging/icons/samedesk-512.png" "$APPDIR/usr/share/icons/hicolor/512x512/apps/samedesk.png"
cp "$ROOT/packaging/icons/samedesk-512.png" "$APPDIR/samedesk.png"
ln -s usr/bin/samedesk "$APPDIR/AppRun"

# --appimage-extract-and-run: CI machines usually have no FUSE.
ARCH=$ARCH VERSION=$VERSION "$TOOL" --appimage-extract-and-run --no-appstream "$APPDIR" "$ROOT/dist/SameDesk-$VERSION-$ARCH.AppImage"
