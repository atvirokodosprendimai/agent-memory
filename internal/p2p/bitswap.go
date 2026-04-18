// Package p2p provides a pure-Go P2P transport layer for agent-memory.
package p2p

// ErrNotImplemented is used as a placeholder until the real implementation is added.

// BitSwap handles block exchange with connected peers via bitswap.
type BitSwap interface {
	// GetBlock retrieves a block by CID from any connected peer.
	// Returns err if ctx is cancelled or block not found among any connected peer.
	GetBlock(ctx interface{}, c interface{}) (interface{}, error) // TODO: cid.Cid, blocks.Block
	// NotifyNewBlocks announces newly stored blocks to connected peers.
	NotifyNewBlocks(ctx interface{}, blks ...interface{}) error // TODO: ...blocks.Block
	// GetSession returns the underlying bitswap session for advanced use.
	GetSession() interface{} // TODO: bitswap.Session
}

// NewBitSwap creates a BitSwap session wired to the given host and blockstore.
func NewBitSwap(ctx interface{}, h interface{}, bs interface{}) (BitSwap, error) {
	return nil, ErrNotImplemented
}
