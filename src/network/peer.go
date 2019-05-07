
package network

import (
	"../util"
	"errors"
	"fmt"
	"github.com/agoussia/godes"
	"sync/atomic"
	"time"
)

var (
	ErrShuttingDown = errors.New("shutting down")
)

const (
	baseProtocolVersion    = 5
	baseProtocolLength     = uint64(16)
	baseProtocolMaxMsgSize = 2 * 1024

	snappyProtocolVersion = 5

	pingInterval = 15 * time.Second
)

const (
	// devp2p message codes
	handshakeMsg = 0x00
	discMsg      = 0x01
	pingMsg      = 0x02
	pongMsg      = 0x03
)

type connFlag int32

const (
	dynDialedConn connFlag = 1 << iota
	staticDialedConn
	inboundConn
	trustedConn
)


type PeerEventType string

const (
	// PeerEventTypeAdd is the type of event emitted when a peer is added
	// to a p2p.Server
	PeerEventTypeAdd PeerEventType = "add"

	// PeerEventTypeDrop is the type of event emitted when a peer is
	// dropped from a p2p.Server
	PeerEventTypeDrop PeerEventType = "drop"

	// PeerEventTypeMsgSend is the type of event emitted when a
	// message is successfully sent to a peer
	PeerEventTypeMsgSend PeerEventType = "msgsend"

	// PeerEventTypeMsgRecv is the type of event emitted when a
	// message is received from a peer
	PeerEventTypeMsgRecv PeerEventType = "msgrecv"
)


// Peer represents a connected remote node.
type Peer struct {
	*godes.Runner
	server *Server

	node	*Node

	quit 	bool

	flags connFlag


}



func NewPeer(server *Server, node *Node) *Peer {
	return &Peer{
		Runner: &godes.Runner{},
		server: server,
		node: node,
	}
}

func (p *Peer) Server() *Server  {
	return p.server
}
func (p *Peer) self() *Node {
	return p.server.Self()
}
// ID returns the node's public key.
func (p *Peer) ID() ID {
	return p.node.ID()
}


// Name returns the node name that the remote node advertised.
func (p *Peer) Name() string {
	return p.node.Name()
}



// Disconnect terminates the peer connection with the given reason.
// It returns immediately and does not wait until the connection is closed.
func (p *Peer) Disconnect(reason DiscReason) {
	p.Close()
}

// String implements fmt.Stringer.
func (p *Peer) String() string {
	return fmt.Sprintf("Peer %s", p.Name())
}

// Inbound returns true if the peer is an inbound connection
func (p *Peer) Inbound() bool {
	return p.is(inboundConn)
}


func (p *Peer) Log(a ...interface{}) {
	util.Log(p, a)
}


func (p *Peer) run() {
 	godes.AddRunner(p)
}




func (p *Peer) Run()  {
	p.startProtocols()
	p.pingLoop()

}


func (p *Peer) pingLoop() {

	pingTime := pingInterval.Seconds()

	for {
		godes.Advance(pingTime)

		if p.quit {
			return
		}

		p.sendPingMsg()
	}

}

func (p *Peer) startProtocols()  {

	protocols := p.Server().Protocols
	if protocols != nil {
		for _, protocol := range protocols {
			protocol.Run(p)
		}
	}
}

func (p *Peer) closeProtocols()  {

	protocols := p.Server().Protocols

	if protocols != nil {
		for _, protocol := range protocols {
			protocol.Close(p)
		}
	}
}


func (p *Peer) Close()  {

	if p.quit {
		return
	}

	p.Log("Closing")
	p.quit = true
	p.Server().DeletePeer(p)

	p.closeProtocols()

	//TODO ostalo kad se peer gasi
}


func (p *Peer) handleError(err error) bool {

	if err != nil {
		p.Close()
		return false
	}
	return true
}


func (p *Peer) newMsg(to *Node, msgType string, content interface{}, responseTo *Message, handler func())  *Message {

	m := newMessage(p.self(), to, msgType, content, 0,
		handler, responseTo,
		func(m *Message, err error) {

			p.handleError(err)

		}, 0)

	return m
}

func (p *Peer) newPingMsg(to *Node) (m *Message) {
	m = p.newMsg(to, DEVP2P_PING, pingMsg, nil,
		func() {
			// ako node sa druge strane takoder ima peer prema nama
			// onda posalji pong messasge
			peer := to.server.FindPeer(p.self())

			if peer != nil {
				peer.sendPongMsg(m)
			}

		})

	return
}

func (p *Peer) newPongMsg(to *Node, responseTo *Message) (m *Message) {
	m = p.newMsg(to, DEVP2P_PONG, pongMsg, responseTo,nil)
	return
}

func (p *Peer) sendPingMsg()  {
	p.newPingMsg(p.node).send()
}

func (p *Peer) sendPongMsg(pingMsg *Message)  {
	p.newPongMsg(p.node, pingMsg).send()
}



func (c *Peer) is(f connFlag) bool {
	flags := connFlag(atomic.LoadInt32((*int32)(&c.flags)))
	return flags&f != 0
}

func (c *Peer) set(f connFlag, val bool) {
	for {
		oldFlags := connFlag(atomic.LoadInt32((*int32)(&c.flags)))
		flags := oldFlags
		if val {
			flags |= f
		} else {
			flags &= ^f
		}
		if atomic.CompareAndSwapInt32((*int32)(&c.flags), int32(oldFlags), int32(flags)) {
			return
		}
	}
}



func (f connFlag) String() string {
	s := ""
	if f&trustedConn != 0 {
		s += "-trusted"
	}
	if f&dynDialedConn != 0 {
		s += "-dyndial"
	}
	if f&staticDialedConn != 0 {
		s += "-staticdial"
	}
	if f&inboundConn != 0 {
		s += "-inbound"
	}
	if s != "" {
		s = s[1:]
	}
	return s
}
