// Copyright 2014 The go-ethereum Authors
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

// Package eth implements the Ethereum protocol.
package eth

import (
	. "../../common"
	. "../../config"
	"../../network/devp2p"
	. "../../util"
	"../eth/common"
	"../eth/consensus"
	"../eth/consensus/clique"
	"../eth/core"
	"../eth/core/rawdb"
	"../eth/core/types"
	"../eth/ethdb"
	. "../eth/event_feed"
	"../eth/miner"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/crypto"
	"math/big"
	"runtime"
	"sync/atomic"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
)

// Ethereum implements the Ethereum full node service.
type Ethereum struct {
	*devp2p.Server

	config *Config

	// Channel for shutting down the service
	shutdownChan chan bool // Channel for shutting down the Ethereum

	// Handlers
	txPool          *core.TxPool
	blockchain      *core.BlockChain
	protocolManager *ProtocolManager

	// DB interfaces
	chainDb ethdb.Database // Block chain database

	eventFeed 	   *EventFeed
	engine         consensus.Engine

	miner     *miner.Miner
	gasPrice  *big.Int
	etherbase common.Address

}

// New creates a new Ethereum object (including the
// initialisation of the common Ethereum object)
func New(node INode, config *Config) (*Ethereum, error) {

	if config == nil {
		config = &DefaultConfig
	}

	// Ensure configuration values are compatible and sane
	if config.SyncMode == downloader.LightSync {
		return nil, errors.New("can't run eth.Ethereum in light sync mode, use les.LightEthereum")
	}
	if !config.SyncMode.IsValid() {
		return nil, fmt.Errorf("invalid sync mode %d", config.SyncMode)
	}
	if config.Miner.GasPrice == nil || config.Miner.GasPrice.Cmp(big.NewInt(0)) <= 0 {
	//	log.Warn("Sanitizing invalid miner gas price", "provided", config.Miner.GasPrice, "updated", DefaultConfig.Miner.GasPrice)
		config.Miner.GasPrice = new(big.Int).Set(DefaultConfig.Miner.GasPrice)
	}
	if config.NoPruning && config.TrieDirtyCache > 0 {
		config.TrieCleanCache += config.TrieDirtyCache
		config.TrieDirtyCache = 0
	}
//	log.Info("Allocated trie memory caches", "clean", common.StorageSize(config.TrieCleanCache)*1024*1024, "dirty", common.StorageSize(config.TrieDirtyCache)*1024*1024)

	// Assemble the Ethereum object
	chainDb := rawdb.NewMemoryDatabase()

	chainConfig, _, genesisErr := core.SetupGenesisBlockWithOverride(chainDb, config.Genesis)
	if _, ok := genesisErr.(*params.ConfigCompatError); genesisErr != nil && !ok {
		return nil, genesisErr
	}
	//log.Info("Initialised chain configuration", "config", chainConfig)

	eth := &Ethereum{
		Server:			devp2p.NewServer(node),
		config:         config,
		chainDb:        chainDb,
		engine:         CreateConsensusEngine(node.Name(), chainConfig, chainDb),
		shutdownChan:   make(chan bool),
		gasPrice:       config.Miner.GasPrice,
		etherbase:      config.Miner.Etherbase,
		eventFeed:		NewEventFeed(),
	}
	if eth.etherbase == (common.Address{}){
		eth.etherbase = node.Address()
	}

	var (
		cacheConfig = &core.CacheConfig{
			TrieCleanLimit:      config.TrieCleanCache,
			TrieCleanNoPrefetch: config.NoPrefetch,
			TrieDirtyLimit:      config.TrieDirtyCache,
			TrieDirtyDisabled:   config.NoPruning,
			TrieTimeLimit:       config.TrieTimeout,
		}
	)

	var err error
	eth.blockchain, err = core.NewBlockChain(node.Name() ,chainDb, cacheConfig, chainConfig, eth.engine, eth.shouldPreserve)
	if err != nil {
		return nil, err
	}

	eth.txPool = core.NewTxPool(node.Name(), config.TxPool, chainConfig, eth.blockchain)

	eth.protocolManager = NewProtocolManager(eth, eth.EventFeed(),eth.blockchain, eth.txPool)
	eth.Server.Protocols = eth.Protocols()

	eth.miner = miner.New(node.Name() ,eth, &config.Miner, chainConfig, eth.EventFeed(), eth.engine, eth.isLocalBlock)
	eth.miner.SetExtra(makeExtraData(config.Miner.ExtraData))

	eth.StartMining()

	return eth, nil
}

func makeExtraData(extra []byte) []byte {
	if len(extra) == 0 {
		// create default extradata
		extra, _ = rlp.EncodeToBytes([]interface{}{
			uint(params.VersionMajor<<16 | params.VersionMinor<<8 | params.VersionPatch),
			"geth",
			runtime.Version(),
			runtime.GOOS,
		})
	}
	if uint64(len(extra)) > params.MaximumExtraDataSize {
		log.Warn("Miner extra data exceed limit", "extra", hexutil.Bytes(extra), "limit", params.MaximumExtraDataSize)
		extra = nil
	}
	return extra
}

// CreateConsensusEngine creates the required type of consensus engine instance for an Ethereum service
func CreateConsensusEngine(name string, chainConfig *params.ChainConfig, db ethdb.Database) consensus.Engine {
	// If proof-of-authority is requested, set it up
	if chainConfig.Clique != nil {
		return clique.New(name, chainConfig.Clique, db)
	}
	return nil
}


func (s *Ethereum) ResetWithGenesisBlock(gb *types.Block) {
	s.blockchain.ResetWithGenesisBlock(gb)
}

func (s *Ethereum) Etherbase() (eb common.Address, err error) {
	etherbase := s.etherbase

	if etherbase != (common.Address{}) {
		return etherbase, nil
	}
	return common.Address{}, fmt.Errorf("etherbase must be explicitly specified")
	/*
	if wallets := s.AccountManager().Wallets(); len(wallets) > 0 {
		if accounts := wallets[0].Accounts(); len(accounts) > 0 {
			etherbase := accounts[0].Address

			s.etherbase = etherbase

			log.Info("Etherbase automatically configured", "address", etherbase)
			return etherbase, nil
		}
	}
	return common.Address{}, fmt.Errorf("etherbase must be explicitly specified")
	 */
}

// isLocalBlock checks whether the specified block is mined
// by local miner accounts.
//
// We regard two types of accounts as local miner account: etherbase
// and accounts specified via `txpool.locals` flag.
func (s *Ethereum) isLocalBlock(block *types.Block) bool {
	author, err := s.engine.Author(block.Header())
	if err != nil {
		s.log("Failed to retrieve block author", "number", block.NumberU64(), "hash", block.Hash(), "err", err)
		return false
	}
	// Check whether the given address is etherbase.
	etherbase := s.etherbase
	if author == etherbase {
		return true
	}
	// Check whether the given address is specified by `txpool.local`
	// CLI flag.
	for _, account := range s.config.TxPool.Locals {
		if account == author {
			return true
		}
	}
	return false
}

// shouldPreserve checks whether we should preserve the given block
// during the chain reorg depending on whether the author of block
// is a local account.
func (s *Ethereum) shouldPreserve(block *types.Block) bool {
	// The reason we need to disable the self-reorg preserving for clique
	// is it can be probable to introduce a deadlock.
	//
	// e.g. If there are 7 available signers
	//
	// r1   A
	// r2     B
	// r3       C
	// r4         D
	// r5   A      [X] F G
	// r6    [X]
	//
	// In the round5, the inturn signer E is offline, so the worst case
	// is A, F and G sign the block of round5 and reject the block of opponents
	// and in the round6, the last available signer B is offline, the whole
	// network is stuck.
	if _, ok := s.engine.(*clique.Clique); ok {
		return false
	}
	return s.isLocalBlock(block)
}

// SetEtherbase sets the mining reward address.
func (s *Ethereum) SetEtherbase(etherbase common.Address) {
	s.etherbase = etherbase

	s.miner.SetEtherbase(etherbase)
}

// StartMining starts the miner with the given number of CPU threads. If mining
// is already running, this method adjust the number of threads allowed to use
// and updates the minimum price required by the transaction pool.
func (s *Ethereum) StartMining() error {
	// If the miner was not running, initialize it
	if !s.IsMining() {
		// Propagate the initial price point to the transaction pool
		price := s.gasPrice
		s.txPool.SetGasPrice(price)

		// Configure the local mining address
		eb, err := s.Etherbase()
		if err != nil {
			s.log("Cannot start mining without etherbase", "err", err)
			return fmt.Errorf("etherbase missing: %v", err)
		}

		if clique, ok := s.engine.(*clique.Clique); ok {
			clique.Authorize(eb, func(account accounts.Account, mimeType string, data []byte) (bytes []byte, e error) {
				hash := crypto.Keccak256(data)
				return crypto.Sign(hash, s.Self().PrivateKey())
			})
		}

		// If mining is started, we can disable the transaction rejection mechanism
		// introduced to speed sync times.
		atomic.StoreUint32(&s.protocolManager.acceptTxs, 1)

		s.miner.SetEtherbase(eb)
	//	if s.Self().Name() == "2" {
			s.miner.Start(eb)
//		}
	}

	s.log("Starting to mine")
	return nil
}

// StopMining terminates the miner, both at the consensus engine level as well as
// at the block creation level.
func (s *Ethereum) StopMining() {
	// Stop the block creating itself
	s.miner.Stop()
}

func (s *Ethereum) SetOnline(online bool)  {

	if online {
		s.Start()
	} else {
		s.Stop()
	}

}


func (s *Ethereum) GetProtocolManager() IProtocolManager {
	return s.protocolManager
}
func (s *Ethereum) IsMining() bool      { return s.miner.Mining() }
func (s *Ethereum) Miner() *miner.Miner { return s.miner }

func (s *Ethereum) BlockChain() *core.BlockChain       { return s.blockchain }
func (s *Ethereum) TxPool() *core.TxPool               { return s.txPool }
func (s *Ethereum) EventFeed() *EventFeed              { return s.eventFeed }
func (s *Ethereum) Engine() consensus.Engine           { return s.engine }
func (s *Ethereum) ChainDb() ethdb.Database            { return s.chainDb }
func (s *Ethereum) IsListening() bool                  { return true } // Always listening
func (s *Ethereum) EthVersion() string                 { return s.protocolManager.SubProtocols[0].GetName() }

// Protocols implements node.Service, returning all the currently configured
// network protocols to start.
func (s *Ethereum) Protocols() []IProtocol {
	return s.protocolManager.SubProtocols
}

// Start implements node.Service, starting all internal goroutines needed by the
// Ethereum protocol implementation.
func (s *Ethereum) Start() {
	s.Server.Start()

	// Start the networking layer and the light server if requested
	s.protocolManager.Start()
}

// Stop implements node.Service, terminating all internal goroutines used by the
// Ethereum protocol.
func (s *Ethereum) Stop() {
	s.Server.Stop()

	s.blockchain.Stop()
	s.engine.Close()
	s.protocolManager.Stop()

	s.txPool.Stop()
	s.miner.Stop()

//	s.eventMux.Stop()

//	s.chainDb.Close()
	close(s.shutdownChan)
}

func (s *Ethereum) log(a ...interface{})  {

	if LogConfig.LogServer {
		Log("Eth Server:", s.Self(), a)
	}

}
