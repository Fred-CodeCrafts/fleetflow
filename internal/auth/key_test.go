package auth_test

import (
	"bytes"
	"strings"
	"testing"

	"fleetflow/internal/auth"
)

// headerSafe is every character base64url uses, which is also every character a
// header value accepts without escaping.
const headerSafe = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-"

func TestGenerateProducesDistinctHeaderSafeKeys(t *testing.T) {
	first, err := auth.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	second, err := auth.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if first == second {
		t.Fatal("Generate returned the same key twice")
	}
	if !strings.HasPrefix(first, auth.KeyPrefix) {
		t.Errorf("Generate returned %q, want the %q prefix", first, auth.KeyPrefix)
	}
	if len(first) <= len(auth.KeyPrefix)+16 {
		t.Errorf("Generate returned %q, want the prefix plus a long secret", first)
	}
	for _, r := range strings.TrimPrefix(first, auth.KeyPrefix) {
		if !strings.ContainsRune(headerSafe, r) {
			t.Fatalf("Generate returned %q, which is not header-safe: %q", first, r)
		}
	}
}

func TestHashIsStableAndIsNotTheKey(t *testing.T) {
	key, err := auth.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	hash := auth.Hash(key)
	if len(hash) != 32 {
		t.Errorf("Hash returned %d bytes, want a SHA-256 digest of 32", len(hash))
	}
	if !bytes.Equal(hash, auth.Hash(key)) {
		t.Error("Hash is not stable for the same key")
	}
	if bytes.Contains(hash, []byte(key)) {
		t.Error("Hash contains the plaintext key")
	}
	if bytes.Equal(hash, []byte(key)) {
		t.Error("Hash returned the plaintext key")
	}
	if bytes.Equal(hash, auth.Hash(key+"x")) {
		t.Error("Hash returned the same digest for two different keys")
	}
}

func TestVerifyAcceptsOwnHashAndRejectsOthers(t *testing.T) {
	key, err := auth.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	other, err := auth.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	hash := auth.Hash(key)

	tests := []struct {
		name      string
		presented string
		want      bool
	}{
		{"the key itself", key, true},
		{"a different key", other, false},
		{"the key with its last character removed", key[:len(key)-1], false},
		{"the key with an extra character", key + "a", false},
		{"a key sharing the long display prefix", auth.DisplayPrefix(key) + strings.Repeat("a", 20), false},
		{"the empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := auth.Verify(tt.presented, hash); got != tt.want {
				t.Errorf("Verify(%q, Hash(key)) = %v, want %v", tt.presented, got, tt.want)
			}
		})
	}
}

func TestVerifyComparesTheWholeDigest(t *testing.T) {
	key, err := auth.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	hash := auth.Hash(key)

	// Flipping one bit of the stored digest must fail, so a store cannot accept
	// a key whose digest only nearly matches.
	for i := range hash {
		tampered := append([]byte(nil), hash...)
		tampered[i] ^= 0x01
		if auth.Verify(key, tampered) {
			t.Fatalf("Verify accepted a digest with bit 0 of byte %d flipped", i)
		}
	}
}

func TestVerifyRejectsMalformedHashesWithoutPanicking(t *testing.T) {
	for _, tt := range []struct {
		name string
		hash []byte
	}{
		{"nil hash", nil},
		{"empty hash", []byte{}},
		{"short hash", []byte("short")},
		{"long hash", bytes.Repeat([]byte{0xff}, 64)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if auth.Verify("anything", tt.hash) {
				t.Errorf("Verify accepted a %s", tt.name)
			}
			if auth.Verify("", tt.hash) {
				t.Errorf("Verify accepted an empty key against a %s", tt.name)
			}
		})
	}
}

func TestDisplayPrefixKeepsOnlyTheHead(t *testing.T) {
	key, err := auth.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	prefix := auth.DisplayPrefix(key)
	if !strings.HasPrefix(key, prefix) {
		t.Errorf("DisplayPrefix(%q) = %q, want a head of the key", key, prefix)
	}
	if len(prefix) >= len(key) {
		t.Errorf("DisplayPrefix returned the whole key %q, want only its head", prefix)
	}
	if !strings.HasPrefix(prefix, auth.KeyPrefix) {
		t.Errorf("DisplayPrefix(%q) = %q, want it to keep the %q marker", key, prefix, auth.KeyPrefix)
	}
	if got := auth.DisplayPrefix("short"); got != "short" {
		t.Errorf("DisplayPrefix(%q) = %q, want it unchanged", "short", got)
	}
}
