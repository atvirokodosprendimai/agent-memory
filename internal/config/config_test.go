package config

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDir(t *testing.T) {
	dir := Dir()
	if dir == "" {
		t.Fatal("Dir() returned empty string")
	}
	if filepath.Base(dir) != "agent-memory" {
		t.Fatalf("Dir() = %q, want base to be 'agent-memory'", dir)
	}
	parent := filepath.Base(filepath.Dir(dir))
	if parent != ".config" {
		t.Fatalf("Dir() = %q, want parent to be '.config'", dir)
	}
}

func TestCreateAndGetKeysRoundTrip(t *testing.T) {
	secret := "super-secret-test-value"
	ipfsAddr := "/ip4/127.0.0.1/tcp/5001"

	cfg, err := Create(secret, ipfsAddr, false, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if cfg.Version != 1 {
		t.Errorf("Version = %d, want 1", cfg.Version)
	}
	if cfg.IPFSAddr != ipfsAddr {
		t.Errorf("IPFSAddr = %q, want %q", cfg.IPFSAddr, ipfsAddr)
	}
	if cfg.SaltHex == "" {
		t.Error("SaltHex is empty")
	}
	if cfg.Created == "" {
		t.Error("Created is empty")
	}
	if cfg.SaltHex == secret {
		t.Error("SaltHex should not equal the secret")
	}

	salt, err := hex.DecodeString(cfg.SaltHex)
	if err != nil {
		t.Fatalf("SaltHex is not valid hex: %v", err)
	}
	if len(salt) != 32 {
		t.Fatalf("salt length = %d, want 32", len(salt))
	}

	// Re-derive keys twice — must be deterministic
	keys1, err := cfg.GetKeys(secret)
	if err != nil {
		t.Fatalf("GetKeys() error: %v", err)
	}

	keys2, err := cfg.GetKeys(secret)
	if err != nil {
		t.Fatalf("GetKeys() second call error: %v", err)
	}

	if keys1.EncryptionKey != keys2.EncryptionKey {
		t.Error("EncryptionKey not deterministic")
	}
	if keys1.IndexKey != keys2.IndexKey {
		t.Error("IndexKey not deterministic")
	}
	if keys1.SigningKey != keys2.SigningKey {
		t.Error("SigningKey not deterministic")
	}

	// Different secret must produce different keys
	keys3, err := cfg.GetKeys("wrong-secret")
	if err != nil {
		t.Fatalf("GetKeys(wrong) error: %v", err)
	}
	if keys1.EncryptionKey == keys3.EncryptionKey {
		t.Error("Different secret produced same EncryptionKey")
	}
}

func TestCreateRejectsEmptySecret(t *testing.T) {
	_, err := Create("", "/ip4/127.0.0.1/tcp/5001", false, "")
	if err == nil {
		t.Fatal("Create with empty secret should return error")
	}
}

func TestGetKeysRejectsEmptySecret(t *testing.T) {
	// 32 zero-bytes as hex = 64 hex chars
	cfg := &Config{SaltHex: hex.EncodeToString(make([]byte, 32))}
	_, err := cfg.GetKeys("")
	if err == nil {
		t.Fatal("GetKeys with empty secret should return error")
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	secret := "test-secret-for-save"
	cfg, err := Create(secret, "/ip4/127.0.0.1/tcp/5001", false, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Verify file permissions
	stat, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("Stat() error: %v", err)
	}
	perm := stat.Mode().Perm()
	if perm != 0600 {
		t.Errorf("file permissions = %o, want 0600", perm)
	}

	// Read and parse manually to verify content
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}

	var parsed Config
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}

	if parsed.Version != cfg.Version {
		t.Errorf("Version mismatch: got %d, want %d", parsed.Version, cfg.Version)
	}
	if parsed.IPFSAddr != cfg.IPFSAddr {
		t.Errorf("IPFSAddr mismatch: got %q, want %q", parsed.IPFSAddr, cfg.IPFSAddr)
	}
	if parsed.SaltHex != cfg.SaltHex {
		t.Errorf("SaltHex mismatch: got %q, want %q", parsed.SaltHex, cfg.SaltHex)
	}

	// Verify keys derived from loaded config match originals
	keysOriginal, _ := cfg.GetKeys(secret)
	keysLoaded, err := parsed.GetKeys(secret)
	if err != nil {
		t.Fatalf("GetKeys from loaded config error: %v", err)
	}
	if keysOriginal.EncryptionKey != keysLoaded.EncryptionKey {
		t.Error("EncryptionKey mismatch after save/load round-trip")
	}
}

func TestKeysAreDifferent(t *testing.T) {
	secret := "key-difference-test"
	cfg, err := Create(secret, "/ip4/127.0.0.1/tcp/5001", false, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	keys, err := cfg.GetKeys(secret)
	if err != nil {
		t.Fatalf("GetKeys() error: %v", err)
	}

	if keys.EncryptionKey == keys.IndexKey {
		t.Error("EncryptionKey == IndexKey, should be different")
	}
	if keys.EncryptionKey == keys.SigningKey {
		t.Error("EncryptionKey == SigningKey, should be different")
	}
	if keys.IndexKey == keys.SigningKey {
		t.Error("IndexKey == SigningKey, should be different")
	}
}

func TestDifferentConfigsProduceDifferentKeys(t *testing.T) {
	secret := "same-secret"
	cfg1, _ := Create(secret, "/ip4/127.0.0.1/tcp/5001", false, "")
	cfg2, _ := Create(secret, "/ip4/127.0.0.1/tcp/5001", false, "")

	if cfg1.SaltHex == cfg2.SaltHex {
		t.Error("Two Create calls produced the same salt (extremely unlikely)")
	}

	keys1, _ := cfg1.GetKeys(secret)
	keys2, _ := cfg2.GetKeys(secret)

	if keys1.EncryptionKey == keys2.EncryptionKey {
		t.Error("Different salts produced same EncryptionKey")
	}
}

func TestIndexCIDIsOmittedWhenEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg, _ := Create("secret", "/ip4/127.0.0.1/tcp/5001", false, "")
	cfg.Save(configPath)

	data, _ := os.ReadFile(configPath)
	if strings.Contains(string(data), "index_cid") {
		t.Error("index_cid should be omitted when empty due to omitempty tag")
	}
}

func TestCreateSetsP2PEnabled(t *testing.T) {
	cfg, err := Create("secret", "/ip4/127.0.0.1/tcp/5001", true, "/custom/p2p")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if !cfg.P2PEnabled {
		t.Error("P2PEnabled = false, want true")
	}
}

func TestCreateSetsDataDir(t *testing.T) {
	dataDir := "/custom/p2p/data"
	cfg, err := Create("secret", "/ip4/127.0.0.1/tcp/5001", false, dataDir)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if cfg.DataDir != dataDir {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, dataDir)
	}
}

func TestCreateAppliesDefaultDataDir(t *testing.T) {
	cfg, err := Create("secret", "/ip4/127.0.0.1/tcp/5001", false, "")
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if cfg.DataDir == "" {
		t.Error("DataDir is empty, want default")
	}
	expected := DefaultDataDir()
	if cfg.DataDir != expected {
		t.Errorf("DataDir = %q, want default %q", cfg.DataDir, expected)
	}
}

func TestDefaultDataDir(t *testing.T) {
	dir := DefaultDataDir()
	if dir == "" {
		t.Fatal("DefaultDataDir() returned empty")
	}
	if filepath.Base(dir) != "p2p" {
		t.Fatalf("DefaultDataDir() = %q, want base to be 'p2p'", dir)
	}
	parent := filepath.Base(filepath.Dir(dir))
	if parent != ".agent-memory" {
		t.Fatalf("DefaultDataDir() = %q, want parent to be '.agent-memory'", dir)
	}
}
