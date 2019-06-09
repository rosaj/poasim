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

package core

import (
	. "../../../common"
	"../../../config"
	"../common"
	"../core/rawdb"
	"../core/state"
	"../core/types"
	"../ethdb"
	"../params"
	"bytes"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	"math/big"
)

const (

	GenesisGasLimit      uint64 = 4712388 // Gas limit of the Genesis block.

)


var (
	BankKey, _  = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	BankAddress = common.Address(crypto.PubkeyToAddress(BankKey.PublicKey))
	BankFunds   = bigFunds()
)

func bigFunds() *big.Int {
	count := new(big.Int)
	count.SetString("10000000000000000000000000000000",10)
	return count
}

var errGenesisNoConfig = errors.New("genesis has no chain configuration")

// Genesis specifies the header fields, state of a genesis block. It also defines hard
// fork switch-over blocks through the chain configuration.
type Genesis struct {
	Config     *params.ChainConfig
	Nonce      uint64
	Timestamp  uint64
	ExtraData  []byte
	GasLimit   uint64
	Difficulty *big.Int
	Mixhash    common.Hash
	Coinbase   common.Address
	Alloc      GenesisAlloc

}

// GenesisAlloc specifies the initial state that is part of the genesis block.
type GenesisAlloc map[common.Address]GenesisAccount

func (ga *GenesisAlloc) UnmarshalJSON(data []byte) error {
	m := make(map[common.UnprefixedAddress]GenesisAccount)
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	*ga = make(GenesisAlloc)
	for addr, a := range m {
		(*ga)[common.Address(addr)] = a
	}
	return nil
}

// GenesisAccount is an account in the state of the genesis block.
type GenesisAccount struct {
	Code       []byte                      `json:"code,omitempty"`
	Storage    map[common.Hash]common.Hash `json:"storage,omitempty"`
	Balance    *big.Int                    `json:"balance" gencodec:"required"`
	Nonce      uint64                      `json:"nonce,omitempty"`
	PrivateKey []byte                      `json:"secretKey,omitempty"` // for tests
}



func SetupGenesisBlockWithOverride(db ethdb.Database, ethConfig *config.EthereumConfig) (*params.ChainConfig, common.Hash, error) {
	config := ethConfig.ChainConfig

	var cliqueConfig *params.CliqueConfig = nil

	if config != nil {
		cliqueConfig = &params.CliqueConfig {
			Period: config.Clique.Period,
			Epoch: config.Clique.Epoch,
		}
	}

	genesis := &Genesis{
		Config: 	&params.ChainConfig {

			Clique: cliqueConfig,
		},
		Timestamp:  0,
		GasLimit:   4700000,
		Difficulty: big.NewInt(1),
		Alloc:      prealloc,
	}
	block, err := genesis.Commit(db)
	return genesis.Config, block.Hash(), err
}

var Sealers []INode

// ToBlock creates the genesis block and writes state of a genesis specification
// to the given database (or discards it if nil).
func (g *Genesis) ToBlock(db ethdb.Database) *types.Block {
	if db == nil {
		db = rawdb.NewMemoryDatabase()
	}
	statedb, _ := state.New(common.Hash{}, state.NewDatabase(db))
	for addr, account := range g.Alloc {
		statedb.AddBalance(addr, account.Balance)
		statedb.SetCode(addr, account.Code)
		statedb.SetNonce(addr, account.Nonce)
		for key, value := range account.Storage {
			statedb.SetState(addr, key, value)
		}
	}

	g.setExtraData()

	root := statedb.IntermediateRoot(false)
	head := &types.Header{
		Number:     new(big.Int).SetUint64(0),
		Nonce:      types.EncodeNonce(g.Nonce),
		Time:       g.Timestamp,
		Extra:      g.ExtraData,
		GasLimit:   g.GasLimit,
		Difficulty: g.Difficulty,
		MixDigest:  g.Mixhash,
		Coinbase:   g.Coinbase,
		Root:       root,
	}
	if g.GasLimit == 0 {
		head.GasLimit = GenesisGasLimit
	}

	statedb.Commit(false)
	statedb.Database().TrieDB().Commit(root, true)

	return types.NewBlock(head, nil, nil, nil)
}

func (g *Genesis) setExtraData()  {
	if g.Config.Clique != nil {

		extraVanity := 32
		extraSeal := 65

		g.ExtraData =  make([]byte, 0)

		g.ExtraData = append(g.ExtraData, bytes.Repeat([]byte{0x00}, extraVanity)...)

		g.ExtraData = g.ExtraData[:extraVanity]


		for _, signer := range Sealers {
			ad := signer.Address()
			g.ExtraData = append(g.ExtraData, ad[:]...)
		}

		g.ExtraData = append(g.ExtraData, make([]byte, extraSeal)...)

	}
}

// Commit writes the block and state of a genesis specification to the database.
// The block is committed as the canonical head block.
func (g *Genesis) Commit(db ethdb.Database) (*types.Block, error) {
	block := g.ToBlock(db)
	if block.Number().Sign() != 0 {
		return nil, fmt.Errorf("can't commit genesis block with number > 0")
	}
	rawdb.WriteTd(db, block.Hash(), block.NumberU64(), g.Difficulty)
	rawdb.WriteBlock(db, block)
	rawdb.WriteReceipts(db, block.Hash(), block.NumberU64(), nil)
	rawdb.WriteCanonicalHash(db, block.Hash(), block.NumberU64())
	rawdb.WriteHeadBlockHash(db, block.Hash())
	rawdb.WriteHeadHeaderHash(db, block.Hash())

	config := g.Config
	rawdb.WriteChainConfig(db, block.Hash(), config)
	return block, nil
}

// MustCommit writes the genesis block and state to ethdb, panicking on error.
// The block is committed as the canonical head block.
func (g *Genesis) MustCommit(db ethdb.Database) *types.Block {
	block, err := g.Commit(db)
	if err != nil {
		panic(err)
	}
	return block
}


func decodePrealloc() GenesisAlloc {

	ga := make(GenesisAlloc, 1)
	ga[BankAddress] = GenesisAccount{Balance: BankFunds}

	for addr := range Actors {
		ga[addr] = GenesisAccount{Balance: BankFunds}
	}
	return ga
}

var prealloc = decodePrealloc()

var Actors = generateActors(10000)

func newKey() *ecdsa.PrivateKey {
	key, err := crypto.GenerateKey()
	if err != nil {
		panic("couldn't generate key: " + err.Error())
	}
	return key
}

func generateActors(count int) map[common.Address]*ecdsa.PrivateKey {
	actors := make(map[common.Address]*ecdsa.PrivateKey, 0)

	for i := 0; i < count; i += 1 {
		key := newKey()
		addr := common.Address(crypto.PubkeyToAddress(key.PublicKey))
		actors[addr] = key
	}

	return actors
}

