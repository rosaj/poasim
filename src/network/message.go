package network

import (
	"../config"
	"../util"
	"fmt"
	"github.com/agoussia/godes"
)

type Message struct {
	godes.Runner
	Type string
	Content interface{}
	From *Node
	To *Node

	handler func()
	onResponse func(m *Message)

	responseTo *Message
}

func (m *Message) Run()  {
	util.Log("Send message: ", m)
	godes.Advance(config.SimConfig.NextNetworkLatency())
	util.Log("Message ", m.Type, " received: ", m.String())

	m.handle()
	m.responded()
}


func newMessage(from *Node, to *Node, msgType string,
	content interface{}, handler func(), responseTo *Message, onResponse func(m *Message)) (m *Message){
	m = &Message{}
	m.Runner = godes.Runner{}
	m.Type = msgType
	m.Content = content
	m.From = from
	m.To = to
	m.handler = handler
	m.onResponse = onResponse
	m.responseTo = responseTo

	return
}

func NewPingMessage(from *Node, to *Node, onResponse func(m *Message)) (m *Message) {
	m = newMessage(from, to, "PING", "ping",
		func() {
		m.To.Pong(m.From, m)
	}, nil, onResponse)
	return
}

func NewPongMessage(from *Node, to *Node, responseTo *Message) (m *Message)  {
	m = newMessage(from, to, "PONG", "pong", nil, responseTo, nil)
	return
}

func (n *Node) Ping(node *Node){
	msg := NewPingMessage(n, node, func(m *Message) {

	})
	msg.send()
}

func (n *Node) Pong(node *Node, responseToPingMsg *Message)  {
	msg := NewPongMessage(n, node, responseToPingMsg)
	msg.send()
}

/*
func NewFindNodeMessage(from Node, to Node, pubkey encPubkey) (m *Message)  {
	m = newMessage(from, to, "FINDNODE", pubkey)
	m.handler = func() {
		m.To.Neighbors(&m.From, m.To.tab.Closest(pubkey))
	}
	return
}

func NewNeighborsMessage(from Node, to Node, nodes nodesByDistance) (m *Message)  {
	m = newMessage(from, to, "NEIGHBORS", nodes)
	m.handler = func() {
		//TODO: handle neighbors message
	}
	return
}
*/
func (m *Message) send(){
	godes.AddRunner(m)
}

func (m *Message) handle()  {
	if m.handler != nil {
		m.handler()
	}
}
func (m *Message) responded(){
	if m.responseTo != nil && m.responseTo.onResponse != nil{
		util.Log("Responded to msg: ", m.responseTo, " with: ", m)
		m.responseTo.onResponse(m)
	}
}
func (m *Message) String() string {
	return fmt.Sprintf("[%s] from %s to %s", m.Type, m.From.Name(), m.To.Name())
}

