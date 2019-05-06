package network

import (
	"fmt"
)

// Protocol represents a P2P subprotocol implementation.
type Protocol struct {
	// Name should contain the official protocol name,
	// often a three-letter word.
	Name string

	// Version should contain the version number of the protocol.
	Version uint

	// Length should contain the number of message codes used
	// by the protocol.
	Length uint64

	// Run is called in a new goroutine when the protocol has been
	// negotiated with a ethPeer. It should read and write messages from
	// rw. The Payload for each message must be fully consumed.
	//
	// The ethPeer connection is closed when Start returns. It should return
	// any protocol-level error (such as an I/O error) that is
	// encountered.
	Run func(peer *Peer) error

	Close func(peer *Peer)
/*
	// NodeInfo is an optional helper method to retrieve protocol specific metadata
	// about the host node.
	NodeInfo func() interface{}

	// PeerInfo is an optional helper method to retrieve protocol specific metadata
	// about a certain ethPeer in the network. If an info retrieval function is set,
	// but returns nil, it is assumed that the protocol handshake is still running.
	PeerInfo func(id enode.ID) interface{}

	// Attributes contains protocol specific information for the node record.
	Attributes []enr.Entry
	*/
}


func (p *Protocol) String() string {
	return fmt.Sprintf("%s/%d", p.Name, p.Version)
}
