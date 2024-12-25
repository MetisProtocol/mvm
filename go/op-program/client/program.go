package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"slices"

	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"

	l2common "github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/common/hexutil"
	"github.com/MetisProtocol/mvm/l2geth/core"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/params"
	"github.com/MetisProtocol/mvm/l2geth/rollup/rcfg"
	"github.com/ethereum-optimism/optimism/go/op-program/chainconfig"
	opderive "github.com/ethereum-optimism/optimism/go/op-program/client/derive"
	opprog "github.com/ethereum-optimism/optimism/go/op-program/client/types"

	"github.com/ethereum-optimism/optimism/op-service/eth"

	preimage "github.com/ethereum-optimism/optimism/go/op-preimage"
	"github.com/ethereum-optimism/optimism/go/op-program/client/claim"
	dtl "github.com/ethereum-optimism/optimism/go/op-program/client/dtl"
	"github.com/ethereum-optimism/optimism/go/op-program/client/l1"
	"github.com/ethereum-optimism/optimism/go/op-program/client/l2"
)

// Main executes the client program in a detached context and exits the current process.
// The client runtime environment must be preset before calling this function.
func Main(logger log.Logger) {
	log.Info("Starting fault proof program client")
	preimageOracle := CreatePreimageChannel()
	preimageHinter := CreateHinterChannel()
	if err := RunProgram(logger, preimageOracle, preimageHinter); errors.Is(err, claim.ErrClaimNotValid) {
		log.Error("Claim is invalid", "err", err)
		os.Exit(1)
	} else if err != nil {
		log.Error("Program failed", "err", err)
		os.Exit(2)
	} else {
		log.Info("Claim successfully verified")
		os.Exit(0)
	}
}

// RunProgram executes the Program, while attached to an IO based pre-image oracle, to be served by a host.
func RunProgram(logger log.Logger, preimageOracle io.ReadWriter, preimageHinter io.ReadWriter) error {
	pClient := preimage.NewOracleClient(preimageOracle)
	hClient := preimage.NewHintWriter(preimageHinter)
	l1PreimageOracle := l1.NewCachingOracle(l1.NewPreimageOracle(pClient, hClient))
	l2PreimageOracle := l2.NewCachingOracle(l2.NewPreimageOracle(pClient, hClient))
	dtlPreimageOracle := dtl.NewPreimageOracle(pClient, hClient)

	bootInfo := NewBootstrapClient(pClient).BootInfo()
	logger.Info("Program Bootstrapped", "bootInfo", bootInfo)
	return runDerivation(
		logger,
		bootInfo.RollupConfig,
		bootInfo.L2ChainConfig,
		bootInfo.L1Head,
		bootInfo.L2OutputRoot,
		bootInfo.L2Claim,
		bootInfo.L2ClaimBlockNumber,
		l1PreimageOracle,
		l2PreimageOracle,
		dtlPreimageOracle,
	)
}

// runDerivation executes the L2 state transition, given a minimal interface to retrieve data.
func runDerivation(logger log.Logger, cfg *chainconfig.RollupConfig, l2Cfg *params.ChainConfig,
	l1Head common.Hash, l2OutputRoot common.Hash,
	l2Claim common.Hash, l2ClaimBlockNum uint64,
	l1Oracle l1.Oracle, l2Oracle l2.Oracle, dtlPreimageOracle dtl.Oracle) error {

	// must use ovm for the derivation
	rcfg.UsingOVM = true

	logger.Info("Derivation start",
		"l1Head", l1Head.Hex(),
		"l2OutputRoot", l2OutputRoot.Hex(),
		"l2Claim", l2Claim.Hex(),
		"l2ClaimBlockNum", l2ClaimBlockNum)

	signer := ethtypes.NewCancunSigner(cfg.L1ChainId)

	rawBatchInfos := make([]*opprog.RawBatchInfo, 0)
	blobTxReverseIndex := make(map[common.Hash]uint64)

	// retrieve the state root for l2 safe head
	stateHeader := dtlPreimageOracle.StateBatchHeaderByHash(l2common.Hash(l2OutputRoot))
	// start from the last block of the safe batch
	l2StartBlock := stateHeader.PrevTotalElements.Uint64() + stateHeader.BatchSize.Uint64()
	// end as the claim block
	l2EndBlock := l2ClaimBlockNum

	if l2StartBlock > l2EndBlock {
		return fmt.Errorf("invalid derivation range, start block %d is greater than end block %d", l2StartBlock, l2EndBlock)
	}

	logger.Info("Derivation range, start traversing L1 backwards", "start", l2StartBlock, "end", l2EndBlock)

	stopWhenAllBlobsCollected := false
	// walk back to the l1 block that contains the given tx chain data,
	// since we will only submit one tx per block, so when reverse walking the l1 chain,
	// the tx order will always be txChain tx --> nth submitted blob tx --> (n-1)th submitted blob tx --> ... --> 1st submitted blob tx
	for l1Header := l1Oracle.HeaderByBlockHash(l1Head); ; l1Header = l1Oracle.HeaderByBlockHash(l1Header.ParentHash()) {
		var txChainBatcher, blobBatcher *common.Address
		for _, batcherAddressAtHeight := range cfg.TxChainBatcherAddresses {
			if l1Header.NumberU64() >= batcherAddressAtHeight.Height {
				txChainBatcher = (*common.Address)(&batcherAddressAtHeight.Address)
				break
			}
		}
		for _, batcherAddressAtHeight := range cfg.BlobBatcherAddresses {
			if l1Header.NumberU64() >= batcherAddressAtHeight.Height {
				blobBatcher = (*common.Address)(&batcherAddressAtHeight.Address)
				break
			}
		}

		logger.Info("Processing L1 block", "block", l1Header.NumberU64(), "txChainBatcher", txChainBatcher.Hex(), "blobBatcher", blobBatcher.Hex())

		if txChainBatcher == nil || blobBatcher == nil {
			logger.Error("Batcher address not found", "block", l1Header.NumberU64())
			return fmt.Errorf("no batcher address found for height %d", l1Header.NumberU64())
		}

		l1BlockRef := eth.InfoToL1BlockRef(l1Header)

		var (
			blockReceipts ethtypes.Receipts
			blobCounter   = 0
		)

		// Find the tx that contains the tx chain data
		blockInfo, txs := l1Oracle.TransactionsByBlockHash(l1Header.Hash())
		logger.Info("Loaded L1 block txs", "block", l1Header.NumberU64(), "txCount", txs.Len())
		for txIndex, tx := range txs {
			if len(tx.BlobHashes()) > 0 {
				blobCounter += len(tx.BlobHashes())
			}

			if tx.To() == nil || *tx.To() != common.Address(cfg.InboxAddress) {
				// ignore invalid inbox txs
				logger.Info("Ignoring non-inbox tx", "tx", tx.Hash().Hex(), "to", tx.To())
				continue
			}

			from, err := signer.Sender(tx)
			if err != nil {
				return fmt.Errorf("failed to recover sender of tx %s: %w", tx.Hash().Hex(), err)
			}

			logger.Info("Processing inbox tx", "tx", tx.Hash().Hex(), "from", from.Hex())

			if from != *txChainBatcher && from != *blobBatcher {
				// ignore invalid inbox txs
				logger.Info("tx is not from tx chain batcher or blob batcher")
				continue
			}

			// lazy load block receipts
			if blockReceipts == nil {
				logger.Info("Loading receipts", "block", blockInfo.NumberU64())
				_, blockReceipts = l1Oracle.ReceiptsByBlockHash(blockInfo.Hash())
				if blockReceipts == nil || blockReceipts.Len() != txs.Len() {
					return fmt.Errorf("receipts not found for tx %s", tx.Hash().Hex())
				}
				logger.Info("Loaded receipts", "block", blockInfo.NumberU64(), "receiptCount", blockReceipts.Len())
			}

			receipt := blockReceipts[txIndex]
			if receipt.Status != ethtypes.ReceiptStatusSuccessful {
				// ignore failed txs
				logger.Info("Ignoring failed tx", "tx", tx.Hash().Hex())
				continue
			}

			if from == *txChainBatcher {
				logger.Info("Processing tx chain tx", "tx", tx.Hash().Hex())
				// decode tx chain data
				var txChainData opprog.BatchSubmissionData
				if err := txChainData.Decode(tx.Data()); err != nil {
					logger.Error("Failed to decode tx chain data", "tx", tx.Hash().Hex(), "err", err)
					return fmt.Errorf("failed to decode tx chain data: %w", err)
				}

				logger.Info("Decoded tx chain data", "tx", tx.Hash().Hex(), "data", txChainData)

				if l2StartBlock >= txChainData.PrevTotalElements+txChainData.BatchSize-1 {
					// stop when all blobs are collected
					logger.Info("All tx chain data collected, will exist when all blobs are collected",
						"lastTxChainBatchStart", txChainData.PrevTotalElements-1, "size", txChainData.BatchSize)
					stopWhenAllBlobsCollected = true
				} else {
					logger.Info("Not reached the start block yet, continue searching",
						"target", l2StartBlock, "batchStart", txChainData.PrevTotalElements-1, "batchSize", txChainData.BatchSize)
				}

				// mark blob txs to collect
				for _, blobTxHash := range txChainData.BlobTxHashes {
					blobTxReverseIndex[blobTxHash] = txChainData.BatchIndex
				}

				rawBatchInfos = append(rawBatchInfos, &opprog.RawBatchInfo{
					BatchIndex:       txChainData.BatchIndex,
					TotalBlobTxCount: uint64(len(txChainData.BlobTxHashes)),
					BlobTransactions: make([]*opprog.BlobTxInfo, 0, len(txChainData.BlobTxHashes)),
				})
			} else if from == *blobBatcher && tx.Type() == ethtypes.BlobTxType {
				// collect blob txs
				batchIndex, ok := blobTxReverseIndex[tx.Hash()]
				if !ok {
					// ignore invalid blob tx
					logger.Error("Ignoring invalid blob tx", "tx", tx.Hash().Hex())
					continue
				}

				// the order of blob txs is reversed, need to reverse it back when converting to frames
				lastFoundBatch := rawBatchInfos[len(rawBatchInfos)-1]
				if lastFoundBatch.BatchIndex != batchIndex {
					logger.Error("Blob tx not in the same batch as tx chain", "tx", tx.Hash().Hex())
					return fmt.Errorf("blob tx %s not in the same batch as tx chain", tx.Hash().Hex())
				}

				txBlobCount := len(tx.BlobHashes())
				blobHashes := make([]eth.IndexedBlobHash, 0, txBlobCount)
				for i, blobHash := range tx.BlobHashes() {
					blobHashes = append(blobHashes, eth.IndexedBlobHash{
						Index: uint64(blobCounter - txBlobCount + i),
						Hash:  blobHash,
					})
					logger.Info("Processing blob", "tx", tx.Hash().Hex(), "blobIndex", blobCounter-txBlobCount+i, "blobHash", blobHash.Hex())
				}

				logger.Info("Proccessed inbox blob tx", "tx", tx.Hash().Hex(), "blobCount", len(blobHashes))

				lastFoundBatch.BlobTransactions = append(lastFoundBatch.BlobTransactions, &opprog.BlobTxInfo{
					BlockRef:   l1BlockRef,
					Tx:         tx,
					BlobHashes: blobHashes,
				})
				if stopWhenAllBlobsCollected && len(lastFoundBatch.BlobTransactions) == int(lastFoundBatch.TotalBlobTxCount) {
					// already collected all batches we need, time to break out from the searching
					logger.Info("All blobs collected, stop searching")
					goto BREAKOUT
				}
			}

			logger.Info("Processed inbox tx", "tx", tx.Hash().Hex())
		}
	}

BREAKOUT:
	// start rebuild the batch
	// reverse the batches, we need to derive from the earliest batch
	slices.Reverse(rawBatchInfos)

	// decode batches and rebuild l2 blocks
	l2Blocks := make([]*types.Block, 0, int(l2EndBlock-l2StartBlock+1))
	framesByChannelId := make(map[derive.ChannelID][]derive.Frame)
	for _, rawBatchInfo := range rawBatchInfos {
		for _, blobTx := range rawBatchInfo.BlobTransactions {
			logger.Debug("Processing blob tx data", "tx", blobTx.Tx.Hash().Hex(), "blobCount", len(blobTx.BlobHashes))
			// derive the batch
			for _, indexedBlobHash := range blobTx.BlobHashes {
				blob := l1Oracle.GetBlob(blobTx.BlockRef, indexedBlobHash)
				if blob == nil {
					return fmt.Errorf("blob %s not found", indexedBlobHash.Hash.Hex())
				}
				rawData, err := blob.ToData()
				if err != nil {
					return fmt.Errorf("failed to convert blob to data: %w", err)
				}

				// parse frames
				frames, err := derive.ParseFrames(rawData)
				if err != nil {
					return fmt.Errorf("failed to parse frames: %w", err)
				} else if len(frames) == 0 {
					return fmt.Errorf("no frames found in blob")
				}

				framesByChannelId[frames[0].ID] = append(framesByChannelId[frames[0].ID], frames...)

				logger.Info("Processed blob tx", "tx", blobTx.Tx.Hash().Hex(), "blobIndex", indexedBlobHash.Index, "blobHash", indexedBlobHash.Hash.Hex(), "frameCount", len(frames))
			}
		}

		// derive the batch
		for channelId, frames := range framesByChannelId {
			// sort the frames by number
			slices.SortFunc(frames, func(i, j derive.Frame) int {
				return int(i.FrameNumber - j.FrameNumber)
			})
			logger.Info("Processing frame", "channel", channelId.String(), "frameCount", len(frames))
			channel, err := opderive.ProcessFrames(l2Cfg.ChainID, channelId, frames)
			if err != nil {
				return fmt.Errorf("failed to process frames: %w", err)
			}

			for _, batch := range channel.Batches {
				spanBatch, ok := batch.AsSpanBatch()
				if !ok {
					return fmt.Errorf("failed to convert batch to span batch: %w", err)
				}

				logger.Info("Deriving span batch", "channel", channelId.String(), "batch", spanBatch)

				derivedBlocks := spanBatch.DeriveL2Blocks()

				logger.Info("Derived blocks", "channel", channelId.String(), "blockCount", len(derivedBlocks))

				// filter out the blocks that are not in the range
				for _, block := range derivedBlocks {
					logger.Info("Checking block", "block", block.NumberU64())
					if block.NumberU64() > l2StartBlock && block.NumberU64() <= l2EndBlock {
						l2Blocks = append(l2Blocks, block)
					}
				}
			}
		}
	}

	if len(l2Blocks) == 0 {
		return errors.New("no blocks to derive")
	}

	slices.SortFunc(l2Blocks, func(i, j *types.Block) int {
		return int(i.NumberU64() - j.NumberU64())
	})

	logger.Info("Building up L2 chain...")
	l2Chain, err := l2.NewOracleBackedL2Chain(logger, l2Oracle, l1Oracle, dtlPreimageOracle, l2Cfg, l2common.Hash(l2OutputRoot), stateHeader)
	if err != nil {
		return fmt.Errorf("failed to build L2 chain: %w", err)
	}

	logger.Info("Created L2 chain, start derivation", "head", l2Chain.CurrentHeader().Number.Uint64(), "headHash", l2Chain.CurrentHeader().Hash().Hex())
	parentHeader := l2Chain.CurrentHeader()

	logger.Info("Derivation range", "start", l2Blocks[0].NumberU64(), "end", l2Blocks[len(l2Blocks)-1].NumberU64())
	intermediateStateRoots := make([]hexutil.Bytes, 0, len(l2Blocks))
	for _, block := range l2Blocks {
		logger.Info("Processing L2 block", "block", block.Number().Uint64())

		logger.Info("Checking parent state availability",
			"block", parentHeader.Number.Uint64(),
			"parentHash", parentHeader.Hash().Hex())
		if !l2Chain.HasBlockAndState(parentHeader.Hash(), parentHeader.Number.Uint64()) {
			logger.Error("missing parent block or state", "block", parentHeader.Number.Uint64())
			return fmt.Errorf("missing parent block %d", parentHeader.Number.Uint64())
		} else {
			logger.Debug("Parent block and state available", "block", parentHeader.Number.Uint64())
		}

		consEngine := l2Chain.Engine()
		blockHeader := &types.Header{
			ParentHash: parentHeader.Hash(),
			Number:     block.Number(),
			GasLimit:   parentHeader.GasLimit,
			Extra:      block.Extra(),
			Time:       block.Time(),
			// Note(@dumdumgoose): Our difficulty is always 2, hardcode the difficulty here, because
			// if we calculate the difficulty with clique, it will traverse the blocks back at most an epoch,
			// which will make our state very large.
			Difficulty: big.NewInt(2),
			Coinbase:   block.Coinbase(),
			Nonce:      types.EncodeNonce(block.Nonce()),
			MixDigest:  block.MixDigest(),
		}

		logger.Info("Block header created", "block", block.Number().Uint64(), "header", blockHeader)
		logger.Info("Creating intermediate state at", "block", block.Number().Uint64(), "root", parentHeader.Root.Hex())
		state, err := l2Chain.StateAt(parentHeader.Root)
		if err != nil {
			return fmt.Errorf("failed to get state at %d: %w", block.Number().Uint64()-1, err)
		}

		logger.Info("State retrieved", "block", block.Number().Uint64())

		txs := block.Transactions()
		emptyAddress := l2common.Address{}
		receipts := make(types.Receipts, 0, len(txs))
		logs := make([]*types.Log, 0)
		if txs.Len() > 0 {
			firstTx := txs[0]
			gp := new(core.GasPool).AddGas(parentHeader.GasLimit)
			// Apply the transactions to the state
			for i, tx := range txs {
				tx.SetL1BlockNumber(firstTx.L1BlockNumber().Uint64())
				tx.SetIndex(*firstTx.GetMeta().Index)
				tx.SetL1Timestamp(firstTx.L1Timestamp())

				state.Prepare(tx.Hash(), l2common.Hash{}, i)

				logger.Info("Applying transaction",
					"block", block.Number().Uint64(),
					"tx", i,
					"hash", tx.Hash(),
					"l1Block", tx.GetMeta().L1BlockNumber.Uint64(),
					"l1MessageSender", tx.GetMeta().L1MessageSender.Hex(),
					"l1Timestamp", tx.GetMeta().L1Timestamp,
					"index", *tx.GetMeta().Index,
					"queueOrigin", tx.GetMeta().QueueOrigin,
					"queueIndex", *tx.GetMeta().QueueIndex,
					"rawTransaction", hexutil.Encode(tx.Data()),
					"seqR", hexutil.Encode(tx.GetMeta().R.Bytes()),
					"seqS", hexutil.Encode(tx.GetMeta().S.Bytes()),
					"seqV", hexutil.Encode(tx.GetMeta().V.Bytes()),
				)

				revid := state.Snapshot()
				receipt, err := core.ApplyTransaction(l2Chain.Config(), l2Chain, &emptyAddress, gp, state, blockHeader, tx, &blockHeader.GasUsed, *l2Chain.GetVMConfig())
				if err != nil {
					// not collecting log for failed tx
					logger.Error("Failed to apply transaction", "block", block.Number().Uint64(), "tx", i, "hash", tx.Hash(), "err", err)
					state.RevertToSnapshot(revid)
				} else if receipt.Logs != nil {
					logs = append(logs, receipt.Logs...)
				}
				receipts = append(receipts, receipt)

				logger.Info("Transaction applied", "block", block.Number().Uint64(), "tx", i, "hash", tx.Hash(), "reverted", receipt.Status != types.ReceiptStatusSuccessful)
			}
		}

		logger.Info("Transactions applied", "block", block.Number().Uint64(), "gasUsed", blockHeader.GasUsed,
			"txCount", len(txs), "receiptCount", len(receipts))

		// Finalize the block
		block, err = consEngine.FinalizeAndAssemble(l2Chain, blockHeader, state, txs, nil, receipts)
		if err != nil {
			return fmt.Errorf("failed to finalize block %d: %w", block.Number().Uint64(), err)
		}

		logger.Info("Block finalized", "block", block.Number().Uint64(),
			"hash", block.Hash().Hex())

		// commit the state
		root, err := state.Commit(l2Chain.Config().IsEIP158(l2Chain.CurrentHeader().Number))
		if err != nil {
			return fmt.Errorf("state write error: %w", err)
		}

		if err := state.Database().TrieDB().Commit(root, true); err != nil {
			return fmt.Errorf("trie write error: %w", err)
		}

		intermediateStateRoots = append(intermediateStateRoots, root.Bytes())
		logger.Info("State commited", "block", block.Number().Uint64(), "root", root.Hex())

		// Set head
		if _, err := l2Chain.SetCanonical(block); err != nil {
			return fmt.Errorf("failed to set canonical block %d: %w", block, err)
		}

		logger.Debug("Canonical block set", "block", block.Number().Uint64())

		// Add block
		l2Chain.InsertBlockWithoutSetHead(block)

		logger.Debug("Block inserted", "block", block.Number().Uint64())

		parentHeader = block.Header()
		blockHeaderJSON, _ := json.Marshal(block.Header())
		logger.Info("Block header inserted", "header", string(blockHeaderJSON))

		logger.Debug("Block processing finished", "block", block.Number().Uint64(), "hash", block.Hash().Hex())
	}

	logger.Info("Derivation finished, validating claim", "head", l2Chain.CurrentHeader().Number.Uint64(),
		"headHash", l2Chain.CurrentHeader().Hash().Hex(),
		"stateRoot", l2Chain.CurrentHeader().Root.Hex())

	return claim.ValidateClaim(logger, l2ClaimBlockNum, eth.Bytes32(l2Claim), dtlPreimageOracle, l2Chain)
}

func CreateHinterChannel() preimage.FileChannel {
	r := os.NewFile(HClientRFd, "preimage-hint-read")
	w := os.NewFile(HClientWFd, "preimage-hint-write")
	return preimage.NewReadWritePair(r, w)
}

// CreatePreimageChannel returns a FileChannel for the preimage oracle in a detached context
func CreatePreimageChannel() preimage.FileChannel {
	r := os.NewFile(PClientRFd, "preimage-oracle-read")
	w := os.NewFile(PClientWFd, "preimage-oracle-write")
	return preimage.NewReadWritePair(r, w)
}
