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

package miner

import (
	. "../../../config"
	. "../../../util"
	. "../../eth/event_feed"
	"../common"
	"../consensus"
	"../core"
	"../core/state"
	"../core/types"
	"../params"
	"errors"
	"fmt"
	"github.com/agoussia/godes"
	mapset "github.com/deckarep/golang-set"
	"math/big"
	"sync/atomic"
	"time"

)

const (
	// resultQueueSize is the size of channel listening to sealing result.
	resultQueueSize = 10

	// txChanSize is the size of channel listening to NewTxsEvent.
	// The number is referenced from the size of tx pool.
	txChanSize = 4096

	// chainHeadChanSize is the size of channel listening to ChainHeadEvent.
	chainHeadChanSize = 10

	// chainSideChanSize is the size of channel listening to ChainSideEvent.
	chainSideChanSize = 10

	// resubmitAdjustChanSize is the size of resubmitting interval adjustment channel.
	resubmitAdjustChanSize = 10

	// miningLogAtDepth is the number of confirmations before logging successful mining.
	miningLogAtDepth = 7

	// minRecommitInterval is the minimal time interval to recreate the mining block with
	// any newly arrived transactions.
	minRecommitInterval = 1 * time.Second

	// maxRecommitInterval is the maximum time interval to recreate the mining block with
	// any newly arrived transactions.
	maxRecommitInterval = 15 * time.Second

	// intervalAdjustRatio is the impact a single interval adjustment has on sealing work
	// resubmitting interval.
	intervalAdjustRatio = 0.1

	// intervalAdjustBias is applied during the new resubmit interval calculation in favor of
	// increasing upper limit or decreasing lower limit so that the limit can be reachable.
	intervalAdjustBias = 200 * 1000.0 * 1000.0

	// staleThreshold is the maximum depth of the acceptable stale block.
	staleThreshold = 7
)

// environment is the worker's current environment and holds all of the current state information.
type environment struct {
	signer types.Signer

	state     *state.StateDB // apply state changes here
	ancestors mapset.Set     // ancestor set (used for checking uncle parent validity)
	family    mapset.Set     // family set (used for checking uncle invalidity)
	uncles    mapset.Set     // uncle set
	tcount    int            // tx count in cycle
	gasPool   *core.GasPool  // available gas used to pack transactions

	header   *types.Header
	txs      []*types.Transaction
	receipts []*types.Receipt
}

// task contains all information for consensus engine sealing and result submitting.
type task struct {
	*godes.Runner
	worker    *worker
	receipts  []*types.Receipt
	state     *state.StateDB
	block     *types.Block
	createdAt uint64
}

const (
	commitInterruptNone int32 = iota
	commitInterruptNewHead
	commitInterruptResubmit
)

// newWorkReq represents a request for new sealing work submitting with relative interrupt notifier.
type newWorkReq struct {
	interrupt *int32
	noempty   bool
	timestamp int64
}

// intervalAdjust represents a resubmitting interval adjustment.
type intervalAdjust struct {
	ratio float64
	inc   bool
}

// worker is the main object which takes care of submitting new work to consensus engine
// and gathering the sealing result.
type worker struct {
	*godes.Runner
	name 		string
	config      *Config
	chainConfig *params.ChainConfig
	engine      consensus.Engine
	eth         Backend
	chain       *core.BlockChain

	// Subscriptions
	mux          *EventFeed

	current      *environment                 // An environment for current running cycle.
	localUncles  map[common.Hash]*types.Block // A set of side blocks generated locally as the possible uncle blocks.
	remoteUncles map[common.Hash]*types.Block // A set of side blocks as the possible uncle blocks.
	unconfirmed  *unconfirmedBlocks           // A set of locally mined blocks pending canonicalness confirmations.

	coinbase common.Address
	extra    []byte

	pendingTasks map[common.Hash]*task

	snapshotBlock *types.Block
	snapshotState *state.StateDB

	// atomic status counters
	running int32 // The indicator whether the consensus engine is running or not.
	newTxs  int32 // New arrival transaction count since last sealing work submitting.

	// External functions
	isLocalBlock func(block *types.Block) bool // Function used to determine whether the specified block is mined by local miner.


	interrupt   *int32
	minRecommit time.Duration // minimal resubmit interval specified by user.
	timestamp   int64      // timestamp for each round of mining.

	recommit time.Duration
	recommitTime time.Duration

	prev   common.Hash
	prevTask *task
}

func newWorker(name string, config *Config, chainConfig *params.ChainConfig, engine consensus.Engine, eth Backend, mux *EventFeed, isLocalBlock func(*types.Block) bool) *worker {
	worker := &worker{
		Runner:				&godes.Runner{},
		name:				name,
		config:             config,
		chainConfig:        chainConfig,
		engine:             engine,
		eth:                eth,
		mux:                mux,
		chain:              eth.BlockChain(),
		isLocalBlock:       isLocalBlock,
		localUncles:        make(map[common.Hash]*types.Block),
		remoteUncles:       make(map[common.Hash]*types.Block),
		unconfirmed:        newUnconfirmedBlocks(eth.BlockChain(), miningLogAtDepth),
		pendingTasks:       make(map[common.Hash]*task),
	}
	// Subscribe NewTxsEvent for tx pool
	eth.TxPool().SubscribeNewTxsEvent(worker.handleTxEvent)

	// Subscribe events for blockchain
	eth.BlockChain().SubscribeChainHeadEvent(worker.handleChainHeadEvent)
	eth.BlockChain().SubscribeChainSideEvent(worker.handleChainSideEvent)

	// Sanitize recommit interval if the user-specified one is too short.
	worker.recommit = worker.config.Recommit
	if worker.recommit < minRecommitInterval {
		worker.log("Sanitizing miner recommit interval", "provided", worker.recommit, "updated", minRecommitInterval)
		worker.recommit = minRecommitInterval
	}

	worker.minRecommit = worker.recommit

	worker.newWorkLoop()

	// Submit first work to initialize pending state.
	worker.startWork()

	return worker
}

// setEtherbase sets the etherbase used to initialize the block coinbase field.
func (w *worker) setEtherbase(addr common.Address) {
	w.coinbase = addr
}

// setExtra sets the content used to initialize the block extra field.
func (w *worker) setExtra(extra []byte) {
	w.extra = extra
}

// setRecommitInterval updates the interval for miner sealing work recommitting.
func (w *worker) setRecommitInterval(interval time.Duration) {
	w.resubmitInterval(interval)
}

// pending returns the pending state and corresponding block.
func (w *worker) pending() (*types.Block, *state.StateDB) {
	// return a snapshot to avoid contention on currentMu mutex
	if w.snapshotState == nil {
		return nil, nil
	}
	return w.snapshotBlock, w.snapshotState.Copy()
}

// pendingBlock returns pending block.
func (w *worker) pendingBlock() *types.Block {
	// return a snapshot to avoid contention on currentMu mutex
	return w.snapshotBlock
}

// start sets the running status as 1 and triggers new work submitting.
func (w *worker) start() {
	atomic.StoreInt32(&w.running, 1)
	ResumeRunner(w)
	w.startWork()
}

// stop sets the running status as 0.
func (w *worker) stop() {
	atomic.StoreInt32(&w.running, 0)
}

// isRunning returns an indicator whether worker is running or not.
func (w *worker) isRunning() bool {
	return atomic.LoadInt32(&w.running) == 1
}

// close terminates all background threads maintained by the worker.
// Note the worker does not support being closed multiple times.
func (w *worker) close() {
	StopRunner(w)
	w.interruptPrevTask()
}


func (w *worker) clearPending(number uint64) {

	w.log("clearPending", len(w.pendingTasks))

	for h, t := range w.pendingTasks {
		if t.block.NumberU64()+staleThreshold <= number {
			delete(w.pendingTasks, h)
		}
	}
}

func (w *worker) commitWork(noempty bool, s int32) {
	w.log("commitWork", noempty)

	if w.interrupt != nil {
		atomic.StoreInt32(w.interrupt, s)
	}
	w.interrupt = new(int32)
	w.feedNewWorkReq( &newWorkReq{interrupt: w.interrupt, noempty: noempty, timestamp: w.timestamp})
	w.recommitTime = w.recommit
	atomic.StoreInt32(&w.newTxs, 0)
}

// recalcRecommit recalculates the resubmitting interval upon feedback.
 func (w *worker) recalcRecommit(target float64, inc bool) {
	var (
		prev = float64(w.recommit.Nanoseconds())
		next float64
	)
	if inc {
		next = prev*(1-intervalAdjustRatio) + intervalAdjustRatio*(target+intervalAdjustBias)
		// Recap if interval is larger than the maximum time interval
		if next > float64(maxRecommitInterval.Nanoseconds()) {
			next = float64(maxRecommitInterval.Nanoseconds())
		}
	} else {
		next = prev*(1-intervalAdjustRatio) + intervalAdjustRatio*(target-intervalAdjustBias)
		// Recap if interval is less than the user specified minimum
		if next < float64(w.minRecommit.Nanoseconds()) {
			next = float64(w.minRecommit.Nanoseconds())
		}
	}
	 w.recommit = time.Duration(int64(next))

	 w.log("recalcRecommit", w.recommit)
}

func (w *worker) startWork()  {
	w.clearPending(w.chain.CurrentBlock().NumberU64())
	w.timestamp = int64(SecondsNow())
	w.commitWork(false, commitInterruptNewHead)

	w.log("startWork")
}



func (w *worker) handleChainHeadEvent(head core.ChainHeadEvent)  {
	w.log("handleChainHeadEvent", head.Block.NumberU64())

	w.clearPending(head.Block.NumberU64())
	w.timestamp = int64(SecondsNow())
	w.commitWork(false, commitInterruptNewHead)

}

func (w *worker) resubmitAdjust(adjust *intervalAdjust)  {
	// Adjust resubmit interval by feedback.
	before := w.recommit

	if adjust.inc {
		w.recalcRecommit(float64(w.recommit.Nanoseconds())/adjust.ratio, true)
		w.log("Increase miner recommit interval", "from", before, "to", w.recommit)
	} else {
		w.recalcRecommit(float64(w.minRecommit.Nanoseconds()), false)
		w.log("Decrease miner recommit interval", "from", before, "to", w.recommit)
	}

}

func (w *worker) resubmitInterval(interval time.Duration)  {
	// Adjust resubmit interval explicitly by user.
	if interval < minRecommitInterval {
		w.log("Sanitizing miner recommit interval", "provided", interval, "updated", minRecommitInterval)
		interval = minRecommitInterval
	}
	w.log("Miner recommit interval update", "from", w.minRecommit, "to", interval)
	w.minRecommit, w.recommit = interval, interval

}

// newWorkLoop is a standalone goroutine to submit new mining work upon received events.
func (w *worker) newWorkLoop() {
	godes.AddRunner(w)
}

// ako se worker zaustavi, run i dalje radi ali uzaludno
func (w *worker) Run()  {

	for {

		godes.Advance(w.recommitTime.Seconds())

		// If mining is running resubmit a new work cycle periodically to pull in
		// higher priced transactions. Disable this overhead for pending blocks.
		if w.isRunning() &&
			(w.chainConfig.Clique == nil || w.chainConfig.Clique.Period > 0)  &&
			(w.chainConfig.Aura == nil || w.chainConfig.Aura.Period > 0){

			// Short circuit if no new transaction arrives.
			if atomic.LoadInt32(&w.newTxs) == 0 {
				w.recommitTime = w.recommit
				continue
			}
			w.commitWork(true, commitInterruptResubmit)
		}
	}

}

func (w *worker) handleChainSideEvent(ev core.ChainSideEvent)  {
	w.log("handleChainSideEvent", ev.Block.NumberU64())

	// Short circuit for duplicate side blocks
	if _, exist := w.localUncles[ev.Block.Hash()]; exist {
		return
	}
	if _, exist := w.remoteUncles[ev.Block.Hash()]; exist {
		return
	}
	// Add side block to possible uncle block set depending on the author.
	if w.isLocalBlock != nil && w.isLocalBlock(ev.Block) {
		w.localUncles[ev.Block.Hash()] = ev.Block
	} else {
		w.remoteUncles[ev.Block.Hash()] = ev.Block
	}
	// If our mining block contains less than 2 uncle blocks,
	// add the new uncle block if valid and regenerate a mining block.
	if w.isRunning() && w.current != nil && w.current.uncles.Cardinality() < 2 {
		start := SecondsNow()
		if err := w.commitUncle(w.current, ev.Block.Header()); err == nil {
			var uncles []*types.Header
			w.current.uncles.Each(func(item interface{}) bool {
				hash, ok := item.(common.Hash)
				if !ok {
					return false
				}
				uncle, exist := w.localUncles[hash]
				if !exist {
					uncle, exist = w.remoteUncles[hash]
				}
				if !exist {
					return false
				}
				uncles = append(uncles, uncle.Header())
				return false
			})
			w.commit(uncles, true, start)
		}
	}

}


func (w *worker) handleTxEvent(ev core.NewTxsEvent)  {

	w.log("handleTxEvent", len(ev.Txs))

	// Apply transactions to the pending state if we're not mining.
	//
	// Note all transactions received may not be continuous with transactions
	// already included in the current mining block. These transactions will
	// be automatically eliminated.
	if !w.isRunning() && w.current != nil {
		coinbase := w.coinbase

		txs := make(map[common.Address]types.Transactions)
		for _, tx := range ev.Txs {
			acc, _ := types.Sender(w.current.signer, tx)
			txs[acc] = append(txs[acc], tx)
		}
		txset := types.NewTransactionsByPriceAndNonce(w.current.signer, txs)
		w.commitTransactions(txset, coinbase, nil)
		w.updateSnapshot()
	} else {
		// If clique or aura is running in dev mode(period is 0), disable
		// advance sealing here.
		if (w.chainConfig.Clique != nil && w.chainConfig.Clique.Period == 0 ) ||
			(w.chainConfig.Aura != nil && w.chainConfig.Aura.Period == 0 ) {
			w.commitNewWork(nil, true, int64(SecondsNow()))
		}
	}
	atomic.AddInt32(&w.newTxs, int32(len(ev.Txs)))

}


func (w *worker) feedNewWorkReq(req *newWorkReq)  {
	if w.isRunning() {
		StartNewRunner(func() {
			w.commitNewWork(req.interrupt, req.noempty, req.timestamp)
		})
	}
}

func (w *worker) postTask(task *task)  {
	godes.AddRunner(task)

}

func (w *worker) interruptPrevTask()  {
	// Interrupt previous sealing operation
	if w.prevTask != nil {
		if w.prevTask.IsShedulled(){
			godes.Interrupt(w.prevTask)
		}
		w.prevTask = nil
	}

}
func (task *task) Run()  {
	w := task.worker
	w.log("Running task for block", task.block.NumberU64())

	// Reject duplicate sealing work due to resubmitting.
	sealHash := w.engine.SealHash(task.block.Header())
	if sealHash == w.prev {
		w.log("Resubmiting task for block", task.block.NumberU64())
		return
	}
	w.interruptPrevTask()
	w.prev = sealHash
	w.prevTask = task

	w.pendingTasks[w.engine.SealHash(task.block.Header())] = task

	// sealing moze potrajati pa se ovaj runner moze i zaustaviti u medjuvremenu
	// ali on ionako nista ne radi nakon seal bloka pa nije ni bitno
	if err := w.engine.Seal(w.chain, task.block, w.handleResult); err != nil {
		w.log("Block sealing failed", "err", err)
	}

}

func (w *worker) handleResult(block *types.Block)  {
	// Short circuit when receiving empty result.
	if block == nil {
		return
	}

	w.log("Seal block result", block.NumberU64(), "with tx count", len(block.Transactions()))

	// Short circuit when receiving duplicate result caused by resubmitting.
	if w.chain.HasBlock(block.Hash(), block.NumberU64()) {
		return
	}
	var (
		sealhash = w.engine.SealHash(block.Header())
		hash     = block.Hash()
	)

	task, exist := w.pendingTasks[sealhash]
	if !exist {
		w.log("Block found but no relative pending task", "number", block.Number(), "sealhash", sealhash, "hash", hash)
		return
	}
	// Different block could share same sealhash, deep copy here to prevent write-write conflict.
	var (
		receipts = make([]*types.Receipt, len(task.receipts))
		logs     []*types.Log
	)
	for i, receipt := range task.receipts {
		// add block location fields
		receipt.BlockHash = hash
		receipt.BlockNumber = block.Number()
		receipt.TransactionIndex = uint(i)

		receipts[i] = new(types.Receipt)
		*receipts[i] = *receipt
		// Update the block hash in all logs since it is now available and not when the
		// receipt/log of individual transactions were created.
		for _, log := range receipt.Logs {
			log.BlockHash = hash
		}
		logs = append(logs, receipt.Logs...)
	}
	// Commit block and state to database.
	stat, err := w.chain.WriteBlockWithState(block, receipts, task.state)
	if err != nil {
		w.log("Failed writing block to chain", "err", err)
		return
	}
	w.log("Successfully sealed new block", "number", block.Number(), "elapsed", TimeSince(task.createdAt))

	// Broadcast the block and announce chain insertion event
	w.mux.Post(core.NewMinedBlockEvent{Block: block})

	var events []interface{}
	switch stat {
	case core.CanonStatTy:
		events = append(events, core.ChainEvent{Block: block, Hash: block.Hash()})
		events = append(events, core.ChainHeadEvent{Block: block})
	case core.SideStatTy:
		events = append(events, core.ChainSideEvent{Block: block})
	}
	w.chain.PostChainEvents(events, logs)

	// Insert the block into the set of pending ones to resultLoop for confirmations
	w.unconfirmed.Insert(block.NumberU64(), block.Hash())

}


// makeCurrent creates a new environment for the current cycle.
func (w *worker) makeCurrent(parent *types.Block, header *types.Header) error {
	state, err := w.chain.StateAt(parent.Root())
	if err != nil {
		return err
	}
	env := &environment{
		signer:    types.NewEIP155Signer(w.chainConfig.ChainID),
		state:     state,
		ancestors: mapset.NewSet(),
		family:    mapset.NewSet(),
		uncles:    mapset.NewSet(),
		header:    header,
	}

	// when 08 is processed ancestors contain 07 (quick block)
	for _, ancestor := range w.chain.GetBlocksFromHash(parent.Hash(), 7) {
		for _, uncle := range ancestor.Uncles() {
			env.family.Add(uncle.Hash())
		}
		env.family.Add(ancestor.Hash())
		env.ancestors.Add(ancestor.Hash())
	}

	// Keep track of transactions which return errors so they can be removed
	env.tcount = 0
	w.current = env

	w.log("makeCurrent", header.Number)
	return nil
}

// commitUncle adds the given block to uncle block set, returns error if failed to add.
func (w *worker) commitUncle(env *environment, uncle *types.Header) error {
	hash := uncle.Hash()
	if env.uncles.Contains(hash) {
		return errors.New("uncle not unique")
	}
	if env.header.ParentHash == uncle.ParentHash {
		return errors.New("uncle is sibling")
	}
	if !env.ancestors.Contains(uncle.ParentHash) {
		return errors.New("uncle's parent unknown")
	}
	if env.family.Contains(hash) {
		return errors.New("uncle already included")
	}
	env.uncles.Add(uncle.Hash())

	w.log("Commited uncle", uncle.Number)

	return nil
}

// updateSnapshot updates pending snapshot block and state.
// Note this function assumes the current variable is thread safe.
func (w *worker) updateSnapshot() {

	var uncles []*types.Header
	w.current.uncles.Each(func(item interface{}) bool {
		hash, ok := item.(common.Hash)
		if !ok {
			return false
		}
		uncle, exist := w.localUncles[hash]
		if !exist {
			uncle, exist = w.remoteUncles[hash]
		}
		if !exist {
			return false
		}
		uncles = append(uncles, uncle.Header())
		return false
	})

	w.snapshotBlock = types.NewBlock(
		w.current.header,
		w.current.txs,
		uncles,
		w.current.receipts,
	)

	w.snapshotState = w.current.state.Copy()

	w.log("updateSnapshot")
}

func (w *worker) commitTransaction(tx *types.Transaction, coinbase common.Address) ([]*types.Log, error) {
	w.log("commitTransaction with nonce", tx.Nonce(), "hash", tx.Hash())

	snap := w.current.state.Snapshot()

	receipt, gas, err := core.ApplyTransaction(w.chainConfig, w.chain, &coinbase, w.current.gasPool, w.current.state, w.current.header, tx, &w.current.header.GasUsed)
	if err != nil {
		w.log("CommitTransaction failed with err", err)
		w.current.state.RevertToSnapshot(snap)
		return nil, err
	}
	w.log("Apply transaction gasUsed", gas, "receipt", receipt)
	w.current.txs = append(w.current.txs, tx)
	w.current.receipts = append(w.current.receipts, receipt)

	//w.log("commitTransaction", tx)
	return receipt.Logs, nil
}

func (w *worker) commitTransactions(txs *types.TransactionsByPriceAndNonce, coinbase common.Address, interrupt *int32) bool {
	// Short circuit if current is nil
	if w.current == nil {
		return true
	}

	if w.current.gasPool == nil {
		w.current.gasPool = new(core.GasPool).AddGas(w.current.header.GasLimit)
	}

	w.log("commitTransactions", txs)

	var coalescedLogs []*types.Log

	for {
		// In the following three cases, we will interrupt the execution of the transaction.
		// (1) new head block event arrival, the interrupt signal is 1
		// (2) worker start or restart, the interrupt signal is 1
		// (3) worker recreate the mining block with any newly arrived transactions, the interrupt signal is 2.
		// For the first two cases, the semi-finished work will be discarded.
		// For the third case, the semi-finished work will be submitted to the consensus engine.
		if interrupt != nil && atomic.LoadInt32(interrupt) != commitInterruptNone {
			// Notify resubmit loop to increase resubmitting interval due to too frequent commits.
			if atomic.LoadInt32(interrupt) == commitInterruptResubmit {
				ratio := float64(w.current.header.GasLimit-w.current.gasPool.Gas()) / float64(w.current.header.GasLimit)
				if ratio < 0.1 {
					ratio = 0.1
				}
				w.resubmitAdjust( &intervalAdjust{
					ratio: ratio,
					inc:   true,
				})
			}
			return atomic.LoadInt32(interrupt) == commitInterruptNewHead
		}
		// If we don't have enough gas for any further transactions then we're done
		if w.current.gasPool.Gas() < params.TxGas {
			w.log("Not enough gas for further transactions", "have", w.current.gasPool, "want", params.TxGas)
			break
		}
		// Retrieve the next transaction and abort if all done
		tx := txs.Peek()
		if tx == nil {
			break
		}


		w.log("commitTransactions processing tx with nonce", tx.Nonce())


		// Error may be ignored here. The error has already been checked
		// during transaction acceptance is the transaction pool.
		//
		// We use the eip155 signer regardless of the current hf.
		from, _ := types.Sender(w.current.signer, tx)

		// Start executing the transaction
		w.current.state.Prepare(tx.Hash(), common.Hash{}, w.current.tcount)

		logs, err := w.commitTransaction(tx, coinbase)
		//Print(w.current.header.Number, "Balance", w.current.state.GetBalance(from), w.current.state.GetBalance(*tx.To()))
		switch err {
		case core.ErrGasLimitReached:
			// Pop the current out-of-gas transaction without shifting in the next from the account
			w.log("Gas limit exceeded for current block", "sender", from)
			txs.Pop()

		case core.ErrNonceTooLow:
			// New head notification data race between the transaction pool and miner, shift
			w.log("Skipping transaction with low nonce", "sender", from, "nonce", tx.Nonce())
			txs.Shift()

		case core.ErrNonceTooHigh:
			// Reorg notification data race between the transaction pool and miner, skip account =
			w.log("Skipping account with hight nonce", "sender", from, "nonce", tx.Nonce())
			txs.Pop()

		case nil:
			// Everything ok, collect the logs and shift in the next transaction from the same account
			coalescedLogs = append(coalescedLogs, logs...)
			w.current.tcount++
			txs.Shift()

		default:
			// Strange error, discard the transaction and get the next in line (note, the
			// nonce-too-high clause will prevent us from executing in vain).
			w.log("Transaction failed, account skipped", "hash", tx.Hash(), "err", err)
			txs.Shift()
		}
	}

	if !w.isRunning() && len(coalescedLogs) > 0 {
		// We don't push the pendingLogsEvent while we are mining. The reason is that
		// when we are mining, the worker will regenerate a mining block every 3 seconds.
		// In order to avoid pushing the repeated pendingLog, we disable the pending log pushing.

		// make a copy, the state caches the logs and these logs get "upgraded" from pending to mined
		// logs by filling in the block hash when the block was mined by the local miner. This can
		// cause a race condition if a log was "upgraded" before the PendingLogsEvent is processed.
		cpy := make([]*types.Log, len(coalescedLogs))
		for i, l := range coalescedLogs {
			cpy[i] = new(types.Log)
			*cpy[i] = *l
		}
		//go w.mux.Post(core.PendingLogsEvent{Logs: cpy})
	}
	// Notify resubmit loop to decrease resubmitting interval if current interval is larger
	// than the user-specified one.
	if interrupt != nil {
		w.resubmitAdjust(&intervalAdjust{inc: false})
	}

	//w.log("commitTransactions", txs)
	return false
}

// commitNewWork generates several new sealing tasks based on the parent block.
func (w *worker) commitNewWork(interrupt *int32, noempty bool, timestamp int64) {
	w.log("commiting new work")

	tstart := SecondsNow()
	parent := w.chain.CurrentBlock()

	if parent.Time() >= uint64(timestamp) {
		timestamp = int64(parent.Time() + 1)
	}
	// this will ensure we're not going off too far in the future
	if now := int64(SecondsNow()); timestamp > now+1 {
		wait := time.Duration(timestamp-now) * time.Second
		w.log("Mining too far in the future", "wait", common.PrettyDuration(wait))
		godes.Advance(wait.Seconds())
	}

	num := parent.Number()
	header := &types.Header{
		ParentHash: parent.Hash(),
		Number:     num.Add(num, big.NewInt(1)),
		GasLimit:   core.CalcGasLimit(parent, w.config.GasFloor, w.config.GasCeil),
		Extra:      w.extra,
		Time:       uint64(timestamp),
	}
	//Print("GasLimit", header.GasLimit)
	// Only set the coinbase if our consensus engine is running (avoid spurious block rewards)
	if w.isRunning() {
		if w.coinbase == (common.Address{}) {
			w.log("Refusing to mine without etherbase")
			return
		}
		header.Coinbase = w.coinbase
	}
	if err := w.engine.Prepare(w.chain, header); err != nil {
		w.log("Failed to prepare header for mining", "err", err)
		return
	}

	// Could potentially happen if starting to mine in an odd state.
	err := w.makeCurrent(parent, header)
	if err != nil {
		w.log("Failed to create mining context", "err", err)
		return
	}
	env := w.current

	// Accumulate the uncles for the current block
	uncles := make([]*types.Header, 0, 2)
	commitUncles := func(blocks map[common.Hash]*types.Block) {
		// Clean up stale uncle blocks first
		for hash, uncle := range blocks {
			if uncle.NumberU64()+staleThreshold <= header.Number.Uint64() {
				delete(blocks, hash)
			}
		}
		for hash, uncle := range blocks {
			if len(uncles) == 2 {
				break
			}
			if err := w.commitUncle(env, uncle.Header()); err != nil {
				w.log("Possible uncle rejected",uncle.NumberU64(),  "hash", hash, "reason", err)
			} else {
				w.log("Committing new uncle to block", "hash", hash)
				uncles = append(uncles, uncle.Header())
			}
		}
	}
	// Prefer to locally generated uncle
	commitUncles(w.localUncles)
	commitUncles(w.remoteUncles)

	if !noempty {
		// Create an empty block based on temporary copied state for sealing in advance without waiting block
		// execution finished.
		w.commit(uncles, false, tstart)
	}

	// Fill the block with all available pending transactions.
	pending := w.eth.TxPool().Pending()
	//Print("worker", w.name, "Block", env.header.Number, "Pending txs", len(pending))

	w.log("Pending txs to be included in block", len(pending))

	// Short circuit if there is no available pending transactions
	if len(pending) == 0 {
		w.updateSnapshot()
		return
	}
	// Split the pending transactions into locals and remotes
	localTxs, remoteTxs := make(map[common.Address]types.Transactions), pending
	for _, account := range w.eth.TxPool().Locals() {
		if txs := remoteTxs[account]; len(txs) > 0 {
			delete(remoteTxs, account)
			localTxs[account] = txs
		}
	}
	if len(localTxs) > 0 {
		txs := types.NewTransactionsByPriceAndNonce(w.current.signer, localTxs)
		if w.commitTransactions(txs, w.coinbase, interrupt) {
			return
		}
	}
	if len(remoteTxs) > 0 {
		txs := types.NewTransactionsByPriceAndNonce(w.current.signer, remoteTxs)
		if w.commitTransactions(txs, w.coinbase, interrupt) {
			return
		}
	}
	w.commit(uncles, true, tstart)

	w.log("commited new work with", len(pending), "pending txs")
}

// commit runs any post-transaction state modifications, assembles the final block
// and commits new work if consensus engine is running.
func (w *worker)  commit(uncles []*types.Header,  update bool, start uint64) error {
	// Deep copy receipts here to avoid interaction between different tasks.
	receipts := make([]*types.Receipt, len(w.current.receipts))
	for i, l := range w.current.receipts {
		receipts[i] = new(types.Receipt)
		*receipts[i] = *l
	}
	s := w.current.state.Copy()
	block, err := w.engine.Finalize(w.chain, w.current.header, s, w.current.txs, uncles, w.current.receipts)
	if err != nil {
		return err
	}

	if w.isRunning() && !(block.Transactions().Len() == 0 && 0 < w.eth.TxPool().PendingCount()) {

		t := &task{Runner: &godes.Runner{},worker: w, receipts: receipts, state: s, block: block, createdAt: SecondsNow()}
		w.postTask(t)

		w.unconfirmed.Shift(block.NumberU64() - 1)

		feesWei := new(big.Int)
		for i, tx := range block.Transactions() {
			feesWei.Add(feesWei, new(big.Int).Mul(new(big.Int).SetUint64(receipts[i].GasUsed), tx.GasPrice()))
		}
		feesEth := new(big.Float).Quo(new(big.Float).SetInt(feesWei), new(big.Float).SetInt(big.NewInt(params.Ether)))

		w.log("Commit new mining work", "number", block.Number(), "sealhash",
			"uncles", len(uncles), "txs", w.current.tcount, "gas", block.GasUsed(), "fees", feesEth, "elapsed", TimeSince(start))

	}
	if update {
		w.updateSnapshot()
	}

	w.log("commit: uncles", len(uncles), "txs:", len(w.current.txs))
	return nil
}

func (w *worker) String() string {
	return fmt.Sprintf("Worker %s", w.name)
}
func (w *worker) log(a ...interface{}) {

	if LogConfig.LogWorker {
		Log(w, a)
	}

}
