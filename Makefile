# noassets: leave out Syncthing's own web UI (SameDesk is the interface).
TAGS    := noassets
VERSION ?= 0.1.0
LDFLAGS := -s -w -X main.version=$(VERSION)
FLAGS   := -tags $(TAGS) -trimpath -ldflags="$(LDFLAGS)"
# Windows: a tray app, so no console window. Symbols are kept (no -s -w): stripped
# Go programs are a common trigger for antivirus false positives.
WINFLAGS := -tags $(TAGS) -trimpath -ldflags="-X main.version=$(VERSION) -H=windowsgui"
PKG     := ./cmd/samedesk

# Packaging tools, pinned and run through the Go toolchain.
WINRES := go run github.com/tc-hib/go-winres@v0.3.3
NFPM   := go run github.com/goreleaser/nfpm/v2/cmd/nfpm@v2.47.0

.PHONY: build release winres dmg windows-installer linux-packages installers test clean

# macOS needs cgo for Syncthing's FSEvents watcher and the menu bar; elsewhere pure Go is fine.
build:
	CGO_ENABLED=$(if $(filter Darwin,$(shell uname)),1,0) go build $(FLAGS) -o bin/samedesk $(PKG)

# Icon, version details and manifest for SameDesk.exe (picked up by go build as .syso files).
winres:
	$(WINRES) simply --arch amd64,arm64 --out cmd/samedesk/rsrc --manifest gui \
		--icon packaging/icons/samedesk.ico --product-name SameDesk --file-description SameDesk \
		--product-version $(VERSION) --file-version $(VERSION) --original-filename SameDesk.exe \
		--copyright "The SameDesk authors, MIT licence"

release: winres
	mkdir -p dist
	CGO_ENABLED=1 GOOS=darwin  GOARCH=arm64 go build $(FLAGS) -o dist/samedesk-macos-arm64     $(PKG)
	CGO_ENABLED=1 GOOS=darwin  GOARCH=amd64 go build $(FLAGS) -o dist/samedesk-macos-x64       $(PKG)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(WINFLAGS) -o dist/samedesk-windows-x64.exe   $(PKG)
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build $(WINFLAGS) -o dist/samedesk-windows-arm64.exe $(PKG)
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build $(FLAGS) -o dist/samedesk-linux-x64       $(PKG)
	CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build $(FLAGS) -o dist/samedesk-linux-arm64     $(PKG)

# macOS: a universal SameDesk.app in a .dmg (build on a Mac).
dmg:
	mkdir -p dist
	CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build $(FLAGS) -o dist/samedesk-macos-arm64 $(PKG)
	CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build $(FLAGS) -o dist/samedesk-macos-x64   $(PKG)
	packaging/macos/build-dmg.sh $(VERSION) dist/samedesk-macos-arm64 dist/samedesk-macos-x64

# Windows: per-user installers for x64 and ARM (needs NSIS's makensis; builds on any OS).
# makensis crashes without a UTF-8 locale, hence LC_ALL.
windows-installer: winres
	mkdir -p dist
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(WINFLAGS) -o dist/samedesk-windows-x64.exe   $(PKG)
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build $(WINFLAGS) -o dist/samedesk-windows-arm64.exe $(PKG)
	cd packaging/windows && for a in x64 arm64; do \
		LC_ALL=C.UTF-8 makensis -V2 -DVERSION=$(VERSION) -DARCH=$$a -DEXE=../../dist/samedesk-windows-$$a.exe \
			-DOUTFILE=../../dist/SameDesk-$(VERSION)-windows-$$a-setup.exe samedesk.nsi || exit 1; done

# Linux: .deb and .rpm for x64 and ARM.
linux-packages:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(FLAGS) -o dist/samedesk-linux-x64   $(PKG)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(FLAGS) -o dist/samedesk-linux-arm64 $(PKG)
	mkdir -p dist/linux
	for a in amd64:x64 arm64:arm64; do cp dist/samedesk-linux-$${a##*:} dist/linux/samedesk; for p in deb rpm; do \
		VERSION=$(VERSION) ARCH=$${a%%:*} $(NFPM) package --config packaging/linux/nfpm.yaml --packager $$p --target dist/ || exit 1; \
	done; done; rm -rf dist/linux

installers: dmg windows-installer linux-packages

test:
	go vet -tags $(TAGS) ./...
	go test -tags $(TAGS) ./...

clean:
	rm -rf bin dist cmd/samedesk/rsrc_windows_*.syso
