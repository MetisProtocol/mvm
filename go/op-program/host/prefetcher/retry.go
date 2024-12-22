package prefetcher

import (
	"context"
	"math"
	"math/big"

	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum-optimism/optimism/op-service/retry"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"

	ethereum "github.com/MetisProtocol/mvm/l2geth"
	l2common "github.com/MetisProtocol/mvm/l2geth/common"
	l2types "github.com/MetisProtocol/mvm/l2geth/core/types"
)

const maxAttempts = math.MaxInt // Succeed or die trying

type RetryingL1Source struct {
	logger   log.Logger
	source   L1Source
	strategy retry.Strategy
}

func NewRetryingL1Source(logger log.Logger, source L1Source) *RetryingL1Source {
	return &RetryingL1Source{
		logger:   logger,
		source:   source,
		strategy: retry.Exponential(),
	}
}

func (s *RetryingL1Source) InfoByHash(ctx context.Context, blockHash common.Hash) (eth.BlockInfo, error) {
	return retry.Do(ctx, maxAttempts, s.strategy, func() (eth.BlockInfo, error) {
		res, err := s.source.InfoByHash(ctx, blockHash)
		if err != nil {
			s.logger.Warn("Failed to retrieve info", "hash", blockHash, "err", err)
		}
		return res, err
	})
}

func (s *RetryingL1Source) InfoAndTxsByHash(ctx context.Context, blockHash common.Hash) (eth.BlockInfo, types.Transactions, error) {
	return retry.Do2(ctx, maxAttempts, s.strategy, func() (eth.BlockInfo, types.Transactions, error) {
		i, t, err := s.source.InfoAndTxsByHash(ctx, blockHash)
		if err != nil {
			s.logger.Warn("Failed to retrieve l1 info and txs", "hash", blockHash, "err", err)
		}
		return i, t, err
	})
}

func (s *RetryingL1Source) FetchReceipts(ctx context.Context, blockHash common.Hash) (eth.BlockInfo, types.Receipts, error) {
	return retry.Do2(ctx, maxAttempts, s.strategy, func() (eth.BlockInfo, types.Receipts, error) {
		i, r, err := s.source.FetchReceipts(ctx, blockHash)
		if err != nil {
			s.logger.Warn("Failed to fetch receipts", "hash", blockHash, "err", err)
		}
		return i, r, err
	})
}

var _ L1Source = (*RetryingL1Source)(nil)

type RetryingL1BlobSource struct {
	logger   log.Logger
	source   L1BlobSource
	strategy retry.Strategy
}

func NewRetryingL1BlobSource(logger log.Logger, source L1BlobSource) *RetryingL1BlobSource {
	return &RetryingL1BlobSource{
		logger:   logger,
		source:   source,
		strategy: retry.Exponential(),
	}
}

func (s *RetryingL1BlobSource) GetBlobSidecars(ctx context.Context, ref eth.L1BlockRef, hashes []eth.IndexedBlobHash) ([]*eth.BlobSidecar, error) {
	return retry.Do(ctx, maxAttempts, s.strategy, func() ([]*eth.BlobSidecar, error) {
		sidecars, err := s.source.GetBlobSidecars(ctx, ref, hashes)
		if err != nil {
			s.logger.Warn("Failed to retrieve blob sidecars", "ref", ref, "err", err)
		}
		return sidecars, err
	})
}

func (s *RetryingL1BlobSource) GetBlobs(ctx context.Context, ref eth.L1BlockRef, hashes []eth.IndexedBlobHash) ([]*eth.Blob, error) {
	return retry.Do(ctx, maxAttempts, s.strategy, func() ([]*eth.Blob, error) {
		blobs, err := s.source.GetBlobs(ctx, ref, hashes)
		if err != nil {
			s.logger.Warn("Failed to retrieve blobs", "ref", ref, "err", err)
		}
		return blobs, err
	})
}

var _ L1BlobSource = (*RetryingL1BlobSource)(nil)

type RetryingL2Source struct {
	logger   log.Logger
	source   L2Source
	strategy retry.Strategy
}

func (s *RetryingL2Source) BlockByHash(ctx context.Context, hash l2common.Hash) (*l2types.Block, error) {
	return retry.Do(ctx, maxAttempts, s.strategy, func() (*l2types.Block, error) {
		return s.source.BlockByHash(ctx, hash)
	})
}

func (s *RetryingL2Source) BlockByNumber(ctx context.Context, number *big.Int) (*l2types.Block, error) {
	return retry.Do(ctx, maxAttempts, s.strategy, func() (*l2types.Block, error) {
		return s.source.BlockByNumber(ctx, number)
	})
}

func (s *RetryingL2Source) HeaderByHash(ctx context.Context, hash l2common.Hash) (*l2types.Header, error) {
	return retry.Do(ctx, maxAttempts, s.strategy, func() (*l2types.Header, error) {
		return s.source.HeaderByHash(ctx, hash)
	})
}

func (s *RetryingL2Source) HeaderByNumber(ctx context.Context, number *big.Int) (*l2types.Header, error) {
	return retry.Do(ctx, maxAttempts, s.strategy, func() (*l2types.Header, error) {
		return s.source.HeaderByNumber(ctx, number)
	})
}

func (s *RetryingL2Source) TransactionCount(ctx context.Context, blockHash l2common.Hash) (uint, error) {
	return retry.Do(ctx, maxAttempts, s.strategy, func() (uint, error) {
		return s.source.TransactionCount(ctx, blockHash)
	})
}

func (s *RetryingL2Source) TransactionInBlock(ctx context.Context, blockHash l2common.Hash, index uint) (*l2types.Transaction, error) {
	return retry.Do(ctx, maxAttempts, s.strategy, func() (*l2types.Transaction, error) {
		return s.source.TransactionInBlock(ctx, blockHash, index)
	})
}

func (s *RetryingL2Source) SubscribeNewHead(ctx context.Context, ch chan<- *l2types.Header) (ethereum.Subscription, error) {
	return s.source.SubscribeNewHead(ctx, ch)
}

func (s *RetryingL2Source) NodeByHash(ctx context.Context, hash l2common.Hash) ([]byte, error) {
	return retry.Do(ctx, maxAttempts, s.strategy, func() ([]byte, error) {
		n, err := s.source.NodeByHash(ctx, hash)
		if err != nil {
			s.logger.Warn("Failed to retrieve node", "hash", hash, "err", err)
		}
		return n, err
	})
}
func NewRetryingL2Source(logger log.Logger, source L2Source) *RetryingL2Source {
	return &RetryingL2Source{
		logger:   logger,
		source:   source,
		strategy: retry.Exponential(),
	}
}

var _ L2Source = (*RetryingL2Source)(nil)
