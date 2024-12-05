package rollup

import (
	"github.com/ethereum-optimism/optimism/op-service/eth"

	preimage "github.com/ethereum-optimism/optimism/go/op-preimage"
)

const (
	HintL2BlockWithBatchInfo   = "l2-block-with-batch-info"
	HintL2BlockStateCommitment = "l2-block-state-commitment"
	HintL1EnqueueTx            = "l1-enqueue-tx"
)

type L2BlockStateCommitment eth.Uint64Quantity

var _ preimage.Hint = L2BlockStateCommitment(0)

func (l L2BlockStateCommitment) Hint() string {
	return HintL2BlockStateCommitment + " " + (eth.Uint64Quantity)(l).String()
}

type L2BlockWithBatchInfo eth.Uint64Quantity

var _ preimage.Hint = L2BlockWithBatchInfo(0)

func (l L2BlockWithBatchInfo) Hint() string {
	return HintL2BlockWithBatchInfo + " " + (eth.Uint64Quantity)(l).String()
}

type L1EnqueueTxHint eth.Uint64Quantity

var _ preimage.Hint = L1EnqueueTxHint(0)

func (l L1EnqueueTxHint) Hint() string {
	return HintL1EnqueueTx + " " + (eth.Uint64Quantity)(l).String()
}
