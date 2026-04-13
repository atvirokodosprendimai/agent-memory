package main

import (
	"os"
	"testing"
)

func TestParseWriteFlags(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantType   string
		wantSource string
		wantContent string
		wantTags   string
	}{
		{
			name:       "all long flags",
			args:       []string{"--type", "decision", "--source", "goose", "--content", "Use Stripe", "--tag", "billing,stripe"},
			wantType:   "decision",
			wantSource: "goose",
			wantContent: "Use Stripe",
			wantTags:   "billing,stripe",
		},
		{
			name:       "short flags",
			args:       []string{"-t", "learning", "-s", "human", "-c", "Learned something"},
			wantType:   "learning",
			wantSource: "human",
			wantContent: "Learned something",
			wantTags:   "",
		},
		{
			name:       "defaults",
			args:       []string{},
			wantType:   "",
			wantSource: "human",
			wantContent: "",
			wantTags:   "",
		},
		{
			name:       "only type and content",
			args:       []string{"--type", "blocker", "--content", "No API key"},
			wantType:   "blocker",
			wantSource: "human",
			wantContent: "No API key",
			wantTags:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entryType, source, content, tags := parseWriteFlags(tt.args)
			if entryType != tt.wantType {
				t.Errorf("type = %q, want %q", entryType, tt.wantType)
			}
			if source != tt.wantSource {
				t.Errorf("source = %q, want %q", source, tt.wantSource)
			}
			if content != tt.wantContent {
				t.Errorf("content = %q, want %q", content, tt.wantContent)
			}
			if tags != tt.wantTags {
				t.Errorf("tags = %q, want %q", tags, tt.wantTags)
			}
		})
	}
}

func TestParseReadFlags(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		wantType  string
		wantSource string
		wantTags  string
		wantSince string
		wantLimit int
	}{
		{
			name:      "all flags",
			args:      []string{"--type", "decision", "--source", "human", "--tag", "billing", "--since", "2026-04-01", "--limit", "5"},
			wantType:  "decision",
			wantSource: "human",
			wantTags:  "billing",
			wantSince: "2026-04-01",
			wantLimit: 5,
		},
		{
			name:      "defaults",
			args:      []string{},
			wantType:  "",
			wantSource: "",
			wantTags:  "",
			wantSince: "",
			wantLimit: 10,
		},
		{
			name:      "only limit",
			args:      []string{"--limit", "3"},
			wantType:  "",
			wantSource: "",
			wantTags:  "",
			wantSince: "",
			wantLimit: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entryType, source, tags, since, limit := parseReadFlags(tt.args)
			if entryType != tt.wantType {
				t.Errorf("type = %q, want %q", entryType, tt.wantType)
			}
			if source != tt.wantSource {
				t.Errorf("source = %q, want %q", source, tt.wantSource)
			}
			if tags != tt.wantTags {
				t.Errorf("tags = %q, want %q", tags, tt.wantTags)
			}
			if since != tt.wantSince {
				t.Errorf("since = %q, want %q", since, tt.wantSince)
			}
			if limit != tt.wantLimit {
				t.Errorf("limit = %d, want %d", limit, tt.wantLimit)
			}
		})
	}
}

func TestGetSecret(t *testing.T) {
	// Save and restore env
	orig := os.Getenv("AGENT_MEMORY_SECRET")
	defer os.Setenv("AGENT_MEMORY_SECRET", orig)

	t.Run("reads from env", func(t *testing.T) {
		os.Setenv("AGENT_MEMORY_SECRET", "test-secret-value")
		got := getSecret()
		if got != "test-secret-value" {
			t.Errorf("getSecret() = %q, want %q", got, "test-secret-value")
		}
	})

	t.Run("empty when env unset", func(t *testing.T) {
		os.Unsetenv("AGENT_MEMORY_SECRET")
		got := getSecret()
		if got != "" {
			t.Errorf("getSecret() = %q, want empty", got)
		}
	})
}

func TestGetIPFSAddr(t *testing.T) {
	// Save and restore env
	orig := os.Getenv("AGENT_MEMORY_IPFS_ADDR")
	defer os.Setenv("AGENT_MEMORY_IPFS_ADDR", orig)

	t.Run("reads from env", func(t *testing.T) {
		os.Setenv("AGENT_MEMORY_IPFS_ADDR", "http://custom:5001")
		got := getIPFSAddr()
		if got != "http://custom:5001" {
			t.Errorf("getIPFSAddr() = %q, want %q", got, "http://custom:5001")
		}
	})

	t.Run("default when env unset", func(t *testing.T) {
		os.Unsetenv("AGENT_MEMORY_IPFS_ADDR")
		got := getIPFSAddr()
		if got != "http://localhost:5001" {
			t.Errorf("getIPFSAddr() = %q, want %q", got, "http://localhost:5001")
		}
	})
}
