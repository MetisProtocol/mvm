package rollup

import (
	"github.com/ethereum-optimism/optimism/op-service/eth"

	preimage "github.com/ethereum-optimism/optimism/go/op-preimage"
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
	return HintRollupBatchOfBlock + " " + (eth.Uint64Quantity)(l).String()
}

type RollupBlockMeta uint64

var _ preimage.Hint = RollupBlockMeta(0)

func (l RollupBlockMeta) Hint() string {
	return HintRollupBlockMeta + " " + (eth.Uint64Quantity)(l).String()
}

type RollupBatchTransactions uint64

var _ preimage.Hint = RollupBatchTransactions(0)

func (l RollupBatchTransactions) Hint() string {
	return HintRollupBatchTransactions + " " + (eth.Uint64Quantity)(l).String()
}

type RollupBlockStateCommitment eth.Uint64Quantity

var _ preimage.Hint = RollupBlockStateCommitment(0)

func (l RollupBlockStateCommitment) Hint() string {
	return HintRollupBlockStateCommitment + " " + (eth.Uint64Quantity)(l).String()
}
