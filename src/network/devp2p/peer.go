
package devp2p

import (
	. "../../common"
	. "../../config"
	. "../../network/message"
	. "../../util"
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

const handshakeTimeout  = 5 * time.Second

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


// Peer represents a connected remote INode.
type Peer struct {
	*godes.Runner

	server *Server

	node	INode

	quit 	bool

	flags connFlag

}



func NewPeer(server *Server, node INode) *Peer {
	return &Peer{
		Runner: &godes.Runner{},
		server: server,
		node: node,
	}
}

func (p *Peer) Server() *Server  {
	return p.server
}
func (p *Peer) Self() INode {
	return p.server.Self()
}
// ID returns the INode's public key.
func (p *Peer) ID() ID {
	return p.Node().ID()
}

// Name returns the INode name that the remote INode advertised.
func (p *Peer) Name() string {
	return p.Node().Name()
}

func (p *Peer) Node() INode {
	return p.node
}


// Disconnect terminates the peer connection with the given reason.
// It returns immediately and does not wait until the connection is closed.
func (p *Peer) Disconnect(reason error) {
	p.Close(reason)
}

// String implements fmt.Stringer.
func (p *Peer) String() string {
	return fmt.Sprintf("Peer %s", p.Name())
}

// Inbound returns true if the peer is an inbound connection
func (p *Peer) Inbound() bool {
	return p.is(inboundConn)
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

		p.SendPingMsg()
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


func (p *Peer) Close(reason error)  {

	if p.quit {
		return
	}

	p.Log("Closing peer for error", reason)
	p.quit = true
	p.Server().DeletePeer(p)

	p.closeProtocols()
}

func (p *Peer) IsClosed() bool {
	return p.quit
}


func (p *Peer) HandleError(err error) bool {

	if err != nil {
		p.Close(err)
		return false
	}
	return true
}
// proto handshake salje handshake poruku peer-u asinkrono
// i istovremeno ceka na handshake poruku od peer-a
// to znaci da oba peer-a moraju cekat poruku prije nego je bilo koji od njih 2 posalje
// jer ako peer posalje handshake poruku a ovaj drugi jos ni pocea citat handshake poruku
// drugi peer nece nikad zaprimit handshake poruku ovog prvog
func (p *Peer) doProtoHandshake(onHandshake func(err error)) {
	p.sendHandshakeMsg(onHandshake)
}

func (p *Peer) sendHandshakeMsg(onHandshake func(err error))  {
	p.newHandshakeMsg(onHandshake).Send()
}

func (p *Peer) newHandshakeMsg(onHandshake func(err error)) *Message {

	return NewMessage(p.Self(), p.Node(), DEVP2P_HANDSHAKE, handshakeMsg, 0,
			func(m *Message) {
				handshakePeer := p.Node().Server().RetrieveHandshakePeer(p.Self())

				if handshakePeer != nil {
					onHandshake(nil)
				} else {
					onHandshake(DiscReadTimeout)
				}

			}, nil, func(m *Message, err error) {
				p.Node().Server().RetrieveHandshakePeer(p.Self())
				onHandshake(err)

		},handshakeTimeout.Seconds())

}


func (p *Peer) NewMsg(to INode, msgType string, content interface{}, responseTo *Message, handler func(m *Message))  *Message {

	return NewMessage(p.Self(), to, msgType, content, 0,
			handler, responseTo,
			func(m *Message, err error) {
				// bilo koji error gasi peer-a
				p.HandleError(err)

			}, 0)

}

func (p *Peer) newPingMsg(to INode) *Message {
	return p.NewMsg(to, DEVP2P_PING, pingMsg, nil,
			func(m *Message) {
				// ako INode sa druge strane takoder ima peer prema nama
				// onda posalji pong messasge
				peer := to.Server().FindPeer(p.Self())

				//interface ni nikad null pa se treba provjerit underlying struct
				if peer.(*Peer) != nil {
					peer.SendPongMsg(m)
				} else {
					p.HandleError(DiscRequested)
				}

			})

}

func (p *Peer) newPongMsg(to INode, responseTo *Message) (m *Message) {
	m = p.NewMsg(to, DEVP2P_PONG, pongMsg, responseTo,nil)
	return
}

func (p *Peer) SendPingMsg()  {
	p.newPingMsg(p.Node()).Send()
}

func (p *Peer) SendPongMsg(pingMsg IMessage)  {
	msg := pingMsg.(*Message)
	p.newPongMsg(p.Node(), msg).Send()
}


func (p *Peer) Log(a ...interface{}) {
	if LogConfig.LogPeer {
		Log(p.Self(), p, a)
	}
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
