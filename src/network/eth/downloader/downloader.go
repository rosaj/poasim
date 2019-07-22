package downloader

import (
	. "../../../common"
	. "../../../config"
	"../../../metrics"
	"../../../network/message"
	. "../../../util"
	"../common"
	"../core/types"
	. "../event_feed"
	"../params"
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

var	(

	SyncDiff				   = metrics.SyncDiff

)

type peerSet interface {
	Peer(id ID) IPeer
}


// LightPeer encapsulates the methods required to synchronise with a remote light peer.
type Peer interface {
	GetBlock(number uint64) *types.Block
	GetHeight() uint64
	Head() (common.Hash, *big.Int)
	GetHeadersByHash(common.Hash, int, int, bool) ([]*types.Header, error)
	GetHeadersByNumber(uint64, int, int, bool) ([]*types.Header, error)
	GetBodies(hashes []common.Hash) (map[common.Hash]*types.Body, error)
}


type Downloader struct {
	IMetricCollector

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
func New(name string, metricsCollector IMetricCollector, mux *EventFeed, peerSet peerSet, chain BlockChain, dropPeer func(ID)) *Downloader {
	dl := &Downloader{
		IMetricCollector: metricsCollector,
		name: 			  name,
		mux:              mux,
		peers:            peerSet,
		blockchain:       chain,
		dropPeer:         dropPeer,
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

	origin, err := d.findAncestor(p, height, localHeight)
	if err != nil {
		return err
	}

	if diff := height - origin; diff > 0 {
		realHeight := p.GetHeight()

		same := d.blockchain.CurrentBlock().Hash() == p.GetBlock(localHeight).Hash()
		realOrigin, _ := d.findAncestor(p, realHeight, localHeight)

		d.log(d.name	, "local", localHeight, "remote", height, "origin", origin, "remote peer", p, "with real height", realHeight, "real origin", realOrigin, "same", same, "diff", diff)
		d.Set(SyncDiff, int(diff))
	}

	localHeight = origin

	d.log("Synchronising with the network", "peer", p)

	hashes := make([]common.Hash, 0)
	headerHashes := make(map[common.Hash]*types.Header,0)

	for i := localHeight+1; i <= height ; i+=1 {
		godes.Advance(SimConfig.NextNetworkLatency())

		headers, err := p.GetHeadersByNumber(i, MaxHeaderFetch, 0, false)
		if err != nil {
			return err
		}

		godes.Advance(message.CalculateSizeLatency(CalcHeadersSize(headers)))

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

	godes.Advance(message.CalculateSizeLatency(CalcBodysSize(results)))

	blocks := make([]*types.Block, len(results))

	for i, hash := range hashes {
		result := results[hash]
		header := headerHashes[hash]
		if result == nil {
			return errInvalidBody
		}
		blocks[i] = types.NewBlockWithHeader(header).WithBody(result.Transactions, result.Uncles)
	//	d.log("Created block", blocks[i].NumberU64())
	}

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

func CalcHeadersSize(headers []*types.Header) common.StorageSize {
	size := common.StorageSize(0)
	for _, header := range headers {
		size += header.Size()
	}

	return size
}

func CalcBodysSize(bodys map[common.Hash]*types.Body) common.StorageSize  {
	size := common.StorageSize(0)

	for _, body := range bodys {
		size += body.Size()
	}

	return size
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

// findAncestor tries to locate the common ancestor link of the local chain and
// a remote peers blockchain. In the general case when our node was in sync and
// on the correct chain, checking the top N links should already get us a match.
// In the rare scenario when we ended up on a long reorganisation (i.e. none of
// the head links match), we do a binary search to find the common ancestor.
func (d *Downloader) findAncestor(p Peer, remoteHeaderHeight uint64, localHeight uint64)(uint64, error) {

	from, count, skip, max := calculateRequestSpan(remoteHeaderHeight, localHeight)

	godes.Advance(SimConfig.NextNetworkLatency())

	headers, err := p.GetHeadersByNumber(uint64(from), count, skip, false)

	if err != nil {
		return 0, err
	}

	godes.Advance(message.CalculateSizeLatency(CalcHeadersSize(headers)))

	floor := int64(-1)
	maxForkAncestry := uint64(90000)
	// Recap floor value for binary search
	if localHeight >= maxForkAncestry {
		// We're above the max reorg threshold, find the earliest fork point
		floor = int64(localHeight - maxForkAncestry)
	}

	// Wait for the remote response to the head fetch
	number, hash := uint64(0), common.Hash{}


	if len(headers) == 0 {
		return 0, errEmptyHeaderSet
	}
	// Make sure the peer's reply conforms to the request
	for i, header := range headers {
		expectNumber := from + int64(i)*int64(skip+1)
		if number := header.Number.Int64(); number != expectNumber {
			return 0, errInvalidChain
		}
	}
	// Check if a common ancestor was found
	for i := len(headers) - 1; i >= 0; i-- {
		// Skip any headers that underflow/overflow our requested set
		if headers[i].Number.Int64() < from || headers[i].Number.Uint64() > max {
			continue
		}
		// Otherwise check if we already know the header or not
		h := headers[i].Hash()
		n := headers[i].Number.Uint64()

		known := d.blockchain.HasBlock(h, n)
		if known {
			number, hash = n, h
			break
		}
	}

	// If the head fetch already found an ancestor, return
	if hash != (common.Hash{}) {
		if int64(number) <= floor {
			d.log("Ancestor below allowance", "number", number, "hash", hash, "allowance", floor)
			return 0, errInvalidAncestor
		}
		d.log("Found common ancestor", "number", number, "hash", hash)
		return number, nil
	}

	return 0, nil

}


// calculateRequestSpan calculates what headers to request from a peer when trying to determine the
// common ancestor.
// It returns parameters to be used for peer.RequestHeadersByNumber:
//  from - starting block number
//  count - number of headers to request
//  skip - number of headers to skip
// and also returns 'max', the last block which is expected to be returned by the remote peers,
// given the (from,count,skip)
func calculateRequestSpan(remoteHeight, localHeight uint64) (int64, int, int, uint64) {
	var (
		from     int
		count    int
		MaxCount = MaxHeaderFetch / 16
	)
	// requestHead is the highest block that we will ask for. If requestHead is not offset,
	// the highest block that we will get is 16 blocks back from head, which means we
	// will fetch 14 or 15 blocks unnecessarily in the case the height difference
	// between us and the peer is 1-2 blocks, which is most common
	requestHead := int(remoteHeight) - 1
	if requestHead < 0 {
		requestHead = 0
	}
	// requestBottom is the lowest block we want included in the query
	// Ideally, we want to include just below own head
	requestBottom := int(localHeight - 1)
	if requestBottom < 0 {
		requestBottom = 0
	}
	totalSpan := requestHead - requestBottom
	span := 1 + totalSpan/MaxCount
	if span < 2 {
		span = 2
	}
	if span > 16 {
		span = 16
	}

	count = 1 + totalSpan/span
	if count > MaxCount {
		count = MaxCount
	}
	if count < 2 {
		count = 2
	}
	from = requestHead - (count-1)*span
	if from < 0 {
		from = 0
	}
	max := from + (count-1)*span
	return int64(from), count, span - 1, uint64(max)
}




// DeliverHeaders injects a new batch of block headers received from a remote
// node into the download schedule.
func (d *Downloader) DeliverHeaders(id ID, headers []*types.Header) (err error) {

	//return d.deliver(id, d.headerCh, &headerPack{id, headers}, headerInMeter, headerDropMeter)
	return nil
}



func (d *Downloader) log(a ...interface{})  {
	if LogConfig.LogDownload {
		Log("Downloader", d.name, a)
	}
}

