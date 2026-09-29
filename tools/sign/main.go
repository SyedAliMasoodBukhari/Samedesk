// Command sign makes and uses the SameDesk release key, which the app checks
// before installing an update.
//
//	go run ./tools/sign -keygen        prints a new key pair (once, ever)
//	go run ./tools/sign <file>         writes <file>.sig using $SAMEDESK_SIGNING_KEY
//
// The private key lives only in the repository's Actions secrets
// (SAMEDESK_SIGNING_KEY); the public key is built into the app
// (internal/update/key.go).
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "-keygen" {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			fail(err)
		}
		fmt.Println("public: ", base64.StdEncoding.EncodeToString(pub))
		fmt.Println("private:", base64.StdEncoding.EncodeToString(priv.Seed()))
		return
	}
	if len(os.Args) != 2 {
		fail(fmt.Errorf("usage: sign -keygen | sign <file>"))
	}
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(os.Getenv("SAMEDESK_SIGNING_KEY")))
	if err != nil || len(seed) != ed25519.SeedSize {
		fail(fmt.Errorf("SAMEDESK_SIGNING_KEY must be the base64 private key from -keygen"))
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		fail(err)
	}
	sig := ed25519.Sign(ed25519.NewKeyFromSeed(seed), data)
	if err := os.WriteFile(os.Args[1]+".sig", []byte(base64.StdEncoding.EncodeToString(sig)+"\n"), 0o644); err != nil {
		fail(err)
	}
	fmt.Println("signed", os.Args[1])
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "sign:", err)
	os.Exit(1)
}
