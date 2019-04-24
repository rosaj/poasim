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
	From Node
	To Node

	handler func()
}

func (m *Message) Run()  {
	util.Log("Send message: ", m)
	// umjesto advance napravit wait
	// jer more na vise nodova poslat vise poruka
	// i onda se samo vrime pomeri napred i zajno salje response
	// i ako ima prvi veci delaj, svejedno ce se objekt promjenit prije nego od onega koji ima manji delaj
	// godes.Advance(1)
	util.Wait(config.SimConfig.NextNodeInterval())
	util.Log("Message ", m.Type, " received: ", m.String())
	m.handler()
}

func newMessage(from Node, to Node, msgType string, content interface{}) (m *Message){
	m = &Message{}
	m.Runner = godes.Runner{}
	m.Type = msgType
	m.Content = content
	m.From = from
	m.To = to

	return
}

func NewPingMessage(from Node, to Node) (m *Message) {
	m = newMessage(from, to, "PING", "ping")
	m.handler = func() {
		m.To.Pong(&m.From)
	}
	return
}

func NewPongMessage(from Node, to Node) (m *Message)  {
	m = newMessage(from, to, "PONG", "pong")
	m.handler = func() {
		//m.To.Pong(&m.From)
		//TODO: what on pong
	}
	return
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

func (m *Message) String() string {
	return fmt.Sprintf("[%s] from %s to %s", m.Type, m.From.Name(), m.To.Name())
}
