package message

import (
	. "../../common"
	"../../config"
	"../../metrics"
	"../../util"
	"errors"
	"fmt"
	"github.com/agoussia/godes"
)



var (
	ErrMsgTimeout  = errors.New("msg timeout")
	ErrNodeOffline = errors.New("node offline")
	ErrPacketLost  = errors.New("packet lost")
)

var (
	PING		= metrics.PING
	PONG		= metrics.PONG
	FINDNODE	= metrics.FINDNODE
	NEIGHBORS	= metrics.NEIGHBORS


	DEVP2P_HANDSHAKE 	= metrics.DEVP2P_HANDSHAKE
	DEVP2P_PING			= metrics.DEVP2P_PING
	DEVP2P_PONG			= metrics.PONG


	// eth protocol message codes

	STATUS_MSG				= metrics.STATUS_MSG
	NEW_BLOCK_HASHES_MSG	= metrics.NEW_BLOCK_HASHES_MSG
	TX_MSG					= metrics.TX_MSG
	GET_BLOCK_HEADERS_MSG	= metrics.GET_BLOCK_HEADERS_MSG
	BLOCK_HEADERS_MSG		= metrics.BLOCK_HEADERS_MSG
	NEW_BLOCK_MSG			= metrics.NEW_BLOCK_MSG

	)



type Message struct {
	godes.Runner
	Type    string
	Content interface{}
	From    INode
	To      INode

	Expiration float64
	//TODO: poruka ima velicinu i latencija ovisi o velicini

	latency    float64
	handler    func(m *Message)
	onResponse func(m *Message, err error)

	ResponseTo *Message

	responseTimeout float64
}

func (m *Message) Run() {

	m.logSent()

//	m.From.MarkMessageSend(m)
	m.From.Update(m.GetType())

	m.latency = config.SimConfig.NextNetworkLatency()

	godes.Advance(m.latency)

	if m.To.IsOnline() {
		m.logReceived()

		m.handle()

		m.responded()

//		m.To.MarkMessageReceived(m)
	} else {
		m.handleError(ErrMsgTimeout)
		//m.Type = "RECIEVE_ERR"
		//m.To.MarkMessageReceived(m)

	}

}


func NewMessage(
	from INode,
	to INode,
	msgType string,
	content interface{},
	expiration float64,
	handler func(m *Message),
	responseTo *Message,
	onResponse func(m *Message, err error),
	responseTimeout float64) (m *Message) {

	m = &Message{
		Runner:          godes.Runner{},
		Type:            msgType,
		Content:         content,
		From:            from,
		To:              to,
		Expiration:      expiration,
		handler:         handler,
		onResponse:      onResponse,
		ResponseTo:      responseTo,
		responseTimeout: responseTimeout,
	}

	return
}

func (m *Message) Send() {

	if !m.From.IsOnline() {
		m.handleError(ErrNodeOffline)


	//	m.Type = "SEND_ERR"
	//	m.From.MarkMessageSend(m)

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

	if m.ResponseTo != nil && m.ResponseTo.onResponse != nil {

		if config.LogConfig.LogMessages {
			util.Log("Responded to msg:", m.ResponseTo, "with:", m)
		}

		m.ResponseTo.onResponse(m, nil)
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
func (m *Message) GetType() string {
	return m.Type
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
