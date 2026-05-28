package quickstart

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
)

// randHex returns n cryptographically random hex characters (always lowercase).
func randHex(n int) string {
	b := make([]byte, (n+1)/2)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("quickstart: rand.Read failed: %v", err))
	}
	return hex.EncodeToString(b)[:n]
}

// randName returns a readable random identifier in the form "<prefix>-<4 hex chars>".
// Example: randName("node") → "node-a7k3"
func randName(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, randHex(4))
}

const passCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// generatePass returns a cryptographically random alphanumeric password of the given length.
func generatePass(length int) string {
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(passCharset))))
		if err != nil {
			b[i] = passCharset[i%len(passCharset)]
			continue
		}
		b[i] = passCharset[n.Int64()]
	}
	return string(b)
}
