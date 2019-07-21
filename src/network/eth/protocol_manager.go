package eth

import (
	. "../../common"
	. "../../config"
	"../../metrics"
	. "../../util"
	"../eth/common"
	"../eth/core"
	"../eth/core/types"
	"../eth/downloader"
	. "../eth/event_feed"
	"../eth/fetcher"
	"../eth/params"
	. "../message"
	. "../protocol"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sync/atomic"
	"time"
)

const (
	softResponseLimit = 2 * 1024 * 1024 // Target maximum size of returned blocks, headers or node data.
	estHeaderRlpSize  = 500             // Approximate size of an RLP encoded block header

	// txChanSize is the size of channel listening to NewTxsEvent.
	// The number is referenced from the size of tx pool.
	txChanSize = 4096

	// minimim number of peers to broadcast new blocks to
	minBroadcastPeers = 4
)

var (
	syncChallengeTimeout = 15 * time.Second // Time allowance for a node to reply to the sync progress challenge
	EthPeers 					= metrics.EthPeers
	MinedBlock 					= metrics.MinedBlock
	TxsPerBlock					= metrics.TxsPerBlock

)


// errIncompatibleConfig is returned if the requested protocols and configs are
// not compatible (low protocol version restrictions and high requirements).
var errIncompatibleConfig = errors.New("incompatible configuration")
/*
func errResp(code errCode, format string, v ...interface{}) error {
	return fmt.Errorf("%v - %v", code, fmt.Sprintf(format, v...))
}
*/

type ProtocolManager struct {
	IMetricCollector
	srv IServer

	networkID int

	fastSync  uint32 // Flag whether fast sync is enabled (gets disabled if we already have blocks)
	acceptTxs uint32 // Flag whether we're considered synchronised (enables transaction processing)

	checkpointNumber uint64      // Block number for the sync progress validator to cross reference
	checkpointHash   common.Hash // Block hash for the sync progress validator to cross reference

	txpool      txPool
	blockchain  *core.BlockChain
	eventFeed 	*EventFeed
	chainconfig *params.ChainConfig
	maxPeers    int

	downloader *downloader.Downloader
	fetcher    *fetcher.Fetcher
	peers      *peerSet
	handshakePeers *peerSet

	SubProtocols []IProtocol

	txSyncManager *txSyncManager

}


// NewProtocolManager returns a new Ethereum sub protocol manager. The Ethereum sub protocol manages peers capable
// with the Ethereum network.
func NewProtocolManager(srv IServer,  metricCollector IMetricCollector, eventFeed 	*EventFeed, blockchain *core.BlockChain, pool txPool) *ProtocolManager {

	// Create the protocol manager with the base fields
	manager := &ProtocolManager{
		IMetricCollector: metricCollector,
		srv:	srv,
		txpool: 	 pool, //core.NewTxPool(core.DefaultTxPoolConfig, params.RinkebyChainConfig, blockchain),
		blockchain:  blockchain,
		networkID:   srv.Self().GetNetworkID(),
		peers:       newPeerSet(),
		handshakePeers: newPeerSet(),
		maxPeers:  	 srv.Self().GetMaxPeers(),
		eventFeed:   eventFeed,
	}


	manager.SubProtocols = make([]IProtocol, 0)


	if containsProtocol(srv.Self().GetProtocols(), ETH) {

		manager.SubProtocols = append(manager.SubProtocols, &Protocol{
			Name:    ProtocolName,
			Version: 63,
			Length:  ProtocolLengths[0],
			RunFunc: func(p IPeer) {
				peer := newPeer(63, p, manager)
				manager.handle(peer)
			},
			CloseFunc: func(peer IPeer) {
				manager.removePeer(peer.ID())
			},
		})

	}

	manager.downloader = downloader.New(srv.Self().Name(), metricCollector, eventFeed, manager.peers, blockchain, manager.removePeer)

	validator := func(header *types.Header) error {
		return blockchain.Engine().VerifyHeader(blockchain, header, true)
	}
	heighter := func() uint64 {
		return blockchain.CurrentBlock().NumberU64()
	}
	inserter := func(blocks types.Blocks) (int, error) {
		atomic.StoreUint32(&manager.acceptTxs, 1) // Mark initial sync done on any fetcher import
		return manager.blockchain.InsertChain(blocks)
	}

	manager.fetcher = fetcher.New(srv.Self().Name(), blockchain.GetBlockByHash, validator, manager.BroadcastBlock, heighter, inserter, manager.removePeer)


	return manager
}

func containsProtocol(protoNames []string, val string)  bool {

	for _, v := range protoNames {
		if val == v {
			return true
		}
	}

	return false
}

func (pm *ProtocolManager) self() INode  {
	return pm.srv.Self()
}

func (pm *ProtocolManager) GetSubProtocols() []IProtocol {

	protos := make([]IProtocol, 0)

	for _, proto := range pm.SubProtocols {
		protos = append(protos, proto)
	}

	return protos
}

func (pm *ProtocolManager) removePeer(id ID) {
	// Short circuit if the peer was already removed
	peer := pm.peers.Peer(id)
	if peer == nil {
		return
	}
	pm.log("Removing Ethereum peer", "peer", peer)

	// Unregister the peer from the downloader and Ethereum peer set
	if err := pm.peers.Unregister(id); err != nil {
		pm.log("Peer removal failed", "peer", peer, "err", err)
	}

	pm.logPeerStats()
}

func (pm *ProtocolManager) Start() {


	pm.peers.Open()

	// broadcast transactions
	pm.txpool.SubscribeNewTxsEvent(func(txEvent core.NewTxsEvent) {
		pm.BroadcastTxs(txEvent.Txs)
	})

	// broadcast mined blocks
	pm.eventFeed.Subscribe(pm.broadcastMinedBlock, core.NewMinedBlockEvent{})


	// start sync handlers
	pm.txSyncManager = newTxSyncManager(pm)
	pm.txSyncManager.syncer()
}

func (pm *ProtocolManager) FindPeer(node INode) IPeer  {

	if pm.peers.closed {
		return nil
	}

	return pm.peers.Peer(node.ID())
}
func (pm *ProtocolManager) RetrieveHandshakePeer(node INode) IPeer  {
	defer pm.handshakePeers.Delete(node.ID())
	return pm.handshakePeers.Peer(node.ID())
}


func (pm *ProtocolManager) Stop() {
	pm.log("Stopping Ethereum protocol")

	pm.txSyncManager.stop()


	// Disconnect existing sessions.
	// This also closes the gate for any new registrations on the peer set.
	// sessions which are already established but not added to pm.peers yet
	// will exit when they try to register.
	pm.peers.Close()
}



// handle is the callback invoked to manage the life cycle of an eth peer. When
// this function terminates, the peer is disconnected.
func (pm *ProtocolManager) handle(p *peer) {
	// Ignore maxPeers if this is a trusted peer
	if pm.peers.Len() >= pm.maxPeers /* && !p.Peer.Info().Network.Trusted*/ {

		p.Log("Ethereum error connecting to peer", errDiscTooManyPeers)
		p.HandleError(errDiscTooManyPeers)
	}

	// dodaj peer u trenutne handshake peer-ove kako bi se moglo provjerit
	// da li je i sa druge strane peer u handshake fazi
	pm.handshakePeers.Register(p)

	p.Log("Ethereum peer connected")

	// Execute the Ethereum handshake
	var (
		genesis = pm.blockchain.Genesis()
		head    = pm.blockchain.CurrentHeader()
		hash    = head.Hash()
		number  = head.Number.Uint64()
		td      = pm.blockchain.GetTd(hash, number)
	)

	p.Handshake(pm.networkID, td, hash, genesis.Hash(), func(err error) {
		if err != nil {
			p.Log("Ethereum handshake failed", "err", err)
			return
		}

		// Register the peer locally
		err = pm.peers.Register(p)
		if err != nil {
			p.Log("Ethereum peer registration failed", "err", err)
			p.HandleError(err)
			return
		}

		pm.syncTransactions(p)
		pm.synchroniseNewPeer(p)

		pm.logPeerStats()
	})

}

func (pm *ProtocolManager) logPeerStats()  {
	pm.Set(EthPeers, pm.PeerCount())
}

func (pm *ProtocolManager) getBlockBodies(hashes []common.Hash) map[common.Hash]*types.Body  {
	bodies := make(map[common.Hash]*types.Body)

	for _, hash := range hashes {
		if len(bodies) >= downloader.MaxBlockFetch {
			break
		}

		// Retrieve the requested block body, stopping if enough was found
		if data := pm.blockchain.GetBody(hash); data != nil {
			bodies[hash] = data
		}
	}
	return bodies
}

func (pm *ProtocolManager) getBlockHeaders(query *getBlockHeadersData) []*types.Header {

	hashMode := query.Origin.Hash != (common.Hash{})
	first := true
	maxNonCanonical := uint64(100)

	// Gather headers until the fetch or network limits is reached
	var (
		bytes   common.StorageSize
		headers []*types.Header
		unknown bool
	)
	for !unknown && len(headers) < int(query.Amount) && bytes < softResponseLimit && len(headers) < downloader.MaxHeaderFetch {
		// Retrieve the next header satisfying the query
		var origin *types.Header
		if hashMode {
			if first {
				first = false
				origin = pm.blockchain.GetHeaderByHash(query.Origin.Hash)
				if origin != nil {
					query.Origin.Number = origin.Number.Uint64()
				}
			} else {
				origin = pm.blockchain.GetHeader(query.Origin.Hash, query.Origin.Number)
			}
		} else {
			origin = pm.blockchain.GetHeaderByNumber(query.Origin.Number)
		}
		if origin == nil {
			break
		}
		headers = append(headers, origin)
		bytes += estHeaderRlpSize

		// Advance to the next header of the query
		switch {
		case hashMode && query.Reverse:
			// Hash based traversal towards the genesis block
			ancestor := query.Skip + 1
			if ancestor == 0 {
				unknown = true
			} else {
				query.Origin.Hash, query.Origin.Number = pm.blockchain.GetAncestor(query.Origin.Hash, query.Origin.Number, ancestor, &maxNonCanonical)
				unknown = (query.Origin.Hash == common.Hash{})
			}
		case hashMode && !query.Reverse:
			// Hash based traversal towards the leaf block
			var (
				current = origin.Number.Uint64()
				next    = current + query.Skip + 1
			)
			if next <= current {
	//			infos, _ := json.MarshalIndent(p.Info(), "", "  ")
	//			p.Log("GetBlockHeaders skip overflow attack", "current", current, "skip", query.Skip, "next", next, "attacker", infos)
				unknown = true
			} else {
				if header := pm.blockchain.GetHeaderByNumber(next); header != nil {
					nextHash := header.Hash()
					expOldHash, _ := pm.blockchain.GetAncestor(nextHash, next, query.Skip+1, &maxNonCanonical)
					if expOldHash == query.Origin.Hash {
						query.Origin.Hash, query.Origin.Number = nextHash, next
					} else {
						unknown = true
					}
				} else {
					unknown = true
				}
			}
		case query.Reverse:
			// Number based traversal towards the genesis block
			if query.Origin.Number >= query.Skip+1 {
				query.Origin.Number -= query.Skip + 1
			} else {
				unknown = true
			}

		case !query.Reverse:
			// Number based traversal towards the leaf block
			query.Origin.Number += query.Skip + 1
		}
	}
	return headers
}

func (pm *ProtocolManager) HandleGetBlockHeadersMsg(p *peer, msg *Message) {
	// Block header query, collect the requested headers and reply
	query := msg.Content.(*getBlockHeadersData)

	headers := pm.getBlockHeaders(query)

	p.SendBlockHeaders(headers, nil)
}

func (pm *ProtocolManager) HandleBlockHeadersMsg(p *peer, msg *Message)  {

	headers := msg.Content.([]*types.Header)

	if len(headers) > 0 {
		err := pm.downloader.DeliverHeaders(p.id, headers)
		if err != nil {
			pm.log("Failed to deliver headers", "err", err)
		}
	}

}

func (pm *ProtocolManager) HandleTxMsg(p *peer, m *Message)  {

	txs := m.Content.(types.Transactions)

	for _, tx := range txs {
		// Validate and mark the remote transaction
		if tx == nil {
			p.HandleError(ErrDecode)//errResp(ErrDecode, "transaction %d is nil", i)
			return
		}
		p.MarkTransaction(tx.Hash())
	}

	pm.txpool.AddRemotes(txs)
}



func (pm *ProtocolManager) HandleNewBlockMsg(p *peer, m *Message)  {
	// Retrieve and decode the propagated block
	var request = m.Content.(newBlockData)

	request.Block.ReceivedAt = SecondsNow()
	request.Block.ReceivedFrom = p

	pm.log("HandleNewBlockMsg", p, "number:",request.Block.NumberU64())

	// Mark the peer as owning the block and schedule it for import
	p.MarkBlock(request.Block.Hash())
	pm.fetcher.Enqueue(p, request.Block)

	// Assuming the block is importable by the peer, but possibly not yet done so,
	// calculate the head hash and TD that the peer truly must have.
	var (
		trueHead = request.Block.ParentHash()
		trueTD   = new(big.Int).Sub(request.TD, request.Block.Difficulty())
	)
	// Update the peer's total difficulty if better than the previous
	if _, td := p.Head(); trueTD.Cmp(td) > 0 {
		p.SetHead(trueHead, trueTD)

		// Schedule a sync if above ours. Note, this will not fire a sync for a gap of
		// a single block (as the true TD is below the propagated block), however this
		// scenario should easily be covered by the fetcher.
		currentBlock := pm.blockchain.CurrentBlock()
		if trueTD.Cmp(pm.blockchain.GetTd(currentBlock.Hash(), currentBlock.NumberU64())) > 0 {
			StartNewRunner(func() {
				pm.synchronise(p)
			})
		}
	}
}



func (pm *ProtocolManager) HandleNewBlockHashesMsg(p *peer, m *Message) {
	var announces  = m.Content.(newBlockHashesData)

	// Mark the hashes as present at the remote node
	for _, block := range announces {
		p.MarkBlock(block.Hash)
	}
	// Schedule all the unknown hashes for retrieval
	unknown := make(newBlockHashesData, 0, len(announces))
	for _, block := range announces {
		if !pm.blockchain.HasBlock(block.Hash, block.Number) {
			unknown = append(unknown, block)
		}
	}


	for _, block := range unknown {
		pm.fetcher.Notify(p, block.Hash, block.Number, p.GetOneHeader, p.GetBodies)
	}
}

// BroadcastBlock will either propagate a block to a subset of it's peers, or
// will only announce it's availability (depending what's requested).
func (pm *ProtocolManager) BroadcastBlock(block *types.Block, propagate bool) {
	hash := block.Hash()
	peers := pm.peers.PeersWithoutBlock(hash)

	// If propagation is requested, send to a subset of the peer
	if propagate {
		// Calculate the TD of the block (it's not imported yet, so block.Td is not valid)
		var td *big.Int
		if parent := pm.blockchain.GetBlock(block.ParentHash(), block.NumberU64()-1); parent != nil {
			td = new(big.Int).Add(block.Difficulty(), pm.blockchain.GetTd(block.ParentHash(), block.NumberU64()-1))
		} else {
			pm.log("Propagating dangling block", "number", block.Number(), "hash", hash)
			return
		}
		// Send the block to a subset of our peers
		transferLen := int(math.Sqrt(float64(len(peers))))
		if transferLen < minBroadcastPeers {
			transferLen = minBroadcastPeers
		}
		if transferLen > len(peers) {
			transferLen = len(peers)
		}
		transfer := peers[:transferLen]
		for _, peer := range transfer {
			peer.AsyncSendNewBlock(block, td)
		}

		pm.log("Propagated block", block.NumberU64(), "recipients", len(transfer), "duration", TimeSince(block.ReceivedAt))

		return
	}
	// Otherwise if the block is indeed in out own chain, announce it
	if pm.blockchain.HasBlock(hash, block.NumberU64()) {
		for _, peer := range peers {
			peer.AsyncSendNewBlockHash(block)
		}
		pm.log("Announced block",block.NumberU64(), "recipients", len(peers), "duration", TimeSince(block.ReceivedAt))
	}
}

// BroadcastTxs will propagate a batch of transactions to all peers which are not known to
// already have the given transaction.
func (pm *ProtocolManager) BroadcastTxs(txs types.Transactions) {
	var txset = make(map[*peer]types.Transactions)

	// Broadcast transactions to a batch of peers not knowing about it
	for _, tx := range txs {
		peers := pm.peers.PeersWithoutTx(tx.Hash())
		for _, peer := range peers {
			txset[peer] = append(txset[peer], tx)
		}
		pm.log("Broadcast transaction", "hash", tx.Hash(), "recipients", len(peers))
	}
	// FIXME include this again: peers = peers[:int(math.Sqrt(float64(len(peers))))]

	pm.log("Broadcast txs", len(txs),  "pending", pm.PendingTxCount())

	for peer, txs := range txset {
		peer.AsyncSendTransactions(txs)
	}
}


func (pm *ProtocolManager) broadcastMinedBlock(data interface{}) {
	if ev, ok := data.(core.NewMinedBlockEvent); ok {
		pm.Update(MinedBlock)
		pm.Set(TxsPerBlock, ev.Block.Transactions().Len())
		pm.BroadcastBlock(ev.Block, true)  // First propagate block to peers
		pm.BroadcastBlock(ev.Block, false) // Only then announce to the rest
	}
}
func (pm *ProtocolManager) PendingTxCount() int  {
	return pm.txpool.PendingCount()
}

func (pm *ProtocolManager) AddTxs(txs types.Transactions) []error {

	pm.log("Added txs", len(txs), "pending", pm.PendingTxCount())

	errors := pm.txpool.AddRemotes(txs)

	return errors
}



// NodeInfo represents a short summary of the Ethereum sub-protocol metadata
// known about the host peer.
type NodeInfo struct {
	Network    int                  // Ethereum network ID (1=Frontier, 2=Morden, Ropsten=3, Rinkeby=4)
	Difficulty *big.Int             // Total difficulty of the host's blockchain
	Genesis    common.Hash          // SHA3 hash of the host's genesis block
	Config     *params.ChainConfig  // Chain configuration for the fork rules
	Head       common.Hash          // SHA3 hash of the host's best owned block
}

// NodeInfo retrieves some protocol metadata about the running host node.
func (pm *ProtocolManager) NodeInfo() *NodeInfo {
	currentBlock := pm.blockchain.CurrentBlock()
	return &NodeInfo{
		Network:    pm.networkID,
		Difficulty: pm.blockchain.GetTd(currentBlock.Hash(), currentBlock.NumberU64()),
		Genesis:    pm.blockchain.Genesis().Hash(),
		Config:     pm.blockchain.Config(),
		Head:       currentBlock.Hash(),
	}
}

func (pm *ProtocolManager) PeerCount() int {
	return pm.peers.Len()
}

func (pm *ProtocolManager) String() string {
	return fmt.Sprintf("%s ProtocolManager",pm.self().Name())
}

func (pm *ProtocolManager) log(a ...interface{})  {
	if LogConfig.LogProtocol {
		Log(pm, a)
	}
}
