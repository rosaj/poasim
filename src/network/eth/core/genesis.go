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
	"../common"
	"../core/rawdb"
	"../core/state"
	"../core/types"
	"../ethdb"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rlp"
)

//go:generate gencodec -type Genesis -field-override genesisSpecMarshaling -out gen_genesis.go
//go:generate gencodec -type GenesisAccount -field-override genesisAccountMarshaling -out gen_genesis_account.go

var errGenesisNoConfig = errors.New("genesis has no chain configuration")

// Genesis specifies the header fields, state of a genesis block. It also defines hard
// fork switch-over blocks through the chain configuration.
type Genesis struct {
	Config     *params.ChainConfig `json:"config"`
	Nonce      uint64              `json:"nonce"`
	Timestamp  uint64              `json:"timestamp"`
	ExtraData  []byte              `json:"extraData"`
	GasLimit   uint64              `json:"gasLimit"   gencodec:"required"`
	Difficulty *big.Int            `json:"difficulty" gencodec:"required"`
	Mixhash    common.Hash         `json:"mixHash"`
	Coinbase   common.Address      `json:"coinbase"`
	Alloc      GenesisAlloc        `json:"alloc"      gencodec:"required"`

	// These fields are used for consensus tests. Please don't use them
	// in actual genesis blocks.
	Number     uint64      `json:"number"`
	GasUsed    uint64      `json:"gasUsed"`
	ParentHash common.Hash `json:"parentHash"`
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

// field type overrides for gencodec
type genesisSpecMarshaling struct {
	Nonce      math.HexOrDecimal64
	Timestamp  math.HexOrDecimal64
	ExtraData  hexutil.Bytes
	GasLimit   math.HexOrDecimal64
	GasUsed    math.HexOrDecimal64
	Number     math.HexOrDecimal64
	Difficulty *math.HexOrDecimal256
	Alloc      map[common.UnprefixedAddress]GenesisAccount
}

type genesisAccountMarshaling struct {
	Code       hexutil.Bytes
	Balance    *math.HexOrDecimal256
	Nonce      math.HexOrDecimal64
	Storage    map[storageJSON]storageJSON
	PrivateKey hexutil.Bytes
}

// storageJSON represents a 256 bit byte array, but allows less than 256 bits when
// unmarshaling from hex.
type storageJSON common.Hash

func (h *storageJSON) UnmarshalText(text []byte) error {
	text = bytes.TrimPrefix(text, []byte("0x"))
	if len(text) > 64 {
		return fmt.Errorf("too many hex characters in storage key/value %q", text)
	}
	offset := len(h) - len(text)/2 // pad on the left
	if _, err := hex.Decode(h[offset:], text); err != nil {
		fmt.Println(err)
		return fmt.Errorf("invalid hex storage key/value %q", text)
	}
	return nil
}

func (h storageJSON) MarshalText() ([]byte, error) {
	return hexutil.Bytes(h[:]).MarshalText()
}

// GenesisMismatchError is raised when trying to overwrite an existing
// genesis block with an incompatible one.
type GenesisMismatchError struct {
	Stored, New common.Hash
}

func (e *GenesisMismatchError) Error() string {
	return fmt.Sprintf("database contains incompatible genesis (have %x, new %x)", e.Stored, e.New)
}

// SetupGenesisBlock writes or updates the genesis block in ethdb.
// The block that will be used is:
//
//                          genesis == nil       genesis != nil
//                       +------------------------------------------
//     ethdb has no genesis |  main-net default  |  genesis
//     ethdb has genesis    |  from DB           |  genesis (if compatible)
//
// The stored chain configuration will be updated if it is compatible (i.e. does not
// specify a fork block below the local head block). In case of a conflict, the
// error is a *params.ConfigCompatError and the new, unwritten config is returned.
//
// The returned chain configuration is never nil.
func SetupGenesisBlock(db ethdb.Database, genesis *Genesis) (*params.ChainConfig, common.Hash, error) {
	return SetupGenesisBlockWithOverride(db, genesis)
}
func SetupGenesisBlockWithOverride(db ethdb.Database, genesis *Genesis) (*params.ChainConfig, common.Hash, error) {
	if genesis != nil && genesis.Config == nil {
		return params.AllEthashProtocolChanges, common.Hash{}, errGenesisNoConfig
	}
	// Just commit the new block if there is no stored genesis block.
	stored := rawdb.ReadCanonicalHash(db, 0)
	if (stored == common.Hash{}) {
		if genesis == nil {
			log.Info("Writing default main-net genesis block")
			genesis = DefaultRinkebyGenesisBlock()
		} else {
			log.Info("Writing custom genesis block")
		}
		block, err := genesis.Commit(db)
		return genesis.Config, block.Hash(), err
	}

	// Check whether the genesis block is already written.
	if genesis != nil {
		hash := genesis.ToBlock(nil).Hash()
		if hash != stored {
			return genesis.Config, hash, &GenesisMismatchError{stored, hash}
		}
	}

	// Get the existing chain configuration.
	newcfg := genesis.configOrDefault(stored)

	storedcfg := rawdb.ReadChainConfig(db, stored)
	if storedcfg == nil {
		log.Warn("Found genesis block without chain config")
		rawdb.WriteChainConfig(db, stored, newcfg)
		return newcfg, stored, nil
	}
	// Special case: don't change the existing config of a non-mainnet chain if no new
	// config is supplied. These chains would get AllProtocolChanges (and a compat error)
	// if we just continued here.
	if genesis == nil && stored != common.Hash(params.RinkebyGenesisHash) {
		return storedcfg, stored, nil
	}

	// Check config compatibility and write the config. Compatibility errors
	// are returned to the caller unless we're already at block zero.
	height := rawdb.ReadHeaderNumber(db, rawdb.ReadHeadHeaderHash(db))
	if height == nil {
		return newcfg, stored, fmt.Errorf("missing block number for head header hash")
	}
	compatErr := storedcfg.CheckCompatible(newcfg, *height)
	if compatErr != nil && *height != 0 && compatErr.RewindTo != 0 {
		return newcfg, stored, compatErr
	}
	rawdb.WriteChainConfig(db, stored, newcfg)
	return newcfg, stored, nil
}

func (g *Genesis) configOrDefault(ghash common.Hash) *params.ChainConfig {
	if g != nil {
		return g.Config
	}

	/*
	switch {
	case g != nil:
		return g.Config
	case ghash == params.MainnetGenesisHash:
		return params.MainnetChainConfig
	case ghash == params.TestnetGenesisHash:
		return params.TestnetChainConfig
	default:
		return params.AllEthashProtocolChanges
	}
	*/
	return params.RinkebyChainConfig
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


	root := statedb.IntermediateRoot(false)
	head := &types.Header{
		Number:     new(big.Int).SetUint64(g.Number),
		Nonce:      types.EncodeNonce(g.Nonce),
		Time:       g.Timestamp,
		ParentHash: g.ParentHash,
		Extra:      g.ExtraData,
		GasLimit:   g.GasLimit,
		GasUsed:    g.GasUsed,
		Difficulty: g.Difficulty,
		MixDigest:  g.Mixhash,
		Coinbase:   g.Coinbase,
		Root:       root,
	}
	if g.GasLimit == 0 {
		head.GasLimit = params.GenesisGasLimit
	}
	if g.Difficulty == nil {
		head.Difficulty = params.GenesisDifficulty
	}
	statedb.Commit(false)
	statedb.Database().TrieDB().Commit(root, true)

	return types.NewBlock(head, nil, nil, nil)
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
	if config == nil {
		config = params.AllEthashProtocolChanges
	}
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

// GenesisBlockForTesting creates and writes a block in which addr has the given wei balance.
func GenesisBlockForTesting(db ethdb.Database, addr common.Address, balance *big.Int) *types.Block {
	g := Genesis{Alloc: GenesisAlloc{addr: {Balance: balance}}}
	return g.MustCommit(db)
}


// DefaultRinkebyGenesisBlock returns the Rinkeby network genesis block.
func DefaultRinkebyGenesisBlock() *Genesis {
	return &Genesis{
		Config:     params.RinkebyChainConfig,
		Timestamp:  0,
		ExtraData:  hexutil.MustDecode("0x52657370656374206d7920617574686f7269746168207e452e436172746d616e42eb768f2244c8811c63729a21a3569731535f067ffc57839b00206d1ad20c69a1981b489f772031b279182d99e65703f0076e4812653aab85fca0f00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"),
		GasLimit:   4700000,
		Difficulty: big.NewInt(1),
		Alloc:      decodePrealloc(rinkebyAllocData),
	}
}


func decodePrealloc(data string) GenesisAlloc {
	var p []struct{ Addr, Balance *big.Int }
	if err := rlp.NewStream(strings.NewReader(data), 0).Decode(&p); err != nil {
		panic(err)
	}
	ga := make(GenesisAlloc, len(p))
	for _, account := range p {
		ga[common.BigToAddress(account.Addr)] = GenesisAccount{Balance: account.Balance}
	}
	return ga
}


const rinkebyAllocData = "\xf9\x03\xb7\u0080\x01\xc2\x01\x01\xc2\x02\x01\xc2\x03\x01\xc2\x04\x01\xc2\x05\x01\xc2\x06\x01\xc2\a\x01\xc2\b\x01\xc2\t\x01\xc2\n\x01\xc2\v\x01\xc2\f\x01\xc2\r\x01\xc2\x0e\x01\xc2\x0f\x01\xc2\x10\x01\xc2\x11\x01\xc2\x12\x01\xc2\x13\x01\xc2\x14\x01\xc2\x15\x01\xc2\x16\x01\xc2\x17\x01\xc2\x18\x01\xc2\x19\x01\xc2\x1a\x01\xc2\x1b\x01\xc2\x1c\x01\xc2\x1d\x01\xc2\x1e\x01\xc2\x1f\x01\xc2 \x01\xc2!\x01\xc2\"\x01\xc2#\x01\xc2$\x01\xc2%\x01\xc2&\x01\xc2'\x01\xc2(\x01\xc2)\x01\xc2*\x01\xc2+\x01\xc2,\x01\xc2-\x01\xc2.\x01\xc2/\x01\xc20\x01\xc21\x01\xc22\x01\xc23\x01\xc24\x01\xc25\x01\xc26\x01\xc27\x01\xc28\x01\xc29\x01\xc2:\x01\xc2;\x01\xc2<\x01\xc2=\x01\xc2>\x01\xc2?\x01\xc2@\x01\xc2A\x01\xc2B\x01\xc2C\x01\xc2D\x01\xc2E\x01\xc2F\x01\xc2G\x01\xc2H\x01\xc2I\x01\xc2J\x01\xc2K\x01\xc2L\x01\xc2M\x01\xc2N\x01\xc2O\x01\xc2P\x01\xc2Q\x01\xc2R\x01\xc2S\x01\xc2T\x01\xc2U\x01\xc2V\x01\xc2W\x01\xc2X\x01\xc2Y\x01\xc2Z\x01\xc2[\x01\xc2\\\x01\xc2]\x01\xc2^\x01\xc2_\x01\xc2`\x01\xc2a\x01\xc2b\x01\xc2c\x01\xc2d\x01\xc2e\x01\xc2f\x01\xc2g\x01\xc2h\x01\xc2i\x01\xc2j\x01\xc2k\x01\xc2l\x01\xc2m\x01\xc2n\x01\xc2o\x01\xc2p\x01\xc2q\x01\xc2r\x01\xc2s\x01\xc2t\x01\xc2u\x01\xc2v\x01\xc2w\x01\xc2x\x01\xc2y\x01\xc2z\x01\xc2{\x01\xc2|\x01\xc2}\x01\xc2~\x01\xc2\u007f\x01\u00c1\x80\x01\u00c1\x81\x01\u00c1\x82\x01\u00c1\x83\x01\u00c1\x84\x01\u00c1\x85\x01\u00c1\x86\x01\u00c1\x87\x01\u00c1\x88\x01\u00c1\x89\x01\u00c1\x8a\x01\u00c1\x8b\x01\u00c1\x8c\x01\u00c1\x8d\x01\u00c1\x8e\x01\u00c1\x8f\x01\u00c1\x90\x01\u00c1\x91\x01\u00c1\x92\x01\u00c1\x93\x01\u00c1\x94\x01\u00c1\x95\x01\u00c1\x96\x01\u00c1\x97\x01\u00c1\x98\x01\u00c1\x99\x01\u00c1\x9a\x01\u00c1\x9b\x01\u00c1\x9c\x01\u00c1\x9d\x01\u00c1\x9e\x01\u00c1\x9f\x01\u00c1\xa0\x01\u00c1\xa1\x01\u00c1\xa2\x01\u00c1\xa3\x01\u00c1\xa4\x01\u00c1\xa5\x01\u00c1\xa6\x01\u00c1\xa7\x01\u00c1\xa8\x01\u00c1\xa9\x01\u00c1\xaa\x01\u00c1\xab\x01\u00c1\xac\x01\u00c1\xad\x01\u00c1\xae\x01\u00c1\xaf\x01\u00c1\xb0\x01\u00c1\xb1\x01\u00c1\xb2\x01\u00c1\xb3\x01\u00c1\xb4\x01\u00c1\xb5\x01\u00c1\xb6\x01\u00c1\xb7\x01\u00c1\xb8\x01\u00c1\xb9\x01\u00c1\xba\x01\u00c1\xbb\x01\u00c1\xbc\x01\u00c1\xbd\x01\u00c1\xbe\x01\u00c1\xbf\x01\u00c1\xc0\x01\u00c1\xc1\x01\u00c1\xc2\x01\u00c1\xc3\x01\u00c1\xc4\x01\u00c1\xc5\x01\u00c1\xc6\x01\u00c1\xc7\x01\u00c1\xc8\x01\u00c1\xc9\x01\u00c1\xca\x01\u00c1\xcb\x01\u00c1\xcc\x01\u00c1\xcd\x01\u00c1\xce\x01\u00c1\xcf\x01\u00c1\xd0\x01\u00c1\xd1\x01\u00c1\xd2\x01\u00c1\xd3\x01\u00c1\xd4\x01\u00c1\xd5\x01\u00c1\xd6\x01\u00c1\xd7\x01\u00c1\xd8\x01\u00c1\xd9\x01\u00c1\xda\x01\u00c1\xdb\x01\u00c1\xdc\x01\u00c1\xdd\x01\u00c1\xde\x01\u00c1\xdf\x01\u00c1\xe0\x01\u00c1\xe1\x01\u00c1\xe2\x01\u00c1\xe3\x01\u00c1\xe4\x01\u00c1\xe5\x01\u00c1\xe6\x01\u00c1\xe7\x01\u00c1\xe8\x01\u00c1\xe9\x01\u00c1\xea\x01\u00c1\xeb\x01\u00c1\xec\x01\u00c1\xed\x01\u00c1\xee\x01\u00c1\xef\x01\u00c1\xf0\x01\u00c1\xf1\x01\u00c1\xf2\x01\u00c1\xf3\x01\u00c1\xf4\x01\u00c1\xf5\x01\u00c1\xf6\x01\u00c1\xf7\x01\u00c1\xf8\x01\u00c1\xf9\x01\u00c1\xfa\x01\u00c1\xfb\x01\u00c1\xfc\x01\u00c1\xfd\x01\u00c1\xfe\x01\u00c1\xff\x01\xf6\x941\xb9\x8d\x14\x00{\xde\xe67)\x80\x86\x98\x8a\v\xbd1\x18E#\xa0\x02\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"