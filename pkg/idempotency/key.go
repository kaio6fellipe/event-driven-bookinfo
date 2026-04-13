package idempotency

import (
	"crypto/sha256"
	"encoding/hex"
)

// fieldSeparator is a byte unlikely to appear in business payloads that
// prevents collisions at field boundaries. Without it, fields ("ab", "c")
// and ("a", "bc") would hash to the same value.
const fieldSeparator byte = 0x1f // ASCII Unit Separator

// NaturalKey computes a SHA-256 hash over the given fields, separated by
// an unambiguous byte to prevent boundary collisions. The returned string
// is 64 hex characters.
func NaturalKey(fields ...string) string {
	h := sha256.New()
	for _, f := range fields {
		_, _ = h.Write([]byte(f))
		_, _ = h.Write([]byte{fieldSeparator})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Resolve returns explicitKey if non-empty, otherwise returns the natural
// key computed from fields. Used by service layers to pick the correct
// idempotency key for a request.
func Resolve(explicitKey string, fields ...string) string {
	if explicitKey != "" {
		return explicitKey
	}
	return NaturalKey(fields...)
}
