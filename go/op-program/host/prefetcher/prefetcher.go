package prefetcher

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"

	ethereum "github.com/MetisProtocol/mvm/l2geth"
	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/crypto"
	"github.com/MetisProtocol/mvm/l2geth/rlp"
	dtl "github.com/MetisProtocol/mvm/l2geth/rollup"
	clientDTL "github.com/ethereum-optimism/optimism/go/op-program/client/dtl"
	"github.com/ethereum-optimism/optimism/go/op-program/client/l1"
	merkletrie "github.com/ethereum-optimism/optimism/go/op-program/client/merkel"

	preimage "github.com/ethereum-optimism/optimism/go/op-preimage"

	"github.com/ethereum-optimism/optimism/go/op-program/client/l2"
	"github.com/ethereum-optimism/optimism/go/op-program/client/mpt"
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
	l1Fetcher     L1Source
	l1BlobFetcher L1BlobSource
	l2Fetcher     L2Source
	dtlFetcher    dtl.RollupClient
	lastHint      string
	kvStore       kvstore.KV
}

func NewPrefetcher(logger log.Logger, l1Fetcher L1Source, l1BlobFetcher L1BlobSource, l2Fetcher L2Source, dtlFetcher dtl.RollupClient, kvStore kvstore.KV) *Prefetcher {
	return &Prefetcher{
		logger:        logger,
		l1Fetcher:     NewRetryingL1Source(logger, l1Fetcher),
		l1BlobFetcher: NewRetryingL1BlobSource(logger, l1BlobFetcher),
		l2Fetcher:     NewRetryingL2Source(logger, l2Fetcher),
		dtlFetcher:    dtlFetcher,
		kvStore:       kvStore,
	}
}

func (p *Prefetcher) Hint(hint string) error {
	p.logger.Trace("Received hint", "hint", hint)
	p.lastHint = hint
	return nil
}

func (p *Prefetcher) GetPreimage(ctx context.Context, key common.Hash) ([]byte, error) {
	p.logger.Trace("Pre-image requested", "key", key)
	pre, err := p.kvStore.Get(ethcommon.Hash(key))
	// Use a loop to keep retrying the prefetch as long as the key is not found
	// This handles the case where the prefetch downloads a preimage, but it is then deleted unexpectedly
	// before we get to read it.
	for errors.Is(err, kvstore.ErrNotFound) && p.lastHint != "" {
		hint := p.lastHint
		if err := p.prefetch(ctx, hint); err != nil {
			return nil, fmt.Errorf("prefetch failed: %w", err)
		}
		pre, err = p.kvStore.Get(ethcommon.Hash(key))
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
	case l1.HintL1BlockHeader:
		if len(hintBytes) != 32 {
			return fmt.Errorf("invalid L1 block hint: %x", hint)
		}
		hash := common.Hash(hintBytes)
		header, err := p.l1Fetcher.InfoByHash(ctx, ethcommon.Hash(hash))
		if err != nil {
			return fmt.Errorf("failed to fetch L1 block %s header: %w", hash, err)
		}
		data, err := header.HeaderRLP()
		if err != nil {
			return fmt.Errorf("marshall header: %w", err)
		}
		return p.kvStore.Put(preimage.Keccak256Key(hash).PreimageKey(), data)
	case l1.HintL1Transactions:
		if len(hintBytes) != 32 {
			return fmt.Errorf("invalid L1 transactions hint: %x", hint)
		}
		hash := common.Hash(hintBytes)
		_, txs, err := p.l1Fetcher.InfoAndTxsByHash(ctx, ethcommon.Hash(hash))
		if err != nil {
			return fmt.Errorf("failed to fetch L1 block %s txs: %w", hash, err)
		}
		return p.storeL1Transactions(txs)
	case l1.HintL1Receipts:
		if len(hintBytes) != 32 {
			return fmt.Errorf("invalid L1 receipts hint: %x", hint)
		}
		hash := common.Hash(hintBytes)
		_, receipts, err := p.l1Fetcher.FetchReceipts(ctx, ethcommon.Hash(hash))
		if err != nil {
			return fmt.Errorf("failed to fetch L1 block %s receipts: %w", hash, err)
		}
		return p.storeReceipts(receipts)
	case l1.HintL1Blob:
		if len(hintBytes) != 48 {
			return fmt.Errorf("invalid blob hint: %x", hint)
		}

		blobVersionHash := ethcommon.Hash(hintBytes[:32])
		blobHashIndex := binary.BigEndian.Uint64(hintBytes[32:40])
		refTimestamp := binary.BigEndian.Uint64(hintBytes[40:48])

		// Fetch the blob sidecar for the indexed blob hash passed in the hint.
		indexedBlobHash := eth.IndexedBlobHash{
			Hash:  blobVersionHash,
			Index: blobHashIndex,
		}
		// We pass an `eth.L1BlockRef`, but `GetBlobSidecars` only uses the timestamp, which we received in the hint.
		sidecars, err := p.l1BlobFetcher.GetBlobSidecars(ctx, eth.L1BlockRef{Time: refTimestamp}, []eth.IndexedBlobHash{indexedBlobHash})
		if err != nil || len(sidecars) != 1 {
			return fmt.Errorf("failed to fetch blob sidecars for %s %d: %w", blobVersionHash, blobHashIndex, err)
		}
		sidecar := sidecars[0]

		// Put the preimage for the versioned hash into the kv store
		if err = p.kvStore.Put(preimage.Sha256Key(blobVersionHash).PreimageKey(), sidecar.KZGCommitment[:]); err != nil {
			return err
		}

		// Put all of the blob's field elements into the kv store. There should be 4096. The preimage oracle key for
		// each field element is the keccak256 hash of `abi.encodePacked(sidecar.KZGCommitment, uint256(i))`
		blobKey := make([]byte, 80)
		copy(blobKey[:48], sidecar.KZGCommitment[:])
		for i := 0; i < params.BlobTxFieldElementsPerBlob; i++ {
			binary.BigEndian.PutUint64(blobKey[72:], uint64(i))
			blobKeyHash := crypto.Keccak256Hash(blobKey)
			if err := p.kvStore.Put(preimage.Keccak256Key(blobKeyHash).PreimageKey(), blobKey); err != nil {
				return err
			}
			if err = p.kvStore.Put(preimage.BlobKey(blobKeyHash).PreimageKey(), sidecar.Blob[i<<5:(i+1)<<5]); err != nil {
				return err
			}
		}
		return nil
	case l1.HintL1Precompile:
		if len(hintBytes) < 20 {
			return fmt.Errorf("invalid precompile hint: %x", hint)
		}
		precompileAddress := common.BytesToAddress(hintBytes[:20])
		// For extra safety, avoid accelerating unexpected precompiles
		if !slices.Contains(acceleratedPrecompiles, precompileAddress) {
			return fmt.Errorf("unsupported precompile address: %s", precompileAddress)
		}
		// NOTE: We use the precompiled contracts from Cancun because it's the only set that contains the addresses of all accelerated precompiles
		// We assume the precompile Run function behavior does not change across EVM upgrades.
		// As such, we must not rely on upgrade-specific behavior such as precompile.RequiredGas.
		precompile := getPrecompiledContract(precompileAddress)

		// KZG Point Evaluation precompile also verifies its input
		result, err := precompile.Run(hintBytes[20:])
		if err == nil {
			result = append(precompileSuccess[:], result...)
		} else {
			result = append(precompileFailure[:], result...)
		}
		inputHash := crypto.Keccak256Hash(hintBytes)
		// Put the input preimage so it can be loaded later
		if err := p.kvStore.Put(preimage.Keccak256Key(inputHash).PreimageKey(), hintBytes); err != nil {
			return err
		}
		return p.kvStore.Put(preimage.PrecompileKey(inputHash).PreimageKey(), result)
	case l1.HintL1PrecompileV2:
		if len(hintBytes) < 28 {
			return fmt.Errorf("invalid precompile hint: %x", hint)
		}
		precompileAddress := common.BytesToAddress(hintBytes[:20])
		// requiredGas := hintBytes[20:28] - unused by the host. Since the client already validates gas requirements.
		// The requiredGas is only used by the L1 PreimageOracle to enforce complete precompile execution.

		// For extra safety, avoid accelerating unexpected precompiles
		if !slices.Contains(acceleratedPrecompiles, precompileAddress) {
			return fmt.Errorf("unsupported precompile address: %s", precompileAddress)
		}
		// NOTE: We use the precompiled contracts from Cancun because it's the only set that contains the addresses of all accelerated precompiles
		// We assume the precompile Run function behavior does not change across EVM upgrades.
		// As such, we must not rely on upgrade-specific behavior such as precompile.RequiredGas.
		precompile := getPrecompiledContract(precompileAddress)

		// KZG Point Evaluation precompile also verifies its input
		result, err := precompile.Run(hintBytes[28:])
		if err == nil {
			result = append(precompileSuccess[:], result...)
		} else {
			result = append(precompileFailure[:], result...)
		}
		inputHash := crypto.Keccak256Hash(hintBytes)
		// Put the input preimage so it can be loaded later
		if err := p.kvStore.Put(preimage.Keccak256Key(inputHash).PreimageKey(), hintBytes); err != nil {
			return err
		}
		return p.kvStore.Put(preimage.PrecompileKey(inputHash).PreimageKey(), result)
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
	case l2.HintL2StateNode, l2.HintL2Code:
		if len(hintBytes) != 32 {
			return fmt.Errorf("invalid L2 state node hint: %x", hint)
		}
		hash := common.Hash(hintBytes)
		node, err := p.l2Fetcher.NodeByHash(ctx, hash)
		if err != nil {
			return fmt.Errorf("failed to fetch L2 state node %s: %w", hash, err)
		}
		return p.kvStore.Put(preimage.Keccak256Key(hash).PreimageKey(), node)
	case clientDTL.HintStateBatch:
		if len(hintBytes) != 32 {
			return fmt.Errorf("invalid State batch hint: %x", hint)
		}
		hash := common.Hash(hintBytes)
		stateHeader, stateRoots, err := p.dtlFetcher.GetStateBatchHeader(hash)
		if err != nil {
			return fmt.Errorf("failed to fetch state header %s: %w", hash, err)
		}

		// use ABI encode here, since our batch header in SCC is encoded by ABI
		packed, err := stateHeader.Pack()
		if err != nil {
			return fmt.Errorf("failed to pack state header: %w", err)
		}

		if err := p.kvStore.Put(preimage.Keccak256Key(hash).PreimageKey(), packed); err != nil {
			return fmt.Errorf("failed to store state header: %w", err)
		}

		return p.storeStateBatches(stateRoots)
	}

	return fmt.Errorf("unknown hint type: %v", hintType)
}

func (p *Prefetcher) storeStateBatches(stateBatches []common.Hash) error {
	opaqueBatches := make([]hexutil.Bytes, len(stateBatches))
	for i, hash := range stateBatches {
		opaqueBatches[i] = hash.Bytes()
	}

	_, nodes := merkletrie.WriteTrie(opaqueBatches)
	for _, node := range nodes {
		key := preimage.Keccak256Key(crypto.Keccak256Hash(node)).PreimageKey()
		if err := p.kvStore.Put(key, node); err != nil {
			return fmt.Errorf("failed to store node: %w", err)
		}
	}
	return nil
}

func (p *Prefetcher) storeReceipts(receipts ethtypes.Receipts) error {
	opaqueReceipts, err := eth.EncodeReceipts(receipts)
	if err != nil {
		return err
	}
	return p.storeTrieNodes(opaqueReceipts)
}

func (p *Prefetcher) storeL1Transactions(txs ethtypes.Transactions) error {
	opaqueTxs, err := eth.EncodeTransactions(txs)
	if err != nil {
		return err
	}
	return p.storeTrieNodes(opaqueTxs)
}

func (p *Prefetcher) storeL2Transactions(txs types.Transactions) error {
	opaqueTxs := make([]hexutil.Bytes, len(txs))
	for i := range txs {
		opaqueTxs[i] = txs.GetRlp(i)
	}

	return p.storeTrieNodes(opaqueTxs)
}

func (p *Prefetcher) storeTrieNodes(values []hexutil.Bytes) error {
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
	return vm.PrecompiledContractsCancun[ethcommon.Address(address)]
}
