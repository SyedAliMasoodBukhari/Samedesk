# noassets: leave out Syncthing's own web UI (Shared Hub is the interface).
TAGS    := noassets
FLAGS   := -tags $(TAGS) -trimpath -ldflags="-s -w"
PKG     := ./cmd/sharedhub

.PHONY: build release test clean

# macOS needs cgo for Syncthing's FSEvents watcher; elsewhere pure Go is fine.
build:
	CGO_ENABLED=$(if $(filter Darwin,$(shell uname)),1,0) go build $(FLAGS) -o bin/sharedhub $(PKG)

release:
	mkdir -p dist
	CGO_ENABLED=1 GOOS=darwin  GOARCH=arm64 go build $(FLAGS) -o dist/sharedhub-macos-arm64     $(PKG)
	CGO_ENABLED=1 GOOS=darwin  GOARCH=amd64 go build $(FLAGS) -o dist/sharedhub-macos-x64       $(PKG)
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(FLAGS) -o dist/sharedhub-windows-x64.exe   $(PKG)
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build $(FLAGS) -o dist/sharedhub-windows-arm64.exe $(PKG)
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build $(FLAGS) -o dist/sharedhub-linux-x64       $(PKG)
	CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build $(FLAGS) -o dist/sharedhub-linux-arm64     $(PKG)

test:
	go vet -tags $(TAGS) ./...
	go test -tags $(TAGS) ./...

clean:
	rm -rf bin dist
