package derive

import (
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-service/eth"

	"github.com/ethereum/go-ethereum/common"

	"github.com/MetisProtocol/mvm/l2geth/core/types"
)

func L2BlockToBlockRef(block *types.Block) eth.L2BlockRef {
	return eth.L2BlockRef{
		Hash:       common.Hash(block.Hash()),
		Number:     block.NumberU64(),
		ParentHash: common.Hash(block.ParentHash()),
		Time:       block.Time(),

		// unlike op using the deposit tx to save l1 block info in every block,
		// for these two fields we need to retrieve them from the DTL,
		// but since we are in the oracle engine, we don't have access to the DTL,
		// need to use hint to get the l1 block info from the host
		L1Origin:       eth.BlockID{},
		SequenceNumber: 0,
	}
}

// PayloadToBlockRef extracts the essential L2BlockRef information from an execution payload,
// falling back to genesis information if necessary.
func PayloadToBlockRef(rollupCfg *rollup.Config, payload *eth.ExecutionPayload) (eth.L2BlockRef, error) {
	return eth.L2BlockRef{
		Hash:       payload.BlockHash,
		Number:     uint64(payload.BlockNumber),
		ParentHash: payload.ParentHash,
		Time:       uint64(payload.Timestamp),

		// unlike op using the deposit tx to save l1 block info in every block,
		// for these two fields we need to retrieve them from the DTL,
		// but since we are in the oracle engine, we don't have access to the DTL,
		// need to use hint to get the l1 block info from the host
		L1Origin:       eth.BlockID{},
		SequenceNumber: 0,
	}, nil
}
