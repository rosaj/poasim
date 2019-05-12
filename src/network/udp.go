package network


import (
	"../util"
	"errors"
	"github.com/agoussia/godes"
	"github.com/ethereum/go-ethereum/crypto"
	"time"
)

// Errors
var (
	errPacketTooSmall   = errors.New("too small")
	errBadHash          = errors.New("bad hash")
	errExpired          = errors.New("expired")
	errUnsolicitedReply = errors.New("unsolicited reply")
	errUnknownNode      = errors.New("unknown node")
	errTimeout          = errors.New("RPC timeout")
	errClockWarp        = errors.New("reply deadline too far in the future")
	errClosed           = errors.New("socket closed")
)

// Timeouts
const (
	respTimeout    = 500 * time.Millisecond
	expiration     = 20 * time.Second
	bondExpiration = 24 * time.Hour


	// Discovery packets are defined to be no larger than 1280 bytes.
	// Packets larger than this size will be cut at the end and treated
	// as invalid because their hash won't match.
	maxPacketSize = 1280
)


type (


	ping struct {
		*Message
	}

	// pong is the reply to ping.
	pong struct {
		*Message
	}

	// findnode is a query for nodes close to the given target.
	findnode struct {
		*Message
	}

	// reply to findnode
	neighbors struct {
		*Message
	}

)

// packet is implemented by all protocol messages.
type packet interface {
	msg() *Message
	// preverify checks whether the packet is valid and should be handled at all.
	preverify(t *udp, from *Node) bool
	// handle handles the packet.
	handle(t *udp, from *Node)
}

// udp implements the discovery v4 UDP wire protocol.
type udp struct {
	node	    *Node
	db          *DB
	tab         *Table
}


func newUDP(node *Node) (*Table, *udp) {
	udp := &udp{
		node:       	 node,
		db:              newDB(),
	}
	tab := newTable(udp, node.bootstrapNodes)
	udp.tab = tab

	return udp.tab, udp
}

func (t *udp) self() *Node {
	return t.node
}


func (t *udp) findnode(node *Node, targetKey encPubkey, onResponse func(nodes *nodesByDistance, err error))  {

	if godes.GetSystemTime() - t.db.LastPingReceived(node) > bondExpiration.Seconds(){

		t.self().sendPingPackage(node, func(m *Message, err error) {
			// Wait for them to ping back and process our pong.
			godes.Advance(respTimeout.Seconds())

			t.self().sendFindNodePackage(node, targetKey, onResponse)
		})
	}else{
		t.self().sendFindNodePackage(node, targetKey, onResponse)
	}
}



// Packet Handlers
func isMsgValid(m *Message) bool {
	if m.HasExpired(){
		util.Log("Message: ", m , " expired with latency ", m.GetLatency())
		return false
	}
	return true
}

func handlePacket(p packet)  {
	msg := p.msg()
	if p.preverify(msg.To.udp, msg.From) {
		p.handle(msg.To.udp, msg.From)
	}
}

func (req *ping) msg() *Message {
	return req.Message
}

func (req *ping) preverify(t *udp, from *Node) bool  {
	return isMsgValid(req.Message)
}


func (req *ping) handle(t *udp, from *Node) {

	// Reply
	req.To.sendPongPackage(req.From, req.Message)


	// Ping back if our last sendPongPackage on file is too far in the past.
	if godes.GetSystemTime() - t.db.LastPongReceived(from) > bondExpiration.Seconds() {

		t.self().sendPingPackage(from, func(m *Message, err error) {
			t.tab.addVerifiedNode(from)
		})
	} else {
		t.tab.addVerifiedNode(from)
	}

	// Update node database
	t.db.UpdateLastPingReceived(from, godes.GetSystemTime())
}


func (req *pong) msg() *Message {
	return req.Message
}

func (req *pong) preverify(t *udp, from *Node) bool {
	if !isMsgValid(req.Message){
		return false
	}

	if req.Message.responseTo == nil {
		util.Log("unsolicited reply: ", req.Message)
		return false
	}

	return true
}

func (req *pong) handle(t *udp, from *Node) {
	t.db.UpdateLastPongReceived(from, godes.GetSystemTime())
}


func (req *findnode) msg() *Message {
	return req.Message
}

func (req *findnode) preverify(t *udp, from *Node) bool {
	if !isMsgValid(req.Message){
		return false
	}

	if godes.GetSystemTime() - t.db.LastPongReceived(from) > bondExpiration.Seconds() {
		// No endpoint proof sendPongPackage exists, we don't process the packet. This prevents an
		// attack vector where the discovery protocol could be used to amplify traffic in a
		// DDOS attack. A malicious actor would send a findnode request with the IP address
		// and UDP port of the target as the source address. The recipient of the findnode
		// packet would then send a sendNeighborsPackage packet (which is a much bigger packet than
		// findnode) to the victim.
		util.Log("unknown node:", from.Name())
		return false
	}
	return true
}

func (req *findnode) handle(t *udp, from *Node) {
	// Determine closest nodes.
	Target := req.Content.(encPubkey)
	target := ID(crypto.Keccak256Hash(Target[:]))

	closest := t.tab.closest(target, bucketSize).entries
	
	req.To.sendNeighborsPackage(from, &nodesByDistance{closest, target}, req.Message)
}


func (req *neighbors) msg() *Message {
	return req.Message
}
func (req *neighbors) preverify(t *udp, from *Node) bool {
	if !isMsgValid(req.Message){
		return false
	}

	if req.Message.responseTo == nil {
		util.Log("unsolicited reply: ", req.Message)
		return false
	}

	return true
}

func (req *neighbors) handle(t *udp, from *Node) {

}


func (n *Node) sendPingPackage(node *Node, onResponse func(m *Message, err error)){
	msg := newPingMessage(n, node, onResponse)
	msg.send()
}

func (n *Node) sendPongPackage(node *Node, responseToPingMsg *Message)  {
	msg := newPongMessage(n, node, responseToPingMsg)
	msg.send()
}

func (n *Node) sendFindNodePackage(node *Node, pubkey encPubkey, onResponse func(nodes *nodesByDistance, err error))  {
	msg := newFindNodeMessage(n, node, pubkey, func(m *Message, err error) {

		if onResponse != nil {
			var nodes *nodesByDistance = nil
			if m != nil {
				nodes = m.Content.(*nodesByDistance)
			}
			onResponse(nodes, err)
		}

	})
	msg.send()
}

func (n *Node) sendNeighborsPackage(node *Node, nodes *nodesByDistance, responseToMsg *Message)()  {
	msg := newNeighborsMessage(n, node, nodes, responseToMsg)
	msg.send()
}




// node messages
func getExpirationTime() float64  {
	return godes.GetSystemTime() + expiration.Seconds()
}

func newPingMessage(from *Node, to *Node, onResponse func(m *Message, err error)) *Message {
	return newMessage(from, to, PING, PING, getExpirationTime(),
			func(m *Message) {
				handlePacket(&ping{m})
			},
			nil, onResponse,
			respTimeout.Seconds())
}

func newPongMessage(from *Node, to *Node, responseTo *Message) *Message  {
	return newMessage(from, to, PONG, PONG, getExpirationTime(),
			func(m *Message) {
				handlePacket(&pong{m})
			},
			responseTo, nil,
			0)
}


func newFindNodeMessage(from *Node, to *Node, pubkey encPubkey, onResponse func(m *Message, err error)) *Message  {
	return newMessage(from, to, FINDNODE, pubkey,getExpirationTime(),
			func(m *Message) {
				handlePacket(&findnode{m})
			},
			nil, onResponse,
			respTimeout.Seconds())
}

func newNeighborsMessage(from *Node, to *Node, nodes *nodesByDistance, responseToMsg *Message) *Message  {
	return newMessage(from, to, NEIGHBORS, nodes,getExpirationTime(),
			func(m *Message) {
				handlePacket(&neighbors{m})
			},
			responseToMsg, nil,
			0)
}


