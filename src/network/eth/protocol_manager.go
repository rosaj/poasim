package eth

import (
	. "../../common"
	. "../../config"
	. "../../util"
	"../eth/common"
	"../eth/consensus/clique"
	"../eth/core"
	"../eth/core/rawdb"
	"../eth/core/types"
	. "../message"
	. "../protocol"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/eth/fetcher"
	"github.com/ethereum/go-ethereum/event"
	"github.com/ethereum/go-ethereum/params"
	"math"
	"math/big"
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
	srv IServer

	networkID int

	fastSync  uint32 // Flag whether fast sync is enabled (gets disabled if we already have blocks)
	acceptTxs uint32 // Flag whether we're considered synchronised (enables transaction processing)

	checkpointNumber uint64      // Block number for the sync progress validator to cross reference
	checkpointHash   common.Hash // Block hash for the sync progress validator to cross reference

	txpool      txPool
	blockchain  *core.BlockChain
	chainconfig *params.ChainConfig
	maxPeers    int

	downloader *downloader.Downloader
	fetcher    *fetcher.Fetcher
	peers      *peerSet
	handshakePeers *peerSet

	SubProtocols []IProtocol

	minedBlockSub *event.TypeMuxSubscription

	whitelist map[uint64]common.Hash

	// channels for fetcher, syncer, txsyncLoop
	txsyncCh    chan *txsync
	newPeerCh   chan *peer
	quitSync    chan struct{}

	txSyncManager *txSyncManager

}


// NewProtocolManager returns a new Ethereum sub protocol manager. The Ethereum sub protocol manages peers capable
// with the Ethereum network.
func NewProtocolManager(srv IServer) *ProtocolManager {

	chainDb := rawdb.NewMemoryDatabase()

	engine := clique.New(params.RinkebyChainConfig.Clique, chainDb)

	chainConfig, _, err := core.SetupGenesisBlockWithOverride(chainDb, nil, nil)
	if err != nil {
		Print("Genesis error ", err)
	}


	blockchain, err := core.NewBlockChain(chainDb, nil, chainConfig,  engine, func(block *types.Block) bool {
		return false
	})
	if err != nil {
		Print("blockchain error ", err)
	}

	// Create the protocol manager with the base fields
	manager := &ProtocolManager{
		srv:	srv,
		txpool: 	 core.NewTxPool(core.DefaultTxPoolConfig, params.RinkebyChainConfig, blockchain),
		blockchain:  blockchain,
		networkID:   srv.Self().GetNetworkID(),
		peers:       newPeerSet(),
		handshakePeers: newPeerSet(),
		newPeerCh:   make(chan *peer),
		quitSync:    make(chan struct{}),
		maxPeers:  	 srv.Self().GetMaxPeers(),
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
				manager.removePeer(peer)
			},
		})

	}


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

func (pm *ProtocolManager) removePeer(p IPeer) {
	// Short circuit if the peer was already removed
	peer := pm.peers.Peer(p.ID())
	if peer == nil {
		return
	}
	pm.log("Removing Ethereum peer", "peer", peer)

	// Unregister the peer from the downloader and Ethereum peer set
	if err := pm.peers.Unregister(p.ID()); err != nil {
		pm.log("Peer removal failed", "peer", peer, "err", err)
	}

}

func (pm *ProtocolManager) Start() {

	// broadcast transactions
//	pm.txsCh = make(chan core.NewTxsEvent, txChanSize)
//	pm.txsSub = pm.txpool.SubscribeNewTxsEvent(pm.txsCh)

	//go pm.txBroadcastLoop()

	pm.txpool.SubscribeNewTxsEvent(func(txEvent core.NewTxsEvent) {
		pm.BroadcastTxs(txEvent.Txs)
	})

	// broadcast mined blocks
	//pm.minedBlockSub = pm.eventMux.Subscribe(core.NewMinedBlockEvent{})
	//go pm.minedBroadcastLoop()

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

//	pm.minedBlockSub.Unsubscribe() // quits blockBroadcastLoop

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

		/*
		// Register the peer in the downloader. If the downloader considers it banned, we disconnect
		if err := pm.downloader.RegisterPeer(p.id, p.version, p); err != nil {
			p.HandleError(err)
			return
		}
		*/

		pm.syncTransactions(p)
		pm.synchroniseNewPeer(p)

	})

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

	//request.Block.ReceivedAt = m.ReceivedAt
	request.Block.ReceivedFrom = p

	// Mark the peer as owning the block and schedule it for import
	p.MarkBlock(request.Block.Hash())
//	pm.fetcher.Enqueue(p.id, request.Block)

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
			//sgo pm.synchronise(p)
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

	/*
	//TODO: tu dohvaca bodie od blokovi
	for _, block := range unknown {
		pm.fetcher.Notify(p.id, block.Hash, block.Number, time.Now(), p.RequestOneHeader, p.RequestBodies)
	}
	*/
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
		pm.log("Propagated block", "hash", hash, "recipients", len(transfer), "duration", common.PrettyDuration(time.Since(block.ReceivedAt)))
		return
	}
	// Otherwise if the block is indeed in out own chain, announce it
	if pm.blockchain.HasBlock(hash, block.NumberU64()) {
		for _, peer := range peers {
			peer.AsyncSendNewBlockHash(block)
		}
		pm.log("Announced block", "hash", hash, "recipients", len(peers), "duration", common.PrettyDuration(time.Since(block.ReceivedAt)))
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
	pm.log("Broadcast txs", len(txs))

	Print(pm.self(), "Sending txs", len(txs), "pending", pm.pendingCount())

	for peer, txs := range txset {
		peer.AsyncSendTransactions(txs)
	}
}

// Mined broadcast loop
func (pm *ProtocolManager) minedBroadcastLoop() {
	// automatically stops if unsubscribe
	for obj := range pm.minedBlockSub.Chan() {
		if ev, ok := obj.Data.(core.NewMinedBlockEvent); ok {
			pm.BroadcastBlock(ev.Block, true)  // First propagate block to peers
			pm.BroadcastBlock(ev.Block, false) // Only then announce to the rest
		}
	}
}
func (pm *ProtocolManager) pendingCount() int  {
	pending:= pm.txpool.Pending()
	count := 0
	for _, v := range pending {
		count += len(v)
	}
	return count
}

func (pm *ProtocolManager) AddTxs(txs types.Transactions) {

	Print(pm.self(), "Added txs", len(txs), "pending", pm.pendingCount())
	errors := pm.txpool.AddRemotes(txs)

	for k, v := range errors {
		Print(pm.self(), "Err", k, v)

	}
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
