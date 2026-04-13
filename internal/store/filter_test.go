package store

import (
	"testing"
	"time"
)

func TestFilterMatch(t *testing.T) {
	ts := "2026-04-12T11:05:00Z"

	tests := []struct {
		name   string
		filter Filter
		entry  IndexEntry
		want   bool
	}{
		{
			name:   "empty filter matches everything",
			filter: Filter{},
			entry:  IndexEntry{Type: "decision", Source: "human", Tags: []string{"billing"}, Timestamp: ts},
			want:   true,
		},
		{
			name:   "type matches",
			filter: Filter{Type: TypeDecision},
			entry:  IndexEntry{Type: "decision", Timestamp: ts},
			want:   true,
		},
		{
			name:   "type mismatch",
			filter: Filter{Type: TypeDecision},
			entry:  IndexEntry{Type: "learning", Timestamp: ts},
			want:   false,
		},
		{
			name:   "source matches",
			filter: Filter{Source: "goose"},
			entry:  IndexEntry{Source: "goose", Timestamp: ts},
			want:   true,
		},
		{
			name:   "source mismatch",
			filter: Filter{Source: "goose"},
			entry:  IndexEntry{Source: "human", Timestamp: ts},
			want:   false,
		},
		{
			name:   "single tag matches",
			filter: Filter{Tags: []string{"billing"}},
			entry:  IndexEntry{Tags: []string{"billing", "stripe"}, Timestamp: ts},
			want:   true,
		},
		{
			name:   "all tags must match",
			filter: Filter{Tags: []string{"billing", "stripe"}},
			entry:  IndexEntry{Tags: []string{"billing", "stripe", "decision"}, Timestamp: ts},
			want:   true,
		},
		{
			name:   "missing required tag",
			filter: Filter{Tags: []string{"billing", "paddle"}},
			entry:  IndexEntry{Tags: []string{"billing", "stripe"}, Timestamp: ts},
			want:   false,
		},
		{
			name:   "tag matching is case-insensitive",
			filter: Filter{Tags: []string{"Billing"}},
			entry:  IndexEntry{Tags: []string{"billing"}, Timestamp: ts},
			want:   true,
		},
		{
			name:   "since includes entries on that date",
			filter: Filter{Since: mustParseTime(t, "2026-04-12")},
			entry:  IndexEntry{Timestamp: ts},
			want:   true,
		},
		{
			name:   "since excludes older entries",
			filter: Filter{Since: mustParseTime(t, "2026-04-13")},
			entry:  IndexEntry{Timestamp: ts},
			want:   false,
		},
		{
			name:   "since with zero time matches all",
			filter: Filter{},
			entry:  IndexEntry{Timestamp: ts},
			want:   true,
		},
		{
			name:   "invalid timestamp rejected by since filter",
			filter: Filter{Since: mustParseTime(t, "2026-01-01")},
			entry:  IndexEntry{Timestamp: "not-a-date"},
			want:   false,
		},
		{
			name: "combined filters all match",
			filter: Filter{
				Type:   TypeDecision,
				Source: "human",
				Tags:   []string{"billing"},
				Since:  mustParseTime(t, "2026-04-01"),
			},
			entry: IndexEntry{
				Type:      "decision",
				Source:    "human",
				Tags:      []string{"billing", "stripe"},
				Timestamp: ts,
			},
			want: true,
		},
		{
			name: "combined filters one mismatch fails",
			filter: Filter{
				Type:   TypeDecision,
				Source: "goose",
				Tags:   []string{"billing"},
			},
			entry: IndexEntry{
				Type:      "decision",
				Source:    "human",
				Tags:      []string{"billing"},
				Timestamp: ts,
			},
			want: false,
		},
		{
			name:   "empty tags in filter matches entry with tags",
			filter: Filter{},
			entry:  IndexEntry{Tags: []string{"a", "b", "c"}, Timestamp: ts},
			want:   true,
		},
		{
			name:   "tags filter against entry with no tags",
			filter: Filter{Tags: []string{"billing"}},
			entry:  IndexEntry{Tags: nil, Timestamp: ts},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.filter.Match(&tt.entry)
			if got != tt.want {
				t.Errorf("Match() = %v, want %v", got, tt.want)
			}
		})
	}
}

func mustParseTime(t *testing.T, date string) time.Time {
	t.Helper()
	ts, err := time.Parse("2006-01-02", date)
	if err != nil {
		t.Fatalf("mustParseTime(%q): %v", date, err)
	}
	return ts
}
