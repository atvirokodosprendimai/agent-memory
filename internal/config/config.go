// Package config manages the agent-memory configuration file and key derivation.
//
// It provides functions to create, load, and save configuration, as well as
// derive encryption, index, and signing keys from a user-provided secret
// using HKDF-SHA256.
package config

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"golang.org/x/crypto/hkdf"
)

const (
	version = 1

	infoEncryption = "agent-memory-encryption-v1"
	infoIndex      = "agent-memory-index-v1"
	infoSigning    = "agent-memory-signing-v1"
)

// Config holds the persistent agent-memory configuration.
// The secret is never stored — only the salt is persisted so that keys
// can be re-derived on demand.
type Config struct {
	Version    int    `json:"version"`
	IPFSAddr   string `json:"ipfs_addr"`
	SaltHex    string `json:"salt_hex"`
	IndexCID   string `json:"index_cid,omitempty"`
	Created    string `json:"created"`
	P2PEnabled bool   `json:"p2p_enabled,omitempty"`
	DataDir    string `json:"data_dir,omitempty"`
}

// Keys holds the three derived keys used by the system.
type Keys struct {
	EncryptionKey [32]byte
	IndexKey      [32]byte
	SigningKey    [32]byte
}

// Dir returns the configuration directory: ~/.config/agent-memory/
func Dir() string {
	home := homeDir()
	return filepath.Join(home, ".config", "agent-memory")
}

// DefaultDataDir returns the default P2P data directory: ~/.agent-memory/p2p/
func DefaultDataDir() string {
	home := homeDir()
	return filepath.Join(home, ".agent-memory", "p2p")
}

// Load reads the configuration from ~/.config/agent-memory/config.json.
// Environment variables override config file values:
//   - AGENT_MEMORY_P2P_ENABLED overrides p2p_enabled
//   - AGENT_MEMORY_DATA_DIR overrides data_dir
func Load() (*Config, error) {
	path := filepath.Join(Dir(), "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Override from environment variables.
	if v := os.Getenv("AGENT_MEMORY_P2P_ENABLED"); v != "" {
		cfg.P2PEnabled = v == "true" || v == "1"
	}
	if v := os.Getenv("AGENT_MEMORY_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}

	return &cfg, nil
}

// Create generates a new Config with a random salt and derives keys
// from the given secret using HKDF-SHA256.
func Create(secret string, ipfsAddr string, p2pEnabled bool, dataDir string) (*Config, error) {
	if secret == "" {
		return nil, fmt.Errorf("secret must not be empty")
	}

	// Generate a 32-byte random salt.
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generating salt: %w", err)
	}

	// Derive keys to validate the secret works correctly.
	if _, err := deriveKeys(secret, salt); err != nil {
		return nil, fmt.Errorf("deriving keys: %w", err)
	}

	// Apply default DataDir if empty.
	if dataDir == "" {
		dataDir = DefaultDataDir()
	}

	cfg := &Config{
		Version:    version,
		IPFSAddr:   ipfsAddr,
		SaltHex:    hex.EncodeToString(salt),
		Created:    nowUTC(),
		P2PEnabled: p2pEnabled,
		DataDir:    dataDir,
	}
	return cfg, nil
}

// Save writes the configuration as pretty-printed JSON to the given path.
func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	// Write with 0600 since config contains sensitive material (the salt).
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing config to %s: %w", path, err)
	}
	return nil
}

// GetKeys re-derives the three keys from the given secret and the stored salt.
func (c *Config) GetKeys(secret string) (*Keys, error) {
	if secret == "" {
		return nil, fmt.Errorf("secret must not be empty")
	}
	salt, err := hex.DecodeString(c.SaltHex)
	if err != nil {
		return nil, fmt.Errorf("decoding salt hex: %w", err)
	}
	return deriveKeys(secret, salt)
}

// --- internal helpers ---

// deriveKeys uses HKDF-SHA256 to derive three 32-byte keys from the
// secret and salt.
func deriveKeys(secret string, salt []byte) (*Keys, error) {
	var keys Keys

	if err := deriveOneKey([]byte(secret), salt, []byte(infoEncryption), keys.EncryptionKey[:]); err != nil {
		return nil, fmt.Errorf("encryption key: %w", err)
	}
	if err := deriveOneKey([]byte(secret), salt, []byte(infoIndex), keys.IndexKey[:]); err != nil {
		return nil, fmt.Errorf("index key: %w", err)
	}
	if err := deriveOneKey([]byte(secret), salt, []byte(infoSigning), keys.SigningKey[:]); err != nil {
		return nil, fmt.Errorf("signing key: %w", err)
	}

	return &keys, nil
}

// deriveOneKey reads exactly 32 bytes from an HKDF-SHA256 reader into dst.
func deriveOneKey(secret, salt, info []byte, dst []byte) error {
	r := hkdf.New(sha256.New, secret, salt, info)
	// Read the full key length. hkdf.Reader returns io.EOF when exhausted;
	// a short read means something is wrong.
	n, err := r.Read(dst)
	if err != nil {
		return fmt.Errorf("hkdf read: %w", err)
	}
	if n != len(dst) {
		return fmt.Errorf("hkdf short read: got %d bytes, want %d", n, len(dst))
	}
	return nil
}

// homeDir returns the current user's home directory.
func homeDir() string {
	// Prefer $HOME so it works in sandboxed/container environments.
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	u, err := user.Current()
	if err != nil {
		// Last resort — should not happen on a normal system.
		return "~"
	}
	return u.HomeDir
}

// nowUTC returns the current UTC time in RFC3339 format.
func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// VerifyMAC reports whether messageMAC is a valid HMAC-SHA256 of message
// using the given key. This is a utility for consumers of the signing key.
func VerifyMAC(key []byte, message, messageMAC []byte) bool {
	mac := hmac.New(sha256.New, key)
	mac.Write(message)
	expected := mac.Sum(nil)
	return hmac.Equal(messageMAC, expected)
}
