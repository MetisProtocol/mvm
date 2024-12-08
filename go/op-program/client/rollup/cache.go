package rollup

import (
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/hashicorp/golang-lru/v2/simplelru"

	"github.com/MetisProtocol/mvm/l2geth/common"
	dtl "github.com/MetisProtocol/mvm/l2geth/rollup"
)

// blockCacheSize should be set large enough to handle the pipeline reset process of walking back from L2 head to find
// the L1 origin that is old enough to start buffering channel data from.
const blockCacheSize = 3_000

var _ Oracle = (*CachingOracle)(nil)

type CachingOracle struct {
	oracle                     Oracle
	rollupBlockStateCommitment *simplelru.LRU[uint64, eth.Bytes32]
	rollupBatches              *simplelru.LRU[uint64, *dtl.Batch]
	rollupBatchTxs             *simplelru.LRU[common.Hash, *dtl.Transaction]
	rollupBlockMetas           *simplelru.LRU[uint64, *BlockMeta]
}

func NewCachingOracle(oracle Oracle) *CachingOracle {
	rollupBatches, _ := simplelru.NewLRU[uint64, *dtl.Batch](blockCacheSize, nil)
	rollupStateCommitment, _ := simplelru.NewLRU[uint64, eth.Bytes32](blockCacheSize, nil)
	rollupBatchTxs, _ := simplelru.NewLRU[common.Hash, *dtl.Transaction](blockCacheSize, nil)
	rollupBlockMetas, _ := simplelru.NewLRU[uint64, *BlockMeta](blockCacheSize, nil)
	return &CachingOracle{
		oracle:                     oracle,
		rollupBlockStateCommitment: rollupStateCommitment,
		rollupBatches:              rollupBatches,
		rollupBatchTxs:             rollupBatchTxs,
		rollupBlockMetas:           rollupBlockMetas,
	}
}

func (o *CachingOracle) L2BatchOfBlock(block uint64) *dtl.Batch {
	rollupBatch, ok := o.rollupBatches.Get(block)
	if ok {
		return rollupBatch
	}

	rollupBatch = o.oracle.L2BatchOfBlock(block)
	o.rollupBatches.Add(block, rollupBatch)
	return rollupBatch
}

func (o *CachingOracle) L2BlockMeta(block uint64) *BlockMeta {
	rollupBlockMeta, ok := o.rollupBlockMetas.Get(block)
	if ok {
		return rollupBlockMeta
	}

	rollupBlockMeta = o.oracle.L2BlockMeta(block)
	o.rollupBlockMetas.Add(block, rollupBlockMeta)
	return rollupBlockMeta
}

func (o *CachingOracle) L2StateCommitment(block uint64) eth.Bytes32 {
	l2StateCommitment, ok := o.rollupBlockStateCommitment.Get(block)
	if ok {
		return l2StateCommitment
	}

	l2StateCommitment = o.oracle.L2StateCommitment(block)
	o.rollupBlockStateCommitment.Add(block, l2StateCommitment)
	return l2StateCommitment
}

func (o *CachingOracle) L2BatchTransaction(block uint64, txIndex uint64) *dtl.Transaction {
	batchTx, ok := o.rollupBatchTxs.Get(common.Hash{})
	if ok {
		return batchTx
	}

	batchTx = o.oracle.L2BatchTransaction(block, txIndex)
	o.rollupBatchTxs.Add(common.Hash{}, batchTx)
	return batchTx
}
