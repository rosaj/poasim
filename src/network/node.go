package network

import (
	. "../common"
	. "../config"
	"../metrics"
	"../network/devp2p"
	"../network/discovery"
	"../network/eth"
	"../network/eth/common"
	"../util"
	"crypto/ecdsa"
	"github.com/agoussia/godes"
	"github.com/ethereum/go-ethereum/crypto"
	"math"
	"strconv"
)

var(
	OnlineNodes = metrics.OnlineNodes
)

var onlineCounter = 0

var nodeStats = GlobalMetricCollector

func GetNodeStats() map[float64]float64 {
	return nodeStats.Collect(OnlineNodes)
}

func nodeCountChanged(arrival bool)  {
	if arrival {
		onlineCounter += 1
	} else {
		onlineCounter -= 1
	}

	nodeStats.Set(OnlineNodes, onlineCounter)
}

type NodeConfig struct {
	*EthereumConfig

	MaxPeers 		int

	DialRatio 		int

	BootstrapNodes 	[]*Node

	NetworkID		int

	Protocols		[]string


}



type Node struct {
	*godes.Runner
	IMetricCollector

	*NodeConfig

	name            string
	online          bool
	lifetime        float64
	isBootstrapNode bool

	privateKey *ecdsa.PrivateKey
	publicKey *ecdsa.PublicKey
	address 		common.Address

	id ID

	udp    IUdp
	tab    IDiscoveryTable
	server IServer

	msgSent     map[string][]Msg
	msgReceived map[string][]Msg

}


type Msg struct {
	Time float64
	Size float64
}

var nodeCounter = 1

func NewBootstrapNode(nodeConfig *NodeConfig) (n *Node) {
	bNode := NewNode(nodeConfig)
	bNode.isBootstrapNode = true
	bNode.name = "BN_" + bNode.name
	return bNode
}


func NewNode(nodeConfig *NodeConfig) (n* Node) {
	n = new(Node)
	n.Runner = &godes.Runner{}
	n.IMetricCollector = NewMetricCollector()

	n.NodeConfig = nodeConfig

	n.name = string(strconv.Itoa(nodeCounter))
	n.online = true
	n.lifetime = SimConfig.NextNodeLifetime()
	n.isBootstrapNode = false

	n.privateKey = NewKey()
	n.publicKey = &n.privateKey.PublicKey
	copy(n.id[:], PublicKeyToId(n.publicKey))

	address := crypto.PubkeyToAddress(*n.PublicKey())
	n.address = common.Address(address)


	n.msgReceived = make(map[string][]Msg)
	n.msgSent = make(map[string][]Msg)

	//n.runFunction = fn

	nodeCounter += 1
	return
}


func (n *Node) Name() string {
	return n.name
}

func (n *Node) PrivateKey() *ecdsa.PrivateKey{
	return n.privateKey
}

func (n *Node) PublicKey() *ecdsa.PublicKey{
	return n.publicKey
}



func (n *Node) ID() ID {
	return n.id
}


func (n *Node) Address() common.Address {
	return n.address
}


func (n *Node) GetMaxPeers() int {
	return n.MaxPeers
}

func (n *Node) GetDialRatio() int {
	return n.DialRatio
}

func (n *Node) GetBootstrapNodes() []INode {
	bNodes := make([]INode, 0)
	for _, bNode := range n.BootstrapNodes {
		bNodes = append(bNodes, bNode)
	}
	return bNodes
}

func (n *Node) GetNetworkID() int {
	return n.NetworkID
}


func (n *Node) GetProtocols() []string {
	return n.Protocols
}

func (n *Node) IsOnline() bool {
	return n.online
}
func (n *Node) Kill()  {
	n.setOnline(false)
	godes.Interrupt(n)
}

func (n *Node) GetDiscoveryTable() IDiscoveryTable {
	return n.tab
}
func (n *Node) GetUDP() IUdp {
	return n.udp
}

func (n *Node) GetConfig() *EthereumConfig {
	return n.EthereumConfig
}


func (n *Node) setOnline(online bool)  {
	n.online = online

	if n.tab != nil {
		n.tab.SetOnline(online)
	}

	if n.server != nil {
		n.server.SetOnline(online)
	}

	n.log("online:", online)

	nodeCountChanged(online)
}

/*

func (n *Node) MarkMessageSend(m IMessage){
	//n.Set(m.GetType(), 1)
	n.addMsg(m, n.msgSent)
}

func (n *Node) addMsg(msg IMessage, msgMap map[string][]Msg)  {
	t := MetricConfig.GetTimeGroup()
	//TODO: msg size
	msgMap[msg.GetType()] = append(msgMap[msg.GetType()], Msg{t, 1})

}

func (n *Node) GetMessagesSent() map[string][]Msg  {
	return n.msgSent
}



func (n *Node) MarkMessageReceived(m IMessage){
	n.addMsg(m, n.msgReceived)
}

func (n *Node) GetMessagesSent(msgType string) int {
	return n.msgSentCount[msgType]
}

func (n *Node) GetMessagesReceived(msgType string) int {
	return n.msgReceivedCount[msgType]
}


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

 */

func (n *Node) startP2P()  {
	n.tab, n.udp = discovery.NewUDP(n, n)
	n.log("P2P running")
}

func (n *Node) startServer()  {

	switch SimConfig.SimMode {

	case DISCOVERY:
		return

	case DEVp2p:
		n.server = devp2p.NewServer(n, n)

	case ETHEREUM:
		var err error
		n.server, err = eth.New(n, n)
		if err != nil {
			n.log(err)
			return
		}
	}


	n.server.Start()
	n.log("Started server")
}

func (n *Node) Server() IServer {
	return n.server
}


func (n *Node) doChurn()  {


	if !n.churn(false, SimConfig.NextNodeSessionTime()){
		return
	}


	if !n.churn(true, SimConfig.NextNodeIntersessionTime()){
		return
	}

	n.doChurn()
}

func (n *Node) churn(online bool, time float64) bool {

	untilEnd := SimConfig.SimulationTime - godes.GetSystemTime()

	if time + godes.GetSystemTime() > n.lifetime || untilEnd <= time {
		n.advanceToEnd()
		return false
	}

	godes.Advance(time)
	n.setOnline(online)

	return true
}

func (n *Node) advanceToEnd()  {

	untilEnd := SimConfig.SimulationTime - godes.GetSystemTime()

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
	if !SimConfig.SimulationEnded() {

		if SimConfig.ChurnEnabled && !n.isBootstrapNode {
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

	if !n.isBootstrapNode {
		n.startServer()
	}

	nodeCountChanged(true)

	n.waitForEnd()

	//n.runFunction(n)
}

func (n *Node) String() string {
	return n.Name()
}

func (n *Node) log(a ...interface{})  {

	if LogConfig.LogNode {
		util.Log(n, a)
	}

}