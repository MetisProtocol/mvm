package test

import (
	"encoding/binary"
	"testing"

	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/MetisProtocol/mvm/l2geth/core/types"
	l2db "github.com/MetisProtocol/mvm/l2geth/ethdb"
)

// Same as l2.StateOracle but need to use our own copy to avoid dependency loops
type stateOracle interface {
	NodeByHash(nodeHash common.Hash) []byte
	CodeByHash(codeHash common.Hash) []byte
}

type StubBlockOracle struct {
	t       *testing.T
	Blocks  map[common.Hash]*types.Block
	Outputs map[common.Hash]eth.Output
	stateOracle
}

func NewStubOracle(t *testing.T) (*StubBlockOracle, *StubStateOracle) {
	stateOracle := NewStubStateOracle(t)
	blockOracle := StubBlockOracle{
		t:           t,
		Blocks:      make(map[common.Hash]*types.Block),
		Outputs:     make(map[common.Hash]eth.Output),
		stateOracle: stateOracle,
	}
	return &blockOracle, stateOracle
}

func (o StubBlockOracle) NodeByHash(nodeHash common.Hash) []byte {
	return o.stateOracle.NodeByHash(nodeHash)
}

func (o StubBlockOracle) BlockByHash(blockHash common.Hash) *types.Block {
	block, ok := o.Blocks[blockHash]
	if !ok {
		o.t.Fatalf("requested unknown block %s", blockHash)
	}
	return block
}

// KvStateOracle loads data from a source ethdb.KeyValueStore
type KvStateOracle struct {
	t      *testing.T
	Source l2db.KeyValueStore
}

func NewKvStateOracle(t *testing.T, db l2db.KeyValueStore) *KvStateOracle {
	return &KvStateOracle{
		t:      t,
		Source: db,
	}
}

func (o *KvStateOracle) NodeByHash(nodeHash common.Hash) []byte {
	val, err := o.Source.Get(nodeHash.Bytes())
	if err != nil {
		o.t.Fatalf("error retrieving node %v: %v", nodeHash, err)
	}
	return val
}

func (o *KvStateOracle) CodeByHash(hash common.Hash) []byte {
	return rawdb.ReadCode(o.Source, common.Hash(hash))
}

func NewStubStateOracle(t *testing.T) *StubStateOracle {
	return &StubStateOracle{
		t:    t,
		Data: make(map[common.Hash][]byte),
		Code: make(map[common.Hash][]byte),
	}
}

// StubStateOracle is a StateOracle implementation that reads from simple maps
type StubStateOracle struct {
	t    *testing.T
	Data map[common.Hash][]byte
	Code map[common.Hash][]byte
}

func (o *StubStateOracle) NodeByHash(nodeHash common.Hash) []byte {
	data, ok := o.Data[nodeHash]
	if !ok {
		o.t.Fatalf("no value for node %v", nodeHash)
	}
	return data
}

func (o *StubStateOracle) CodeByHash(hash common.Hash) []byte {
	data, ok := o.Code[hash]
	if !ok {
		o.t.Fatalf("no value for code %v", hash)
	}
	return data
}

type StubPrecompileOracle struct {
	t       *testing.T
	Results map[common.Hash]PrecompileResult
	Calls   int
}

func NewStubPrecompileOracle(t *testing.T) *StubPrecompileOracle {
	return &StubPrecompileOracle{t: t, Results: make(map[common.Hash]PrecompileResult)}
}

type PrecompileResult struct {
	Result []byte
	Ok     bool
}

func (o *StubPrecompileOracle) Precompile(address common.Address, input []byte, requiredGas uint64) ([]byte, bool) {
	arg := append(address.Bytes(), binary.BigEndian.AppendUint64(nil, requiredGas)...)
	arg = append(arg, input...)
	result, ok := o.Results[crypto.Keccak256Hash(arg)]
	if !ok {
		o.t.Fatalf("no value for point evaluation %x required gas %v", input, requiredGas)
	}
	o.Calls++
	return result.Result, result.Ok
}
