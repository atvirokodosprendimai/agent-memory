package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/atvirokodosprendimai/agent-memory/internal/config"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

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
	case "version":
		fmt.Println("agent-memory v0.1.0-dev")
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
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
  agent-memory version           Print version

Common flags:
  --secret string    Agent secret (or set AGENT_MEMORY_SECRET)
  --tag strings      Comma-separated tags
  --type string      Entry type (decision|learning|trace|observation|blocker|context)
  --source string    Agent source (goose|copilot|claude-code|observation-loop|human)
  --content string   Entry content (or pipe to stdin)
  --since date       Filter entries since date (YYYY-MM-DD)
  --limit int        Max entries to return (default 10)`)
}

func runInit() error {
	secret := getSecret()
	if secret == "" {
		return fmt.Errorf("secret required: use --secret or AGENT_MEMORY_SECRET env var")
	}

	// Check if ipfs is available
	ipfsPath, err := exec.LookPath("ipfs")
	if err != nil {
		return fmt.Errorf("ipfs not found in PATH — install Kubo (go-ipfs): https://docs.ipfs.tech/install/command-line/")
	}
	fmt.Printf("Found ipfs at %s\n", ipfsPath)

	// Test IPFS daemon
	ipfsAddr := getIPFSAddr()
	if err := testIPFSConnection(ipfsAddr); err != nil {
		return fmt.Errorf("IPFS daemon not reachable at %s: %w\nStart with: ipfs daemon", ipfsAddr, err)
	}
	fmt.Printf("IPFS daemon connected at %s\n", ipfsAddr)

	// Create config
	cfg, err := config.Create(secret, ipfsAddr)
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
