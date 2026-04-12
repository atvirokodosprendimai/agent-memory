package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestSealOpenRoundTrip(t *testing.T) {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		t.Fatal(err)
	}

	plaintext := []byte("hello, agent-memory world!")
	sealed, err := Seal(key, plaintext)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	opened, err := Open(key, sealed)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if !bytes.Equal(opened, plaintext) {
		t.Errorf("round-trip mismatch: got %q, want %q", opened, plaintext)
	}
}

func TestTamperedCiphertextFails(t *testing.T) {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		t.Fatal(err)
	}

	sealed, err := Seal(key, []byte("secret data"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	// Flip a bit in the ciphertext portion (after the nonce).
	tampered := make([]byte, len(sealed))
	copy(tampered, sealed)
	tampered[len(tampered)-1] ^= 0x01

	_, err = Open(key, tampered)
	if err == nil {
		t.Error("expected error decrypting tampered ciphertext, got nil")
	}
}

func TestEmptyPlaintext(t *testing.T) {
	// Empty plaintext should work — the sealed output still has nonce + tag,
	// but the ciphertext portion is empty (total = 12 + 16 = 28 bytes).
	// However, our min-length check requires 29 bytes (nonce + tag + ≥1 byte).
	// So we need to adjust: Seal/Open should handle the Go GCM convention where
	// empty plaintext yields nonce+tag only (28 bytes total).
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		t.Fatal(err)
	}

	sealed, err := Seal(key, []byte{})
	if err != nil {
		t.Fatalf("Seal empty: %v", err)
	}

	// AES-GCM with empty plaintext produces nonce (12) + tag (16) = 28 bytes.
	if len(sealed) != 28 {
		t.Errorf("sealed empty plaintext length: got %d, want 28", len(sealed))
	}

	opened, err := Open(key, sealed)
	if err != nil {
		t.Fatalf("Open empty: %v", err)
	}

	if len(opened) != 0 {
		t.Errorf("expected empty plaintext, got %q", opened)
	}
}

func TestDifferentKeysDifferentCiphertext(t *testing.T) {
	var key1, key2 [32]byte
	if _, err := rand.Read(key1[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := rand.Read(key2[:]); err != nil {
		t.Fatal(err)
	}

	plaintext := []byte("same message, different keys")

	sealed1, err := Seal(key1, plaintext)
	if err != nil {
		t.Fatalf("Seal key1: %v", err)
	}
	sealed2, err := Seal(key2, plaintext)
	if err != nil {
		t.Fatalf("Seal key2: %v", err)
	}

	if bytes.Equal(sealed1, sealed2) {
		t.Error("different keys produced identical ciphertext (unlikely)")
	}

	// Decrypting sealed1 with key2 should fail.
	_, err = Open(key2, sealed1)
	if err == nil {
		t.Error("expected error decrypting with wrong key, got nil")
	}
}

func TestEntryIDDeterministic(t *testing.T) {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		t.Fatal(err)
	}

	data := []byte("deterministic test data")

	id1 := EntryID(key, data)
	id2 := EntryID(key, data)

	if id1 != id2 {
		t.Errorf("EntryID not deterministic: %x != %x", id1, id2)
	}
}

func TestEntryIDDifferentInputs(t *testing.T) {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		t.Fatal(err)
	}

	id1 := EntryID(key, []byte("input A"))
	id2 := EntryID(key, []byte("input B"))

	if id1 == id2 {
		t.Error("different inputs produced same EntryID")
	}
}

func TestOpenCiphertextTooShort(t *testing.T) {
	var key [32]byte

	_, err := Open(key, []byte("too short"))
	if err == nil {
		t.Error("expected error for short ciphertext, got nil")
	}
}

func TestGenerateNonce(t *testing.T) {
	n1, err := GenerateNonce()
	if err != nil {
		t.Fatalf("GenerateNonce: %v", err)
	}
	if len(n1) != 12 {
		t.Errorf("nonce length: got %d, want 12", len(n1))
	}

	n2, err := GenerateNonce()
	if err != nil {
		t.Fatalf("GenerateNonce: %v", err)
	}
	if n1 == n2 {
		t.Error("two consecutive nonces are identical (extremely unlikely)")
	}
}
