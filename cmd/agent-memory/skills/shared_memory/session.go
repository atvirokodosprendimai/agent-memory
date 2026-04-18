package shared_memory

import (
	"encoding/hex"
	"fmt"
	"strings"
	"sync"

	"github.com/atvirokodosprendimai/agent-memory/internal/config"
	"github.com/atvirokodosprendimai/agent-memory/internal/store"
)

var (
	SkillVersion = "1.0"
)

type SkillState struct {
	Store  *store.Store
	Source string
	Active bool
}

var (
	sessions   = make(map[string]*SkillState)
	sessionsMu sync.RWMutex
)

func normalizeSecret(secret string) (string, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "", fmt.Errorf("a shared secret is required. Provide it at skill-load time.")
	}

	has0xPrefix := strings.HasPrefix(secret, "0x") || strings.HasPrefix(secret, "0X")
	isHex := len(secret)%2 == 0 && isHexString(secret[2:])

	if has0xPrefix {
		decoded, err := hex.DecodeString(secret[2:])
		if err != nil {
			return "", fmt.Errorf("key derivation failed — verify the shared secret is correct.")
		}
		return string(decoded), nil
	}

	if isHex {
		decoded, err := hex.DecodeString(secret)
		if err != nil {
			return "", fmt.Errorf("key derivation failed — verify the shared secret is correct.")
		}
		return string(decoded), nil
	}

	return secret, nil
}

func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func InitSession(sessionID, secret, source string) error {
	secret, err := normalizeSecret(secret)
	if err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("agent-memory config not found at ~/.config/agent-memory/config.json")
	}

	s, err := store.New(cfg, secret)
	if err != nil {
		return fmt.Errorf("key derivation failed — verify the shared secret is correct.")
	}

	if err := s.IPFSClient().Ping(); err != nil {
		s.Close()
		return fmt.Errorf("IPFS daemon unreachable at %s. Start the daemon and retry.", cfg.IPFSAddr)
	}

	if source == "" {
		source = "opencode-agent"
	}

	sessionsMu.Lock()
	sessions[sessionID] = &SkillState{
		Store:  s,
		Source: source,
		Active: true,
	}
	sessionsMu.Unlock()

	return nil
}

func CloseSession(sessionID string) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	if s, ok := sessions[sessionID]; ok && s.Active {
		s.Store.Close()
		s.Active = false
	}
	delete(sessions, sessionID)
}

func GetSession(sessionID string) *SkillState {
	sessionsMu.RLock()
	defer sessionsMu.RUnlock()
	s := sessions[sessionID]
	if s == nil || !s.Active {
		return nil
	}
	return s
}

func GetSessionSource(sessionID string) string {
	s := GetSession(sessionID)
	if s == nil {
		return ""
	}
	return s.Source
}
