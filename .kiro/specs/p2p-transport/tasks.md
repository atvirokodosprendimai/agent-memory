# Tasks Document

## 1. Foundation: Environment and Dependency Setup

- [x] 1.1 Add P2P transport dependencies to go.mod
  - Add `github.com/libp2p/go-libp2p v0.48.0`
  - Add `github.com/libp2p/go-libp2p-kad-dht v0.39.0`
  - Add `github.com/ipfs/go-libipfs/bitswap v0.7.0`
  - Add `github.com/ipfs/boxo v0.38.0`
  - Add `github.com/ipfs/go-ds-badger v0.3.4`
  - Add `github.com/ipfs/go-libipfs/bitswap v0.7.0` network package
  - Add `github.com/crelper/crypto v1.0.0` or use `golang.org/x/crypto` (already present) for HKDF
  - Run `go mod tidy` and verify all dependencies resolve as pure Go
  - Observable completion: `go build ./...` succeeds with CGO_ENABLED=0
  - _Requirements: 9.1_
  - _Boundary: go.mod_

- [x] 1.2 Create internal/p2p/ directory and package scaffold
  - Create `internal/p2p/` directory with `// Package p2p provides a pure-Go P2P transport layer`
  - Create placeholder files: `host.go`, `discovery.go`, `bitswap.go`, `blockstore.go`, `p2pclient.go`
  - Each file has package declaration and blank main function that returns nil or ErrNotImplemented
  - Observable completion: `go build ./internal/p2p/` compiles without errors
  - _Requirements: 9.1_
  - _Boundary: internal/p2p/_

---

## 2. Core: P2P Transport Components

- [x] 2.1 (P) Implement host.go — libp2p host lifecycle and peer identity
  - Derive peer ID from shared secret using HKDF-SHA256 (salt="agent-memory-peer-id", info="v1")
  - Create libp2p host with: no listen addrs (works from NAT), EnableAutoRelayWithStaticRelays pointing to public bootstrap relays, EnableRelay()
  - Store libp2p host state in dataDir/p2p/ for persistence across restarts
  - Implement Host interface: Host() host.Host, DHT() *dht.IpfsDHT, PeerID() peer.ID, Close() error
  - NewHost(ctx, secret, dataDir) returns (Host, error)
  - Observable completion: NewHost produces a running host; same secret produces same peer ID; `go build ./internal/p2p/` succeeds
  - _Requirements: 1.1, 1.2, 4.1, 10.1, 11.1, 13.1_
  - _Boundary: internal/p2p/host.go_

- [x] 2.2 (P) Implement discovery.go — DHT-based peer discovery by shared secret
  - Derive DHT key from secret: SHA256 hash of secret, truncated to first 16 bytes, hex-encoded with prefix "/agent-memory/discovery/"
  - Implement Discovery interface: Advertise(ctx), FindPeers(ctx) ([]peer.AddrInfo, error), ConnectToPeers(ctx) error
  - Advertise: PutValue to DHT with peer AddrInfo under the discovery key; re-advertise every 30 minutes and on root CID changes
  - FindPeers: GetValue from DHT to retrieve all advertised peer AddrInfos
  - ConnectToPeers: FindPeers then dial each peer using relay addresses
  - Observable completion: Two hosts with same secret can find each other via mocked DHT; different secrets produce different DHT keys
  - _Requirements: 2.1, 2.2, 3.1, 3.2_
  - _Boundary: internal/p2p/discovery.go_

- [x] 2.3 (P) Implement blockstore.go — boxo blockstore backed by BadgerDS
  - Create BadgerDS datastore at dataDir/p2p/badgerds/ using go-ds-badger
  - Wrap with boxo/blockstore.NewBlockstore(ds, blockstore.NoPrefix()) to get Blockstore interface
  - Implement blockstore.NewBadgerBlockstore(ctx, dataDir string) (blockstore.Blockstore, error)
  - Verify CID format matches Kubo output (sha2-256, raw multicodec) with a roundtrip test
  - Observable completion: Put + Get roundtrip returns identical bytes; AllKeysChan lists all stored CIDs
  - _Requirements: 6.1, 6.2, 9.1, 12.1_
  - _Boundary: internal/p2p/blockstore.go_

- [x] 2.4 (P) Implement bitswap.go — bitswap session for block exchange
  - Create bitswap session using go-libipfs/bitswap: bitswap.New(ctx, network, blockstore)
  - network = bsnet.NewFromIpfsHost(host, dht routing)
  - Implement BitSwap interface: GetBlock(ctx, cid) (blocks.Block, error), NotifyNewBlocks(ctx, ...blocks.Block) error, GetSession() bitswap.Session
  - GetBlock: check blockstore first (fast path); if miss, call session.GetBlock (queries connected peers)
  - NotifyNewBlocks: call session.NotifyNewBlocks (broadcasts to peers via want-have)
  - Observable completion: local block returns from blockstore without peer interaction; non-local CID triggers bitswap peer query
  - _Requirements: 5.1, 5.2, 5.3, 12.1_
  - _Boundary: internal/p2p/bitswap.go_

- [x] 2.5 (P) Implement p2pclient.go — P2PClient implementing StorageClient interface
  - Define StorageClient interface: Add(data), Get(cid), PinLs(), PinRm(cid), Close(), Ping(), ID()
  - P2PClient struct: holds host Host, discovery Discovery, bitswap BitSwap, store blockstore.Blockstore
  - NewP2PClient(ctx, secret, dataDir): creates host, discovery, bitswap, blockstore; calls discovery.Advertise; calls discovery.ConnectToPeers
  - Add(data): put block to blockstore, call bitswap.NotifyNewBlocks, trigger DHT re-advertise
  - Get(cid): check blockstore; if miss, call bitswap.GetBlock
  - PinLs(): call blockstore.AllKeysChan, return as map[string]bool
  - PinRm(cid): call blockstore.DeleteBlock
  - Ping(): check host is running (host.Host().ID() succeeds)
  - ID(): return host.PeerID().String()
  - Observable completion: All StorageClient methods return correct types and values; `go build ./internal/p2p/` succeeds
  - _Requirements: 1.1, 2.1, 3.1, 5.1, 5.2, 5.3, 7.1, 7.2, 13.1_
  - _Boundary: internal/p2p/p2pclient.go_

---

## 3. Integration: Store Backend Selection and Wiring

- [x] 3.1 Extract StorageClient interface in store.go; replace concrete *ipfs.Client
  - Define StorageClient interface with all methods matching ipfs.Client: Add, Get, PinLs, PinRm, Close, Ping, ID
  - Change Store.ipfs field from *ipfs.Client to StorageClient
  - Change Store.New: when P2PEnabled=true call p2pclient.NewP2PClient; else call ipfs.NewClient
  - All existing store methods (Write, Read, List, GC, Import, Export) continue using s.client (StorageClient) — no other changes needed
  - Store.IPFSClient() method removed or returns nil when P2P backend is used
  - Observable completion: Store with P2PEnabled=false compiles and all existing tests pass; Store with P2PEnabled=true compiles
  - _Requirements: 7.1, 7.2, 8.1, 8.2, 8.3_
  - _Boundary: internal/store/store.go_
  - _Depends: 2.1, 2.2, 2.3, 2.4, 2.5_

- [x] 3.2 Add P2PEnabled and DataDir fields to config.go
  - Add P2PEnabled bool field to Config struct (default false for backward compatibility)
  - Add DataDir string field to Config struct (default "$HOME/.agent-memory/p2p")
  - Update config loading to read P2P_ENABLED env var and AGENT_MEMORY_DATA_DIR
  - Keep IPFSAddr field for Kubo mode
  - Observable completion: cfg.P2PEnabled and cfg.DataDir are accessible; go build succeeds
  - _Requirements: 14.1_
  - _Boundary: internal/config/config.go_

- [ ] 3.3 Wire P2PClient into store.New() backend selection path
  - In store.New: when cfg.P2PEnabled, create p2pclient.NewP2PClient(ctx, secret, cfg.DataDir) and assign to client field
  - Handle context: store.New receives context as first argument (add if not present)
  - P2PClient errors during creation propagate as wrapped errors from store.New
  - Observable completion: store.New with P2PEnabled=true returns a Store backed by P2PClient; Store.Close calls P2PClient.Close
  - _Requirements: 1.1, 1.2, 8.1, 8.2_
  - _Boundary: internal/store/store.go, internal/p2p/p2pclient.go_
  - _Depends: 3.1, 3.2_

---

## 4. Validation: Testing

- [x] 4.1 Unit tests for host.go (committed in 2.1: 7 tests in host_test.go)
  - Test HKDF derivation: same secret → same peer ID; different secret → different peer ID
  - Test NewHost: valid inputs produce running host; invalid context produces error
  - Test host.Close: subsequent calls return error or are idempotent
  - Observable completion: `go test ./internal/p2p/ -run TestHost` passes
  - _Requirements: 1.1, 13.1_
  - _Boundary: internal/p2p/host.go_

- [x] 4.2 Unit tests for discovery.go (committed in 2.2: 10 tests in discovery_test.go)
  - Test DHT key derivation is deterministic and secret-specific
  - Test FindPeers returns advertised peers (mock DHT)
  - Test Advertise writes correct AddrInfo to DHT
  - Observable completion: `go test ./internal/p2p/ -run TestDiscovery` passes
  - _Requirements: 2.1, 2.2, 3.1_
  - _Boundary: internal/p2p/discovery.go_

- [x] 4.3 Unit tests for p2pclient.go with mocked sub-components (committed in 2.5: 14 tests in p2pclient_test.go)
  - Mock Host, Discovery, BitSwap, Blockstore interfaces
  - Test Add calls blockstore.Put and bitswap.NotifyNewBlocks
  - Test Get checks blockstore first, then calls bitswap.GetBlock on miss
  - Test PinLs returns all CIDs from blockstore
  - Test Ping and ID delegate to host
  - Observable completion: `go test ./internal/p2p/ -run TestP2PClient` passes with mocks
  - _Requirements: 5.1, 5.2, 7.1, 7.2_
  - _Boundary: internal/p2p/p2pclient.go_

- [x] 4.4 Integration test — two P2PClients with same secret discover and exchange blocks
  - Create temp directory with two BadgerDS instances
  - Create two P2PClients with same secret, different data dirs
  - Verify: Add on client A produces CID; Get on client B retrieves same CID via bitswap
  - Use a shared DHT bootstrap for discovery (or mock DHT for isolated testing)
  - Observable completion: Two P2PClients can exchange a block; `go test ./internal/p2p/ -run TestP2PIntegration -v` passes
  - _Requirements: 2.1, 2.2, 3.1, 3.2, 5.1, 5.2, 5.3, 7.1_
  - _Boundary: internal/p2p/_

- [x] 4.5 CID interoperability test
  - Compare CID produced by Add(data) on P2PClient vs KuboClient for same input bytes
  - Both must produce byte-for-byte identical CID (sha2-256, raw multicodec)
  - Observable completion: CID roundtrip test passes; `go test ./internal/p2p/ -run TestCIDInterop` passes
  - _Requirements: 12.1_
  - _Boundary: internal/p2p/_

- [ ] 4.6 Full test suite regression
  - Run `go test ./...` with CGO_ENABLED=0
  - Verify no existing tests break (store tests with mock client, crypto tests, config tests)
  - Observable completion: All tests pass; `go vet ./...` produces no errors
  - _Requirements: 7.1, 8.1, 8.2, 8.3_
  - _Boundary: all packages_
  - _Depends: 4.1, 4.2, 4.3, 4.4, 4.5_
