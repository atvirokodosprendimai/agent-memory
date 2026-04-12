package store

import (
	"strings"
	"time"
)

// Filter defines criteria for querying memory entries.
type Filter struct {
	Type   EntryType
	Tags   []string
	Source string
	Since  time.Time
	Limit  int
}

// Match checks if an IndexEntry matches the filter criteria.
func (f *Filter) Match(ie *IndexEntry) bool {
	// Type filter
	if f.Type != "" && ie.Type != string(f.Type) {
		return false
	}

	// Source filter
	if f.Source != "" && ie.Source != f.Source {
		return false
	}

	// Tags filter — all specified tags must be present
	if len(f.Tags) > 0 {
		entryTags := make(map[string]bool)
		for _, t := range ie.Tags {
			entryTags[strings.ToLower(t)] = true
		}
		for _, required := range f.Tags {
			if !entryTags[strings.ToLower(required)] {
				return false
			}
		}
	}

	// Since filter
	if !f.Since.IsZero() {
		ts, err := time.Parse(time.RFC3339, ie.Timestamp)
		if err != nil || ts.Before(f.Since) {
			return false
		}
	}

	return true
}
