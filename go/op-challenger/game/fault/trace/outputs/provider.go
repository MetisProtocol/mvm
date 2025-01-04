package outputs

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"runtime"

	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
	l2hex "github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/log"

	l2common "github.com/MetisProtocol/mvm/l2geth/common"
	l2types "github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/rollup"
	"github.com/ethereum-optimism/optimism/go/op-challenger/game/fault/trace/utils"
	"github.com/ethereum-optimism/optimism/go/op-challenger/game/fault/types"
	merkletrie "github.com/ethereum-optimism/optimism/go/op-program/client/merkel"
)

var (
	// l2 header fetch concurrency
	maxL2HeaderFetchConcurrency = runtime.NumCPU()*2 + 1
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
}

func NewTraceProvider(logger log.Logger, prestateProvider types.PrestateProvider, rollupProvider rollup.RollupClient, l2Client utils.L2HeaderSource, l1Head eth.BlockID, gameDepth types.Depth, prestateBlock, poststateBlock uint64) *OutputTraceProvider {
	return &OutputTraceProvider{
		PrestateProvider: prestateProvider,
		logger:           logger,
		rollupProvider:   rollupProvider,
		l2Client:         l2Client,
		prestateBlock:    prestateBlock,
		poststateBlock:   poststateBlock,
		l1Head:           l1Head,
		gameDepth:        gameDepth,
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

	resp, err := o.getStateRoot(outputBlock - 1)
	if err != nil {
		return 0, fmt.Errorf("failed to get state root %v: %w", outputBlock, err)
	}

	// unlike only can move block by block, we need to move batch by batch
	batchEndBlock := uint64(resp.Batch.PrevTotalElements) + uint64(resp.Batch.Size)
	if outputBlock != batchEndBlock {
		outputBlock = batchEndBlock
	}

	o.logger.Debug("Claimed", "outputBlock", outputBlock)

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

	claimedBatch, err := o.getStateRoot(outputBlock - 1)
	if err != nil {
		return 0, fmt.Errorf("failed to get state root %v: %w", outputBlock, err)
	}

	// back searching for output block at target l1 head
	for claimedBatch.Batch.BlockNumber > o.l1Head.Number {
		claimedBatch, err = o.getStateRoot(uint64(claimedBatch.Batch.PrevTotalElements))
		if err != nil {
			return 0, fmt.Errorf("failed to get state root %v: %w", outputBlock, err)
		}

		o.logger.Debug("Traversing back for l1 head", "target", o.l1Head.Number,
			"current", claimedBatch.Batch.BlockNumber, "index", claimedBatch.Batch.Index)
	}

	// safe head is the last block in previous batch
	maxSafeHead := uint64(claimedBatch.Batch.PrevTotalElements + claimedBatch.Batch.Size)
	if outputBlock > maxSafeHead {
		outputBlock = maxSafeHead
	}

	o.logger.Debug("Honest", "outputBlock", outputBlock)

	return outputBlock, nil
}

func (o *OutputTraceProvider) Get(ctx context.Context, pos types.Position) (common.Hash, error) {
	outputBlock, err := o.ClaimedBlockNumber(pos)
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
	batchIndex, batchHeader, err := o.batchHeaderAtBlock(block)
	if err != nil {
		return common.Hash{}, err
	}

	// we need to recalculate the state root merkle proof of the blocks in the given batch
	batchSizeU64 := batchHeader.BatchSize.Uint64()
	headersRequestCh := make(chan uint64, batchSizeU64)
	headersResultCh := make(chan *l2types.Header, batchSizeU64)
	for i := batchHeader.PrevTotalElements.Uint64() + 1; i < batchHeader.PrevTotalElements.Uint64()+batchSizeU64+1; i++ {
		headersRequestCh <- i
	}

	for i := 0; i < maxL2HeaderFetchConcurrency; i++ {
		go func(index int) {
			for {
				select {
				case block, ok := <-headersRequestCh:
					if !ok {
						o.logger.Debug("header fetcher done", "index", index)
						return
					}
					header, err := o.getL2Header(ctx, block)
					if err != nil {
						o.logger.Error("failed to get L2 header at block", "block", block, "err", err)
					}

					select {
					case headersResultCh <- header:
					default:
						o.logger.Error("header fetcher result channel is full", "index", index)
						return
					}
				case <-ctx.Done():
					o.logger.Debug("header fetcher cancelled", "index", index)
					return
				}
			}
		}(i)
	}

	stateRoots := make([]l2hex.Bytes, batchSizeU64)
	collected := uint64(0)
	for {
		select {
		case <-ctx.Done():
			return common.Hash{}, fmt.Errorf("context cancelled: %w", ctx.Err())
		case header := <-headersResultCh:
			offset := header.Number.Uint64() - 1 - batchHeader.PrevTotalElements.Uint64()
			stateRoots[offset] = header.Root.Bytes()
			collected++
		}

		if collected == batchSizeU64 {
			// all headers fetched
			close(headersRequestCh)
			o.logger.Debug("all headers fetched", "collected", collected, "batchIndex", batchIndex)
			break
		}
	}

	// replace the root with new calculated one
	root, _ := merkletrie.WriteTrie(stateRoots)
	batchHeader.BatchRoot = l2common.Hash(root)

	// calculate new state batch header hash
	return common.Hash(batchHeader.Hash()), nil
}

func (o *OutputTraceProvider) batchHeaderAtBlock(block uint64) (uint64, *rollup.BatchHeader, error) {
	stateBatchResp, err := o.getStateRoot(block - 1)
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
	return o.l2Client.HeaderByNumber(ctx, new(big.Int).SetUint64(block))
}

func (o *OutputTraceProvider) getStateRoot(index uint64) (root *rollup.StateRootResponse, err error) {
	return o.rollupProvider.GetRawStateRoot(index)
}

func (o *OutputTraceProvider) getStateBatch(batchIndex uint64) (resp *rollup.StateRootBatchResponse, err error) {
	return o.rollupProvider.GetStateBatchByIndex(batchIndex)
}
