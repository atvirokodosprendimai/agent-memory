package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/atvirokodosprendimai/agent-memory/internal/config"
	"github.com/atvirokodosprendimai/agent-memory/internal/ipfs"
	"github.com/atvirokodosprendimai/agent-memory/internal/store"
)

// getSecret returns the agent secret from --secret flag, AGENT_MEMORY_SECRET env, or stdin prompt.
func getSecret() string {
	// Check env first
	if s := os.Getenv("AGENT_MEMORY_SECRET"); s != "" {
		return s
	}
	// TODO: prompt interactively if terminal
	return ""
}

// getIPFSAddr returns the IPFS daemon address.
func getIPFSAddr() string {
	if addr := os.Getenv("AGENT_MEMORY_IPFS_ADDR"); addr != "" {
		return addr
	}
	return "http://localhost:5001"
}

// testIPFSConnection pings the IPFS daemon.
func testIPFSConnection(addr string) error {
	client := ipfs.NewClient(addr)
	defer client.Close()
	return client.Ping()
}

// parseWriteFlags parses flags for the write command.
func parseWriteFlags(args []string) (entryType, source, content, tagsStr string) {
	fs := flag.NewFlagSet("write", flag.ExitOnError)
	fs.StringVar(&entryType, "type", "", "Entry type")
	fs.StringVar(&entryType, "t", "", "Entry type (shorthand)")
	fs.StringVar(&source, "source", "human", "Agent source")
	fs.StringVar(&source, "s", "human", "Agent source (shorthand)")
	fs.StringVar(&content, "content", "", "Entry content")
	fs.StringVar(&content, "c", "", "Entry content (shorthand)")
	fs.StringVar(&tagsStr, "tag", "", "Comma-separated tags")
	fs.Parse(args)
	return
}

// parseReadFlags parses flags for the read command.
func parseReadFlags(args []string) (entryType, source, tagsStr, sinceStr string, limit int) {
	fs := flag.NewFlagSet("read", flag.ExitOnError)
	fs.StringVar(&entryType, "type", "", "Filter by entry type")
	fs.StringVar(&source, "source", "", "Filter by agent source")
	fs.StringVar(&tagsStr, "tag", "", "Filter by comma-separated tags")
	fs.StringVar(&sinceStr, "since", "", "Filter entries since date (YYYY-MM-DD)")
	fs.IntVar(&limit, "limit", 10, "Max entries to return")
	fs.Parse(args)
	return
}

func runWrite(cfg *config.Config) error {
	secret := getSecret()
	if secret == "" {
		return fmt.Errorf("secret required: set AGENT_MEMORY_SECRET or use --secret")
	}

	entryType, source, content, tagsStr := parseWriteFlags(os.Args[2:])

	// Validate type
	if entryType == "" {
		return fmt.Errorf("--type is required (decision|learning|trace|observation|blocker|context)")
	}
	validTypes := map[string]bool{
		"decision": true, "learning": true, "trace": true,
		"observation": true, "blocker": true, "context": true,
	}
	if !validTypes[entryType] {
		return fmt.Errorf("invalid type %q: must be one of decision, learning, trace, observation, blocker, context", entryType)
	}

	// Read content from stdin if not provided via flag
	if content == "" {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("reading stdin: %w", err)
			}
			content = strings.TrimSpace(string(data))
		}
	}
	if content == "" {
		return fmt.Errorf("--content is required (or pipe to stdin)")
	}

	// Parse tags
	var tags []string
	if tagsStr != "" {
		tags = strings.Split(tagsStr, ",")
	}

	// Create store and write entry
	s, err := store.New(cfg, secret)
	if err != nil {
		return err
	}
	defer s.Close()

	entry, err := s.Write(store.EntryType(entryType), source, tags, content, nil)
	if err != nil {
		return err
	}

	fmt.Printf("Written: %s (type=%s, tags=%v)\n", entry.ID[:16]+"...", entryType, tags)
	return nil
}

func runRead(cfg *config.Config) error {
	secret := getSecret()
	if secret == "" {
		return fmt.Errorf("secret required: set AGENT_MEMORY_SECRET or use --secret")
	}

	entryType, source, tagsStr, sinceStr, limit := parseReadFlags(os.Args[2:])

	s, err := store.New(cfg, secret)
	if err != nil {
		return err
	}
	defer s.Close()

	// Build filter
	filter := store.Filter{
		Type:   store.EntryType(entryType),
		Source: source,
		Limit:  limit,
	}

	if tagsStr != "" {
		filter.Tags = strings.Split(tagsStr, ",")
	}

	if sinceStr != "" {
		t, err := time.Parse("2006-01-02", sinceStr)
		if err != nil {
			return fmt.Errorf("invalid --since date: %w", err)
		}
		filter.Since = t
	}

	entries, err := s.Read(filter)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		fmt.Println("No entries found.")
		return nil
	}

	for _, e := range entries {
		fmt.Printf("─── %s ───\n", e.Timestamp)
		fmt.Printf("Type: %s | Source: %s | Tags: %v\n", e.Type, e.Source, e.Tags)
		fmt.Printf("\n%s\n\n", e.Content)
	}

	return nil
}

func runList(cfg *config.Config) error {
	secret := getSecret()
	if secret == "" {
		return fmt.Errorf("secret required: set AGENT_MEMORY_SECRET or use --secret")
	}

	entryType, source, tagsStr, sinceStr, limit := parseReadFlags(os.Args[2:])

	s, err := store.New(cfg, secret)
	if err != nil {
		return err
	}
	defer s.Close()

	filter := store.Filter{
		Type:   store.EntryType(entryType),
		Source: source,
		Limit:  limit,
	}
	if tagsStr != "" {
		filter.Tags = strings.Split(tagsStr, ",")
	}
	if sinceStr != "" {
		t, err := time.Parse("2006-01-02", sinceStr)
		if err != nil {
			return fmt.Errorf("invalid --since date: %w", err)
		}
		filter.Since = t
	}

	entries, err := s.List(filter)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		fmt.Println("No entries found.")
		return nil
	}

	// JSON output for machine readability
	out, _ := json.MarshalIndent(entries, "", "  ")
	fmt.Println(string(out))

	return nil
}

func runPins(cfg *config.Config) error {
	client := ipfs.NewClient(cfg.IPFSAddr)
	defer client.Close()

	pins, err := client.PinLs()
	if err != nil {
		return err
	}

	if len(pins) == 0 {
		fmt.Println("No pins found.")
		return nil
	}

	fmt.Printf("%d pinned objects:\n", len(pins))
	for cid := range pins {
		fmt.Printf("  %s\n", cid)
	}

	return nil
}
