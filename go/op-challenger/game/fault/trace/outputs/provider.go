package outputs

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
	l2hex "github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/log"
	"github.com/hashicorp/golang-lru/v2/simplelru"

	l2common "github.com/MetisProtocol/mvm/l2geth/common"
	l2types "github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/rollup"
	"github.com/ethereum-optimism/optimism/go/op-challenger/game/fault/trace/utils"
	"github.com/ethereum-optimism/optimism/go/op-challenger/game/fault/types"
	merkletrie "github.com/ethereum-optimism/optimism/go/op-program/client/merkel"
)

const (
	// usually we have about 1 hour per batch,
	// 500 is more than enough to cache all batches in the last 7 days
	maxCachedStateBatches = 500
	// assume we have 2000 blocks per batch, and we will cache headers for a single batch
	maxCachedL2Headers = 2000
)

var (
	ErrGetStepData = errors.New("GetStepData not supported")
	ErrIndexTooBig = errors.New("trace index is greater than max uint64")
)

var _ types.TraceProvider = (*OutputTraceProvider)(nil)

// OutputTraceProvider is a [types.TraceProvider] implementation that uses
// output roots for given L2 Blocks as a trace.
type OutputTraceProvider struct {
	types.PrestateProvider
	logger         log.Logger
	rollupProvider rollup.RollupClient
	l2Client       utils.L2HeaderSource
	prestateBlock  uint64
	poststateBlock uint64
	l1Head         eth.BlockID
	gameDepth      types.Depth

	stateBatchCache    *simplelru.LRU[uint64, *rollup.StateRootBatchResponse]
	l2BlockHeaderCache *simplelru.LRU[uint64, *l2types.Header]
}

func NewTraceProvider(logger log.Logger, prestateProvider types.PrestateProvider, rollupProvider rollup.RollupClient, l2Client utils.L2HeaderSource, l1Head eth.BlockID, gameDepth types.Depth, prestateBlock, poststateBlock uint64) *OutputTraceProvider {
	stateBatchCache, _ := simplelru.NewLRU[uint64, *rollup.StateRootBatchResponse](maxCachedStateBatches, nil)
	l2BlockHeaderCache, _ := simplelru.NewLRU[uint64, *l2types.Header](maxCachedStateBatches, nil)

	return &OutputTraceProvider{
		PrestateProvider: prestateProvider,
		logger:           logger,
		rollupProvider:   rollupProvider,
		l2Client:         l2Client,
		prestateBlock:    prestateBlock,
		poststateBlock:   poststateBlock,
		l1Head:           l1Head,
		gameDepth:        gameDepth,

		stateBatchCache:    stateBatchCache,
		l2BlockHeaderCache: l2BlockHeaderCache,
	}
}

// ClaimedBlockNumber returns the block number for a position restricted only by the claimed L2 block number.
// The returned block number may be after the safe head reached by processing batch data up to the game's L1 head
func (o *OutputTraceProvider) ClaimedBlockNumber(pos types.Position) (uint64, error) {
	traceIndex := pos.TraceIndex(o.gameDepth)
	if !traceIndex.IsUint64() {
		return 0, fmt.Errorf("%w: %v", ErrIndexTooBig, traceIndex)
	}

	outputBlock := traceIndex.Uint64() + o.prestateBlock + 1
	if outputBlock > o.poststateBlock {
		outputBlock = o.poststateBlock
	}
	return outputBlock, nil
}

// HonestBlockNumber returns the block number for a position in the game restricted to the minimum of the claimed L2
// block number or the safe head reached by processing batch data up to the game's L1 head.
// This is used when posting honest output roots to ensure that only roots supported by L1 data are posted
func (o *OutputTraceProvider) HonestBlockNumber(_ context.Context, pos types.Position) (uint64, error) {
	outputBlock, err := o.ClaimedBlockNumber(pos)
	if err != nil {
		return 0, err
	}
	resp, err := o.rollupProvider.GetLatestStateBatches()
	if err != nil {
		return 0, fmt.Errorf("failed to get safe head at L1 block %v: %w", o.l1Head, err)
	}

	// search backwards until we found a batch that is small or equal to the given l1 head
	for resp.Batch.BlockNumber > o.l1Head.Number {
		prevIndex := resp.Batch.Index - 1
		resp, err = o.getStateBatch(prevIndex)
		if err != nil {
			return 0, fmt.Errorf("failed to get safe head at L1 block %v: %w", o.l1Head, err)
		}
	}

	// safe head is the last block in this batch
	maxSafeHead := uint64(resp.Batch.PrevTotalElements + resp.Batch.Size)
	if outputBlock > maxSafeHead {
		outputBlock = maxSafeHead
	}
	return outputBlock, nil
}

func (o *OutputTraceProvider) Get(ctx context.Context, pos types.Position) (common.Hash, error) {
	outputBlock, err := o.HonestBlockNumber(ctx, pos)
	if err != nil {
		return common.Hash{}, err
	}
	return o.outputAtBlock(ctx, outputBlock)
}

// GetStepData is not supported in the [OutputTraceProvider].
func (o *OutputTraceProvider) GetStepData(_ context.Context, _ types.Position) (prestate []byte, proofData []byte, preimageData *types.PreimageOracleData, err error) {
	return nil, nil, nil, ErrGetStepData
}

func (o *OutputTraceProvider) GetL2BlockNumberChallenge(ctx context.Context) (*types.InvalidL2BlockNumberChallenge, error) {
	outputBlock, err := o.HonestBlockNumber(ctx, types.RootPosition)
	if err != nil {
		return nil, err
	}
	claimedBlock, err := o.ClaimedBlockNumber(types.RootPosition)
	if err != nil {
		return nil, err
	}
	if claimedBlock == outputBlock {
		return nil, types.ErrL2BlockNumberValid
	}
	batchIndex, batchHeader, err := o.batchHeaderAtBlock(outputBlock)
	if err != nil {
		return nil, err
	}
	header, err := o.getL2Header(ctx, outputBlock)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve L2 block header %v: %w", outputBlock, err)
	}
	return types.NewInvalidL2BlockNumberProof(new(big.Int).SetUint64(batchIndex), batchHeader, header), nil
}

// outputAtBlock returns the output root for a given L2 block number. Since we are not like OP which have an output
// for each block, the returned hash will be the hash of the state batch header (same as what saves in SCC).
func (o *OutputTraceProvider) outputAtBlock(ctx context.Context, block uint64) (common.Hash, error) {
	_, batchHeader, err := o.batchHeaderAtBlock(block)
	if err != nil {
		return common.Hash{}, err
	}

	// we need to recalculate the state root merkle proof of the blocks in the given batch
	// TODO: query blocks concurrently
	stateRoots := make([]l2hex.Bytes, 0, batchHeader.BatchSize.Uint64())
	for i := batchHeader.PrevTotalElements.Uint64() + 1; i < batchHeader.PrevTotalElements.Uint64()+batchHeader.BatchSize.Uint64()+1; i++ {
		header, err := o.getL2Header(ctx, i)
		if err != nil {
			return common.Hash{}, fmt.Errorf("failed to get L2 header at block %v: %w", i, err)
		}

		stateRoots = append(stateRoots, header.Root[:])
	}

	// replace the root with new calculated one
	root, _ := merkletrie.WriteTrie(stateRoots)
	batchHeader.BatchRoot = l2common.Hash(root)

	// calculate new state batch header hash
	return common.Hash(batchHeader.Hash()), nil
}

func (o *OutputTraceProvider) batchHeaderAtBlock(block uint64) (uint64, *rollup.BatchHeader, error) {
	txChainResp, err := o.rollupProvider.GetRawBlock(block, rollup.BackendL1)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to fetch output at block %v: %w", block, err)
	}
	stateBatchResp, err := o.getStateBatch(txChainResp.Batch.Index)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to fetch state batch for block %v: %w", block, err)
	}

	return stateBatchResp.Batch.Index, &rollup.BatchHeader{
		BatchRoot:         stateBatchResp.Batch.Root,
		BatchSize:         big.NewInt(int64(stateBatchResp.Batch.Size)),
		PrevTotalElements: big.NewInt(int64(stateBatchResp.Batch.PrevTotalElements)),
		ExtraData:         rollup.ExtraData(stateBatchResp.Batch.ExtraData),
	}, nil
}

func (o *OutputTraceProvider) getL2Header(ctx context.Context, block uint64) (head *l2types.Header, err error) {
	if o.l2BlockHeaderCache.Contains(block) {
		// load from cache if available
		head, _ = o.l2BlockHeaderCache.Get(block)
	}
	if head == nil {
		// load from dtl if cache miss
		head, err = o.l2Client.HeaderByNumber(ctx, new(big.Int).SetUint64(block))
		if err != nil {
			return nil, fmt.Errorf("failed to get safe head at L1 block %v: %w", o.l1Head, err)
		}
		o.l2BlockHeaderCache.Add(head.Number.Uint64(), head)
	}

	return head, nil
}

func (o *OutputTraceProvider) getStateBatch(batchIndex uint64) (resp *rollup.StateRootBatchResponse, err error) {
	if o.stateBatchCache.Contains(batchIndex) {
		// load from cache if available
		resp, _ = o.stateBatchCache.Get(batchIndex)
	}
	if resp == nil {
		// load from dtl if cache miss
		resp, err = o.rollupProvider.GetStateBatchByIndex(batchIndex)
		if err != nil {
			return nil, fmt.Errorf("failed to get safe head at L1 block %v: %w", o.l1Head, err)
		}
		o.stateBatchCache.Add(resp.Batch.Index, resp)
	}

	return resp, nil
}
