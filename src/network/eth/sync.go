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

package eth

import (
	. "../../common"
	"../eth/common"
	"../eth/core/types"
	"github.com/agoussia/godes"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/eth/downloader"
)

const (
	forceSyncCycle      = 10 * time.Second // Time interval to force syncs, even if few peers are available
	minDesiredPeerCount = 5                // Amount of peers desired to start syncing

	// This is the target size for the packs of transactions sent by txsyncLoop.
	// A pack can get larger than this if a single transactions exceeds this size.
	txsyncPackSize = 100 * 1024
)

type txSyncManager struct {
	*godes.Runner
	pm 			*ProtocolManager
	pending 	map[ID]*txsync
	sending 	bool               // whether a send is active
	pack    	*txsync
}

func newTxSyncManager(pm *ProtocolManager) *txSyncManager  {
	return &txSyncManager{
		Runner: &godes.Runner{},
		pm: pm,
		pending : make(map[ID]*txsync),
		sending: false,
		pack: new(txsync),
	}
}


type txsync struct {
	p   *peer
	txs []*types.Transaction
}

// syncTransactions starts sending all currently pending transactions to the given peer.
func (pm *ProtocolManager) syncTransactions(p *peer) {
	var txs types.Transactions
	pending := pm.txpool.Pending()
	for _, batch := range pending {
		txs = append(txs, batch...)
	}
	if len(txs) == 0 {
		return
	}

	sync := &txsync{p, txs}
	pm.txSyncManager.sync(sync)
}

func (mg *txSyncManager) send(s *txsync)  {
	// Fill pack with transactions up to the target size.
	size := common.StorageSize(0)
	mg.pack.p = s.p
	mg.pack.txs = mg.pack.txs[:0]
	for i := 0; i < len(s.txs) && size < txsyncPackSize; i++ {
		mg.pack.txs = append(mg.pack.txs, s.txs[i])
		size += s.txs[i].Size()
	}
	// Remove the transactions that will be sent.
	s.txs = s.txs[:copy(s.txs, s.txs[len(mg.pack.txs):])]
	if len(s.txs) == 0 {
		delete(mg.pending, s.p.ID())
	}

	// Send the pack in the background.
	s.p.Log("Sending batch of transactions", "count", len(mg.pack.txs), "bytes", size)
	mg.sending = true

	mg.pack.p.SendTransactions(mg.pack.txs, func(err error) {
		mg.sending = false
		// Stop tracking peers that cause send failures.
		if err != nil {
			mg.pack.p.Log("Transaction send failed", "err", err)
			delete(mg.pending, mg.pack.p.ID())
		}
		// Schedule the next send.
		if s := mg.pick(); s != nil {
			mg.send(s)
		}
	})
}

func (mg *txSyncManager) pick() *txsync {
	if len(mg.pending) == 0 {
		return nil
	}
	n := rand.Intn(len(mg.pending)) + 1
	for _, s := range mg.pending {
		if n--; n == 0 {
			return s
		}
	}
	return nil
}

func (mg *txSyncManager) sync(s *txsync)  {
	mg.pending[s.p.ID()] = s
	if !mg.sending {
		mg.send(s)
	}

}

func (mg *txSyncManager) Run()  {
	for {
		godes.Advance(forceSyncCycle.Seconds())
		mg.pm.synchronise(mg.pm.peers.BestPeer())
	}
}

func (pm *ProtocolManager) synchroniseNewPeer(peer *peer)  {
	if pm.peers.Len() < minDesiredPeerCount {
		return
	}
	pm.synchronise(pm.peers.BestPeer())
}

// syncer is responsible for periodically synchronising with the network, both
// downloading hashes and blocks as well as handling the announcement handler.
func (mg *txSyncManager) syncer() {
	godes.AddRunner(mg)
}

func (mg *txSyncManager) stop()  {
	if mg.IsShedulled() {
		godes.Interrupt(mg)
	}
}

// synchronise tries to sync up our local block chain with a remote peer.
func (pm *ProtocolManager) synchronise(peer *peer) {

	// Short circuit if no peers are available
	if peer == nil {
		return
	}

	pm.log("synchronise with peer", peer)

	// Make sure the peer's TD is higher than our own
	currentBlock := pm.blockchain.CurrentBlock()
	td := pm.blockchain.GetTd(currentBlock.Hash(), currentBlock.NumberU64())

	pHead, pTd := peer.Head()
	if pTd.Cmp(td) <= 0 {
		return
	}
	// Otherwise try to sync with the downloader
	mode := downloader.FullSync
	if atomic.LoadUint32(&pm.fastSync) == 1 {
		// Fast sync was explicitly requested, and explicitly granted
		mode = downloader.FastSync
	} else if currentBlock.NumberU64() == 0 && pm.blockchain.CurrentFastBlock().NumberU64() > 0 {
		// The database seems empty as the current block is the genesis. Yet the fast
		// block is ahead, so fast sync was enabled for this node at a certain point.
		// The only scenario where this can happen is if the user manually (or via a
		// bad block) rolled back a fast sync node below the sync point. In this case
		// however it's safe to reenable fast sync.
		atomic.StoreUint32(&pm.fastSync, 1)
		mode = downloader.FastSync
	}
	if mode == downloader.FastSync {
		// Make sure the peer's total difficulty we are synchronizing is higher.
		if pm.blockchain.GetTdByHash(pm.blockchain.CurrentFastBlock().Hash()).Cmp(pTd) >= 0 {
			return
		}
	}


	// Run the sync cycle, and disable fast sync if we've went past the pivot block
	if err := pm.downloader.Synchronise(peer.id, pHead, pTd); err != nil {
		return
	}

	if atomic.LoadUint32(&pm.fastSync) == 1 {
		pm.log("Fast sync complete, auto disabling")
		atomic.StoreUint32(&pm.fastSync, 0)
	}
	atomic.StoreUint32(&pm.acceptTxs, 1) // Mark initial sync done
	if head := pm.blockchain.CurrentBlock(); head.NumberU64() > 0 {
		// We've completed a sync cycle, notify all peers of new state. This path is
		// essential in star-topology networks where a gateway node needs to notify
		// all its out-of-date peers of the availability of a new block. This failure
		// scenario will most often crop up in private and hackathon networks with
		// degenerate connectivity, but it should be healthy for the mainnet too to
		// more reliably update peers or the local TD state.
		pm.BroadcastBlock(head, false)
	}
}

