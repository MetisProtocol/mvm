package rollup

import (
	"github.com/ethereum-optimism/optimism/op-service/eth"

	preimage "github.com/ethereum-optimism/optimism/go/op-preimage"
	"github.com/ethereum-optimism/optimism/go/op-program/common"
)

const (
	HintRollupBatchOfBlock         = "rollup-batch-of-block"
	HintRollupBlockMeta            = "rollup-block-meta"
	HintRollupBatchTransactions    = "rollup-batch-transactions"
	HintRollupBlockStateCommitment = "rollup-block-state-commitment"
)

type RollupBatchOfBlock uint64

var _ preimage.Hint = RollupBatchOfBlock(0)

func (l RollupBatchOfBlock) Hint() string {
	return HintRollupBatchOfBlock + " " + (common.Uint64)(l).EncodeHex()
}

type RollupBlockMeta uint64

var _ preimage.Hint = RollupBlockMeta(0)

func (l RollupBlockMeta) Hint() string {
	return HintRollupBlockMeta + " " + (common.Uint64)(l).EncodeHex()
}

type RollupBatchTransactions uint64

var _ preimage.Hint = RollupBatchTransactions(0)

func (l RollupBatchTransactions) Hint() string {
	return HintRollupBatchTransactions + " " + (common.Uint64)(l).EncodeHex()
}

type RollupBlockStateCommitment eth.Uint64Quantity

var _ preimage.Hint = RollupBlockStateCommitment(0)

func (l RollupBlockStateCommitment) Hint() string {
	return HintRollupBlockStateCommitment + " " + (common.Uint64)(l).EncodeHex()
}
