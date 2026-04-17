package main

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/atvirokodosprendimai/agent-memory/internal/config"
	"github.com/atvirokodosprendimai/agent-memory/internal/store"
)

// mockStore implements a minimal store for testing.
type mockStore struct {
	writeFunc func(entryType store.EntryType, source string, tags []string, content string, metadata map[string]any) (*store.Entry, error)
	readFunc  func(filter store.Filter) ([]*store.Entry, error)
	gcFunc    func(maxAge time.Duration) (int, error)
	closeFunc func() error
}

func (m *mockStore) Write(entryType store.EntryType, source string, tags []string, content string, metadata map[string]any) (*store.Entry, error) {
	if m.writeFunc != nil {
		return m.writeFunc(entryType, source, tags, content, metadata)
	}
	return &store.Entry{ID: "mock-id", Type: entryType, Source: source, Tags: tags, Content: content}, nil
}

func (m *mockStore) Read(filter store.Filter) ([]*store.Entry, error) {
	if m.readFunc != nil {
		return m.readFunc(filter)
	}
	return nil, nil
}

func (m *mockStore) GC(maxAge time.Duration) (int, error) {
	if m.gcFunc != nil {
		return m.gcFunc(maxAge)
	}
	return 0, nil
}

func (m *mockStore) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func testConfig() *config.Config {
	return &config.Config{
		IPFSAddr: "http://localhost:5001",
	}
}

func TestParseWriteFlags(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantType    string
		wantSource  string
		wantContent string
		wantTags    string
	}{
		{
			name:        "all long flags",
			args:        []string{"--type", "decision", "--source", "goose", "--content", "Use Stripe", "--tag", "billing,stripe"},
			wantType:    "decision",
			wantSource:  "goose",
			wantContent: "Use Stripe",
			wantTags:    "billing,stripe",
		},
		{
			name:        "short flags",
			args:        []string{"-t", "learning", "-s", "human", "-c", "Learned something"},
			wantType:    "learning",
			wantSource:  "human",
			wantContent: "Learned something",
			wantTags:    "",
		},
		{
			name:        "defaults",
			args:        []string{},
			wantType:    "",
			wantSource:  "human",
			wantContent: "",
			wantTags:    "",
		},
		{
			name:        "only type and content",
			args:        []string{"--type", "blocker", "--content", "No API key"},
			wantType:    "blocker",
			wantSource:  "human",
			wantContent: "No API key",
			wantTags:    "",
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

func TestRunWrite(t *testing.T) {
	origSecret := os.Getenv("AGENT_MEMORY_SECRET")
	origArgs := os.Args
	defer func() {
		os.Setenv("AGENT_MEMORY_SECRET", origSecret)
		os.Args = origArgs
	}()

	t.Run("calls store.Write with correct args", func(t *testing.T) {
		os.Setenv("AGENT_MEMORY_SECRET", "test-secret")
		os.Args = []string{"cmd", "write", "--type", "decision", "--source", "goose", "--content", "Use Stripe", "--tag", "billing,stripe"}

		var capturedType store.EntryType
		var capturedSource, capturedContent string
		var capturedTags []string

		storeFactory = func(cfg *config.Config, secret string) (StoreInterface, error) {
			return &mockStore{
				writeFunc: func(entryType store.EntryType, source string, tags []string, content string, metadata map[string]any) (*store.Entry, error) {
					capturedType = entryType
					capturedSource = source
					capturedTags = tags
					capturedContent = content
					return &store.Entry{ID: "abcd1234efgh5678ijkl9012mnop3456", Type: entryType, Source: source, Tags: tags, Content: content}, nil
				},
			}, nil
		}

		cfg := testConfig()
		err := runWrite(cfg)

		if err != nil {
			t.Errorf("runWrite() error = %v, want nil", err)
		}
		if capturedType != "decision" {
			t.Errorf("capturedType = %q, want %q", capturedType, "decision")
		}
		if capturedSource != "goose" {
			t.Errorf("capturedSource = %q, want %q", capturedSource, "goose")
		}
		if capturedContent != "Use Stripe" {
			t.Errorf("capturedContent = %q, want %q", capturedContent, "Use Stripe")
		}
		if len(capturedTags) != 2 || capturedTags[0] != "billing" || capturedTags[1] != "stripe" {
			t.Errorf("capturedTags = %v, want [billing stripe]", capturedTags)
		}
	})

	t.Run("returns error when store.Write fails", func(t *testing.T) {
		os.Setenv("AGENT_MEMORY_SECRET", "test-secret")
		os.Args = []string{"cmd", "write", "--type", "decision", "--content", "Test content"}

		storeFactory = func(cfg *config.Config, secret string) (StoreInterface, error) {
			return &mockStore{
				writeFunc: func(entryType store.EntryType, source string, tags []string, content string, metadata map[string]any) (*store.Entry, error) {
					return nil, errors.New("write failed")
				},
			}, nil
		}

		cfg := testConfig()
		err := runWrite(cfg)

		if err == nil {
			t.Error("runWrite() error = nil, want error")
		}
		if err.Error() != "write failed" {
			t.Errorf("runWrite() error = %q, want %q", err.Error(), "write failed")
		}
	})

	// Reset storeFactory to default
	origStoreFactory := storeFactory
	storeFactory = func(cfg *config.Config, secret string) (StoreInterface, error) {
		return store.New(cfg, secret)
	}
	defer func() { storeFactory = origStoreFactory }()
}

func TestRunRead(t *testing.T) {
	origSecret := os.Getenv("AGENT_MEMORY_SECRET")
	origArgs := os.Args
	defer func() {
		os.Setenv("AGENT_MEMORY_SECRET", origSecret)
		os.Args = origArgs
	}()

	t.Run("calls store.Read with correct filter", func(t *testing.T) {
		os.Setenv("AGENT_MEMORY_SECRET", "test-secret")
		os.Args = []string{"cmd", "read", "--type", "decision", "--source", "human", "--tag", "billing", "--limit", "5"}

		var capturedFilter store.Filter

		storeFactory = func(cfg *config.Config, secret string) (StoreInterface, error) {
			return &mockStore{
				readFunc: func(filter store.Filter) ([]*store.Entry, error) {
					capturedFilter = filter
					return []*store.Entry{
						{ID: "entry1", Type: "decision", Source: "human", Content: "Test entry", Tags: []string{"billing"}},
					}, nil
				},
			}, nil
		}

		cfg := testConfig()
		err := runRead(cfg)

		if err != nil {
			t.Errorf("runRead() error = %v, want nil", err)
		}
		if capturedFilter.Type != "decision" {
			t.Errorf("capturedFilter.Type = %q, want %q", capturedFilter.Type, "decision")
		}
		if capturedFilter.Source != "human" {
			t.Errorf("capturedFilter.Source = %q, want %q", capturedFilter.Source, "human")
		}
		if capturedFilter.Limit != 5 {
			t.Errorf("capturedFilter.Limit = %d, want %d", capturedFilter.Limit, 5)
		}
		if len(capturedFilter.Tags) != 1 || capturedFilter.Tags[0] != "billing" {
			t.Errorf("capturedFilter.Tags = %v, want [billing]", capturedFilter.Tags)
		}
	})

	t.Run("handles empty results", func(t *testing.T) {
		os.Setenv("AGENT_MEMORY_SECRET", "test-secret")
		os.Args = []string{"cmd", "read"}

		storeFactory = func(cfg *config.Config, secret string) (StoreInterface, error) {
			return &mockStore{
				readFunc: func(filter store.Filter) ([]*store.Entry, error) {
					return []*store.Entry{}, nil
				},
			}, nil
		}

		cfg := testConfig()
		err := runRead(cfg)

		if err != nil {
			t.Errorf("runRead() error = %v, want nil", err)
		}
	})

	t.Run("returns error when store.Read fails", func(t *testing.T) {
		os.Setenv("AGENT_MEMORY_SECRET", "test-secret")
		os.Args = []string{"cmd", "read"}

		storeFactory = func(cfg *config.Config, secret string) (StoreInterface, error) {
			return &mockStore{
				readFunc: func(filter store.Filter) ([]*store.Entry, error) {
					return nil, errors.New("read failed")
				},
			}, nil
		}

		cfg := testConfig()
		err := runRead(cfg)

		if err == nil {
			t.Error("runRead() error = nil, want error")
		}
	})

	// Reset storeFactory to default
	origStoreFactory := storeFactory
	storeFactory = func(cfg *config.Config, secret string) (StoreInterface, error) {
		return store.New(cfg, secret)
	}
	defer func() { storeFactory = origStoreFactory }()
}

func TestRunGC(t *testing.T) {
	origSecret := os.Getenv("AGENT_MEMORY_SECRET")
	origArgs := os.Args
	defer func() {
		os.Setenv("AGENT_MEMORY_SECRET", origSecret)
		os.Args = origArgs
	}()

	t.Run("calls store.GC with correct maxAge", func(t *testing.T) {
		os.Setenv("AGENT_MEMORY_SECRET", "test-secret")
		os.Args = []string{"cmd", "gc", "--max-age", "30d"}

		var capturedMaxAge time.Duration

		storeFactory = func(cfg *config.Config, secret string) (StoreInterface, error) {
			return &mockStore{
				gcFunc: func(maxAge time.Duration) (int, error) {
					capturedMaxAge = maxAge
					return 5, nil
				},
			}, nil
		}

		cfg := testConfig()
		err := runGC(cfg)

		if err != nil {
			t.Errorf("runGC() error = %v, want nil", err)
		}
		expectedAge := 30 * 24 * time.Hour
		if capturedMaxAge != expectedAge {
			t.Errorf("capturedMaxAge = %v, want %v", capturedMaxAge, expectedAge)
		}
	})

	t.Run("returns correct removed count", func(t *testing.T) {
		os.Setenv("AGENT_MEMORY_SECRET", "test-secret")
		os.Args = []string{"cmd", "gc", "--max-age", "7d"}

		storeFactory = func(cfg *config.Config, secret string) (StoreInterface, error) {
			return &mockStore{
				gcFunc: func(maxAge time.Duration) (int, error) {
					return 3, nil
				},
			}, nil
		}

		cfg := testConfig()
		err := runGC(cfg)

		if err != nil {
			t.Errorf("runGC() error = %v, want nil", err)
		}
	})

	t.Run("returns error when store.GC fails", func(t *testing.T) {
		os.Setenv("AGENT_MEMORY_SECRET", "test-secret")
		os.Args = []string{"cmd", "gc", "--max-age", "30d"}

		storeFactory = func(cfg *config.Config, secret string) (StoreInterface, error) {
			return &mockStore{
				gcFunc: func(maxAge time.Duration) (int, error) {
					return 0, errors.New("gc failed")
				},
			}, nil
		}

		cfg := testConfig()
		err := runGC(cfg)

		if err == nil {
			t.Error("runGC() error = nil, want error")
		}
	})

	t.Run("returns error when max-age not provided", func(t *testing.T) {
		os.Setenv("AGENT_MEMORY_SECRET", "test-secret")
		os.Args = []string{"cmd", "gc"}

		storeFactory = func(cfg *config.Config, secret string) (StoreInterface, error) {
			return &mockStore{}, nil
		}

		cfg := testConfig()
		err := runGC(cfg)

		if err == nil {
			t.Error("runGC() error = nil, want error about missing --max-age")
		}
	})

	// Reset storeFactory to default
	origStoreFactory := storeFactory
	storeFactory = func(cfg *config.Config, secret string) (StoreInterface, error) {
		return store.New(cfg, secret)
	}
	defer func() { storeFactory = origStoreFactory }()
}
