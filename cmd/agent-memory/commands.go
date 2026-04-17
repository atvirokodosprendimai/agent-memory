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

const maxStdinSize = 10 * 1024 * 1024 // 10MB limit for stdin content

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
func parseReadFlags(args []string) (entryType, source, tagsStr, sinceStr string, limit int, raw bool) {
	fs := flag.NewFlagSet("read", flag.ExitOnError)
	fs.StringVar(&entryType, "type", "", "Filter by entry type")
	fs.StringVar(&source, "source", "", "Filter by agent source")
	fs.StringVar(&tagsStr, "tag", "", "Filter by comma-separated tags")
	fs.StringVar(&sinceStr, "since", "", "Filter entries since date (YYYY-MM-DD)")
	fs.IntVar(&limit, "limit", 10, "Max entries to return")
	fs.BoolVar(&raw, "raw", false, "Print full JSON including metadata")
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
			limited := io.LimitReader(os.Stdin, maxStdinSize)
			data, err := io.ReadAll(limited)
			if err != nil {
				return fmt.Errorf("reading stdin: %w", err)
			}
			// Check if we hit the limit
			if len(data) >= maxStdinSize {
				return fmt.Errorf("content exceeds maximum size of 10MB")
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

	entryType, source, tagsStr, sinceStr, limit, raw := parseReadFlags(os.Args[2:])

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

	if raw {
		out, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling entries: %w", err)
		}
		fmt.Println(string(out))
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

	entryType, source, tagsStr, sinceStr, limit, _ := parseReadFlags(os.Args[2:])

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

// parseGCFlags parses flags for the gc command.
func parseGCFlags(args []string) (maxAge string) {
	fs := flag.NewFlagSet("gc", flag.ExitOnError)
	fs.StringVar(&maxAge, "max-age", "", "Max entry age (e.g., 30d, 24h, 720h)")
	fs.Parse(args)
	return
}

// parseDuration parses a duration string that supports 'd' suffix for days
// in addition to standard Go duration suffixes (h, m, s).
func parseDuration(s string) (time.Duration, error) {
	if len(s) > 1 && s[len(s)-1] == 'd' {
		// Parse days
		days := s[:len(s)-1]
		var n int
		if _, err := fmt.Sscanf(days, "%d", &n); err != nil {
			return 0, fmt.Errorf("invalid duration %q: %w", s, err)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

func runGC(cfg *config.Config) error {
	secret := getSecret()
	if secret == "" {
		return fmt.Errorf("secret required: set AGENT_MEMORY_SECRET or use --secret")
	}

	maxAgeStr := parseGCFlags(os.Args[2:])
	if maxAgeStr == "" {
		return fmt.Errorf("--max-age is required (e.g., 30d, 720h)")
	}

	maxAge, err := parseDuration(maxAgeStr)
	if err != nil {
		return fmt.Errorf("invalid --max-age: %w", err)
	}

	s, err := store.New(cfg, secret)
	if err != nil {
		return err
	}
	defer s.Close()

	removed, err := s.GC(maxAge)
	if err != nil {
		return err
	}

	if removed == 0 {
		fmt.Println("No entries older than", maxAgeStr, "found.")
	} else {
		fmt.Printf("Removed %d entries older than %s.\n", removed, maxAgeStr)
	}

	return nil
}

// parseExportFlags parses flags for the export command.
func parseExportFlags(args []string) (entryType, tagsStr, output string) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	fs.StringVar(&entryType, "type", "", "Filter by entry type")
	fs.StringVar(&tagsStr, "tag", "", "Filter by comma-separated tags")
	fs.StringVar(&output, "output", "", "Output file path (JSONL)")
	fs.StringVar(&output, "o", "", "Output file path (shorthand)")
	fs.Parse(args)
	return
}

func runExport(cfg *config.Config) error {
	secret := getSecret()
	if secret == "" {
		return fmt.Errorf("secret required: set AGENT_MEMORY_SECRET or use --secret")
	}

	entryType, tagsStr, output := parseExportFlags(os.Args[2:])
	if output == "" {
		return fmt.Errorf("--output is required")
	}

	s, err := store.New(cfg, secret)
	if err != nil {
		return err
	}
	defer s.Close()

	filter := store.Filter{
		Type: store.EntryType(entryType),
	}
	if tagsStr != "" {
		filter.Tags = strings.Split(tagsStr, ",")
	}

	entries, err := s.Export(filter)
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		fmt.Println("No entries found to export.")
		return nil
	}

	f, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("creating output file: %w", err)
	}
	defer f.Close()

	for _, e := range entries {
		line, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("marshaling entry: %w", err)
		}
		f.Write(line)
		f.Write([]byte("\n"))
	}

	fmt.Printf("Exported %d entries to %s\n", len(entries), output)
	return nil
}

// parseImportFlags parses flags for the import command.
func parseImportFlags(args []string) (input, source string) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	fs.StringVar(&input, "input", "", "Input file path (JSONL)")
	fs.StringVar(&input, "i", "", "Input file path (shorthand)")
	fs.StringVar(&source, "source", "", "Override source field on imported entries")
	fs.Parse(args)
	return
}

func runImport(cfg *config.Config) error {
	secret := getSecret()
	if secret == "" {
		return fmt.Errorf("secret required: set AGENT_MEMORY_SECRET or use --secret")
	}

	input, sourceOverride := parseImportFlags(os.Args[2:])
	if input == "" {
		return fmt.Errorf("--input is required")
	}

	data, err := os.ReadFile(input)
	if err != nil {
		return fmt.Errorf("reading input file: %w", err)
	}

	var entries []*store.Entry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var entry store.Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return fmt.Errorf("parsing JSONL line: %w", err)
		}
		if sourceOverride != "" {
			entry.Source = sourceOverride
		}
		entries = append(entries, &entry)
	}

	if len(entries) == 0 {
		fmt.Println("No entries found in input file.")
		return nil
	}

	s, err := store.New(cfg, secret)
	if err != nil {
		return err
	}
	defer s.Close()

	imported, err := s.Import(entries)
	if err != nil {
		return fmt.Errorf("import failed after %d entries: %w", imported, err)
	}

	fmt.Printf("Imported %d entries from %s\n", imported, input)
	return nil
}
