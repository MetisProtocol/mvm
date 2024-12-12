package l2

import (
	"math/big"

	"github.com/ethereum/go-ethereum/log"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/consensus"
	"github.com/MetisProtocol/mvm/l2geth/consensus/clique"
	"github.com/MetisProtocol/mvm/l2geth/core/rawdb"
	"github.com/MetisProtocol/mvm/l2geth/core/state"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/core/vm"
	"github.com/MetisProtocol/mvm/l2geth/ethdb"
	"github.com/MetisProtocol/mvm/l2geth/params"

	"github.com/ethereum-optimism/optimism/go/op-program/client/l2/engineapi"
)

type OracleBackedL2Chain struct {
	log        log.Logger
	oracle     Oracle
	chainCfg   *params.ChainConfig
	engine     consensus.Engine
	oracleHead *types.Header
	head       *types.Header
	vmCfg      vm.Config

	// Block by number cache
	hashByNum            map[uint64]common.Hash
	earliestIndexedBlock *types.Header

	// Inserted blocks
	blocks map[common.Hash]*types.Block
	db     ethdb.KeyValueStore
}

func NewOracleBackedL2Chain(logger log.Logger, oracle Oracle, precompileOracle engineapi.PrecompileOracle, chainCfg *params.ChainConfig, l2Head common.Hash) (*OracleBackedL2Chain, error) {
	db := NewOracleBackedDB(oracle)
	head := oracle.BlockByHash(l2Head)
	logger.Info("Loaded L2 head", "hash", head.Hash(), "number", head.Number())
	return &OracleBackedL2Chain{
		log:      logger,
		oracle:   oracle,
		chainCfg: chainCfg,
		engine:   clique.New(chainCfg.Clique, db),

		hashByNum: map[uint64]common.Hash{
			head.NumberU64(): head.Hash(),
		},
		earliestIndexedBlock: head.Header(),

		// Treat the agreed starting head as finalized - nothing before it can be disputed
		head:       head.Header(),
		oracleHead: head.Header(),
		blocks:     make(map[common.Hash]*types.Block),
		db:         db,
		vmCfg: vm.Config{
			// TODO(@dumdumgoose): need to test whether precompiles are working properly without overrides
			PrecompileOverrides: engineapi.CreatePrecompileOverrides(precompileOracle),
		},
	}, nil
}

func (o *OracleBackedL2Chain) CurrentHeader() *types.Header {
	return o.head
}

func (o *OracleBackedL2Chain) GetHeaderByNumber(n uint64) *types.Header {
	if o.head.Number.Uint64() < n {
		return nil
	}
	hash, ok := o.hashByNum[n]
	if ok {
		return o.GetHeaderByHash(hash)
	}
	// Walk back from current head to the requested block number
	h := o.head
	for h.Number.Uint64() > n {
		h = o.GetHeaderByHash(h.ParentHash)
		o.hashByNum[h.Number.Uint64()] = h.Hash()
	}
	o.earliestIndexedBlock = h
	return h
}

func (o *OracleBackedL2Chain) GetTd(hash common.Hash, number uint64) *big.Int {
	// Difficulty is always 0 post-merge and bedrock starts post-merge so total difficulty also always 0
	return common.Big0
}

func (o *OracleBackedL2Chain) GetHeaderByHash(hash common.Hash) *types.Header {
	return o.GetBlockByHash(hash).Header()
}

func (o *OracleBackedL2Chain) GetBlockByHash(hash common.Hash) *types.Block {
	// Check inserted blocks
	block, ok := o.blocks[hash]
	if ok {
		return block
	}
	// Retrieve from the oracle
	return o.oracle.BlockByHash(hash)
}

func (o *OracleBackedL2Chain) GetBlock(hash common.Hash, number uint64) *types.Block {
	var block *types.Block
	if o.oracleHead.Number.Uint64() < number {
		// For blocks above the chain head, only consider newly built blocks
		// Avoids requesting an unknown block from the oracle which would panic.
		block = o.blocks[hash]
	} else {
		block = o.GetBlockByHash(hash)
	}
	if block == nil {
		return nil
	}
	if block.NumberU64() != number {
		return nil
	}
	return block
}

func (o *OracleBackedL2Chain) GetHeader(hash common.Hash, u uint64) *types.Header {
	block := o.GetBlock(hash, u)
	return block.Header()
}

func (o *OracleBackedL2Chain) HasBlockAndState(hash common.Hash, number uint64) bool {
	block := o.GetBlock(hash, number)
	return block != nil
}

func (o *OracleBackedL2Chain) GetCanonicalHash(n uint64) common.Hash {
	header := o.GetHeaderByNumber(n)
	if header == nil {
		return common.Hash{}
	}
	return header.Hash()
}

func (o *OracleBackedL2Chain) GetVMConfig() *vm.Config {
	return &o.vmCfg
}

func (o *OracleBackedL2Chain) Config() *params.ChainConfig {
	return o.chainCfg
}

func (o *OracleBackedL2Chain) Engine() consensus.Engine {
	return o.engine
}

func (o *OracleBackedL2Chain) StateAt(root common.Hash) (*state.StateDB, error) {
	stateDB, err := state.New(root, state.NewDatabase(rawdb.NewDatabase(o.db)))
	if err != nil {
		return nil, err
	}
	return stateDB, nil
}

func (o *OracleBackedL2Chain) InsertBlockWithoutSetHead(block *types.Block) {
	o.blocks[block.Hash()] = block
}

func (o *OracleBackedL2Chain) SetCanonical(head *types.Block) (common.Hash, error) {
	oldHead := o.head
	o.head = head.Header()

	// Remove canonical hashes after the new header
	for n := head.NumberU64() + 1; n <= oldHead.Number.Uint64(); n++ {
		delete(o.hashByNum, n)
	}

	// Add new canonical blocks to the block by number cache
	// Since the original head is added to the block number cache and acts as the finalized block,
	// at some point we must reach the existing canonical chain and can stop updating.
	h := o.head
	for {
		newHash := h.Hash()
		prevHash, ok := o.hashByNum[h.Number.Uint64()]
		if ok && prevHash == newHash {
			// Connected with the existing canonical chain so stop updating
			break
		}
		o.hashByNum[h.Number.Uint64()] = newHash
		h = o.GetHeaderByHash(h.ParentHash)
	}
	return head.Hash(), nil
}
