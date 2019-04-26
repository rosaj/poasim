package network

import (
	"crypto/ecdsa"
	"github.com/agoussia/godes"
	"strconv"
)

type INode interface {
	HasMessage()
	AddMessage()
	Run()
}

type RunFunction func(*Node)
// ID is a unique identifier for each node.
type ID [32]byte

type encPubkey [64]byte


type Node struct {
	*godes.Runner

	name string

	publicKey ecdsa.PublicKey

	id             ID

//	tab		       Table

	queue          *godes.FIFOQueue

	runFunction    RunFunction
	TotalMessages  int
	addedAt        float64
	livenessChecks uint
}


var nodeCounter = 1

func NewNode(fn RunFunction) (n* Node) {
	n = new(Node)
	n.Runner = &godes.Runner{}
	n.name = string(strconv.Itoa(nodeCounter))
//	newUDP(n, nil)
	n.publicKey = NewKey().PublicKey

	copy(n.id[:], PublicKeyToId(n.publicKey))

	n.queue = godes.NewFIFOQueue("messages")
	//n.peers = peers
	n.runFunction = fn
	//nodes[id] = n

//	fmt.Printf("Added node %s with peers \n", n.name)//%+v

	nodeCounter += 1
	return
}


func (n *Node) Name() string {
	return n.name
}
func (n *Node) PublicKey() ecdsa.PublicKey{
	return n.publicKey
}

func (n *Node) ID() ID {
	return n.id
}

func (n *Node) Run() {
	//fmt.Println("Running...")

	for {
		n.runFunction(n)
	}
}



/*

func (n *Node) FindNode(node *Node, pubkey encPubkey)  {
	msg := NewFindNodeMessage(*n, *node, pubkey)
	msg.send()
}
func (n *Node) Neighbors(node *Node, nodes *nodesByDistance)  {
	msg := NewNeighborsMessage(*n, *node, *nodes)
	msg.send()
}*/

/*
func (n *Node) HandleMessage(msg Message)  {
	util.Log("Message ", msg.Type, " received: ", msg.String())

	if msg.Type == "PING"{
		n.Pong(&msg.From)
	}
	if msg.Type == "PONG"{
	}

	if msg.Type == "FINDNODE"{
		pubKey := msg.Content.(*encPubkey)
		target := ID(crypto.Keccak256Hash(pubKey[:]))
		n.Neighbors(&msg.From, *n.tab.closest(target, bucketSize))
	}
	if msg.Type == "NEIGHBORS"{
		util.Log(msg.Content.(*nodesByDistance).entries)
	}
}
*/


