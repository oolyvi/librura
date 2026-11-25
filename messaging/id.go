package messaging

import (
	"crypto/rand"
	"encoding/hex"
)

func newEventID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
