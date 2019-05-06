package network

import (
	"../config"
	"../util"
	"crypto/ecdsa"
	"github.com/agoussia/godes"
	"math"
	"strconv"
)


var onlineCounter = 0

var nodeStats = make(map[float64][]int)

func GetNodeStats() map[float64][]int {
	return nodeStats
}

func nodeCountChanged(arrival bool)  {
	if arrival {
		onlineCounter += 1
	} else {
		onlineCounter -= 1
	}

	t := config.MetricConfig.GetTimeGroup()
	nodeStats[t] = append(nodeStats[t], onlineCounter)

}



// ID is a unique identifier for each node.
type ID [32]byte

type encPubkey [64]byte


type Node struct {
	*godes.Runner

	name            string
	online          bool
	lifetime        float64
	isBootstrapNode bool

	publicKey *ecdsa.PublicKey

	id ID

	udp   	*udp
	tab   	*Table
	server	*Server


	bootstrapNodes []*Node
	addedAt        map[*Node]float64
	livenessChecks map[*Node]uint

	msgSent     map[string][]Msg
	msgReceived map[string][]Msg

}

type Msg struct {
	Time float64
	Size float64
}

var nodeCounter = 1

func NewBootstrapNode(bootstrapNodes []*Node) (n *Node) {
	bNode := NewNode(bootstrapNodes)
	bNode.isBootstrapNode = true
	bNode.name = "BN_" + bNode.name
	return bNode
}


func NewNode(bootstrapNodes []*Node) (n* Node) {
	n = new(Node)
	n.Runner = &godes.Runner{}
	n.name = string(strconv.Itoa(nodeCounter))
	n.online = true
	n.lifetime = config.SimConfig.NextNodeLifetime()
	n.isBootstrapNode = false

	n.publicKey = &NewKey().PublicKey
	copy(n.id[:], PublicKeyToId(n.publicKey))

	n.addedAt = make(map[*Node]float64)
	n.livenessChecks = make(map[*Node]uint)
	n.msgReceived = make(map[string][]Msg)
	n.msgSent = make(map[string][]Msg)

	//n.runFunction = fn
	n.bootstrapNodes = bootstrapNodes

	nodeCounter += 1
	return
}


func (n *Node) Name() string {
	return n.name
}
func (n *Node) PublicKey() *ecdsa.PublicKey{
	return n.publicKey
}

func (n *Node) ID() ID {
	return n.id
}

func (n *Node) IsOnline() bool {
	return n.online
}
func (n *Node) Kill()  {
	n.setOnline(false)
	godes.Interrupt(n)
}

func (n *Node) GetTableStats() map[float64]int {
	return n.tab.nodeStat
}

func (n *Node) setOnline(online bool)  {
	n.online = online
	n.tab.SetOnline(online)
	n.log("online:", online)

	nodeCountChanged(online)
}

func (n *Node) MarkMessageSend(m *Message){
	n.addMsg(m, n.msgSent)
}
func (n *Node) MarkMessageReceived(m *Message){
	n.addMsg(m, n.msgReceived)
}

func (n *Node) addMsg(msg *Message, msgMap map[string][]Msg)  {
	t := config.MetricConfig.GetTimeGroup()
	//TODO: msg size
	msgMap[msg.Type] = append(msgMap[msg.Type], Msg{t, 1})
}
/*
func (n *Node) GetMessagesSent(msgType string) int {
	return n.msgSentCount[msgType]
}

func (n *Node) GetMessagesReceived(msgType string) int {
	return n.msgReceivedCount[msgType]
}

 */

func (n *Node) GetTotalMessagesSent() int {
	return mapSum(n.msgSent)
}
func (n *Node) GetTotalMessagesReceived() int  {
	return mapSum(n.msgReceived)
}

func (n *Node) GetMessagesSent() map[string][]Msg  {
	return n.msgSent
}
func (n *Node) GetMessagesReceived() map[string][]Msg  {
	return n.msgReceived
}

func mapSum(data map[string][]Msg) int {
	var sum int

	for _, value := range data {
		sum += len(value)
	}

	return sum
}

func (n *Node) startP2P()  {
	n.tab, n.udp = newUDP(n)
	n.log("P2P running")
}

func (n *Node) startServer()  {
	n.server = NewServer(n)
	n.server.Start()
}

func (n *Node) doChurn()  {


	if !n.churn(false, config.SimConfig.NextNodeSessionTime()){
		return
	}


	if !n.churn(true, config.SimConfig.NextNodeIntersessionTime()){
		return
	}

	n.doChurn()
}

func (n *Node) churn(online bool, time float64) bool {

	untilEnd := config.SimConfig.SimulationTime - godes.GetSystemTime()

	if time + godes.GetSystemTime() > n.lifetime || untilEnd <= time {
		n.advanceToEnd()
		return false
	}

	godes.Advance(time)
	n.setOnline(online)

	return true
}

func (n *Node) advanceToEnd()  {

	untilEnd := config.SimConfig.SimulationTime - godes.GetSystemTime()

	if !n.isBootstrapNode {
		// advance do lifetime ili kraj simulacije, ovisi sto je krace
		godes.Advance(math.Min(n.lifetime, untilEnd))
	} else {
		godes.Advance(untilEnd)
	}

	n.setOnline(false)
}

func (n *Node) waitForEnd()  {
	// ako jos simulacija ni zavrsila, sto je moguce ako smo dodani nakon zavrsetka simulacije
	if !config.SimConfig.SimulationEnded() {

		if config.SimConfig.ChurnEnabled && !n.isBootstrapNode {
			n.doChurn()
		} else {

			n.advanceToEnd()
		}
	}

	n.log("Ended")
}

func (n *Node) Run() {
	n.log("Starting node")

	// kad se pokrene p2p ovdje je i dalje godes vrijeme 0
	n.startP2P()


	n.startServer()

	nodeCountChanged(true)

	n.waitForEnd()

	//n.runFunction(n)
}

func (n *Node) String() string {
	return n.Name()
}

func (n *Node) log(a ...interface{})  {

	if config.LogConfig.LogNode {
		util.Log(n, a)
	}

}