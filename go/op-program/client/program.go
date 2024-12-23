package client

import (
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

	signer := ethtypes.NewCancunSigner(cfg.L1ChainId)

	rawBatchInfos := make([]opprog.RawBatchInfo, 0)
	blobTxReverseIndex := make(map[common.Hash]uint64)

	// retrieve the state root for l2 safe head
	stateHeader := dtlPreimageOracle.StateBatchHeaderByHash(l2common.Hash(l2OutputRoot))
	// start from the last block of the safe batch
	l2StartBlock := stateHeader.PrevTotalElements.Uint64() + stateHeader.BatchSize.Uint64() + 1
	// end as the claim block
	l2EndBlock := l2ClaimBlockNum

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
		if txChainBatcher == nil || blobBatcher == nil {
			return fmt.Errorf("no batcher address found for height %d", l1Header.NumberU64())
		}

		l1BlockRef := eth.InfoToL1BlockRef(l1Header)

		var (
			blockReceipts ethtypes.Receipts
			blobCounter   = 0
		)

		// Find the tx that contains the tx chain data
		blockInfo, txs := l1Oracle.TransactionsByBlockHash(l1Header.Hash())
		for txIndex, tx := range txs {
			if len(tx.BlobHashes()) > 0 {
				blobCounter += len(tx.BlobHashes())
			}

			if tx.To() == nil || *tx.To() != common.Address(cfg.InboxAddress) {
				// ignore invalid inbox txs
				continue
			}

			from, err := signer.Sender(tx)
			if err != nil {
				return fmt.Errorf("failed to recover sender of tx %s: %w", tx.Hash().Hex(), err)
			}

			if from != *txChainBatcher && from != *blobBatcher {
				// ignore invalid inbox txs
				continue
			}

			// lazy load block receipts
			if blockReceipts == nil {
				_, blockReceipts = l1Oracle.ReceiptsByBlockHash(blockInfo.Hash())
				if blockReceipts == nil || blockReceipts.Len() != txs.Len() {
					return fmt.Errorf("receipts not found for tx %s", tx.Hash().Hex())
				}
			}

			receipt := blockReceipts[txIndex]
			if receipt.Status != ethtypes.ReceiptStatusSuccessful {
				// ignore failed txs
				continue
			}

			if from == *txChainBatcher {

				// decode tx chain data
				var txChainData opprog.BatchSubmissionData
				if err := txChainData.Decode(tx.Data()); err != nil {
					return fmt.Errorf("failed to decode tx chain data: %w", err)
				}

				if l2StartBlock >= txChainData.PrevTotalElements+txChainData.BatchSize+1 {
					// stop when all blobs are collected
					stopWhenAllBlobsCollected = true
				}

				// mark blob txs to collect
				for _, blobTxHash := range txChainData.BlobTxHashes {
					blobTxReverseIndex[blobTxHash] = txChainData.BatchIndex
				}

				rawBatchInfos = append(rawBatchInfos, opprog.RawBatchInfo{
					BatchIndex:       txChainData.BatchIndex,
					TotalBlobTxCount: uint64(len(txChainData.BlobTxHashes)),
					BlobTransactions: make([]*opprog.BlobTxInfo, 0, len(txChainData.BlobTxHashes)),
				})
			} else if from == *blobBatcher && tx.Type() == ethtypes.BlobTxType {
				// collect blob txs
				batchIndex, ok := blobTxReverseIndex[tx.Hash()]
				if !ok {
					// ignore invalid blob tx
					continue
				}

				// the order of blob txs is reversed, need to reverse it back when converting to frames
				lastFoundBatch := rawBatchInfos[len(rawBatchInfos)-1]
				if lastFoundBatch.BatchIndex != batchIndex {
					return fmt.Errorf("blob tx %s not in the same batch as tx chain", tx.Hash().Hex())
				}

				txBlobCount := len(tx.BlobHashes())
				blobHashes := make([]eth.IndexedBlobHash, 0, txBlobCount)
				for i, blobHash := range tx.BlobHashes() {
					blobHashes = append(blobHashes, eth.IndexedBlobHash{
						Index: uint64(blobCounter - txBlobCount + i),
						Hash:  blobHash,
					})
				}

				lastFoundBatch.BlobTransactions = append(lastFoundBatch.BlobTransactions, &opprog.BlobTxInfo{
					BlockRef:   l1BlockRef,
					Tx:         tx,
					BlobHashes: nil,
				})
				if stopWhenAllBlobsCollected && len(lastFoundBatch.BlobTransactions) == int(lastFoundBatch.TotalBlobTxCount) {
					// already collected all batches we need, time to break out from the searching
					goto BREAKOUT
				}
			}
		}
	}

BREAKOUT:
	// start rebuild the batch
	// reverse the batches, we need to derive from the earliest batch
	slices.Reverse(rawBatchInfos)

	// decode batches and rebuild l2 blocks
	l2Blocks := make([]*types.Block, 0, int(l2EndBlock-l2StartBlock+1))
	for _, rawBatchInfo := range rawBatchInfos {
		// reverse the blob txs, we need to derive from the earliest blob tx
		slices.Reverse(rawBatchInfo.BlobTransactions)

		blobData := make([]eth.Data, 0, len(rawBatchInfo.BlobTransactions))
		totalLength := 0
		for _, blobTx := range rawBatchInfo.BlobTransactions {
			logger.Debug("Processing blob tx", "tx", blobTx.Tx.Hash().Hex())
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

				blobData = append(blobData, rawData)
				totalLength += len(blobData)
			}
		}

		// concat all blob datas
		rawData := make([]byte, 0, totalLength)
		for _, data := range blobData {
			rawData = append(rawData, data...)
		}

		// parse frames
		frames, err := derive.ParseFrames(rawData)
		if err != nil {
			return fmt.Errorf("failed to parse frames: %w", err)
		} else if len(frames) == 0 {
			return fmt.Errorf("no frames found in blob")
		}

		// derive the batch
		channel, err := opderive.ProcessFrames(l2Cfg.ChainID, frames[0].ID, frames)
		if err != nil {
			return fmt.Errorf("failed to process frames: %w", err)
		}

		for _, batch := range channel.Batches {
			spanBatch, ok := batch.AsSpanBatch()
			if !ok {
				return fmt.Errorf("failed to convert batch to span batch: %w", err)
			}

			derivedBlocks := spanBatch.DeriveL2Blocks()

			// filter out the blocks that are not in the range
			for _, block := range derivedBlocks {
				if block.NumberU64() >= l2StartBlock && block.NumberU64() <= l2EndBlock {
					l2Blocks = append(l2Blocks, block)
				}
			}
		}
	}

	logger.Info("Building up L2 chain...")
	l2Chain, err := l2.NewOracleBackedL2Chain(logger, l2Oracle, l1Oracle, dtlPreimageOracle, l2Cfg, l2common.Hash(l2OutputRoot), stateHeader)
	if err != nil {
		return fmt.Errorf("failed to build L2 chain: %w", err)
	}

	logger.Info("Created L2 chain, start derivation", "head", l2Chain.CurrentHeader().Number.Uint64(), "headHash", l2Chain.CurrentHeader().Hash().Hex())
	parentHeader := l2Chain.CurrentHeader()

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

		logger.Debug("Block header created", "block", block.Number().Uint64(), "header", blockHeader)
		state, err := l2Chain.StateAt(parentHeader.Root)
		if err != nil {
			return fmt.Errorf("failed to get state at %d: %w", block.Number().Uint64()-1, err)
		}

		logger.Debug("State retrieved", "block", block.Number().Uint64())

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

				logger.Debug("Applying transaction", "block", block.Number().Uint64(), "tx", i, "hash", tx.Hash())

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

				logger.Debug("Transaction applied", "block", block.Number().Uint64(), "tx", i, "hash", tx.Hash(), "reverted", receipt.Status != types.ReceiptStatusSuccessful)
			}
		}

		logger.Debug("Transactions applied", "block", block.Number().Uint64(), "gasUsed", blockHeader.GasUsed)

		// Finalize the block
		block, err = consEngine.FinalizeAndAssemble(l2Chain, blockHeader, state, txs, nil, receipts)
		if err != nil {
			return fmt.Errorf("failed to finalize block %d: %w", block.Number().Uint64(), err)
		}

		logger.Debug("Block finalized", "block", block.Number().Uint64())

		// Set head
		if _, err := l2Chain.SetCanonical(block); err != nil {
			return fmt.Errorf("failed to set canonical block %d: %w", block, err)
		}

		logger.Debug("Canonical block set", "block", block.Number().Uint64())

		// Add block
		l2Chain.InsertBlockWithoutSetHead(block)

		logger.Debug("Block inserted", "block", block.Number().Uint64())

		// commit the state
		root, err := state.Commit(l2Chain.Config().IsEIP158(l2Chain.CurrentHeader().Number))
		if err != nil {
			return fmt.Errorf("state write error: %w", err)
		}

		if err := state.Database().TrieDB().Commit(root, true); err != nil {
			return fmt.Errorf("trie write error: %w", err)
		}

		intermediateStateRoots = append(intermediateStateRoots, root.Bytes())
		logger.Debug("State commited", "block", block.Number().Uint64())

		parentHeader = block.Header()

		logger.Debug("Block processing finished", "block", block.Number().Uint64(), "hash", block.Hash().Hex())

		if block.NumberU64() == l2ClaimBlockNum {
			logger.Info("Reached claim block, stop processing", "block", block.NumberU64())
			break
		}
	}

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
