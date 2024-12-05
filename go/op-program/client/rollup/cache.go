package rollup

import (
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/hashicorp/golang-lru/v2/simplelru"

	"github.com/MetisProtocol/mvm/l2geth/core/types"
	dtl "github.com/MetisProtocol/mvm/l2geth/rollup"
)

// blockCacheSize should be set large enough to handle the pipeline reset process of walking back from L2 head to find
// the L1 origin that is old enough to start buffering channel data from.
const blockCacheSize = 3_000
const enqueueTxCacheSize = 3_000

type CachingOracle struct {
	oracle                 Oracle
	l2BlockStateCommitment *simplelru.LRU[uint64, eth.Bytes32]
	l2BlockWithBatchInfo   *simplelru.LRU[uint64, *dtl.BlockResponse]
	enqueueTxs             *simplelru.LRU[uint64, *types.Transaction]
}

func NewCachingOracle(oracle Oracle) *CachingOracle {
	l2BlockWithBatchInfo, _ := simplelru.NewLRU[uint64, *dtl.BlockResponse](blockCacheSize, nil)
	l2BlockStateCommitment, _ := simplelru.NewLRU[uint64, eth.Bytes32](blockCacheSize, nil)
	enqueueTxs, _ := simplelru.NewLRU[uint64, *types.Transaction](enqueueTxCacheSize, nil)
	return &CachingOracle{
		oracle:                 oracle,
		l2BlockWithBatchInfo:   l2BlockWithBatchInfo,
		l2BlockStateCommitment: l2BlockStateCommitment,
		enqueueTxs:             enqueueTxs,
	}
}

func (o *CachingOracle) L2BlockStateCommitment(block uint64) eth.Bytes32 {
	l2BlockStateCommitment, ok := o.l2BlockStateCommitment.Get(block)
	if ok {
		return l2BlockStateCommitment
	}
	l2BlockStateCommitment = o.oracle.L2BlockStateCommitment(block)
	o.l2BlockStateCommitment.Add(block, l2BlockStateCommitment)
	return l2BlockStateCommitment
}

func (o *CachingOracle) L2BlockWithBatchInfo(block uint64) *dtl.BlockResponse {
	l2Block, ok := o.l2BlockWithBatchInfo.Get(block)
	if ok {
		return l2Block
	}
	l2Block = o.oracle.L2BlockWithBatchInfo(block)
	o.l2BlockWithBatchInfo.Add(block, l2Block)
	return l2Block
}

func (o *CachingOracle) EnqueueTxByIndex(index uint64) *types.Transaction {
	enqueueTx, ok := o.enqueueTxs.Get(index)
	if ok {
		return enqueueTx
	}
	enqueueTx = o.oracle.EnqueueTxByIndex(index)
	o.enqueueTxs.Add(index, enqueueTx)
	return enqueueTx
}
