package rollup

import (
	"github.com/ethereum-optimism/optimism/op-service/eth"

	"github.com/MetisProtocol/mvm/l2geth/common/hexutil"
	"github.com/MetisProtocol/mvm/l2geth/rlp"
	preimage "github.com/ethereum-optimism/optimism/go/op-preimage"
)

const (
	HintRollupBatchOfBlock         = "rollup-batch-of-block"
	HintRollupBlockMeta            = "rollup-block-meta"
	HintRollupBatchTransaction     = "rollup-batch-transaction"
	HintRollupBlockStateCommitment = "rollup-block-state-commitment"
)

type RollupBatchOfBlock uint64

var _ preimage.Hint = RollupBatchOfBlock(0)

func (l RollupBatchOfBlock) Hint() string {
	return HintRollupBatchOfBlock + " " + (eth.Uint64Quantity)(l).String()
}

type RollupBlockMeta uint64

var _ preimage.Hint = RollupBlockMeta(0)

func (l RollupBlockMeta) Hint() string {
	return HintRollupBlockMeta + " " + (eth.Uint64Quantity)(l).String()
}

type RollupBatchTransaction struct {
	BlockIndex uint64
	TxIndex    uint64
}

var _ preimage.Hint = RollupBatchTransaction{}

func (l RollupBatchTransaction) Hint() string {
	rlpEncoded, _ := rlp.EncodeToBytes(l)
	return HintRollupBatchTransaction + " " + hexutil.Encode(rlpEncoded)
}

type RollupBlockStateCommitment eth.Uint64Quantity

var _ preimage.Hint = RollupBlockStateCommitment(0)

func (l RollupBlockStateCommitment) Hint() string {
	return HintRollupBlockStateCommitment + " " + (eth.Uint64Quantity)(l).String()
}
