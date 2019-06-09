// Copyright 2017 The go-ethereum Authors
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
	"../../network/eth/params"

	"../eth/core"
	"../eth/miner"
	"math/big"
	"time"
)

// DefaultConfig contains default settings for use on the Ethereum main net.
var DefaultConfig = Config{
	DatabaseCache:  512,
	TrieCleanCache: 256,
	TrieDirtyCache: 256,
	TrieTimeout:    60 * time.Minute,
	Miner: miner.Config{
		GasFloor: 8000000,
		GasCeil:  8000000,
		GasPrice: big.NewInt(params.Wei),
		Recommit: 3 * time.Second,
	},
	TxPool: core.DefaultTxPoolConfig,
}


type Config struct {
	// The genesis block, which is inserted if the database is empty.
	// If nil, the Ethereum main net block is used.
	Genesis *core.Genesis `toml:",omitempty"`

	NoPruning  bool // Whether to disable pruning and flush everything to disk
	NoPrefetch bool // Whether to disable prefetching and only load state on demand

	// Database options
	SkipBcVersionCheck bool `toml:"-"`
	DatabaseHandles    int  `toml:"-"`
	DatabaseCache      int

	TrieCleanCache int
	TrieDirtyCache int
	TrieTimeout    time.Duration

	// Mining options
	Miner miner.Config

	// Transaction pool options
	TxPool core.TxPoolConfig
}
