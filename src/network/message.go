package network

import (
	"../config"
	"../util"
	"errors"
	"fmt"
	"github.com/agoussia/godes"
)



var (
	errMsgTimeout          = errors.New("msg timeout")
	errNodeOffline		   = errors.New("node offline")
	errPacketLost		   = errors.New("packet lost")
)

var (
	PING		= "PING"
	PONG		= "PONG"
	FINDNODE	= "FINDNODE"
	NEIGHBORS	= "NEIGHBORS"


	DEVP2P_PING	= "DEVP2P_PING"
	DEVP2P_PONG	= "DEVP2P_PING"


	// eth protocol message codes

	// Protocol messages belonging to eth/62
	STATUS_MSG				= "StatusMsg"
	NEW_BLOCK_HASHES_MSG	= "NewBlockHashesMsg"
	TX_MSG					= "TxMsg"
	GET_BLOCK_HEADERS_MSG	= "GetBlockHeadersMsg"
	BLOCK_HEADERS_MSG		= "BlockHeadersMsg"
	GET_BLOCK_BODIES_MSG	= "	GetBlockBodiesMsg"
	BLOCK_BODIES_MSG		= "BlockBodiesMsg"
	NEW_BLOCK_MSG			= "NewBlockMsg"

	// Protocol messages belonging to eth/63
	GET_NODE_DATA_MSG		= "GetNodeDataMsg"
	NODE_DATA_MSG			= "NodeDataMsg"
	GET_RECEIPTS_MSG		= "GetReceiptsMsg"
	RECEIPTS_MSG			= "ReceiptsMsg"

	)



type Message struct {
	godes.Runner
	Type    string
	Content interface{}
	From    *Node
	To      *Node

	Expiration float64
	//TODO: poruka ima velicinu i latencija ovisi o velicini

	latency    float64
	handler    func(m *Message)
	onResponse func(m *Message, err error)

	responseTo *Message

	responseTimeout float64
}

func (m *Message) Run() {

	m.logSent()

	m.From.MarkMessageSend(m)

	m.latency = config.SimConfig.NextNetworkLatency()

	godes.Advance(m.latency)

	if m.To.IsOnline() {
		m.logReceived()

		m.handle()

		m.responded()

		m.To.MarkMessageReceived(m)
	} else {
		m.handleError(errMsgTimeout)
		m.Type = "RECIEVE_ERR"
		m.To.MarkMessageReceived(m)

	}

}


func newMessage(
	from *Node,
	to *Node,
	msgType string,
	content interface{},
	expiration float64,
	handler func(m *Message),
	responseTo *Message,
	onResponse func(m *Message, err error),
	responseTimeout float64) (m *Message) {

	m = &Message{
		Runner: godes.Runner{},
		Type: msgType,
		Content: content,
		From: from,
		To: to,
		Expiration: expiration,
		handler: handler,
		onResponse: onResponse,
		responseTo: responseTo,
		responseTimeout: responseTimeout,
	}

	return
}

func (m *Message) send() {

	if !m.From.IsOnline() {
		m.handleError(errNodeOffline)

		m.Type = "SEND_ERR"
		m.From.MarkMessageSend(m)

		return
	}

	godes.AddRunner(m)
}

func (m *Message) handle() {
	if m.handler != nil {
		m.handler(m)
	}
}


func (m *Message) handleError(err error)  {
	// ako je latencija veca od timeouta onda bi razlika bila negativna
	if m.responseTimeout > 0 && m.responseTimeout > m.latency {
		// vec je pozvana advance metoda za latency
		godes.Advance(m.responseTimeout - m.latency)
	}

	m.logError(err)

	if m.onResponse != nil {
		m.onResponse(nil, err)
	}

}

func (m *Message) responded() {

	if m.responseTo != nil && m.responseTo.onResponse != nil {

		if config.LogConfig.LogMessages {
			util.Log("Responded to msg:", m.responseTo, "with:", m)
		}

		m.responseTo.onResponse(m, nil)
	}

}
func (m *Message) String() string {
	return fmt.Sprintf("[%s] from %s to %s", m.Type, m.From.Name(), m.To.Name())
}
func (m *Message) HasExpired() (expired bool) {
	expired = m.Expiration < godes.GetSystemTime()
	return
}

func (m *Message) GetLatency() float64 {
	return m.latency
}

func (m *Message) logSent() {
	if config.LogConfig.LogMessages {
		util.Log("Send message:", m)
	}
}

func (m *Message) logReceived() {
	if config.LogConfig.LogMessages {
		util.Log("Message", m.Type, "received:", m.String(), "in", m.latency, "sec")
	}
}

func (m *Message) logError(err error) {
	if config.LogConfig.LogMessages {
		util.Log("Message", m, "- error:",err, "in", m.responseTimeout, "sec, Sender online:", m.From.IsOnline(), "Receiver online:", m.To.IsOnline())
	}
}
