package update

import (
	"crypto/ed25519"
	"encoding/base64"
)

// publicKey checks release signatures. Its private half is the repository's
// SAMEDESK_SIGNING_KEY Actions secret; see tools/sign.
var publicKey = mustKey("wZoPgbA64CtmrsOmR28qQMasCM3v1s1ozKifoK9TXfU=")

func mustKey(s string) ed25519.PublicKey {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b) != ed25519.PublicKeySize {
		panic("update: bad public key")
	}
	return b
}
