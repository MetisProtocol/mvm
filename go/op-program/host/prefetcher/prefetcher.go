package prefetcher

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum-optimism/optimism/op-service/eth"

	ethcommon "github.com/ethereum/go-ethereum/common"
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
	progcommon "github.com/ethereum-optimism/optimism/go/op-program/common"

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

var zero uint64 = 0

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

	signer types.EIP155Signer
}

func NewPrefetcher(logger log.Logger, chainId *big.Int, l2Fetcher L2Source, rollupFetcher dtl.RollupClient, kvStore kvstore.KV) *Prefetcher {
	return &Prefetcher{
		logger:        logger,
		l2Fetcher:     NewRetryingL2Source(logger, l2Fetcher),
		rollupFetcher: rollupFetcher,
		kvStore:       kvStore,
		signer:        types.NewEIP155Signer(chainId),
	}
}

func (p *Prefetcher) Hint(hint string) error {
	p.logger.Debug("Received hint", "hint", hint)
	p.lastHint = hint
	return nil
}

func (p *Prefetcher) GetPreimage(ctx context.Context, key common.Hash) ([]byte, error) {
	p.logger.Info("Pre-image requested", "key", key)
	pre, err := p.kvStore.Get(key)
	// Use a loop to keep retrying the prefetch as long as the key is not found
	// This handles the case where the prefetch downloads a preimage, but it is then deleted unexpectedly
	// before we get to read it.
	for errors.Is(err, kvstore.ErrNotFound) && p.lastHint != "" {
		hint := p.lastHint
		if err := p.prefetch(ctx, hint); err != nil {
			return nil, fmt.Errorf("prefetch failed: %w", err)
		}
		p.logger.Info("🐸 Prefetched, and start getting key", "hint", hint, "key", key.Hex())
		pre, err = p.kvStore.Get(key)
		p.logger.Info("🐸 Got key", "key", key.Hex(), "preimage-length", len(pre), "err", err)
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
		p.logger.Debug("Fetching L2 block", "hash", hash.Hex())
		block, err := p.l2Fetcher.BlockByHash(ctx, hash)
		if err != nil {
			p.logger.Error("Failed to fetch L2 block", "hash", hash.Hex(), "error", err)
			return fmt.Errorf("failed to fetch L2 block %s: %w", hash, err)
		}
		p.logger.Debug("Fetched L2 block",
			"blockNumber", block.NumberU64(),
			"hash", block.Hash().Hex(),
			"txRoot", block.TxHash().Hex(),
			"txCount", len(block.Transactions()),
		)
		data, err := rlp.EncodeToBytes(block.Header())
		if err != nil {
			return fmt.Errorf("failed to encode header to RLP: %w", err)
		}
		err = p.kvStore.Put(preimage.Keccak256Key(hash).PreimageKey(), data)
		if err != nil {
			return err
		}
		p.logger.Debug("We are now storing txs for block", "block", block.NumberU64(), "tx-count", len(block.Transactions()))
		return p.storeL2Transactions(block.Transactions())
	case l2.HintL2StateNode, l2.HintL2Code:
		if len(hintBytes) != 32 {
			return fmt.Errorf("invalid L2 state node / code hint: %x", hint)
		}
		hash := common.Hash(hintBytes)
		node, err := p.l2Fetcher.NodeByHash(ctx, hash)
		if err != nil {
			return fmt.Errorf("failed to fetch L2 state node / code %s: %w", hash, err)
		}
		return p.kvStore.Put(preimage.Keccak256Key(hash).PreimageKey(), node)
	case rollup.HintRollupBlockMeta, rollup.HintRollupBatchOfBlock, rollup.HintRollupBatchTransactions:
		if len(hintBytes) > 8 {
			return fmt.Errorf("invalid L2 block batch key: %x", hint)
		}

		l2Block, err := progcommon.DecodeUint64Hint(hex.EncodeToString(hintBytes))
		if err != nil {
			return fmt.Errorf("failed to unmarshal batch index: %w", err)
		}
		if l2Block < 1 {
			return fmt.Errorf("invalid index: %d, need to be at least 1", l2Block)
		}

		// index is always less than 1 to the block
		block := l2Block
		index := l2Block - 1

		p.logger.Debug("Fetching L2 block with batch info", "index", index)

		blockResp, err := p.rollupFetcher.GetRawBlock(index, dtl.BackendL1)
		if err != nil {
			return fmt.Errorf("failed to fetch index %d from dtl: %w", index, err)
		}

		marshaled, err := rlp.EncodeToBytes(blockResp.Batch)
		if err != nil {
			return fmt.Errorf("failed to marshal block response: %w", err)
		}

		if err := p.kvStore.Put(preimage.RollupBlockBatchKey(block).PreimageKey(), marshaled); err != nil {
			return err
		}

		meta := rollup.BlockMeta{
			Index:            blockResp.Block.Index,
			BatchIndex:       blockResp.Block.BatchIndex,
			Timestamp:        blockResp.Block.Timestamp,
			TransactionCount: uint64(len(blockResp.Block.Transactions)),
			Confirmed:        blockResp.Block.Confirmed,
		}

		marshaled, err = rlp.EncodeToBytes(&meta)
		if err != nil {
			return fmt.Errorf("failed to marshal block response: %w", err)
		}

		if err := p.kvStore.Put(preimage.RollupBlockMetaKey(block).PreimageKey(), marshaled); err != nil {
			return err
		}

		parsedBlock, err := dtl.BatchedBlockToBlock(blockResp.Block, &p.signer)
		if err != nil {
			return fmt.Errorf("failed to parse block from DTL: %w", err)
		}

		// Note(@dumdumgoose): need to parse the tx for the second time, since we are missing some fields in the first parse,
		// need to insert the default values to the txs, otherwise tx root will be different
		for _, tx := range parsedBlock.Transactions() {
			meta := tx.GetMeta()
			// set default value for l1 message sender
			if meta.L1MessageSender == nil {
				meta.L1MessageSender = &common.Address{}
			}
			// set default value for queue index
			if meta.QueueIndex == nil {
				meta.QueueIndex = &zero
			}
			// set default values for seq sign
			if meta.R == nil {
				meta.R = new(big.Int)
				meta.S = new(big.Int)
				meta.V = new(big.Int)
			}
		}

		if marshaled, err = rlp.EncodeToBytes(parsedBlock.Transactions()); err != nil {
			return fmt.Errorf("failed to marshal txs: %w", err)
		}
		if err := p.kvStore.Put(preimage.RollupBatchTransactionsKey(block).PreimageKey(), marshaled); err != nil {
			return err
		}

		return nil
	case rollup.HintRollupBlockStateCommitment:
		if len(hintBytes) > 8 {
			return fmt.Errorf("invalid L2 block statecommitment key: %x", hint)
		}

		l2Block, err := progcommon.DecodeUint64Hint(hex.EncodeToString(hintBytes))
		if err != nil {
			return fmt.Errorf("failed to unmarshal block number: %w", err)
		}

		p.logger.Debug("Fetching L2 block with state commitment", "block", l2Block)
		scResp, err := p.rollupFetcher.GetStateRoot(uint64(l2Block) - 1)
		if err != nil {
			return fmt.Errorf("failed to fetch state commitment %d from dtl: %w", l2Block, err)
		}

		return p.kvStore.Put(preimage.RollupBlockStateCommitmentKey(l2Block).PreimageKey(), scResp.Bytes())
	case l2.HintL2BlockNumber:
		if len(hintBytes) > 8 {
			return fmt.Errorf("invalid L2 block number hint: %x", hint)
		}

		l2Block, err := progcommon.DecodeUint64Hint(hex.EncodeToString(hintBytes))
		if err != nil {
			return fmt.Errorf("failed to unmarshal block number: %w", err)
		}

		p.logger.Debug("Fetching L2 block number", "block", l2Block)
		block, err := p.l2Fetcher.BlockByNumber(ctx, big.NewInt(int64(l2Block)))
		if err != nil {
			return fmt.Errorf("failed to fetch block number %d: %w", l2Block, err)
		}

		headerBytes, err := rlp.EncodeToBytes(block.Header())
		if err != nil {
			return fmt.Errorf("failed to encode header: %w", err)
		}

		return p.kvStore.Put(preimage.BlockNumberKey(block.NumberU64()).PreimageKey(), headerBytes)
	}

	return fmt.Errorf("unknown hint type: %v", hintType)
}

func (p *Prefetcher) storeL2Transactions(txs types.Transactions) error {
	opaqueTxs := make([]hexutil.Bytes, len(txs))
	for i := range txs {
		opaqueTxs[i] = txs.GetRlp(i)
		p.logger.Debug("Storing tx", "index", i, "tx", txs[i].Hash().Hex(), "l2Tx", txs[i].L2Tx(), "rlpHash", crypto.Keccak256Hash(opaqueTxs[i]).Hex())
	}

	return p.storeTrieNodes(opaqueTxs)
}

func (p *Prefetcher) storeTrieNodes(values []hexutil.Bytes) error {
	root, nodes := mpt.WriteTrie(values)
	p.logger.Debug("Wrote MPT", "root", root.Hex())
	for _, node := range nodes {
		if err := p.kvStore.Put(preimage.Keccak256Key(crypto.Keccak256Hash(node)).PreimageKey(), node); err != nil {
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
