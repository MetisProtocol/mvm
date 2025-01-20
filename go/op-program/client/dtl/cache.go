package rollup

import (
	"github.com/hashicorp/golang-lru/v2/simplelru"

	"github.com/MetisProtocol/mvm/l2geth/common"
	dtl "github.com/MetisProtocol/mvm/l2geth/rollup"
)

// blockCacheSize should be set large enough to handle the pipeline reset process of walking back from L2 head to find
// the L1 origin that is old enough to start buffering channel data from.
const blockCacheSize = 3_000

var _ Oracle = (*CachingOracle)(nil)

type CachingOracle struct {
	oracle                  Oracle
	rollupBatchHeaders      *simplelru.LRU[common.Hash, *dtl.BatchHeader]
	rollupBatchStateBatches *simplelru.LRU[common.Hash, []common.Hash]
}

func NewCachingOracle(oracle Oracle) *CachingOracle {
	rollupBatchHeaders, _ := simplelru.NewLRU[common.Hash, *dtl.BatchHeader](blockCacheSize, nil)
	rollupBatchStateBatches, _ := simplelru.NewLRU[common.Hash, []common.Hash](blockCacheSize, nil)
	return &CachingOracle{
		oracle:                  oracle,
		rollupBatchHeaders:      rollupBatchHeaders,
		rollupBatchStateBatches: rollupBatchStateBatches,
	}
}

func (o *CachingOracle) StateBatchHeaderByHash(batchHash common.Hash) *dtl.BatchHeader {
	if batchHeader, ok := o.rollupBatchHeaders.Get(batchHash); ok {
		return batchHeader
	}

	batchHeader := o.oracle.StateBatchHeaderByHash(batchHash)
	o.rollupBatchHeaders.Add(batchHash, batchHeader)
	return batchHeader
}

func (o *CachingOracle) StateBatchesByHash(batchHash common.Hash) []common.Hash {
	if stateBatches, ok := o.rollupBatchStateBatches.Get(batchHash); ok {
		return stateBatches
	}

	stateBatches := o.oracle.StateBatchesByHash(batchHash)
	o.rollupBatchStateBatches.Add(batchHash, stateBatches)
	return stateBatches
}
