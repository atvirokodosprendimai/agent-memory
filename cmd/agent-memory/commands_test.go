package main

import (
	"os"
	"testing"
	"time"
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
		name       string
		args       []string
		wantType   string
		wantSource string
		wantTags   string
		wantSince  string
		wantLimit  int
		wantRaw    bool
	}{
		{
			name:       "all flags",
			args:       []string{"--type", "decision", "--source", "human", "--tag", "billing", "--since", "2026-04-01", "--limit", "5"},
			wantType:   "decision",
			wantSource: "human",
			wantTags:   "billing",
			wantSince:  "2026-04-01",
			wantLimit:  5,
		},
		{
			name:      "defaults",
			args:      []string{},
			wantLimit: 10,
		},
		{
			name:      "only limit",
			args:      []string{"--limit", "3"},
			wantLimit: 3,
		},
		{
			name:      "raw flag",
			args:      []string{"--raw", "--type", "decision"},
			wantType:  "decision",
			wantLimit: 10,
			wantRaw:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entryType, source, tags, since, limit, raw := parseReadFlags(tt.args)
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
			if raw != tt.wantRaw {
				t.Errorf("raw = %v, want %v", raw, tt.wantRaw)
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

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		{name: "days", input: "30d", want: 30 * 24 * time.Hour},
		{name: "1 day", input: "1d", want: 24 * time.Hour},
		{name: "hours", input: "720h", want: 720 * time.Hour},
		{name: "minutes", input: "30m", want: 30 * time.Minute},
		{name: "mixed", input: "2h30m", want: 2*time.Hour + 30*time.Minute},
		{name: "invalid days", input: "abcd", wantErr: true},
		{name: "invalid format", input: "30x", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDuration(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseDuration(%q) = %v, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDuration(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseGCFlags(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantAge string
	}{
		{name: "with max-age", args: []string{"--max-age", "30d"}, wantAge: "30d"},
		{name: "defaults empty", args: []string{}, wantAge: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGCFlags(tt.args)
			if got != tt.wantAge {
				t.Errorf("maxAge = %q, want %q", got, tt.wantAge)
			}
		})
	}
}

func TestParseExportFlags(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantType   string
		wantTags   string
		wantOutput string
	}{
		{
			name:       "all flags",
			args:       []string{"--type", "decision", "--tag", "billing", "--output", "/tmp/export.jsonl"},
			wantType:   "decision",
			wantTags:   "billing",
			wantOutput: "/tmp/export.jsonl",
		},
		{
			name:       "shorthand output",
			args:       []string{"-o", "/tmp/out.jsonl"},
			wantOutput: "/tmp/out.jsonl",
		},
		{
			name: "defaults",
			args: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entryType, tags, output := parseExportFlags(tt.args)
			if entryType != tt.wantType {
				t.Errorf("type = %q, want %q", entryType, tt.wantType)
			}
			if tags != tt.wantTags {
				t.Errorf("tags = %q, want %q", tags, tt.wantTags)
			}
			if output != tt.wantOutput {
				t.Errorf("output = %q, want %q", output, tt.wantOutput)
			}
		})
	}
}

func TestParseImportFlags(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantInput  string
		wantSource string
	}{
		{
			name:       "all flags",
			args:       []string{"--input", "/tmp/import.jsonl", "--source", "goose"},
			wantInput:  "/tmp/import.jsonl",
			wantSource: "goose",
		},
		{
			name:      "shorthand input",
			args:      []string{"-i", "/tmp/in.jsonl"},
			wantInput: "/tmp/in.jsonl",
		},
		{
			name: "defaults",
			args: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, source := parseImportFlags(tt.args)
			if input != tt.wantInput {
				t.Errorf("input = %q, want %q", input, tt.wantInput)
			}
			if source != tt.wantSource {
				t.Errorf("source = %q, want %q", source, tt.wantSource)
			}
		})
	}
}
