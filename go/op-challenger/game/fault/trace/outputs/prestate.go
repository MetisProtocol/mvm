package outputs

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/MetisProtocol/mvm/l2geth/rollup"
	"github.com/ethereum-optimism/optimism/go/op-challenger/game/fault/types"
)

var _ types.PrestateProvider = (*OutputPrestateProvider)(nil)

type OutputPrestateProvider struct {
	prestateBlock uint64
	rollupClient  rollup.RollupClient
}

func NewPrestateProvider(rollupClient rollup.RollupClient, prestateBlock uint64) *OutputPrestateProvider {
	return &OutputPrestateProvider{
		prestateBlock: prestateBlock,
		rollupClient:  rollupClient,
	}
}

func (o *OutputPrestateProvider) AbsolutePreStateCommitment(_ context.Context) (hash common.Hash, err error) {
	return o.outputAtBlock(o.prestateBlock)
}

func (o *OutputPrestateProvider) outputAtBlock(block uint64) (common.Hash, error) {
	txChainResp, err := o.rollupClient.GetRawBlock(block, rollup.BackendL1)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to fetch output at block %v: %w", block, err)
	}
	stateBatchResp, err := o.rollupClient.GetStateBatchByIndex(txChainResp.Batch.Index)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to fetch state batch for block %v: %w", block, err)
	}

	return common.Hash(rollup.BatchHeader{
		BatchRoot:         stateBatchResp.Batch.Root,
		BatchSize:         big.NewInt(int64(stateBatchResp.Batch.Size)),
		PrevTotalElements: big.NewInt(int64(stateBatchResp.Batch.PrevTotalElements)),
		ExtraData:         rollup.ExtraData(stateBatchResp.Batch.ExtraData),
	}.Hash()), nil
}
