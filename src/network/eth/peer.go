package eth

import (
	. "../../common"
	"../eth/common"
	"../eth/core/types"
	. "../message"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/deckarep/golang-set"
)

var (
	errEthPeerClosed     = errors.New("peer set is closed")
	errAlreadyRegistered = errors.New("peer is already registered")
	errNotRegistered     = errors.New("peer is not registered")
	errDiscNetworkError  = errors.New("network error")
	errDiscRequested     = errors.New("disconnect requested")
	errDiscQuitting		 = errors.New("client quiting")
	errDiscTooManyPeers  = errors.New("too many peers")

)



const (
	maxKnownTxs    = 32768 // Maximum transactions hashes to keep in the known list (prevent DOS)
	maxKnownBlocks = 1024  // Maximum block hashes to keep in the known list (prevent DOS)

	// maxQueuedTxs is the maximum number of transaction lists to queue up before
	// dropping broadcasts. This is a sensitive number as a transaction list might
	// contain a single transaction, or thousands.
	maxQueuedTxs = 128

	// maxQueuedProps is the maximum number of block propagations to queue up before
	// dropping broadcasts. There's not much point in queueing stale blocks, so a few
	// that might cover uncles should be enough.
	maxQueuedProps = 4

	// maxQueuedAnns is the maximum number of block announcements to queue up before
	// dropping broadcasts. Similarly to block propagations, there's no point to queue
	// above some healthy uncle limit, so use that.
	maxQueuedAnns = 4

	handshakeTimeout = 5 * time.Second
)

// PeerInfo represents a short summary of the Ethereum sub-protocol metadata known
// about a connected peer.
type PeerInfo struct {
	Version    int      `json:"version"`    // Ethereum protocol version negotiated
	Difficulty *big.Int `json:"difficulty"` // Total difficulty of the peer's blockchain
	Head       string   `json:"head"`       // SHA3 hash of the peer's best owned block
}

// propEvent is a block propagation, waiting for its turn in the broadcast queue.
type propEvent struct {
	block *types.Block
	td    *big.Int
}

type peer struct {
	id ID

	IPeer

	pm *ProtocolManager

	version  int         // Protocol version negotiated

	head common.Hash
	td   *big.Int

	knownTxs     mapset.Set                // Set of transaction hashes known to be known by this peer
	knownBlocks  mapset.Set                // Set of block hashes known to be known by this peer
	queuedTxs    chan []*types.Transaction // Queue of transactions to broadcast to the peer
	queuedProps  chan *propEvent           // Queue of blocks to broadcast to the peer
	queuedAnns   chan *types.Block         // Queue of blocks to announce to the peer
	broadcasting bool


	handshakeListener func(err error)
}

func newPeer(version int, p IPeer, pm *ProtocolManager) *peer {
	return &peer{
		IPeer:        p,
		pm:          pm,
		version:     version,
		id:          p.ID(),
		knownTxs:    mapset.NewSet(),
		knownBlocks: mapset.NewSet(),
		queuedTxs:   make(chan []*types.Transaction, maxQueuedTxs),
		queuedProps: make(chan *propEvent, maxQueuedProps),
		queuedAnns:  make(chan *types.Block, maxQueuedAnns),
	}
}

// broadcast is a write loop that multiplexes block propagations, announcements
// and transaction broadcasts into the remote peer. The goal is to have an async
// writer that does not lock up node internals.
func (p *peer) broadcast() {
	if p.broadcasting || p.IsClosed() {
		return
	}

	select {
	case txs := <-p.queuedTxs:

		p.SendTransactions(txs, nil)
		p.Log("Broadcast transactions", "count", len(txs))

	case prop := <-p.queuedProps:

		p.SendNewBlock(prop.block, prop.td, nil)
		p.Log("Propagated block", "number", prop.block.Number(), "hash", prop.block.Hash(), "td", prop.td)

	case block := <-p.queuedAnns:
		p.SendNewBlockHashes([]common.Hash{block.Hash()}, []uint64{block.NumberU64()}, nil)
		p.Log("Announced block", "number", block.Number(), "hash", block.Hash())

	default:
		return
	}
}


// Info gathers and returns a collection of metadata known about a peer.
func (p *peer) Info() *PeerInfo {
	hash, td := p.Head()

	return &PeerInfo{
		Version:    p.version,
		Difficulty: td,
		Head:       hash.Hex(),
	}
}

// Head retrieves a copy of the current head hash and total difficulty of the
// peer.
func (p *peer) Head() (hash common.Hash, td *big.Int) {
	copy(hash[:], p.head[:])
	return hash, new(big.Int).Set(p.td)
}

// SetHead updates the head hash and total difficulty of the peer.
func (p *peer) SetHead(hash common.Hash, td *big.Int) {
	copy(p.head[:], hash[:])
	p.td.Set(td)
}

// MarkBlock marks a block as known for the peer, ensuring that the block will
// never be propagated to this particular peer.
func (p *peer) MarkBlock(hash common.Hash) {
	// If we reached the memory allowance, drop a previously known block hash
	for p.knownBlocks.Cardinality() >= maxKnownBlocks {
		p.knownBlocks.Pop()
	}
	p.knownBlocks.Add(hash)
}

// MarkTransaction marks a transaction as known for the peer, ensuring that it
// will never be propagated to this particular peer.
func (p *peer) MarkTransaction(hash common.Hash) {
	// If we reached the memory allowance, drop a previously known transaction hash
	for p.knownTxs.Cardinality() >= maxKnownTxs {
		p.knownTxs.Pop()
	}
	p.knownTxs.Add(hash)
}


func (p *peer) NewMsg(to INode, msgType string, content interface{}, responseTo *Message, handler func(m *Message))  *Message {

	return NewMessage(p.Self(), to, msgType, content, 0,
		handler, responseTo,
		func(m *Message, err error) {
			// bilo koji error gasi peer-a
			p.HandleError(err)
			handler(nil)

		}, 0)

}


func (p *peer) send(msgType string, content interface{}, handler func(m *Message), listener func(err error))  {

	p.broadcasting = true

	m := p.NewMsg(p.Node(), msgType, content, nil,
			func(m *Message) {

				if m != nil {

					// ako se je peer na suprotonom nodu ugasija onda se gasi i ovaj
					if p.nodesEthPeer() != nil {
						handler(m)
						if listener != nil{
							listener(nil)
						}
					} else {
						p.HandleError(errEthPeerClosed)
						if listener != nil{
							listener(errEthPeerClosed)
						}
					}
				} else if listener != nil{
					listener(errEthPeerClosed)
				}
			})


	m.Send()

	//TODO: da li cekat slanje ili ne
//	godes.Yield()

	p.broadcasting = false
	p.broadcast()

	return
}


// SendTransactions sends transactions to the peer and includes the hashes
// in its transaction hash set for future reference.
func (p *peer) SendTransactions(txs types.Transactions, listener func(err error)) {
	for _, tx := range txs {
		p.knownTxs.Add(tx.Hash())
	}
	p.send(TX_MSG, txs, func(m *Message) {
		otherPeer := p.nodesEthPeer()
		otherPeer.pm.HandleTxMsg(otherPeer, m)
	}, listener)
}



// AsyncSendTransactions queues list of transactions propagation to a remote
// peer. If the peer's broadcast queue is full, the event is silently dropped.
func (p *peer) AsyncSendTransactions(txs []*types.Transaction) {
	select {
	case p.queuedTxs <- txs:
		for _, tx := range txs {
			p.knownTxs.Add(tx.Hash())
		}
		p.broadcast()
	default:
		p.Log("Dropping transaction propagation", "count", len(txs))
	}
}

// SendNewBlockHashes announces the availability of a number of blocks through
// a hash notification.
func (p *peer) SendNewBlockHashes(hashes []common.Hash, numbers []uint64,  listener func(err error)) {
	for _, hash := range hashes {
		p.knownBlocks.Add(hash)
	}

	request := make(newBlockHashesData, len(hashes))
	for i := 0; i < len(hashes); i++ {
		request[i].Hash = hashes[i]
		request[i].Number = numbers[i]
	}

	p.send(NEW_BLOCK_HASHES_MSG, request, func(m *Message) {
		otherPeer := p.nodesEthPeer()
		otherPeer.pm.HandleNewBlockHashesMsg(otherPeer, m)
	}, listener)
}

// AsyncSendNewBlockHash queues the availability of a block for propagation to a
// remote peer. If the peer's broadcast queue is full, the event is silently
// dropped.
func (p *peer) AsyncSendNewBlockHash(block *types.Block) {
	select {
	case p.queuedAnns <- block:
		p.knownBlocks.Add(block.Hash())
		p.broadcast()
	default:
		p.Log("Dropping block announcement", "number", block.NumberU64(), "hash", block.Hash())
	}
}

// SendNewBlock propagates an entire block to a remote peer.
func (p *peer) SendNewBlock(block *types.Block, td *big.Int,  listener func(err error)) {
	p.knownBlocks.Add(block.Hash())

	p.send(NEW_BLOCK_MSG, newBlockData{block, td}, func(m *Message) {
		otherPeer := p.nodesEthPeer()
		otherPeer.pm.HandleNewBlockMsg(otherPeer, m)
	}, listener)

}

// AsyncSendNewBlock queues an entire block for propagation to a remote peer. If
// the peer's broadcast queue is full, the event is silently dropped.
func (p *peer) AsyncSendNewBlock(block *types.Block, td *big.Int) {
	select {
	case p.queuedProps <- &propEvent{block: block, td: td}:
		p.knownBlocks.Add(block.Hash())
		p.broadcast()
	default:
		p.Log("Dropping block propagation", "number", block.NumberU64(), "hash", block.Hash())
	}
}


// SendBlockHeaders sends a batch of block headers to the remote peer.
func (p *peer) SendBlockHeaders(headers []*types.Header, listener func(err error)) {

	p.send(BLOCK_HEADERS_MSG, headers, func(m *Message) {
		otherPeer := p.nodesEthPeer()
		otherPeer.pm.HandleBlockHeadersMsg(otherPeer, m)
	}, listener)
}



// RequestHeadersByHash fetches a batch of blocks' headers corresponding to the
// specified header query, based on the hash of an origin block.
func (p *peer) RequestHeadersByHash(origin common.Hash, amount int, skip int, reverse bool) {
	p.Log("Fetching batch of headers", "count", amount, "fromhash", origin, "skip", skip, "reverse", reverse)
	data := &getBlockHeadersData{Origin: hashOrNumber{Hash: origin}, Amount: uint64(amount), Skip: uint64(skip), Reverse: reverse}

	p.send(GET_BLOCK_HEADERS_MSG, data, func(m *Message) {
		otherPeer := p.nodesEthPeer()
		otherPeer.pm.HandleGetBlockHeadersMsg(otherPeer, m)
	}, nil)

}

func (p *peer) GetHeadersByHash(origin common.Hash, amount int, skip int, reverse bool) ([]*types.Header, error) {
	data := &getBlockHeadersData{Origin: hashOrNumber{Hash: origin}, Amount: uint64(amount), Skip: uint64(skip), Reverse: reverse}

	otherPeer := p.nodesEthPeer()
	if otherPeer != nil {
		return otherPeer.pm.getBlockHeaders(data), nil
	}

	return nil, errEthPeerClosed
}
func (p *peer) GetHeadersByNumber(origin uint64, amount int, skip int, reverse bool) ([]*types.Header, error)  {
	p.Log("Fetching batch of headers", "count", amount, "fromnum", origin, "skip", skip, "reverse", reverse)
	data := &getBlockHeadersData{Origin: hashOrNumber{Number: origin}, Amount: uint64(amount), Skip: uint64(skip), Reverse: reverse}

	otherPeer := p.nodesEthPeer()
	if otherPeer != nil {
		return otherPeer.pm.getBlockHeaders(data), nil
	}

	return nil, errEthPeerClosed
}


// RequestBodies fetches a batch of blocks' bodies corresponding to the hashes
// specified.
func (p *peer) GetBodies(hashes []common.Hash) (map[common.Hash]*types.Body, error) {
	p.Log("Getting batch of block bodies", "count", len(hashes))

	otherPeer := p.nodesEthPeer()
	if otherPeer != nil {
		return otherPeer.pm.getBlockBodies(hashes), nil
	}

	return nil, errEthPeerClosed

}


/*

// SendBlockBodies sends a batch of block contents to the remote peer.
func (p *peer) SendBlockBodies(bodies []*blockBody) error {
	return p2p.Send(p.rw, BlockBodiesMsg, blockBodiesData(bodies))
}

// SendBlockBodiesRLP sends a batch of block contents to the remote peer from
// an already RLP encoded format.
func (p *peer) SendBlockBodiesRLP(bodies []rlp.RawValue) error {
	return p2p.Send(p.rw, BlockBodiesMsg, bodies)
}

// SendNodeDataRLP sends a batch of arbitrary internal data, corresponding to the
// hashes requested.
func (p *peer) SendNodeDataRLP(data [][]byte) error {
	return p2p.Send(p.rw, NodeDataMsg, data)
}

// SendReceiptsRLP sends a batch of transaction receipts, corresponding to the
// ones requested from an already RLP encoded format.
func (p *peer) SendReceiptsRLP(receipts []rlp.RawValue) error {
	return p2p.Send(p.rw, ReceiptsMsg, receipts)
}

// RequestOneHeader is a wrapper around the header query functions to fetch a
// single header. It is used solely by the fetcher.
func (p *peer) RequestOneHeader(hash common.Hash) error {
	p.Log("Fetching single header", "hash", hash)
	return p2p.Send(p.rw, GetBlockHeadersMsg, &getBlockHeadersData{Origin: hashOrNumber{Hash: hash}, Amount: uint64(1), Skip: uint64(0), Reverse: false})
}


// RequestHeadersByNumber fetches a batch of blocks' headers corresponding to the
// specified header query, based on the number of an origin block.
func (p *peer) RequestHeadersByNumber(origin uint64, amount int, skip int, reverse bool) error {
	p.Log("Fetching batch of headers", "count", amount, "fromnum", origin, "skip", skip, "reverse", reverse)
	return p2p.Send(p.rw, GetBlockHeadersMsg, &getBlockHeadersData{Origin: hashOrNumber{Number: origin}, Amount: uint64(amount), Skip: uint64(skip), Reverse: reverse})
}

// RequestBodies fetches a batch of blocks' bodies corresponding to the hashes
// specified.
func (p *peer) RequestBodies(hashes []common.Hash) error {
	p.Log("Fetching batch of block bodies", "count", len(hashes))
	return p2p.Send(p.rw, GetBlockBodiesMsg, hashes)
}

// RequestNodeData fetches a batch of arbitrary data from a node's known state
// data, corresponding to the specified hashes.
func (p *peer) RequestNodeData(hashes []common.Hash) error {
	p.Log("Fetching batch of state data", "count", len(hashes))
	return p2p.Send(p.rw, GetNodeDataMsg, hashes)
}

// RequestReceipts fetches a batch of transaction receipts from a remote node.
func (p *peer) RequestReceipts(hashes []common.Hash) error {
	p.Log("Fetching batch of receipts", "count", len(hashes))
	return p2p.Send(p.rw, GetReceiptsMsg, hashes)
}


*/

func (p *peer) nodesEthPeer() *peer {
	iPeer :=  p.Node().Server().GetProtocolManager().FindPeer(p.Self())
	return iPeer.(*peer)
}



func (p *peer) Close(reason error)  {
	p.IPeer.Close(reason)
}



func (p *peer) newEthMsg(msgType string, content interface{}, responseTo *Message,
							handler func(m *Message), onResponse func(m *Message, err error), responseTimeout float64)  *Message {

	m := NewMessage(p.Self(), p.Node(), msgType, content, 0,
		handler, responseTo,
		onResponse, responseTimeout)

	return m
}
func (p *peer) newStatusMsg(statusData *statusData, listener func(peer *peer, msg *Message, err error)) *Message  {

	return p.newEthMsg(STATUS_MSG, statusData, nil, func(m *Message) {

		otherPeer := p.Node().Server().GetProtocolManager().RetrieveHandshakePeer(p.Self())

		if otherPeer.(*peer) != nil {
			listener(otherPeer.(*peer), m,  nil)

		} else {
			listener(nil, m, ErrMsgTimeout)
		}

	}, func(m *Message, err error) {
		listener(nil, m, err)
	}, handshakeTimeout.Seconds())
}

// Handshake executes the eth protocol handshake, negotiating version number,
// network IDs, difficulties, head and genesis blocks.
func (p *peer) Handshake(network int, td *big.Int, head common.Hash, genesis common.Hash, listener func(err error)) {

	p.handshakeListener = listener

	statusMsg := p.newStatusMsg(&statusData{
			ProtocolVersion: uint32(p.version),
			NetworkId:       network,
			TD:              td,
			CurrentBlock:    head,
			GenesisBlock:    genesis,
			listener:		 listener,
		},
		func(peer *peer, msg *Message, err error) {


			if p.HandleError(err) {
				var status *statusData
				status, err = peer.readStatus(peer.pm.networkID, peer.pm.blockchain.Genesis().Hash(), msg)

				if status != nil {
					peer.td, peer.head = status.TD, status.CurrentBlock
				}

				p.HandleError(err)
			}

			if peer != nil {
				if peer.handshakeListener != nil {
					peer.handshakeListener(err)
				}
			}
		})

	statusMsg.Send()

}



func (p *peer) readStatus(network int, genesis common.Hash, msg *Message) (*statusData, error) {
	statusData := msg.Content.(*statusData)

	if network != statusData.NetworkId {
		return nil, ErrNetworkIdMismatch
	}
	if genesis != statusData.GenesisBlock {
		return nil, ErrGenesisBlockMismatch
	}

	return statusData, nil
}



// String implements fmt.Stringer.
func (p *peer) String() string {
	return fmt.Sprintf("Peer %s [%s]", p.Name(),
		fmt.Sprintf("eth/%2d", p.version),
	)
}

// peerSet represents the collection of active peers currently participating in
// the Ethereum sub-protocol.
type peerSet struct {
	peers  map[ID]*peer
	closed bool
}

// newPeerSet creates a new peer set to track the active participants.
func newPeerSet() *peerSet {
	return &peerSet{
		peers: make(map[ID]*peer),
	}
}

// Register injects a new peer into the working set, or returns an error if the
// peer is already known. If a new peer it registered, its broadcast loop is also
// started.
func (ps *peerSet) Register(p *peer) error {

	if ps.closed {
		return errEthPeerClosed
	}

	if _, ok := ps.peers[p.id]; ok {
		return errAlreadyRegistered
	}

	ps.peers[p.id] = p


	return nil
}

// Unregister removes a remote peer from the active set, disabling any further
// actions to/from that particular entity.
func (ps *peerSet) Unregister(id ID) error {

	p, ok := ps.peers[id]
	if !ok {
		return errNotRegistered
	}
	delete(ps.peers, id)

	p.Close(errDiscRequested)

	return nil
}

func (ps *peerSet) Delete(id ID)  {
	delete(ps.peers, id)
}

// Peer retrieves the registered peer with the given id.
func (ps *peerSet) Peer(id ID) IPeer {
	return ps.peers[id]
}

// Len returns if the current number of peers in the set.
func (ps *peerSet) Len() int {
	return len(ps.peers)
}

// PeersWithoutBlock retrieves a list of peers that do not have a given block in
// their set of known hashes.
func (ps *peerSet) PeersWithoutBlock(hash common.Hash) []*peer {

	list := make([]*peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		if !p.knownBlocks.Contains(hash) {
			list = append(list, p)
		}
	}
	return list
}

// PeersWithoutTx retrieves a list of peers that do not have a given transaction
// in their set of known hashes.
func (ps *peerSet) PeersWithoutTx(hash common.Hash) []*peer {

	list := make([]*peer, 0, len(ps.peers))
	for _, p := range ps.peers {
		if !p.knownTxs.Contains(hash) {
			list = append(list, p)
		}
	}
	return list
}

// BestPeer retrieves the known peer with the currently highest total difficulty.
func (ps *peerSet) BestPeer() *peer {

	var (
		bestPeer *peer
		bestTd   *big.Int
	)
	for _, p := range ps.peers {
		if _, td := p.Head(); bestPeer == nil || td.Cmp(bestTd) > 0 {
			bestPeer, bestTd = p, td
		}
	}
	return bestPeer
}

// Close disconnects all peers.
// No new peers can be registered after Close has returned.
func (ps *peerSet) Close() {

	for _, p := range ps.peers {
		p.Disconnect(errDiscQuitting)
	}
	ps.closed = true
}

