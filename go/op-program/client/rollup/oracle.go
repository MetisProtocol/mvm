package rollup

import (
	"encoding/json"

	"github.com/ethereum-optimism/optimism/op-service/eth"

	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/rlp"
	dtl "github.com/MetisProtocol/mvm/l2geth/rollup"

	preimage "github.com/ethereum-optimism/optimism/go/op-preimage"
)

// Oracle defines the high-level API used to retrieve rollup data.
// The returned data is always the preimage of the requested hash.
type Oracle interface {
	L2BlockWithBatchInfo(block uint64) *dtl.BlockResponse
	L2BlockStateCommitment(block uint64) eth.Bytes32
	EnqueueTxByIndex(index uint64) *types.Transaction
}

// PreimageOracle implements Oracle using by interfacing with the pure preimage.Oracle
// to fetch pre-images to decode into the requested data.
type PreimageOracle struct {
	oracle preimage.Oracle
	hint   preimage.Hinter
}

var _ Oracle = (*PreimageOracle)(nil)

func NewPreimageOracle(raw preimage.Oracle, hint preimage.Hinter) *PreimageOracle {
	return &PreimageOracle{
		oracle: raw,
		hint:   hint,
	}
}

func (p *PreimageOracle) L2BlockStateCommitment(block uint64) eth.Bytes32 {
	p.hint.Hint(L2BlockStateCommitment(block))

	return eth.Bytes32(p.oracle.Get(preimage.L2BlockStateCommitmentKey(block)))
}

func (p *PreimageOracle) L2BlockWithBatchInfo(block uint64) *dtl.BlockResponse {
	p.hint.Hint(L2BlockWithBatchInfo(block))

	l2BlockBytes := p.oracle.Get(preimage.L2BlockWittBatchInfoKey(block))

	var l2BlockWithBathInfo dtl.BlockResponse
	if err := json.Unmarshal(l2BlockBytes, &l2BlockWithBathInfo); err != nil {
		panic("failed to unmarshal l2BlockBytes, err: " + err.Error())
	}

	return &l2BlockWithBathInfo
}

func (p *PreimageOracle) EnqueueTxByIndex(index uint64) *types.Transaction {
	p.hint.Hint(L1EnqueueTxHint(index))

	opaqueTx := p.oracle.Get(preimage.EnqueueTxKey(index))

	var tx types.Transaction
	rlp.DecodeBytes(opaqueTx, &tx)

	return &tx
}
