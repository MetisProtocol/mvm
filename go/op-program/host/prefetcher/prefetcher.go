package prefetcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ethereum-optimism/optimism/op-service/eth"

	ethcommon "github.com/ethereum/go-ethereum/common"
	ethhex "github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"

	ethereum "github.com/MetisProtocol/mvm/l2geth"
	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/common/hexutil"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/core/vm"
	"github.com/MetisProtocol/mvm/l2geth/crypto"
	"github.com/MetisProtocol/mvm/l2geth/rlp"
	dtl "github.com/MetisProtocol/mvm/l2geth/rollup"

	preimage "github.com/ethereum-optimism/optimism/go/op-preimage"

	"github.com/ethereum-optimism/optimism/go/op-program/client/l2"
	"github.com/ethereum-optimism/optimism/go/op-program/client/mpt"
	"github.com/ethereum-optimism/optimism/go/op-program/client/rollup"
	"github.com/ethereum-optimism/optimism/go/op-program/host/kvstore"
)

var (
	precompileSuccess = [1]byte{1}
	precompileFailure = [1]byte{0}
)

var acceleratedPrecompiles = []common.Address{
	common.BytesToAddress([]byte{0x1}),  // ecrecover
	common.BytesToAddress([]byte{0x8}),  // bn256Pairing
	common.BytesToAddress([]byte{0x0a}), // KZG Point Evaluation
}

type L1Source interface {
	InfoByHash(ctx context.Context, blockHash ethcommon.Hash) (eth.BlockInfo, error)
	InfoAndTxsByHash(ctx context.Context, blockHash ethcommon.Hash) (eth.BlockInfo, ethtypes.Transactions, error)
	FetchReceipts(ctx context.Context, blockHash ethcommon.Hash) (eth.BlockInfo, ethtypes.Receipts, error)
}

type L1BlobSource interface {
	GetBlobSidecars(ctx context.Context, ref eth.L1BlockRef, hashes []eth.IndexedBlobHash) ([]*eth.BlobSidecar, error)
	GetBlobs(ctx context.Context, ref eth.L1BlockRef, hashes []eth.IndexedBlobHash) ([]*eth.Blob, error)
}

type L2Source interface {
	ethereum.ChainReader

	NodeByHash(ctx context.Context, hash common.Hash) ([]byte, error)
}

type Prefetcher struct {
	logger        log.Logger
	l2Fetcher     L2Source
	rollupFetcher dtl.RollupClient
	lastHint      string
	kvStore       kvstore.KV
}

func NewPrefetcher(logger log.Logger, l2Fetcher L2Source, rollupFetcher dtl.RollupClient, kvStore kvstore.KV) *Prefetcher {
	return &Prefetcher{
		logger:        logger,
		l2Fetcher:     NewRetryingL2Source(logger, l2Fetcher),
		rollupFetcher: rollupFetcher,
		kvStore:       kvStore,
	}
}

func (p *Prefetcher) Hint(hint string) error {
	p.logger.Debug("Received hint", "hint", hint)
	p.lastHint = hint
	return nil
}

func (p *Prefetcher) GetPreimage(ctx context.Context, key common.Hash) ([]byte, error) {
	p.logger.Debug("Pre-image requested", "key", key)
	pre, err := p.kvStore.Get(key)
	// Use a loop to keep retrying the prefetch as long as the key is not found
	// This handles the case where the prefetch downloads a preimage, but it is then deleted unexpectedly
	// before we get to read it.
	for errors.Is(err, kvstore.ErrNotFound) && p.lastHint != "" {
		hint := p.lastHint
		if err := p.prefetch(ctx, hint); err != nil {
			return nil, fmt.Errorf("prefetch failed: %w", err)
		}
		pre, err = p.kvStore.Get(key)
		if err != nil {
			p.logger.Error("Fetched pre-images for last hint but did not find required key", "hint", hint, "key", key)
		}
	}
	return pre, err
}

func (p *Prefetcher) prefetch(ctx context.Context, hint string) error {
	hintType, hintBytes, err := parseHint(hint)
	if err != nil {
		return err
	}
	p.logger.Debug("Prefetching", "type", hintType, "bytes", hexutil.Bytes(hintBytes))
	switch hintType {
	case l2.HintL2BlockHeader, l2.HintL2Transactions:
		if len(hintBytes) != 32 {
			return fmt.Errorf("invalid L2 header/tx hint: %x", hint)
		}
		hash := common.Hash(hintBytes)
		block, err := p.l2Fetcher.BlockByHash(ctx, hash)
		if err != nil {
			return fmt.Errorf("failed to fetch L2 block %s: %w", hash, err)
		}
		data, err := rlp.EncodeToBytes(block.Header())
		if err != nil {
			return fmt.Errorf("failed to encode header to RLP: %w", err)
		}
		err = p.kvStore.Put(preimage.Keccak256Key(hash).PreimageKey(), data)
		if err != nil {
			return err
		}
		return p.storeL2Transactions(block.Transactions())
	case l2.HintL2StateNode:
		if len(hintBytes) != 32 {
			return fmt.Errorf("invalid L2 state node hint: %x", hint)
		}
		hash := common.Hash(hintBytes)
		node, err := p.l2Fetcher.NodeByHash(ctx, hash)
		if err != nil {
			return fmt.Errorf("failed to fetch L2 state node %s: %w", hash, err)
		}
		return p.kvStore.Put(preimage.Keccak256Key(hash).PreimageKey(), node)
	case l2.HintL2Code:
		if len(hintBytes) != 32 {
			return fmt.Errorf("invalid L2 code hint: %x", hint)
		}
		hash := common.Hash(hintBytes)
		code, err := p.l2Fetcher.NodeByHash(ctx, hash)
		if err != nil {
			return fmt.Errorf("failed to fetch L2 contract code %s: %w", hash, err)
		}
		return p.kvStore.Put(preimage.Keccak256Key(hash).PreimageKey(), code)
	case rollup.HintL2BlockWithBatchInfo:
		if len(hintBytes) > 8 {
			return fmt.Errorf("invalid L2 block batch key: %x", hint)
		}

		var l2Block eth.Uint64Quantity
		if err := l2Block.UnmarshalText([]byte(hexutil.Encode(hintBytes))); err != nil {
			return fmt.Errorf("failed to unmarshal batch index: %w", err)
		}

		p.logger.Debug("Fetching L2 block with batch info", "block", l2Block)

		blockResp, err := p.rollupFetcher.GetRawBlock(uint64(l2Block), dtl.BackendL1)
		if err != nil {
			return fmt.Errorf("failed to fetch block %d from dtl: %w", l2Block, err)
		}

		marshaled, err := json.Marshal(blockResp)
		if err != nil {
			return fmt.Errorf("failed to marshal block response: %w", err)
		}

		return p.kvStore.Put(preimage.L2BlockWittBatchInfoKey(l2Block).PreimageKey(), marshaled)
	case rollup.HintL1EnqueueTx:
		if len(hintBytes) > 8 {
			return fmt.Errorf("invalid l1 enqueue tx hint: %x", hint)
		}

		var enqueueIndex eth.Uint64Quantity
		if err := enqueueIndex.UnmarshalText([]byte(hexutil.Encode(hintBytes))); err != nil {
			return fmt.Errorf("failed to unmarshal enqueue index: %w", err)
		}
		enqueueTx, err := p.rollupFetcher.GetEnqueue(uint64(enqueueIndex))
		if err != nil {
			return fmt.Errorf("failed to fetch batch %d: %w", enqueueIndex, err)
		}

		opaqueTx, err := rlp.EncodeToBytes(enqueueTx)
		if err != nil {
			return fmt.Errorf("failed to marshal enqueue tx: %w", err)
		}

		return p.kvStore.Put(preimage.EnqueueTxKey(enqueueIndex).PreimageKey(), opaqueTx)
	}

	return fmt.Errorf("unknown hint type: %v", hintType)
}

func (p *Prefetcher) storeL1Receipts(receipts ethtypes.Receipts) error {
	data, err := eth.EncodeReceipts(receipts)
	if err != nil {
		return fmt.Errorf("failed to encode receipts: %w", err)
	}

	return p.storeTrieNodes(data)
}

func (p *Prefetcher) storeL2Receipts(receipts types.Receipts) error {
	opaqueReceipts := make([]ethhex.Bytes, len(receipts))
	for i, el := range receipts {
		dat, err := rlp.EncodeToBytes(el)
		if err != nil {
			return fmt.Errorf("failed to marshal receipt %d: %w", i, err)
		}
		opaqueReceipts[i] = dat
	}

	return p.storeTrieNodes(opaqueReceipts)
}

func (p *Prefetcher) storeL1Transactions(txs ethtypes.Transactions) error {
	data, err := eth.EncodeTransactions(txs)
	if err != nil {
		return fmt.Errorf("failed to encode transactions: %w", err)
	}

	return p.storeTrieNodes(data)
}

func (p *Prefetcher) storeL2Transactions(txs types.Transactions) error {
	opaqueTxs := make([]ethhex.Bytes, len(txs))
	for i, el := range txs {
		dat, err := rlp.EncodeToBytes(el)
		if err != nil {
			return fmt.Errorf("failed to marshal tx %d: %w", i, err)
		}
		opaqueTxs[i] = dat
	}

	return p.storeTrieNodes(opaqueTxs)
}

func (p *Prefetcher) storeTrieNodes(values []ethhex.Bytes) error {
	_, nodes := mpt.WriteTrie(values)
	for _, node := range nodes {
		key := preimage.Keccak256Key(crypto.Keccak256Hash(node)).PreimageKey()
		if err := p.kvStore.Put(key, node); err != nil {
			return fmt.Errorf("failed to store node: %w", err)
		}
	}
	return nil
}

// parseHint parses a hint string in wire protocol. Returns the hint type, requested hash and error (if any).
func parseHint(hint string) (string, []byte, error) {
	hintType, bytesStr, found := strings.Cut(hint, " ")
	if !found {
		return "", nil, fmt.Errorf("unsupported hint: %s", hint)
	}

	hintBytes, err := hexutil.Decode(bytesStr)
	if err != nil {
		return "", make([]byte, 0), fmt.Errorf("invalid bytes: %s", bytesStr)
	}
	return hintType, hintBytes, nil
}

func getPrecompiledContract(address common.Address) vm.PrecompiledContract {
	return vm.PrecompiledContractsBerlin[address]
}
