package network

import (
	//"../util"
	"crypto/ecdsa"
	"fmt"
	"github.com/agoussia/godes"
	//"github.com/ethereum/go-ethereum/crypto"
	"strconv"
	"time"
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
	addedAt        time.Time
	livenessChecks uint
}

var TotalMessages = 0
var nodes = make(map[int]*Node)
var uniform = godes.NewUniformDistr(true)

func GetNodeById(id int) (*Node) {
	return nodes[id]
}
var nodeCounter = 1

func NewNode(fn RunFunction) (n* Node) {
	n = new(Node)
	n.Runner = &godes.Runner{}
	n.name = string(strconv.Itoa(nodeCounter))
//	newUDP(n, nil)
	n.publicKey = newKey().PublicKey

	copy(n.id[:], publicKeyToId(n.publicKey))

	n.queue = godes.NewFIFOQueue("messages")
	//n.peers = peers
	n.runFunction = fn
	//nodes[id] = n

	fmt.Printf("Added node %s with peers \n", n.name)//%+v

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

/*
func (n *Node) GetPeers() ([]int) {
	return n.peers
}*/

func (n *Node) HasMessage() bool {
	return n.queue.Len() > 0
}
func (n *Node) PopMessage() (m *Message) {
	TotalMessages++
	n.TotalMessages++
	return n.queue.Get().(*Message)
}
func (n *Node) AddMessage(m *Message) {
	n.queue.Place(m)
}

func (n *Node) Run() {
	fmt.Println("Running...")

	for {
		n.runFunction(n)
	}
}


func (n *Node) Ping(node *Node){
	msg := NewPingMessage(*n, *node)
	msg.send()
}

func (n *Node) Pong(node *Node)  {
	msg := NewPongMessage(*n, *node)
	msg.send()
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


