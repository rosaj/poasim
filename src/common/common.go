package common

import (
	"../config"
	"../network/eth/common"
	"../network/eth/core/types"
	"crypto/ecdsa"
)

// ID is a unique identifier for each node.
type ID [32]byte


type INode interface {
	IMetricCollector

	Name() string
	ID() ID
	PublicKey() *ecdsa.PublicKey
	PrivateKey() *ecdsa.PrivateKey
	Address() common.Address
	IsOnline() bool
	GetDiscoveryTable() IDiscoveryTable
	GetUDP() IUdp
	Server() IServer

//	MarkMessageSend(m IMessage)
//	MarkMessageReceived(m IMessage)


	GetMaxPeers() int
	GetDialRatio() int
	GetBootNodes() []INode
	GetNetworkID() int
	GetProtocols() []string

	GetConfig() *config.EthereumConfig
}


type IMessage interface {
	Send()
	GetLatency() float64

	GetType() string
}

type IUdp interface {

}

type IDiscoveryTable interface {
	Close()
	Resolve(INode) INode
	LookupRandom() []INode
	ReadRandomNodes([]INode) int

	SetOnline(online bool)
}

type IServer interface {
	Start()
	Stop()
	RetrieveHandshakePeer(node INode) IPeer
	FindPeer(node INode) IPeer
	Peers() []INode
	GetProtocols() []IProtocol
	SetupPeerConn(connFlags int32, node INode) error
	SetOnline(online bool)

	Self() INode

	GetProtocolManager() IProtocolManager
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
	Start()
	Stop()
	FindPeer(node INode) IPeer
	RetrieveHandshakePeer(node INode) IPeer
	GetSubProtocols() []IProtocol

	AddTxs(txs types.Transactions) []error
	PendingTxCount() int

}

type IMetricCollector interface {
	Set(name string, value int)
	Update(name string)
	UpdateWithValue(name string, value int)
	Collect(name string) map[float64]float64
}