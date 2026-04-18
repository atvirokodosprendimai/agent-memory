package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"

	"github.com/atvirokodosprendimai/agent-memory/internal/config"
)

var interrupted atomic.Bool

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	// Set up signal handling for graceful drain
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		interrupted.Store(true)
	}()

	cfg, err := config.Load()
	if err != nil {
		// init doesn't need config — everything else does
		if os.Args[1] != "init" {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\nRun 'agent-memory init' first.\n", err)
			os.Exit(1)
		}
	}

	switch os.Args[1] {
	case "init":
		if err := runInit(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "write":
		if err := runWrite(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "read":
		if err := runRead(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "list":
		if err := runList(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "pins":
		if err := runPins(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "gc":
		if err := runGC(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "export":
		if err := runExport(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "import":
		if err := runImport(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "version":
		fmt.Println("agent-memory v0.1.0-dev")
	case "skill":
		if err := runSkill(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}

	if interrupted.Load() {
		fmt.Fprintln(os.Stderr, "\nInterrupted — exiting gracefully.")
	}
}

func printUsage() {
	fmt.Println(`agent-memory — E2E encrypted agent memory on IPFS

Usage:
  agent-memory init              Initialize (create keys, test IPFS)
  agent-memory write             Write a memory entry
  agent-memory read              Read and decrypt memory entries
  agent-memory list              List entry metadata (no decryption)
  agent-memory pins              List pinned CIDs
  agent-memory gc                Remove old entries (unpin + rebuild index)
  agent-memory export            Export entries as JSONL
  agent-memory import            Import entries from JSONL
  agent-memory skill <cmd>       Skill commands (load|unload|tool)
  agent-memory version           Print version

Common flags:
  --secret string    Agent secret (or set AGENT_MEMORY_SECRET)
  --tag strings      Comma-separated tags
  --type string      Entry type (decision|learning|trace|observation|blocker|context)
  --source string    Agent source (goose|copilot|claude-code|observation-loop|human)
  --content string   Entry content (or pipe to stdin)
  --since date       Filter entries since date (YYYY-MM-DD)
  --limit int        Max entries to return (default 10)
  --raw              Print full JSON (read only)
  --max-age duration Max entry age for gc (e.g., 30d, 720h)
  --output file      Output file for export
  --input file       Input file for import`)
}

func runInit() error {
	if interrupted.Load() {
		return fmt.Errorf("interrupted")
	}
	secret := getSecret()
	if secret == "" {
		return fmt.Errorf("secret required: use --secret or AGENT_MEMORY_SECRET env var")
	}

	// Test IPFS daemon
	ipfsAddr := getIPFSAddr()
	if err := testIPFSConnection(ipfsAddr); err != nil {
		return fmt.Errorf("IPFS daemon not reachable at %s: %w\nStart with: ipfs daemon", ipfsAddr, err)
	}
	fmt.Printf("IPFS daemon connected at %s\n", ipfsAddr)

	// Create config (P2P disabled by default for backward compatibility)
	cfg, err := config.Create(secret, ipfsAddr, false, "")
	if err != nil {
		return fmt.Errorf("creating config: %w", err)
	}

	// Write config
	configDir := config.Dir()
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	configPath := filepath.Join(configDir, "config.json")
	if err := cfg.Save(configPath); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("Config written to %s\n", configPath)
	fmt.Println("agent-memory initialized successfully.")
	fmt.Println("\nNext steps:")
	fmt.Println("  agent-memory write --type decision --tag billing --content 'First memory entry'")
	return nil
}
