# noassets: leave out Syncthing's own web UI (SameDesk is the interface).
TAGS    := noassets
FLAGS   := -tags $(TAGS) -trimpath -ldflags="-s -w"
# Windows: a tray app, so no console window.
WINFLAGS := -tags $(TAGS) -trimpath -ldflags="-s -w -H=windowsgui"
PKG     := ./cmd/samedesk

.PHONY: build release test clean

# macOS needs cgo for Syncthing's FSEvents watcher; elsewhere pure Go is fine.
build:
	CGO_ENABLED=$(if $(filter Darwin,$(shell uname)),1,0) go build $(FLAGS) -o bin/samedesk $(PKG)

release:
	mkdir -p dist
	CGO_ENABLED=1 GOOS=darwin  GOARCH=arm64 go build $(FLAGS) -o dist/samedesk-macos-arm64     $(PKG)
	CGO_ENABLED=1 GOOS=darwin  GOARCH=amd64 go build $(FLAGS) -o dist/samedesk-macos-x64       $(PKG)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(WINFLAGS) -o dist/samedesk-windows-x64.exe   $(PKG)
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build $(WINFLAGS) -o dist/samedesk-windows-arm64.exe $(PKG)
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build $(FLAGS) -o dist/samedesk-linux-x64       $(PKG)
	CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build $(FLAGS) -o dist/samedesk-linux-arm64     $(PKG)

test:
	go vet -tags $(TAGS) ./...
	go test -tags $(TAGS) ./...

clean:
	rm -rf bin dist
