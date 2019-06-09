package downloader

import (
	"../params"
	. "../../../common"
	. "../../../config"
	. "../../../util"
	"../common"
	"../core/types"
	. "../event_feed"
	"errors"
	"github.com/agoussia/godes"
	"math/big"
	"sync/atomic"
	"time"
)

var (
	MaxHashFetch    = 512 // Amount of hashes to be fetched per retrieval request
	MaxBlockFetch   = 128 // Amount of blocks to be fetched per retrieval request
	MaxHeaderFetch  = 192 // Amount of block headers to be fetched per retrieval request
	MaxSkeletonSize = 128 // Number of header fetches to need for a skeleton assembly
	MaxBodyFetch    = 128 // Amount of block bodies to be fetched per retrieval request
	MaxReceiptFetch = 256 // Amount of transaction receipts to allow fetching per request
	MaxStateFetch   = 384 // Amount of node state values to allow fetching per request

	MaxForkAncestry  = 3 * params.EpochDuration // Maximum chain reorganisation
	rttMinEstimate   = 2 * time.Second          // Minimum round-trip time to target for download requests
	rttMaxEstimate   = 20 * time.Second         // Maximum round-trip time to target for download requests
	rttMinConfidence = 0.1                      // Worse confidence factor in our estimated RTT value
	ttlScaling       = 3                        // Constant scaling factor for RTT -> TTL conversion
	ttlLimit         = time.Minute              // Maximum TTL allowance to prevent reaching crazy timeouts

	qosTuningPeers   = 5    // Number of peers to tune based on (best peers)
	qosConfidenceCap = 10   // Number of peers above which not to modify RTT confidence
	qosTuningImpact  = 0.25 // Impact that a new tuning target has on the previous value

	maxQueuedHeaders  = 32 * 1024 // [eth/62] Maximum number of headers to queue for import (DOS protection)
	maxHeadersProcess = 2048      // Number of header download results to import at once into the chain
	maxResultsProcess = 2048      // Number of content download results to import at once into the chain

	reorgProtThreshold   = 48 // Threshold number of recent blocks to disable mini reorg protection
	reorgProtHeaderDelay = 2  // Number of headers to delay delivering to cover mini reorgs

	fsHeaderCheckFrequency = 100             // Verification frequency of the downloaded headers during fast sync
	fsHeaderSafetyNet      = 2048            // Number of headers to discard in case a chain violation is detected
	fsHeaderForceVerify    = 24              // Number of headers to verify before and after the pivot to accept it
	fsHeaderContCheck      = 3 * time.Second // Time interval to check for header continuations during state download
	fsMinFullBlocks        = 64              // Number of blocks to retrieve fully even in fast sync
)

var (
	errBusy                    = errors.New("busy")
	errUnknownPeer             = errors.New("peer is unknown or unhealthy")
	errBadPeer                 = errors.New("action from bad peer ignored")
	errStallingPeer            = errors.New("peer is stalling")
	errUnsyncedPeer            = errors.New("unsynced peer")
	errNoPeers                 = errors.New("no peers to keep download active")
	errTimeout                 = errors.New("timeout")
	errEmptyHeaderSet          = errors.New("empty header set by peer")
	errPeersUnavailable        = errors.New("no peers available or all tried for download")
	errInvalidAncestor         = errors.New("retrieved ancestor is invalid")
	errInvalidChain            = errors.New("retrieved hash chain is invalid")
	errInvalidBlock            = errors.New("retrieved block is invalid")
	errInvalidBody             = errors.New("retrieved block body is invalid")
	errInvalidReceipt          = errors.New("retrieved receipt is invalid")
	errCancelBlockFetch        = errors.New("block download canceled (requested)")
	errCancelHeaderFetch       = errors.New("block header download canceled (requested)")
	errCancelBodyFetch         = errors.New("block body download canceled (requested)")
	errCancelReceiptFetch      = errors.New("receipt download canceled (requested)")
	errCancelStateFetch        = errors.New("state data download canceled (requested)")
	errCancelHeaderProcessing  = errors.New("header processing canceled (requested)")
	errCancelContentProcessing = errors.New("content processing canceled (requested)")
	errNoSyncActive            = errors.New("no sync active")
	errTooOld                  = errors.New("peer doesn't speak recent enough protocol version (need version >= 62)")
)

type peerSet interface {
	Peer(id ID) IPeer
}


// LightPeer encapsulates the methods required to synchronise with a remote light peer.
type Peer interface {
	Head() (common.Hash, *big.Int)
	GetHeadersByHash(common.Hash, int, int, bool) ([]*types.Header, error)
	GetHeadersByNumber(uint64, int, int, bool) ([]*types.Header, error)
	GetBodies(hashes []common.Hash) (map[common.Hash]*types.Body, error)
}


type Downloader struct {
	name string
	mux  *EventFeed // Event multiplexer to announce sync operation events

	peers      peerSet // Set of active peers from which download can proceed

	blockchain BlockChain

	// Callbacks
	dropPeer func(ID) // Drops a peer for misbehaving

	// Status
	synchronising   int32
	notified        int32
}

// LightChain encapsulates functions required to synchronise a light chain.
type LightChain interface {
	// HasHeader verifies a header's presence in the local chain.
	HasHeader(common.Hash, uint64) bool

	// GetHeaderByHash retrieves a header from the local chain.
	GetHeaderByHash(common.Hash) *types.Header

	// CurrentHeader retrieves the head header from the local chain.
	CurrentHeader() *types.Header

	// GetTd returns the total difficulty of a local block.
	GetTd(common.Hash, uint64) *big.Int

	// InsertHeaderChain inserts a batch of headers into the local chain.
	InsertHeaderChain([]*types.Header, int) (int, error)

	// Rollback removes a few recently added elements from the local chain.
	Rollback([]common.Hash)
}

// BlockChain encapsulates functions required to sync a (full or fast) blockchain.
type BlockChain interface {
	LightChain

	// HasBlock verifies a block's presence in the local chain.
	HasBlock(common.Hash, uint64) bool

	// HasFastBlock verifies a fast block's presence in the local chain.
	HasFastBlock(common.Hash, uint64) bool

	// GetBlockByHash retrieves a block from the local chain.
	GetBlockByHash(common.Hash) *types.Block

	// CurrentBlock retrieves the head block from the local chain.
	CurrentBlock() *types.Block

	// CurrentFastBlock retrieves the head fast block from the local chain.
	CurrentFastBlock() *types.Block

	// FastSyncCommitHead directly commits the head block to a certain entity.
	FastSyncCommitHead(common.Hash) error

	// InsertChain inserts a batch of blocks into the local chain.
	InsertChain(types.Blocks) (int, error)

	// InsertReceiptChain inserts a batch of receipts into the local chain.
	InsertReceiptChain(types.Blocks, []types.Receipts) (int, error)
}


// New creates a new downloader to fetch hashes and blocks from remote peers.
func New(name string, mux *EventFeed, peerSet peerSet, chain BlockChain, dropPeer func(ID)) *Downloader {
	dl := &Downloader{
		name: 			name,
		mux:            mux,
		peers:          peerSet,
		blockchain:     chain,
		dropPeer:       dropPeer,
	}
	return dl
}


// Synchronise tries to sync up our local block chain with a remote peer, both
// adding various sanity checks as well as wrapping it with various log entries.
func (d *Downloader) Synchronise(id ID, head common.Hash, td *big.Int) error {
	err := d.synchronise(id, head, td)
	switch err {
	case nil:
	case errBusy:

	case errTimeout, errBadPeer, errStallingPeer, errUnsyncedPeer,
		errEmptyHeaderSet, errPeersUnavailable, errTooOld,
		errInvalidAncestor, errInvalidChain:
		d.log("Synchronisation failed, dropping peer", "peer", id, "err", err)
		if d.dropPeer == nil {
			// The dropPeer method is nil when `--copydb` is used for a local copy.
			// Timeouts can occur if e.g. compaction hits at the wrong time, and can be ignored
			d.log("Downloader wants to drop peer, but peerdrop-function is not set", "peer", id)
		} else {
			d.dropPeer(id)
		}
	default:
		d.log("Synchronisation failed, retrying", "err", err)
	}
	return err
}

// synchronise will select the peer and use it for synchronising. If an empty string is given
// it will use the best peer possible and synchronize if its TD is higher than our own. If any of the
// checks fail an error will be returned. This method is synchronous
func (d *Downloader) synchronise(id ID, hash common.Hash, td *big.Int) error {
	// Make sure only one goroutine is ever allowed past this point at once
	if !atomic.CompareAndSwapInt32(&d.synchronising, 0, 1) {
		return errBusy
	}
	defer atomic.StoreInt32(&d.synchronising, 0)

	// Post a user notification of the sync (only once per session)
	if atomic.CompareAndSwapInt32(&d.notified, 0, 1) {
		d.log("Block synchronisation started")
	}
	// Retrieve the origin peer and initiate the downloading process
	p := d.peers.Peer(id)
	if p == nil {
		return errUnknownPeer
	}
	peer := p.(Peer)
	return d.syncWithPeer(peer, hash, td)
}

// syncWithPeer starts a block synchronization based on the hash chain from the
// specified peer and head hash.
func (d *Downloader) syncWithPeer(p Peer, hash common.Hash, td *big.Int) (err error) {
	d.log("Posting StartEvent")
	d.mux.Post(StartEvent{})
	defer func() {
		// reset on error
		if err != nil {
			d.log("Posting FailedEvent")
			d.mux.Post(FailedEvent{err})
		} else {
			d.log("Posting DoneEvent")
			latest := d.blockchain.CurrentHeader()
			d.mux.Post(DoneEvent{latest})
		}
	}()

	d.log("Synchronising with the network", "peer", p)
	defer func(start float64) {
		d.log("Synchronisation terminated", "elapsed", TimeSince(uint64(start)))
	}(godes.GetSystemTime())


	// Look up the sync boundaries: the common ancestor and the target block
	latest, err := d.fetchHeight(p)
	if err != nil {
		return err
	}

	localHeight := d.blockchain.CurrentBlock().NumberU64()
	height := latest.Number.Uint64()

	d.log("Mine height", localHeight, "peer height", height)

	hashes := make([]common.Hash, 0)
	headerHashes := make(map[common.Hash]*types.Header,0)

	for i := localHeight+1; i <= height ; i+=1 {
		godes.Advance(SimConfig.NextNetworkLatency())

		headers, err := p.GetHeadersByNumber(i, MaxHeaderFetch, 0, false)
		if err != nil {
			return err
		}
		d.log("Inserting headers", len(headers))

		d.blockchain.InsertHeaderChain(headers, fsHeaderCheckFrequency)
		for _, h := range headers {
			headerHashes[h.Hash()] = h
			hashes = append(hashes, h.Hash())
		}

		i += uint64(len(headers))
	}

	godes.Advance(SimConfig.NextNetworkLatency())

	results, err := p.GetBodies(hashes)
	if err != nil {
		return err
	}

	blocks := make([]*types.Block, len(results))

	for i, hash := range hashes {
		result := results[hash]
		header := headerHashes[hash]
		blocks[i] = types.NewBlockWithHeader(header).WithBody(result.Transactions, result.Uncles)
		d.log("Created block", blocks[i].NumberU64())
	}
	/*
	i := 0
	for hash, result := range results {
		header := headerHashes[hash]
		blocks[i] = types.NewBlockWithHeader(header).WithBody(result.Transactions, result.Uncles)
		d.log("Created block", blocks[i].NumberU64())
		i+=1
	}

	 */
	if index, err := d.blockchain.InsertChain(blocks); err != nil {
		if index < len(results) {
			d.log("Downloaded item processing failed", "number", blocks[index].Header().Number, "err", err)
		} else {
			// The InsertChain method in blockchain.go will sometimes return an out-of-bounds index,
			// when it needs to preprocess blocks to import a sidechain.
			// The importer will put together a new list of blocks to import, which is a superset
			// of the blocks delivered from the downloader, and the indexing will be off.
			d.log("Downloaded item processing failed on sidechain import", "index", index, "err", err)
		}
		return errInvalidChain
	}


	return nil
}


// fetchHeight retrieves the head header of the remote peer to aid in estimating
// the total time a pending synchronisation would take.
func (d *Downloader) fetchHeight(p Peer) (*types.Header, error){

	godes.Advance(SimConfig.NextNetworkLatency()*2)

	// Request the advertised remote head block and wait for the response
	head, _ := p.Head()
	headers, err := p.GetHeadersByHash(head, 1, 0, false)

	if err != nil {
		return nil, err
	}

	return headers[0], nil

}



// DeliverHeaders injects a new batch of block headers received from a remote
// node into the download schedule.
func (d *Downloader) DeliverHeaders(id ID, headers []*types.Header) (err error) {

	//return d.deliver(id, d.headerCh, &headerPack{id, headers}, headerInMeter, headerDropMeter)
	return nil
}



func (d *Downloader) log(a ...interface{})  {
	if LogConfig.LogDownload {
		Print("Downloader", d.name, a)
	}
}

