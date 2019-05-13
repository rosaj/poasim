package eth

import (
	. "../../common"
	. "../message"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/deckarep/golang-set"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

var (
	errEthPeerClosed     = errors.New("ethPeer set is closed")
	errAlreadyRegistered = errors.New("ethPeer is already registered")
	errNotRegistered     = errors.New("ethPeer is not registered")
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
// about a connected ethPeer.
type PeerInfo struct {
	Version    int      `json:"version"`    // Ethereum protocol version negotiated
	Difficulty *big.Int `json:"difficulty"` // Total difficulty of the ethPeer's blockchain
	Head       string   `json:"head"`       // SHA3 hash of the ethPeer's best owned block
}

// propEvent is a block propagation, waiting for its turn in the broadcast queue.
type propEvent struct {
	block *types.Block
	td    *big.Int
}

type ethPeer struct {
	id ID

	IPeer

	pm *ProtocolManager

	version  int         // Protocol version negotiated
	syncDrop *time.Timer // Timed connection dropper if sync progress isn't validated in time

	head common.Hash
	td   *big.Int

	knownTxs     mapset.Set                // Set of transaction hashes known to be known by this ethPeer
	knownBlocks  mapset.Set                // Set of block hashes known to be known by this ethPeer
	queuedTxs    chan []*types.Transaction // Queue of transactions to broadcast to the ethPeer
	queuedProps  chan *propEvent           // Queue of blocks to broadcast to the ethPeer
	queuedAnns   chan *types.Block         // Queue of blocks to announce to the ethPeer
	broadcasting bool
	// Termination channel to stop the broadcaster
}

func newPeer(version int, p IPeer, pm *ProtocolManager) *ethPeer {
	return &ethPeer{
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
// and transaction broadcasts into the remote ethPeer. The goal is to have an async
// writer that does not lock up node internals.
func (p *ethPeer) broadcast() {
	if p.broadcasting || p.IsClosed() {
		return
	}

	select {
	case txs := <-p.queuedTxs:

		p.SendTransactions(txs)
		p.Log("Broadcast transactions", "count", len(txs))

	case prop := <-p.queuedProps:

		p.SendNewBlock(prop.block, prop.td)
		p.Log("Propagated block", "number", prop.block.Number(), "hash", prop.block.Hash(), "td", prop.td)

	case block := <-p.queuedAnns:
		p.SendNewBlockHashes([]common.Hash{block.Hash()}, []uint64{block.NumberU64()})
		p.Log("Announced block", "number", block.Number(), "hash", block.Hash())

	default:
		return
	}
}


// Info gathers and returns a collection of metadata known about a ethPeer.
func (p *ethPeer) Info() *PeerInfo {
	hash, td := p.Head()

	return &PeerInfo{
		Version:    p.version,
		Difficulty: td,
		Head:       hash.Hex(),
	}
}

// Head retrieves a copy of the current head hash and total difficulty of the
// ethPeer.
func (p *ethPeer) Head() (hash common.Hash, td *big.Int) {
	copy(hash[:], p.head[:])
	return hash, new(big.Int).Set(p.td)
}

// SetHead updates the head hash and total difficulty of the ethPeer.
func (p *ethPeer) SetHead(hash common.Hash, td *big.Int) {
	copy(p.head[:], hash[:])
	p.td.Set(td)
}

// MarkBlock marks a block as known for the ethPeer, ensuring that the block will
// never be propagated to this particular ethPeer.
func (p *ethPeer) MarkBlock(hash common.Hash) {
	// If we reached the memory allowance, drop a previously known block hash
	for p.knownBlocks.Cardinality() >= maxKnownBlocks {
		p.knownBlocks.Pop()
	}
	p.knownBlocks.Add(hash)
}

// MarkTransaction marks a transaction as known for the ethPeer, ensuring that it
// will never be propagated to this particular ethPeer.
func (p *ethPeer) MarkTransaction(hash common.Hash) {
	// If we reached the memory allowance, drop a previously known transaction hash
	for p.knownTxs.Cardinality() >= maxKnownTxs {
		p.knownTxs.Pop()
	}
	p.knownTxs.Add(hash)
}


func (p *ethPeer) NewMsg(to INode, msgType string, content interface{}, responseTo *Message, handler func(m *Message))  *Message {

	return NewMessage(p.Self(), to, msgType, content, 0,
		handler, responseTo,
		func(m *Message, err error) {
			// bilo koji error gasi peer-a
			p.HandleError(err)

		}, 0)

}

func (p *ethPeer) send(msgType string, content interface{}, handler func(m *Message))  {
	m := p.NewMsg(p.Node(), msgType, content, nil,
			func(m *Message) {

				p.broadcasting = false
				//TODO: dali cekat network latency da se posalje ili samo neko vrijeme writera
				p.broadcast()

				// ako se je peer na suprotonom nodu ugasija onda se gasi i ovaj
				if p.nodesEthPeer() != nil {
					handler(m)
				} else {
					p.HandleError(errEthPeerClosed)
				}

			})

	p.broadcasting = true

	m.Send()

	return
}


// SendTransactions sends transactions to the ethPeer and includes the hashes
// in its transaction hash set for future reference.
func (p *ethPeer) SendTransactions(txs types.Transactions) {
	for _, tx := range txs {
		p.knownTxs.Add(tx.Hash())
	}
	p.send(TX_MSG, txs, func(m *Message) {
		otherPeer := p.nodesEthPeer()
		otherPeer.pm.HandleTxMsg(otherPeer, m)
	})
}



// AsyncSendTransactions queues list of transactions propagation to a remote
// ethPeer. If the ethPeer's broadcast queue is full, the event is silently dropped.
func (p *ethPeer) AsyncSendTransactions(txs []*types.Transaction) {
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
func (p *ethPeer) SendNewBlockHashes(hashes []common.Hash, numbers []uint64) {
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
	})
}

// AsyncSendNewBlockHash queues the availability of a block for propagation to a
// remote ethPeer. If the ethPeer's broadcast queue is full, the event is silently
// dropped.
func (p *ethPeer) AsyncSendNewBlockHash(block *types.Block) {
	select {
	case p.queuedAnns <- block:
		p.knownBlocks.Add(block.Hash())
		p.broadcast()
	default:
		p.Log("Dropping block announcement", "number", block.NumberU64(), "hash", block.Hash())
	}
}

// SendNewBlock propagates an entire block to a remote ethPeer.
func (p *ethPeer) SendNewBlock(block *types.Block, td *big.Int) {
	p.knownBlocks.Add(block.Hash())

	p.send(NEW_BLOCK_MSG, newBlockData{block, td}, func(m *Message) {
		otherPeer := p.nodesEthPeer()
		otherPeer.pm.HandleNewBlockMsg(otherPeer, m)
	})

}

// AsyncSendNewBlock queues an entire block for propagation to a remote ethPeer. If
// the ethPeer's broadcast queue is full, the event is silently dropped.
func (p *ethPeer) AsyncSendNewBlock(block *types.Block, td *big.Int) {
	select {
	case p.queuedProps <- &propEvent{block: block, td: td}:
		p.knownBlocks.Add(block.Hash())
		p.broadcast()
	default:
		p.Log("Dropping block propagation", "number", block.NumberU64(), "hash", block.Hash())
	}
}



/*
// SendBlockHeaders sends a batch of block headers to the remote ethPeer.
func (p *ethPeer) SendBlockHeaders(headers []*types.Header) error {
	return p2p.Send(p.rw, BlockHeadersMsg, headers)
}

// SendBlockBodies sends a batch of block contents to the remote ethPeer.
func (p *ethPeer) SendBlockBodies(bodies []*blockBody) error {
	return p2p.Send(p.rw, BlockBodiesMsg, blockBodiesData(bodies))
}

// SendBlockBodiesRLP sends a batch of block contents to the remote ethPeer from
// an already RLP encoded format.
func (p *ethPeer) SendBlockBodiesRLP(bodies []rlp.RawValue) error {
	return p2p.Send(p.rw, BlockBodiesMsg, bodies)
}

// SendNodeDataRLP sends a batch of arbitrary internal data, corresponding to the
// hashes requested.
func (p *ethPeer) SendNodeData(data [][]byte) error {
	return p2p.Send(p.rw, NodeDataMsg, data)
}

// SendReceiptsRLP sends a batch of transaction receipts, corresponding to the
// ones requested from an already RLP encoded format.
func (p *ethPeer) SendReceiptsRLP(receipts []rlp.RawValue) error {
	return p2p.Send(p.rw, ReceiptsMsg, receipts)
}

// RequestOneHeader is a wrapper around the header query functions to fetch a
// single header. It is used solely by the fetcher.
func (p *ethPeer) RequestOneHeader(hash common.Hash) error {
	p.Log("Fetching single header", "hash", hash)
	return p2p.Send(p.rw, GetBlockHeadersMsg, &getBlockHeadersData{Origin: hashOrNumber{Hash: hash}, Amount: uint64(1), Skip: uint64(0), Reverse: false})
}

// RequestHeadersByHash fetches a batch of blocks' headers corresponding to the
// specified header query, based on the hash of an origin block.
func (p *ethPeer) RequestHeadersByHash(origin common.Hash, amount int, skip int, reverse bool) error {
	p.Log("Fetching batch of headers", "count", amount, "fromhash", origin, "skip", skip, "reverse", reverse)
	return p2p.Send(p.rw, GetBlockHeadersMsg, &getBlockHeadersData{Origin: hashOrNumber{Hash: origin}, Amount: uint64(amount), Skip: uint64(skip), Reverse: reverse})
}

// RequestHeadersByNumber fetches a batch of blocks' headers corresponding to the
// specified header query, based on the number of an origin block.
func (p *ethPeer) RequestHeadersByNumber(origin uint64, amount int, skip int, reverse bool) error {
	p.Log("Fetching batch of headers", "count", amount, "fromnum", origin, "skip", skip, "reverse", reverse)
	return p2p.Send(p.rw, GetBlockHeadersMsg, &getBlockHeadersData{Origin: hashOrNumber{Number: origin}, Amount: uint64(amount), Skip: uint64(skip), Reverse: reverse})
}

// RequestBodies fetches a batch of blocks' bodies corresponding to the hashes
// specified.
func (p *ethPeer) RequestBodies(hashes []common.Hash) error {
	p.Log("Fetching batch of block bodies", "count", len(hashes))
	return p2p.Send(p.rw, GetBlockBodiesMsg, hashes)
}

// RequestNodeData fetches a batch of arbitrary data from a node's known state
// data, corresponding to the specified hashes.
func (p *ethPeer) RequestNodeData(hashes []common.Hash) error {
	p.Log("Fetching batch of state data", "count", len(hashes))
	return p2p.Send(p.rw, GetNodeDataMsg, hashes)
}

// RequestReceipts fetches a batch of transaction receipts from a remote node.
func (p *ethPeer) RequestReceipts(hashes []common.Hash) error {
	p.Log("Fetching batch of receipts", "count", len(hashes))
	return p2p.Send(p.rw, GetReceiptsMsg, hashes)
}


*/

func (p *ethPeer) nodesEthPeer() *ethPeer {
	iPeer :=  p.Node().Server().GetProtocolManager().FindPeer(p.Self())
	return iPeer.(*ethPeer)
}



func (p *ethPeer) Close(reason error)  {
	p.IPeer.Close(reason)
}



func (p *ethPeer) newEthMsg(msgType string, content interface{}, responseTo *Message,
							handler func(m *Message), onResponse func(m *Message, err error), responseTimeout float64)  *Message {

	m := NewMessage(p.Self(), p.Node(), msgType, content, 0,
		handler, responseTo,
		onResponse, responseTimeout)

	return m
}
func (p *ethPeer) newStatusMsg(listener func(err error)) *Message  {

	return p.newEthMsg(STATUS_MSG, StatusMsg, nil, func(m *Message) {

		peer := p.Node().Server().GetProtocolManager().RetrieveHandshakePeer(p.Self())

		if peer.(*ethPeer) != nil {
			listener(nil)

		} else {
			listener(ErrMsgTimeout)
		}

	}, func(m *Message, err error) {
		listener(err)
	}, handshakeTimeout.Seconds())
}

// Handshake executes the eth protocol handshake, negotiating version number,
// network IDs, difficulties, head and genesis blocks.
func (p *ethPeer) Handshake(listener func(err error)) {
	statusMsg := p.newStatusMsg(func(err error) {
		if err == nil {
			if p.pm.networkID != p.Node().GetNetworkID() {
				err = ErrNetworkIdMismatch
				p.HandleError(err)
			}
		}
		listener(err)
	})

	statusMsg.Send()
  }



/*
func (p *ethPeer) newStatusMsg(responseTo *Message, onResponse func(m *Message, err error)) *Message  {

	return p.newEthMsg(STATUS_MSG, nil, responseTo, func(m *Message) {
				//TODO: remove this node ne odogovara sa status nego samo salje
					peer := p.Node().Server().ProtocolManager().FindHandshakePeer(p.Self())

					if peer != nil {
						peer.newStatusMsg(m, nil).Send()

					} else if onResponse != nil {
						onResponse(nil, ErrMsgTimeout)
					}

			}, onResponse, handshakeTimeout.Seconds())
}

// Handshake executes the eth protocol handshake, negotiating version number,
// network IDs, difficulties, head and genesis blocks.
func (p *ethPeer) Handshake(onSuccess func()) {

	statusMsg := p.newStatusMsg(nil, func(m *Message, err error) {

		p.pm.DeleteHandshakePeer(p.Node())

		if p.HandleError(err) {
			onSuccess()
			/*
				&statusData{
					ProtocolVersion: uint32(p.version),
					NetworkId:       p.pm.networkID,
					TD:              nil,
					CurrentBlock:    nil,
					GenesisBlock:    nil,
				}

			*/
/*
			if p.pm.networkID != p.Node().NetworkID {
				p.HandleError(ErrNetworkIdMismatch)
			} else {
				onSuccess()
			}

		}
	})

	statusMsg.Send()
}
*/



// String implements fmt.Stringer.
func (p *ethPeer) String() string {
	return fmt.Sprintf("Peer %s [%s]", p.Name(),
		fmt.Sprintf("eth/%2d", p.version),
	)
}

// peerSet represents the collection of active peers currently participating in
// the Ethereum sub-protocol.
type peerSet struct {
	peers  map[ID]*ethPeer
	closed bool
}

// newPeerSet creates a new ethPeer set to track the active participants.
func newPeerSet() *peerSet {
	return &peerSet{
		peers: make(map[ID]*ethPeer),
	}
}

// Register injects a new ethPeer into the working set, or returns an error if the
// ethPeer is already known. If a new ethPeer it registered, its broadcast loop is also
// started.
func (ps *peerSet) Register(p *ethPeer) error {

	if ps.closed {
		return errEthPeerClosed
	}

	if _, ok := ps.peers[p.id]; ok {
		return errAlreadyRegistered
	}

	ps.peers[p.id] = p


	return nil
}

// Unregister removes a remote ethPeer from the active set, disabling any further
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

// Peer retrieves the registered ethPeer with the given id.
func (ps *peerSet) Peer(id ID) *ethPeer {
	return ps.peers[id]
}

// Len returns if the current number of peers in the set.
func (ps *peerSet) Len() int {
	return len(ps.peers)
}

// PeersWithoutBlock retrieves a list of peers that do not have a given block in
// their set of known hashes.
func (ps *peerSet) PeersWithoutBlock(hash common.Hash) []*ethPeer {

	list := make([]*ethPeer, 0, len(ps.peers))
	for _, p := range ps.peers {
		if !p.knownBlocks.Contains(hash) {
			list = append(list, p)
		}
	}
	return list
}

// PeersWithoutTx retrieves a list of peers that do not have a given transaction
// in their set of known hashes.
func (ps *peerSet) PeersWithoutTx(hash common.Hash) []*ethPeer {

	list := make([]*ethPeer, 0, len(ps.peers))
	for _, p := range ps.peers {
		if !p.knownTxs.Contains(hash) {
			list = append(list, p)
		}
	}
	return list
}

// BestPeer retrieves the known ethPeer with the currently highest total difficulty.
func (ps *peerSet) BestPeer() *ethPeer {

	var (
		bestPeer *ethPeer
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

