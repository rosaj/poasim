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



type Message struct {
	godes.Runner
	Type    string
	Content interface{}
	From    *Node
	To      *Node

	Expiration float64
	//TODO: poruka ima velicinu i latencija ovisi o velicini

	latency    float64
	handler    func()
	onResponse func(m *Message, err error)

	responseTo *Message

	responseTimeout float64
}

func (m *Message) Run() {

	m.logSent()

	if !m.From.IsOnline() {
		m.handleError(errNodeOffline)
		return
	}

	m.From.MarkMessageSend(m)

	m.latency = config.SimConfig.NextNetworkLatency()

	godes.Advance(m.latency)

	if m.To.IsOnline() {

		m.logReceived()

		m.handle()

		m.responded()

		m.To.MarkMessageReceived(m)
	} else {
		m.handleError(errTimeout)
	}

}


func newMessage(
	from *Node,
	to *Node,
	msgType string,
	content interface{},
	expiration float64,
	handler func(),
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
	godes.AddRunner(m)
}

func (m *Message) handle() {
	if m.handler != nil {
		m.handler()
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
		util.Log("Message", m, "- error:",err, "in", m.responseTimeout, "sec, Sender online:",m.From.IsOnline(), "Receiver online:", m.To.IsOnline())
	}
}
