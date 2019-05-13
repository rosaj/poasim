package common

import "crypto/ecdsa"

// ID is a unique identifier for each node.
type ID [32]byte


type INode interface {
	Name() string
	ID() ID
	PublicKey() *ecdsa.PublicKey
	IsOnline() bool
	GetDiscoveryTable() IDiscoverTable
	GetUDP() IUdp
	Server() IServer

	MarkMessageSend(m IMessage)
	MarkMessageReceived(m IMessage)


	GetMaxPeers() int
	GetDialRatio() int
	GetBootstrapNodes() []INode
	GetNetworkID() int
	GetProtocols() []string
}


type IMessage interface {
	Send()
	GetLatency() float64

	GetType() string
}

type IUdp interface {

}

type IDiscoverTable interface {
	Close()
	Resolve(INode) INode
	LookupRandom() []INode
	ReadRandomNodes([]INode) int

	SetOnline(online bool)
	GetTableStats() map[float64][]int
}

type IServer interface {
	Start()
	RetrieveHandshakePeer(node INode) IPeer
	FindPeer(node INode) IPeer
	GetProtocols() []IProtocol
	SetupPeerConn(connFlags int32, node INode) error
	SetOnline(online bool)

	Self() INode

	GetProtocolManager() IProtocolManager

	GetPeerStats() map[float64][]int
}

type IPeer interface {
	Name() string

	Self() INode
	Node() INode

	SendPingMsg()
	SendPongMsg(pingMsg IMessage)

	ID() ID
	Close(err error)
	Disconnect(err error)
	IsClosed() bool

	HandleError(err error) bool

	Log(a ...interface{})
}

type IProtocol interface {
	GetName() string
	Run(peer IPeer)
	Close(peer IPeer)
}

type IProtocolManager interface {
	FindPeer(node INode) IPeer
	RetrieveHandshakePeer(node INode) IPeer
	GetSubProtocols() []IProtocol

}