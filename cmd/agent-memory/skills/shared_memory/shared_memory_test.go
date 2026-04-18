package shared_memory

import (
	"testing"
)

func TestNormalizeSecret(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		wantOk  string
	}{
		{
			name:    "passphrase",
			input:   "my-secret-passphrase",
			wantErr: false,
			wantOk:  "my-secret-passphrase",
		},
		{
			name:    "passphrase with spaces",
			input:   "  my-secret  ",
			wantErr: false,
			wantOk:  "my-secret",
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			input:   "   ",
			wantErr: true,
		},
		{
			name:    "hex with 0x prefix",
			input:   "0xdeadbeef",
			wantErr: false,
			wantOk:  "\xde\xad\xbe\xef",
		},
		{
			name:    "hex with 0X prefix",
			input:   "0XDEADBEef",
			wantErr: false,
			wantOk:  "\xde\xad\xbe\xef",
		},
		{
			name:    "plain even-length hex",
			input:   "deadbeef",
			wantErr: false,
			wantOk:  "\xde\xad\xbe\xef",
		},
		{
			name:    "plain hex odd length treated as passphrase",
			input:   "deadbeef0",
			wantErr: false,
			wantOk:  "deadbeef0",
		},
		{
			name:    "invalid hex with 0x prefix",
			input:   "0xZZZZ",
			wantErr: true,
		},
		{
			name:    "all zeros",
			input:   "00000000",
			wantErr: false,
			wantOk:  "\x00\x00\x00\x00",
		},
		{
			name:    "mixed case hex",
			input:   "aAbBcCdD",
			wantErr: false,
			wantOk:  "\xaa\xbb\xcc\xdd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeSecret(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("normalizeSecret(%q) = %q, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("normalizeSecret(%q) unexpected error: %v", tt.input, err)
				return
			}
			if got != tt.wantOk {
				t.Errorf("normalizeSecret(%q) = %q (% x), want %q (% x)", tt.input, got, got, tt.wantOk, tt.wantOk)
			}
		})
	}
}

func TestIsHexString(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"deadbeef", true},
		{"DEADBEEF", true},
		{"aAbBcCdD", true},
		{"0123456789abcdefABCDEF", true},
		{"", true},
		{"g", false},
		{"G", false},
		{"deadbeefg", false},
		{"-1", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isHexString(tt.input)
			if got != tt.want {
				t.Errorf("isHexString(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetSession_NotFound(t *testing.T) {
	sessionsMu.Lock()
	sessions = make(map[string]*SkillState)
	sessionsMu.Unlock()

	got := GetSession("nonexistent")
	if got != nil {
		t.Errorf("GetSession(nonexistent) = %v, want nil", got)
	}
}

func TestCloseSession_NotFound(t *testing.T) {
	sessionsMu.Lock()
	sessions = make(map[string]*SkillState)
	sessionsMu.Unlock()

	CloseSession("nonexistent")
}

func TestBuildFilter(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]any
	}{
		{
			name:   "empty params",
			params: map[string]any{},
		},
		{
			name: "type only",
			params: map[string]any{
				"type": "decision",
			},
		},
		{
			name: "tags only",
			params: map[string]any{
				"tags": []any{"billing", "API"},
			},
		},
		{
			name: "limit only",
			params: map[string]any{
				"limit": 5,
			},
		},
		{
			name: "limit exceeds max",
			params: map[string]any{
				"limit": 500,
			},
		},
		{
			name: "source only",
			params: map[string]any{
				"source": "gpt-4",
			},
		},
		{
			name: "since RFC3339",
			params: map[string]any{
				"since": "2026-04-01T00:00:00Z",
			},
		},
		{
			name: "since invalid",
			params: map[string]any{
				"since": "not-a-date",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := buildFilter(tt.params)
			if tt.name == "since invalid" {
				if err == nil {
					t.Error("buildFilter with invalid since should return error")
				}
				return
			}
			if err != nil {
				t.Errorf("buildFilter(%v) unexpected error: %v", tt.params, err)
			}
			_ = f
		})
	}
}

func TestIsValidEntryType(t *testing.T) {
	valid := []string{
		"decision", "learning", "trace",
		"observation", "blocker", "context",
	}
	for _, v := range valid {
		if !isValidEntryType(v) {
			t.Errorf("isValidEntryType(%q) = false, want true", v)
		}
	}

	invalid := []string{
		"", "DECISION", "Decision",
		"note", "fact", "unknown",
		"decision ", " decision",
	}
	for _, v := range invalid {
		if isValidEntryType(v) {
			t.Errorf("isValidEntryType(%q) = true, want false", v)
		}
	}
}

func TestHandleSession_Inactive(t *testing.T) {
	result, err := HandleSession(nil)
	if err != nil {
		t.Errorf("HandleSession(nil) error: %v", err)
	}
	if result["active"] != false {
		t.Errorf("HandleSession(nil) active = %v, want false", result["active"])
	}
}

func TestSkillVersion(t *testing.T) {
	if SkillVersion != "1.0" {
		t.Errorf("SkillVersion = %q, want 1.0", SkillVersion)
	}
}
