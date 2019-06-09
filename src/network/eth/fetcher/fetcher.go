// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package fetcher contains the block announcement based synchronisation.
package fetcher

import (
	. "../../../common"
	. "../../../config"
	. "../../../util"
	"../common"
	"../consensus"
	"../core/types"
	"errors"
	"fmt"
	"time"
)

const (
	arriveTimeout = 500 * time.Millisecond // Time allowance before an announced block is explicitly requested
	gatherSlack   = 100 * time.Millisecond // Interval used to collate almost-expired announces with fetches
	fetchTimeout  = 5 * time.Second        // Maximum allotted time to return an explicitly requested block
	maxUncleDist  = 7                      // Maximum allowed backward distance from the chain head
	maxQueueDist  = 32                     // Maximum allowed distance from the chain head to queue
	hashLimit     = 256                    // Maximum number of unique blocks a peer may have announced
	blockLimit    = 64                     // Maximum number of unique blocks a peer may have delivered
)

var (
	errTerminated = errors.New("terminated")
)

// blockRetrievalFn is a callback type for retrieving a block from the local chain.
type blockRetrievalFn func(common.Hash) *types.Block

// headerRequesterFn is a callback type for sending a header retrieval request.
type headerRequesterFn func(common.Hash) error

// bodyRequesterFn is a callback type for sending a body retrieval request.
type bodyRequesterFn func([]common.Hash) error

// headerVerifierFn is a callback type to verify a block's header for fast propagation.
type headerVerifierFn func(header *types.Header) error

// blockBroadcasterFn is a callback type for broadcasting a block to connected peers.
type blockBroadcasterFn func(block *types.Block, propagate bool)

// chainHeightFn is a callback type to retrieve the current chain height.
type chainHeightFn func() uint64

// chainInsertFn is a callback type to insert a batch of blocks into the local chain.
type chainInsertFn func(types.Blocks) (int, error)

// peerDropFn is a callback type for dropping a peer detected as malicious.
type peerDropFn func(id ID)


// Fetcher is responsible for accumulating block announcements from various peers
// and scheduling them for retrieval.
type Fetcher struct {
	name 		  string
	// Callbacks
	getBlock       blockRetrievalFn   // Retrieves a block from the local chain
	verifyHeader   headerVerifierFn   // Checks if a block's headers have a valid proof of work
	broadcastBlock blockBroadcasterFn // Broadcasts a block to connected peers
	chainHeight    chainHeightFn      // Retrieves the current chain's height
	insertChain    chainInsertFn      // Injects a batch of blocks into the chain
	dropPeer       peerDropFn         // Drops a peer for misbehaving

}

// New creates a block fetcher to retrieve blocks based on hash announcements.
func New(name string, getBlock blockRetrievalFn, verifyHeader headerVerifierFn, broadcastBlock blockBroadcasterFn, chainHeight chainHeightFn, insertChain chainInsertFn, dropPeer peerDropFn) *Fetcher {
	return &Fetcher{
		name: 			name,
		getBlock:       getBlock,
		verifyHeader:   verifyHeader,
		broadcastBlock: broadcastBlock,
		chainHeight:    chainHeight,
		insertChain:    insertChain,
		dropPeer:       dropPeer,
	}
}



// Notify announces the fetcher of the potential availability of a new block in
// the network.
func (f *Fetcher) Enqueue(peer IPeer, block *types.Block) error {
	err := f.insert(peer, block)
	if err != nil{
		f.log(err)
	}
	return err
}

// insert spawns a new goroutine to run a block insertion into the chain. If the
// block's number is at the same height as the current import phase, it updates
// the phase states accordingly.
func (f *Fetcher) insert(peer IPeer, block *types.Block) error {
	//hash := block.Hash()

	// Run the import on a new thread
	f.log("Importing propagated block", "number", block.Number(), "from peer",peer.Name(), "with", block.Transactions().Len(), "txs" )

	// If the parent's unknown, abort insertion
	parent := f.getBlock(block.ParentHash())
	if parent == nil {
		//f.log("Unknown parent of propagated block", "peer", peer.Name(), "number", block.Number(), "hash", hash, "parent", block.ParentHash())
		return errors.New(fmt.Sprintf("Unknown parent of propagated block number %d from peer %s ", block.Number(), peer.Name()))
	}
	// Quickly validate the header and propagate the block if it passes
	switch err := f.verifyHeader(block.Header()); err {
	case nil:
		// All ok, quickly propagate to our peers
		f.broadcastBlock(block, true)

	case consensus.ErrFutureBlock:
		// Weird future block, don't fail, but neither propagate

	default:
		// Something went very wrong, drop the peer
		//f.log("Propagated block verification failed", "peer", peer.Name(), "number", block.Number(), "hash", hash, "err", err)
		f.dropPeer(peer.ID())
		return errors.New(fmt.Sprintf("Propagated block verification failed number %d peer %s ", block.Number(), peer.Name()))
	}
	// Run the actual import and log any issues
	if _, err := f.insertChain(types.Blocks{block}); err != nil {
		//f.log("Propagated block import failed", "peer", peer.Name(), "number", block.Number(), "hash", hash, "err", err)
		return errors.New(fmt.Sprintf("Propagated block import failed  number %d peer %s with err %s",block.Number(), peer.Name(), err.Error()))
	}
	// If import succeeded, broadcast the block
	f.broadcastBlock(block, false)

	return nil
}

func (f *Fetcher) log(a ...interface{})  {
	if LogConfig.LogDownload {
		Log("Fetcher", f.name, a)
	}
}

