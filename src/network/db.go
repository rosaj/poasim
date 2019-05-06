package network

import (
	"../config"
	"../util"
	"bytes"
	crand "crypto/rand"
)

type DB struct {
	fails map[ID]int
	lastPingReceived map[ID]float64
	lastPongReceived map[ID]float64
	lastLiveNodes []*Node
}




func newDB() *DB{

	return &DB{
		fails: make(map[ID]int),
		lastPingReceived: make(map[ID]float64),
		lastPongReceived: make(map[ID]float64),
	}
}




func (db *DB) FindFails(n *Node) int {
	return db.fails[n.ID()]
}
func (db *DB) UpdateFindFails(n *Node, fails int)  {
	db.fails[n.ID()] = fails
}

func (db *DB) UpdateLastPingReceived(n *Node, time float64)  {
	db.lastPingReceived[n.ID()] = time
}

func (db *DB) LastPingReceived(n *Node) float64 {
	return db.lastPingReceived[n.ID()]
}

func (db *DB) UpdateLastPongReceived(n *Node, time float64) ()  {
	db.lastPongReceived[n.ID()] = time
}

func (db *DB) LastPongReceived(n *Node) float64 {
	return db.lastPongReceived[n.ID()]
}



func findNode(nodes []*Node, id ID) *Node {
	var key = id[:]

	for _, n := range nodes {
		nId := n.ID()
		nodeKey := nId[:]

		if bytes.Compare(nodeKey, key) >= 0{
			return n
		}
	}
	return nil
}

// QuerySeeds retrieves random nodes to be used as potential seed nodes
// for bootstrapping.
func (db *DB) QuerySeeds(n int) []*Node {

	var (
		nodes = make([]*Node, 0, n)
		id    ID
	)

	if db.lastLiveNodes == nil {
		return nodes
	}

seek:
	for seeks := 0; len(nodes) < n && seeks < n*5; seeks++ {
		// Seek to a random entry. The first byte is incremented by a
		// random amount each time in order to increase the likelihood
		// of hitting all existing nodes in very small databases.
		ctr := id[0]
		crand.Read(id[:])
		id[0] = ctr + id[0]%16

		n := findNode(db.lastLiveNodes, id)

		if n == nil {
			id[0] = 0
			continue seek // iterator exhausted
		}

		for i := range nodes {
			if nodes[i].ID() == n.ID() {
				continue seek // duplicate
			}
		}
		nodes = append(nodes, n)
	}


	if config.LogConfig.LogDiscovery {
		util.Log("Queryed", len(nodes), "from", len(db.lastLiveNodes))
	}

	db.lastLiveNodes = nil
	return nodes

}
