// Package crypto provides AES-256-GCM encryption/decryption and HMAC-SHA256
// hashing for memory entries.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
)

const (
	nonceSize = 12
	tagSize   = 16
	// MinSealedSize is the minimum valid size for a sealed ciphertext:
	// 12 bytes nonce + 16 bytes GCM auth tag.
	MinSealedSize = nonceSize + tagSize
)

// GenerateNonce returns a cryptographically random 12-byte nonce suitable for
// AES-GCM.
func GenerateNonce() ([nonceSize]byte, error) {
	var nonce [nonceSize]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nonce, fmt.Errorf("generating nonce: %w", err)
	}
	return nonce, nil
}

// Seal encrypts plaintext using AES-256-GCM with the given 32-byte key.
// It generates a random 12-byte nonce and returns nonce || ciphertext || tag.
func Seal(key [32]byte, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("creating aes cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating gcm: %w", err)
	}

	nonce, err := GenerateNonce()
	if err != nil {
		return nil, err
	}

	// Seal appends the ciphertext and tag to the nonce.
	// Result: [12 bytes nonce][N bytes ciphertext][16 bytes auth tag]
	out := aead.Seal(nonce[:], nonce[:], plaintext, nil)
	return out, nil
}

// Open decrypts ciphertext produced by Seal using AES-256-GCM with the given
// 32-byte key. It validates that the ciphertext meets the minimum length
// requirement (28 bytes: 12 nonce + 16 tag).
func Open(key [32]byte, ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < MinSealedSize {
		return nil, errors.New("ciphertext too short")
	}

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("creating aes cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating gcm: %w", err)
	}

	nonce := ciphertext[:nonceSize]
	payload := ciphertext[nonceSize:]

	plaintext, err := aead.Open(nil, nonce, payload, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypting: %w", err)
	}

	return plaintext, nil
}

// EntryID returns an HMAC-SHA256 hash of data using the given signing key.
// The result is deterministic: the same key and data always produce the same
// 32-byte hash.
func EntryID(signingKey [32]byte, data []byte) [32]byte {
	mac := hmac.New(sha256.New, signingKey[:])
	mac.Write(data)
	var out [32]byte
	copy(out[:], mac.Sum(nil))
	return out
}
