package network
/*
import (
	godesUtils "../util"

	"bytes"
	"container/list"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"github.com/agoussia/godes"
	"net"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/netutil"
	"github.com/ethereum/go-ethereum/rlp"
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

	ntpFailureThreshold = 32               // Continuous timeouts after which to check NTP
	ntpWarningCooldown  = 10 * time.Minute // Minimum amount of time to pass before repeating NTP warning
	driftThreshold      = 10 * time.Second // Allowed clock drift before warning user

	// Discovery packets are defined to be no larger than 1280 bytes.
	// Packets larger than this size will be cut at the end and treated
	// as invalid because their hash won't match.
	maxPacketSize = 1280
)

// RPC packet types
const (
	pingPacket = iota + 1 // zero is 'reserved'
	pongPacket
	findnodePacket
	neighborsPacket
)

type (


	ping struct {
		senderKey *ecdsa.PublicKey // filled in by preverify

		Version    uint
		From, To   Node
		Expiration uint64
	}

	// pong is the reply to ping.
	pong struct {
		// This field should mirror the UDP envelope address
		// of the ping packet, which provides a way to discover the
		// the external address (after NAT).
		To Node

		ReplyTok   []byte // This contains the hash of the ping packet.
		Expiration uint64 // Absolute timestamp at which the packet becomes invalid.
	}

	// findnode is a query for nodes close to the given target.
	findnode struct {
		Target     encPubkey
		Expiration uint64
	}

	// reply to findnode
	neighbors struct {
		Nodes      []Node
		Expiration uint64
	}

)

func makeEndpoint(addr *net.UDPAddr, tcpPort uint16) rpcEndpoint {
	ip := net.IP{}
	if ip4 := addr.IP.To4(); ip4 != nil {
		ip = ip4
	} else if ip6 := addr.IP.To16(); ip6 != nil {
		ip = ip6
	}
	return rpcEndpoint{IP: ip, UDP: uint16(addr.Port), TCP: tcpPort}
}

func (t *udp) nodeFromRPC(sender *net.UDPAddr, rn rpcNode) (*node, error) {
	if rn.UDP <= 1024 {
		return nil, errors.New("low port")
	}
	if err := netutil.CheckRelayIP(sender.IP, rn.IP); err != nil {
		return nil, err
	}
	if t.netrestrict != nil && !t.netrestrict.Contains(rn.IP) {
		return nil, errors.New("not contained in netrestrict whitelist")
	}
	key, err := decodePubkey(rn.ID)
	if err != nil {
		return nil, err
	}
	n := wrapNode(enode.NewV4(key, rn.IP, int(rn.TCP), int(rn.UDP)))
	err = n.ValidateComplete()
	return n, err
}

func nodeToRPC(n *node) rpcNode {
	var key ecdsa.PublicKey
	var ekey encPubkey
	if err := n.Load((*enode.Secp256k1)(&key)); err == nil {
		ekey = encodePubkey(&key)
	}
	return rpcNode{ID: ekey, IP: n.IP(), UDP: uint16(n.UDP()), TCP: uint16(n.TCP())}
}

// packet is implemented by all protocol messages.
type packet interface {
	// preverify checks whether the packet is valid and should be handled at all.
	preverify(t *udp, from *net.UDPAddr, fromID enode.ID, fromKey encPubkey) error
	// handle handles the packet.
	handle(t *udp, from *net.UDPAddr, fromID enode.ID, mac []byte)
	// name returns the name of the packet for logging purposes.
	name() string
}

// udp implements the discovery v4 UDP wire protocol.
type udp struct {
	conn        conn
	netrestrict *netutil.Netlist
	priv        *ecdsa.PrivateKey
	node	    *Node
	db          *DB
	tab         *Table
	wg          sync.WaitGroup

	addReplyMatcher chan *replyMatcher
	gotreply        chan reply
	closing         chan struct{}
}

// pending represents a pending reply.
//
// Some implementations of the protocol wish to send more than one
// reply packet to findnode. In general, any neighbors packet cannot
// be matched up with a specific findnode packet.
//
// Our implementation handles this by storing a callback function for
// each pending reply. Incoming packets from a node are dispatched
// to all callback functions for that node.
type replyMatcher struct {
	// these fields must match in the reply.
	from  enode.ID
	ip    net.IP
	ptype byte

	// time when the request must complete
	deadline time.Time

	// callback is called when a matching reply arrives. If it returns matched == true, the
	// reply was acceptable. The second return value indicates whether the callback should
	// be removed from the pending reply queue. If it returns false, the reply is considered
	// incomplete and the callback will be invoked again for the next matching reply.
	callback replyMatchFunc

	// errc receives nil when the callback indicates completion or an
	// error if no further reply is received within the timeout.
	errc chan<- error
}

type replyMatchFunc func(interface{}) (matched bool, requestDone bool)

type reply struct {
	from  enode.ID
	ip    net.IP
	ptype byte
	data  packet

	// loop indicates whether there was
	// a matching request by sending on this channel.
	matched chan<- bool
}

// ReadPacket is sent to the unhandled channel when it could not be processed
type ReadPacket struct {
	Data []byte
	Addr *net.UDPAddr
}

// Config holds Table-related settings.
type Config struct {
	// These settings are required and configure the UDP listener:
	PrivateKey *ecdsa.PrivateKey

	// These settings are optional:
	NetRestrict *netutil.Netlist  // network whitelist
	Bootnodes   []*enode.Node     // list of bootstrap nodes
	Unhandled   chan<- ReadPacket // unhandled packets are sent on this channel
}

func newUDP(node *Node, cfg Config) (*Table, *udp, error) {
	udp := &udp{
		priv:            cfg.PrivateKey,
		netrestrict:     cfg.NetRestrict,
		node:       	 node,
		db:              newDB(),
		closing:         make(chan struct{}),
		gotreply:        make(chan reply),
		addReplyMatcher: make(chan *replyMatcher),
	}
	tab := newTable(*udp)
	udp.tab = tab

	udp.wg.Add(2)
	go udp.loop()
	go udp.readLoop(cfg.Unhandled)
	return udp.tab, udp, nil
}

func (t *udp) self() Node {
	return *t.node
}

// ping sends a ping message to the given node and waits for a reply.
func (t *udp) ping(toid enode.ID, toaddr *net.UDPAddr) error {
	return <-t.sendPing(toid, toaddr, nil)
}

// sendPing sends a ping message to the given node and invokes the callback
// when the reply arrives.
func (t *udp) sendPing(toid enode.ID, toaddr *net.UDPAddr, callback func()) <-chan error {
	req := &ping{
		Version:    4,
		From:       t.self(),
		To:         makeEndpoint(toaddr, 0), // TODO: maybe use known TCP port from DB
		Expiration: uint64(time.Now().Add(expiration).Unix()),
	}
	packet, hash, err := encodePacket(t.priv, pingPacket, req)
	if err != nil {
		errc := make(chan error, 1)
		errc <- err
		return errc
	}
	// Add a matcher for the reply to the pending reply queue. Pongs are matched if they
	// reference the ping we're about to send.
	errc := t.pending(toid, toaddr.IP, pongPacket, func(p interface{}) (matched bool, requestDone bool) {
		matched = bytes.Equal(p.(*pong).ReplyTok, hash)
		if matched && callback != nil {
			callback()
		}
		return matched, matched
	})
	// Send the packet.
	t.localNode.UDPContact(toaddr)
	t.write(toaddr, toid, req.name(), packet)
	return errc
}


func (t *udp) findnode(node *Node, targetKey encPubkey, onResponse func(m *Message))  {
	var bondExpiration float64 = 24 * 60 * 60

	if godes.GetSystemTime() - t.db.LastPingReceived(node) > bondExpiration{

		t.self().Ping(node)

		var respTimeout = .5
		godesUtils.Wait(respTimeout)
	}


	t.self().FindNode(node, targetKey)


}

// findnode sends a findnode request to the given node and waits until
// the node has sent up to k neighbors.
func (t *udp) findnode(toid enode.ID, toaddr *net.UDPAddr, target encPubkey) ([]*node, error) {
	// If we haven't seen a ping from the destination node for a while, it won't remember
	// our endpoint proof and reject findnode. Solicit a ping first.
	if time.Since(t.db.LastPingReceived(toid, toaddr.IP)) > bondExpiration {
		t.ping(toid, toaddr)
		// Wait for them to ping back and process our pong.
		time.Sleep(respTimeout)
	}

	// Add a matcher for 'neighbours' replies to the pending reply queue. The matcher is
	// active until enough nodes have been received.
	nodes := make([]*node, 0, bucketSize)
	nreceived := 0
	errc := t.pending(toid, toaddr.IP, neighborsPacket, func(r interface{}) (matched bool, requestDone bool) {
		reply := r.(*neighbors)
		for _, rn := range reply.Nodes {
			nreceived++
			n, err := t.nodeFromRPC(toaddr, rn)
			if err != nil {
				log.Trace("Invalid neighbor node received", "ip", rn.IP, "addr", toaddr, "err", err)
				continue
			}
			nodes = append(nodes, n)
		}
		return true, nreceived >= bucketSize
	})
	t.send(toaddr, toid, findnodePacket, &findnode{
		Target:     target,
		Expiration: uint64(time.Now().Add(expiration).Unix()),
	})
	return nodes, <-errc
}

// pending adds a reply matcher to the pending reply queue.
// see the documentation of type replyMatcher for a detailed explanation.
func (t *udp) pending(id enode.ID, ip net.IP, ptype byte, callback replyMatchFunc) <-chan error {
	ch := make(chan error, 1)
	p := &replyMatcher{from: id, ip: ip, ptype: ptype, callback: callback, errc: ch}
	select {
	case t.addReplyMatcher <- p:
		// loop will handle it
	case <-t.closing:
		ch <- errClosed
	}
	return ch
}

// handleReply dispatches a reply packet, invoking reply matchers. It returns
// whether any matcher considered the packet acceptable.
func (t *udp) handleReply(from enode.ID, fromIP net.IP, ptype byte, req packet) bool {
	matched := make(chan bool, 1)
	select {
	case t.gotreply <- reply{from, fromIP, ptype, req, matched}:
		// loop will handle it
		return <-matched
	case <-t.closing:
		return false
	}
}

// loop runs in its own goroutine. it keeps track of
// the refresh timer and the pending reply queue.
func (t *udp) loop() {
	defer t.wg.Done()

	var (
		plist        = list.New()
		timeout      = time.NewTimer(0)
		nextTimeout  *replyMatcher // head of plist when timeout was last reset
		contTimeouts = 0           // number of continuous timeouts to do NTP checks
		ntpWarnTime  = time.Unix(0, 0)
	)
	<-timeout.C // ignore first timeout
	defer timeout.Stop()

	resetTimeout := func() {
		if plist.Front() == nil || nextTimeout == plist.Front().Value {
			return
		}
		// Start the timer so it fires when the next pending reply has expired.
		now := time.Now()
		for el := plist.Front(); el != nil; el = el.Next() {
			nextTimeout = el.Value.(*replyMatcher)
			if dist := nextTimeout.deadline.Sub(now); dist < 2*respTimeout {
				timeout.Reset(dist)
				return
			}
			// Remove pending replies whose deadline is too far in the
			// future. These can occur if the system clock jumped
			// backwards after the deadline was assigned.
			nextTimeout.errc <- errClockWarp
			plist.Remove(el)
		}
		nextTimeout = nil
		timeout.Stop()
	}

	for {
		resetTimeout()

		select {
		case <-t.closing:
			for el := plist.Front(); el != nil; el = el.Next() {
				el.Value.(*replyMatcher).errc <- errClosed
			}
			return

		case p := <-t.addReplyMatcher:
			p.deadline = time.Now().Add(respTimeout)
			plist.PushBack(p)

		case r := <-t.gotreply:
			var matched bool // whether any replyMatcher considered the reply acceptable.
			for el := plist.Front(); el != nil; el = el.Next() {
				p := el.Value.(*replyMatcher)
				if p.from == r.from && p.ptype == r.ptype && p.ip.Equal(r.ip) {
					ok, requestDone := p.callback(r.data)
					matched = matched || ok
					// Remove the matcher if callback indicates that all replies have been received.
					if requestDone {
						p.errc <- nil
						plist.Remove(el)
					}
					// Reset the continuous timeout counter (time drift detection)
					contTimeouts = 0
				}
			}
			r.matched <- matched

		case now := <-timeout.C:
			nextTimeout = nil

			// Notify and remove callbacks whose deadline is in the past.
			for el := plist.Front(); el != nil; el = el.Next() {
				p := el.Value.(*replyMatcher)
				if now.After(p.deadline) || now.Equal(p.deadline) {
					p.errc <- errTimeout
					plist.Remove(el)
					contTimeouts++
				}
			}
			// If we've accumulated too many timeouts, do an NTP time sync check
			if contTimeouts > ntpFailureThreshold {
				if time.Since(ntpWarnTime) >= ntpWarningCooldown {
					ntpWarnTime = time.Now()
					go checkClockDrift()
				}
				contTimeouts = 0
			}
		}
	}
}

const (
	macSize  = 256 / 8
	sigSize  = 520 / 8
	headSize = macSize + sigSize // space of packet frame data
)

var (
	headSpace = make([]byte, headSize)

	// Neighbors replies are sent across multiple packets to
	// stay below the packet size limit. We compute the maximum number
	// of entries by stuffing a packet until it grows too large.
	maxNeighbors int
)

func init() {
	p := neighbors{Expiration: ^uint64(0)}
	maxSizeNode := rpcNode{IP: make(net.IP, 16), UDP: ^uint16(0), TCP: ^uint16(0)}
	for n := 0; ; n++ {
		p.Nodes = append(p.Nodes, maxSizeNode)
		size, _, err := rlp.EncodeToReader(p)
		if err != nil {
			// If this ever happens, it will be caught by the unit tests.
			panic("cannot encode: " + err.Error())
		}
		if headSize+size+1 >= maxPacketSize {
			maxNeighbors = n
			break
		}
	}
}

func (t *udp) send(toaddr *net.UDPAddr, toid enode.ID, ptype byte, req packet) ([]byte, error) {
	packet, hash, err := encodePacket(t.priv, ptype, req)
	if err != nil {
		return hash, err
	}
	return hash, t.write(toaddr, toid, req.name(), packet)
}

func (t *udp) write(toaddr *net.UDPAddr, toid enode.ID, what string, packet []byte) error {
	_, err := t.conn.WriteToUDP(packet, toaddr)
	log.Trace(">> "+what, "id", toid, "addr", toaddr, "err", err)
	return err
}

func encodePacket(priv *ecdsa.PrivateKey, ptype byte, req interface{}) (packet, hash []byte, err error) {
	b := new(bytes.Buffer)
	b.Write(headSpace)
	b.WriteByte(ptype)
	if err := rlp.Encode(b, req); err != nil {
		log.Error("Can't encode discv4 packet", "err", err)
		return nil, nil, err
	}
	packet = b.Bytes()
	sig, err := crypto.Sign(crypto.Keccak256(packet[headSize:]), priv)
	if err != nil {
		log.Error("Can't sign discv4 packet", "err", err)
		return nil, nil, err
	}
	copy(packet[macSize:], sig)
	// add the hash to the front. Note: this doesn't protect the
	// packet in any way. Our public key will be part of this hash in
	// The future.
	hash = crypto.Keccak256(packet[macSize:])
	copy(packet, hash)
	return packet, hash, nil
}

// readLoop runs in its own goroutine. it handles incoming UDP packets.
func (t *udp) readLoop(unhandled chan<- ReadPacket) {
	defer t.wg.Done()
	if unhandled != nil {
		defer close(unhandled)
	}

	buf := make([]byte, maxPacketSize)
	for {
		nbytes, from, err := t.conn.ReadFromUDP(buf)
		if netutil.IsTemporaryError(err) {
			// Ignore temporary read errors.
			log.Debug("Temporary UDP read error", "err", err)
			continue
		} else if err != nil {
			// Shut down the loop for permament errors.
			log.Debug("UDP read error", "err", err)
			return
		}
		if t.handlePacket(from, buf[:nbytes]) != nil && unhandled != nil {
			select {
			case unhandled <- ReadPacket{buf[:nbytes], from}:
			default:
			}
		}
	}
}

func (t *udp) handlePacket(from *net.UDPAddr, buf []byte) error {
	packet, fromKey, hash, err := decodePacket(buf)
	if err != nil {
		log.Debug("Bad discv4 packet", "addr", from, "err", err)
		return err
	}
	fromID := fromKey.id()
	if err == nil {
		err = packet.preverify(t, from, fromID, fromKey)
	}
	log.Trace("<< "+packet.name(), "id", fromID, "addr", from, "err", err)
	if err == nil {
		packet.handle(t, from, fromID, hash)
	}
	return err
}

func decodePacket(buf []byte) (packet, encPubkey, []byte, error) {
	if len(buf) < headSize+1 {
		return nil, encPubkey{}, nil, errPacketTooSmall
	}
	hash, sig, sigdata := buf[:macSize], buf[macSize:headSize], buf[headSize:]
	shouldhash := crypto.Keccak256(buf[macSize:])
	if !bytes.Equal(hash, shouldhash) {
		return nil, encPubkey{}, nil, errBadHash
	}
	fromKey, err := recoverNodeKey(crypto.Keccak256(buf[headSize:]), sig)
	if err != nil {
		return nil, fromKey, hash, err
	}

	var req packet
	switch ptype := sigdata[0]; ptype {
	case pingPacket:
		req = new(ping)
	case pongPacket:
		req = new(pong)
	case findnodePacket:
		req = new(findnode)
	case neighborsPacket:
		req = new(neighbors)
	default:
		return nil, fromKey, hash, fmt.Errorf("unknown type: %d", ptype)
	}
	s := rlp.NewStream(bytes.NewReader(sigdata[1:]), 0)
	err = s.Decode(req)
	return req, fromKey, hash, err
}

// Packet Handlers

func (req *ping) preverify(t *udp, from *net.UDPAddr, fromID enode.ID, fromKey encPubkey) error {
	if expired(req.Expiration) {
		return errExpired
	}
	key, err := decodePubkey(fromKey)
	if err != nil {
		return errors.New("invalid public key")
	}
	req.senderKey = key
	return nil
}

func (req *ping) handle(t *udp, from *net.UDPAddr, fromID enode.ID, mac []byte) {
	// Reply.
	t.send(from, fromID, pongPacket, &pong{
		To:         makeEndpoint(from, req.From.TCP),
		ReplyTok:   mac,
		Expiration: uint64(time.Now().Add(expiration).Unix()),
	})

	// Ping back if our last pong on file is too far in the past.
	n := wrapNode(enode.NewV4(req.senderKey, from.IP, int(req.From.TCP), from.Port))
	if time.Since(t.db.LastPongReceived(n.ID(), from.IP)) > bondExpiration {
		t.sendPing(fromID, from, func() {
			t.tab.addVerifiedNode(n)
		})
	} else {
		t.tab.addVerifiedNode(n)
	}

	// Update node database and endpoint predictor.
	t.db.UpdateLastPingReceived(n.ID(), from.IP, time.Now())
	t.localNode.UDPEndpointStatement(from, &net.UDPAddr{IP: req.To.IP, Port: int(req.To.UDP)})
}

func (req *ping) name() string { return "PING/v4" }

func (req *pong) preverify(t *udp, from *net.UDPAddr, fromID enode.ID, fromKey encPubkey) error {
	if expired(req.Expiration) {
		return errExpired
	}
	if !t.handleReply(fromID, from.IP, pongPacket, req) {
		return errUnsolicitedReply
	}
	return nil
}

func (req *pong) handle(t *udp, from *net.UDPAddr, fromID enode.ID, mac []byte) {
	t.localNode.UDPEndpointStatement(from, &net.UDPAddr{IP: req.To.IP, Port: int(req.To.UDP)})
	t.db.UpdateLastPongReceived(fromID, from.IP, time.Now())
}

func (req *pong) name() string { return "PONG/v4" }

func (req *findnode) preverify(t *udp, from *net.UDPAddr, fromID enode.ID, fromKey encPubkey) error {
	if expired(req.Expiration) {
		return errExpired
	}
	if time.Since(t.db.LastPongReceived(fromID, from.IP)) > bondExpiration {
		// No endpoint proof pong exists, we don't process the packet. This prevents an
		// attack vector where the discovery protocol could be used to amplify traffic in a
		// DDOS attack. A malicious actor would send a findnode request with the IP address
		// and UDP port of the target as the source address. The recipient of the findnode
		// packet would then send a neighbors packet (which is a much bigger packet than
		// findnode) to the victim.
		return errUnknownNode
	}
	return nil
}

func (req *findnode) handle(t *udp, from *net.UDPAddr, fromID enode.ID, mac []byte) {
	// Determine closest nodes.
	target := ID(crypto.Keccak256Hash(req.Target[:]))
	t.tab.mutex.Lock()
	closest := t.tab.closest(target, bucketSize).entries
	t.tab.mutex.Unlock()

	// Send neighbors in chunks with at most maxNeighbors per packet
	// to stay below the packet size limit.
	p := neighbors{Expiration: uint64(time.Now().Add(expiration).Unix())}
	var sent bool
	for _, n := range closest {
		if netutil.CheckRelayIP(from.IP, n.IP()) == nil {
			p.Nodes = append(p.Nodes, nodeToRPC(n))
		}
		if len(p.Nodes) == maxNeighbors {
			t.send(from, fromID, neighborsPacket, &p)
			p.Nodes = p.Nodes[:0]
			sent = true
		}
	}
	if len(p.Nodes) > 0 || !sent {
		t.send(from, fromID, neighborsPacket, &p)
	}
}

func (req *findnode) name() string { return "FINDNODE/v4" }

func (req *neighbors) preverify(t *udp, from *net.UDPAddr, fromID enode.ID, fromKey encPubkey) error {
	if expired(req.Expiration) {
		return errExpired
	}
	if !t.handleReply(fromID, from.IP, neighborsPacket, req) {
		return errUnsolicitedReply
	}
	return nil
}

func (req *neighbors) handle(t *udp, from *net.UDPAddr, fromID enode.ID, mac []byte) {
}

func (req *neighbors) name() string { return "NEIGHBORS/v4" }

func expired(ts uint64) bool {
	return time.Unix(int64(ts), 0).Before(time.Now())
}
*/