package shared_memory

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/atvirokodosprendimai/agent-memory/internal/store"
)

func HandleWrite(params map[string]any, session *SkillState) (map[string]any, error) {
	content, ok := params["content"].(string)
	if !ok || content == "" {
		return nil, fmt.Errorf("parameter 'content' is required and must be a non-empty string")
	}

	typeStr, ok := params["type"].(string)
	if !ok || typeStr == "" {
		return nil, fmt.Errorf("parameter 'type' is required and must be a non-empty string")
	}

	if !isValidEntryType(typeStr) {
		return nil, fmt.Errorf("invalid type %q: must be one of decision, learning, trace, observation, blocker, context", typeStr)
	}

	tags, _ := params["tags"].([]any)
	var tagStrs []string
	for _, t := range tags {
		if s, ok := t.(string); ok {
			tagStrs = append(tagStrs, s)
		}
	}

	source, _ := params["source"].(string)
	if source == "" {
		source = session.Source
	}

	entryType := store.EntryType(typeStr)

	entry, err := session.Store.Write(entryType, source, tagStrs, content, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to write entry: %w", err)
	}

	return map[string]any{
		"id":        entry.ID,
		"timestamp": entry.Timestamp,
		"message":   "Entry written successfully",
	}, nil
}

func HandleRead(params map[string]any, session *SkillState) (map[string]any, error) {
	filter, err := buildFilter(params)
	if err != nil {
		return nil, err
	}

	entries, err := session.Store.Read(filter)
	if err != nil {
		return nil, fmt.Errorf("failed to read entries: %w", err)
	}

	result := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		result = append(result, map[string]any{
			"id":        entry.ID,
			"type":      entry.Type,
			"source":    entry.Source,
			"timestamp": entry.Timestamp,
			"tags":      entry.Tags,
			"content":   entry.Content,
			"metadata":  entry.Metadata,
		})
	}

	return map[string]any{
		"entries": entries,
	}, nil
}

func HandleList(params map[string]any, session *SkillState) (map[string]any, error) {
	filter, err := buildFilter(params)
	if err != nil {
		return nil, err
	}

	indexEntries, err := session.Store.List(filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list entries: %w", err)
	}

	result := make([]map[string]any, 0, len(indexEntries))
	for _, ie := range indexEntries {
		result = append(result, map[string]any{
			"id":              ie.ID,
			"cid":             ie.CID,
			"type":            ie.Type,
			"tags":            ie.Tags,
			"timestamp":       ie.Timestamp,
			"source":          ie.Source,
			"content_preview": ie.ContentPreview,
		})
	}

	return map[string]any{
		"entries": result,
	}, nil
}

func HandleSession(session *SkillState) (map[string]any, error) {
	if session == nil || !session.Active {
		return map[string]any{
			"active": false,
		}, nil
	}

	count, err := session.Store.EntryCount()
	if err != nil {
		return map[string]any{
			"active":      true,
			"ipfs_addr":   session.Store.Config().IPFSAddr,
			"entry_count": 0,
			"source":      session.Source,
			"version":     SkillVersion,
		}, nil
	}

	return map[string]any{
		"active":      true,
		"ipfs_addr":   session.Store.Config().IPFSAddr,
		"entry_count": count,
		"source":      session.Source,
		"version":     SkillVersion,
	}, nil
}

func buildFilter(params map[string]any) (store.Filter, error) {
	f := store.Filter{Limit: 10}

	if t, ok := params["type"].(string); ok && t != "" {
		if !isValidEntryType(t) {
			return f, fmt.Errorf("invalid type %q: must be one of decision, learning, trace, observation, blocker, context", t)
		}
		f.Type = store.EntryType(t)
	}

	if tags, ok := params["tags"].([]any); ok {
		f.Tags = make([]string, 0, len(tags))
		for _, t := range tags {
			if s, ok := t.(string); ok {
				f.Tags = append(f.Tags, strings.ToLower(strings.TrimSpace(s)))
			}
		}
		sort.Strings(f.Tags)
	}

	if src, ok := params["source"].(string); ok && src != "" {
		f.Source = src
	}

	if since, ok := params["since"].(string); ok && since != "" {
		t, err := time.Parse(time.RFC3339, since)
		if err != nil {
			return f, fmt.Errorf("invalid 'since' timestamp %q: must be RFC3339 format", since)
		}
		f.Since = t
	}

	if limit, ok := params["limit"].(int); ok && limit > 0 {
		if limit > 100 {
			limit = 100
		}
		f.Limit = limit
	}

	return f, nil
}

func isValidEntryType(t string) bool {
	switch store.EntryType(t) {
	case store.TypeDecision, store.TypeLearning, store.TypeTrace,
		store.TypeObservation, store.TypeBlocker, store.TypeContext:
		return true
	}
	return false
}
