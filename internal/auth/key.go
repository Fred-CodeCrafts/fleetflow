// Package auth issues and verifies FleetFlow's API keys (DESIGN 3.7, PRD US-13).
//
// A key is generated once, shown to the caller once, and stored only as a
// SHA-256 hash. Verification compares digests in constant time, so a wrong key
// cannot be probed one byte at a time, and the plaintext key is never part of a
// stored record or a log line.
//
// The layer order matches every other entity: the Repository interface lives
// here in the service package so the service owns the seam, internal/auth/memory
// backs the unit tests, and a Postgres implementation can replace it without the
// service changing. Nothing in this package imports net/http or a database
// package.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"

	"fleetflow/internal/domain"
)

// KeyPrefix marks a string as a FleetFlow API key. It is not secret: it only
// tells a human or a secret scanner what kind of token this is, so a key found
// in the wrong place is identifiable without the key itself ever being logged.
const KeyPrefix = "flf_"

// secretBytes is the entropy in a generated key: 256 bits, far beyond brute
// force. It is the reason Hash can be a plain SHA-256 with no salt and no
// server-side pepper (see Hash).
const secretBytes = 32

// displayChars is how much of a key is kept, in the clear, as its label. Enough
// to tell two keys apart in a list, far too little to reconstruct the secret.
const displayChars = len(KeyPrefix) + 6

// codeGenerateFailed is internal: the caller cannot act on it, and the transport
// substitutes its own message.
const codeGenerateFailed = "api_key_generate_failed"

// Generate returns a new plaintext API key and never returns it again: the
// caller hands it to the user once and persists only the hash from Hash.
//
// The format is KeyPrefix followed by base64url (no padding) of 32 random
// bytes, for example "flf_9f3c...". Base64url uses only [A-Za-z0-9_-], so the
// key is safe in an Authorization or X-API-Key header without further encoding,
// and it contains no "/" or "+" to be mangled by a proxy.
func Generate() (string, error) {
	buf := make([]byte, secretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", domain.Internal(codeGenerateFailed, "could not generate an api key").WithCause(err)
	}
	return KeyPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// Hash returns the SHA-256 digest of a plaintext key. The digest is what is
// stored, and what Authenticate looks a presented key up by, so the plaintext
// never has to be persisted.
//
// SHA-256 rather than HMAC or a salted KDF is deliberate: an API key is 256
// bits of uniform randomness, not a human-chosen password, so there is nothing
// for an offline attacker to brute-force and no need for a pepper that would
// itself have to be stored and rotated. A per-key salt would also break the
// indexed lookup by hash that keeps verification cheap.
func Hash(key string) []byte {
	sum := sha256.Sum256([]byte(key))
	return sum[:]
}

// Verify reports whether presented hashes to the stored digest. The comparison
// is constant time (crypto/subtle), so how long it takes does not depend on how
// many leading bytes matched: a wrong key cannot be discovered byte by byte.
//
// A nil or wrong-length hash simply does not match. ConstantTimeCompare reports
// unequal lengths without indexing either slice, so a malformed record or an
// empty presented key is rejected rather than panicking.
func Verify(presented string, hash []byte) bool {
	return subtle.ConstantTimeCompare(Hash(presented), hash) == 1
}

// DisplayPrefix returns the non-secret head of a key, kept as the record's label
// so a key can be recognised in a list without revealing it. It never returns
// more than displayChars characters.
func DisplayPrefix(key string) string {
	if len(key) <= displayChars {
		return key
	}
	return key[:displayChars]
}
