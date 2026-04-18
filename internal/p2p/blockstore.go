// Package p2p provides a pure-Go P2P transport layer for agent-memory.
package p2p

// ErrNotImplemented is used as a placeholder until the real implementation is added.

// NewBadgerBlockstore creates a boxo blockstore backed by go-ds-badger.
func NewBadgerBlockstore(ctx interface{}, dataDir string) (interface{}, error) { // TODO: blockstore.Blockstore
	return nil, ErrNotImplemented
}
