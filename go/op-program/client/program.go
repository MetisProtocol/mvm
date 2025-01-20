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
	ethhex "github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"

	l2common "github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/common/hexutil"
	"github.com/MetisProtocol/mvm/l2geth/core"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/params"
	"github.com/MetisProtocol/mvm/l2geth/rollup"
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

func init() {
	// must use ovm for the derivation
	rcfg.UsingOVM = true
}

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
	l1Oracle l1.Oracle, l2Oracle l2.Oracle, dtlPreimageOracle dtl.Oracle) (err error) {

	logger.Info("Derivation start",
		"l1Head", l1Head.Hex(),
		"l2OutputRoot", l2OutputRoot.Hex(),
		"l2Claim", l2Claim.Hex(),
		"l2ClaimBlockNum", l2ClaimBlockNum)

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

	// derive l1 raw batch infos
	disputedBatchHeader, rawBatchInfos, enqueueTxs, lastL1Head, earliestEnqueue, err := deriveL1Info(logger, l1Oracle, cfg, l2Cfg, l1Head, l2StartBlock, l2EndBlock)
	if err != nil {
		return fmt.Errorf("failed to derive L1 info: %w", err)
	}
	if disputedBatchHeader == nil {
		return fmt.Errorf("disputed batch header not found for %s", l2Claim.Hex())
	}

	// rebuild the l2 blocks from the l1 raw batch infos
	l2Blocks, err := rebuildL2Blocks(logger, l2Cfg, l1Oracle, rawBatchInfos, enqueueTxs, l2StartBlock, l2EndBlock)
	if err != nil {
		return err
	}
	if len(l2Blocks) == 0 {
		return errors.New("no blocks to derive")
	}

	// collect enqueue txs if there is any missing
	if err := collectMissingEnqueueTx(logger, l2Cfg.ChainID, cfg, lastL1Head, l2Blocks, earliestEnqueue, enqueueTxs, l1Oracle); err != nil {
		return fmt.Errorf("failed to collect missing enqueue tx: %w", err)
	}

	// run l2 blocks and generate the state roots
	intermediateStateRoots, err := deriveL2States(logger, l2Oracle, l1Oracle, dtlPreimageOracle, l2Cfg, l2OutputRoot, l2Blocks, stateHeader, disputedBatchHeader)
	if err != nil {
		return fmt.Errorf("failed to derive L2 states: %w", err)
	}

	// verify the claim
	return claim.ValidateClaim(l2common.Hash(l2Claim), intermediateStateRoots, disputedBatchHeader)
}

func deriveL1Info(logger log.Logger, l1Oracle l1.Oracle,
	rollupCfg *chainconfig.RollupConfig,
	l2Cfg *params.ChainConfig,
	l1Head common.Hash,
	l2StartBlock, l2EndBlock uint64) (disputedBatchHeader *rollup.BatchHeader,
	rawBatchInfos []*opprog.RawBatchInfo,
	enqueueTxs map[uint64]*rollup.Enqueue,
	lastL1Head *ethtypes.Header,
	earliestEnqueue int64,
	err error) {
	signer := ethtypes.NewCancunSigner(rollupCfg.L1ChainId)
	rawBatchInfos = make([]*opprog.RawBatchInfo, 0)
	blobTxReverseIndex := make(map[common.Hash]uint64)
	stopWhenAllBlobsCollected := false
	enqueueTxs = make(map[uint64]*rollup.Enqueue)
	earliestEnqueue = -1

	enqueueTxEventID := l1.CTCABI.Events["TransactionEnqueued"].ID().Bytes()
	stateCommitmentEventID := l1.SCCABI.Events["StateBatchAppended"].ID()

	// walk back to the l1 block that contains the given tx chain data,
	// since we will only submit one tx per block, so when reverse walking the l1 chain,
	// the tx order will always be txChain tx --> nth submitted blob tx --> (n-1)th submitted blob tx --> ... --> 1st submitted blob tx
	for l1Header := l1Oracle.HeaderByBlockHash(l1Head); ; l1Header = l1Oracle.HeaderByBlockHash(l1Header.ParentHash()) {
		var txChainBatcher, blobBatcher *common.Address
		for _, batcherAddressAtHeight := range rollupCfg.TxChainBatcherAddresses {
			if l1Header.NumberU64() >= batcherAddressAtHeight.Height {
				txChainBatcher = (*common.Address)(&batcherAddressAtHeight.Address)
				break
			}
		}
		for _, batcherAddressAtHeight := range rollupCfg.BlobBatcherAddresses {
			if l1Header.NumberU64() >= batcherAddressAtHeight.Height {
				blobBatcher = (*common.Address)(&batcherAddressAtHeight.Address)
				break
			}
		}

		logger.Info("Processing L1 block", "block", l1Header.NumberU64(), "txChainBatcher", txChainBatcher.Hex(), "blobBatcher", blobBatcher.Hex())

		if txChainBatcher == nil || blobBatcher == nil {
			logger.Error("Batcher address not found", "block", l1Header.NumberU64())
			return nil, nil, nil, nil, -1, fmt.Errorf("no batcher address found for height %d", l1Header.NumberU64())
		}

		l1BlockRef := eth.InfoToL1BlockRef(l1Header)

		var (
			blockReceipts ethtypes.Receipts
			blobCounter   = 0
		)

		// Find the tx that contains the tx chain data
		blockInfo, txs := l1Oracle.TransactionsByBlockHash(l1Header.Hash())
		logger.Info("Loaded L1 block txs", "block", l1Header.NumberU64(), "txCount", txs.Len())

		// first check whether there is any enqueue tx event
		var rawHeaderBytes []byte
		rawHeaderBytes, err = l1Header.HeaderRLP()
		if err != nil {
			return nil, nil, nil, nil, -1, fmt.Errorf("failed to encode header: %w", err)
		}

		var rawHeader ethtypes.Header
		if err = rlp.DecodeBytes(rawHeaderBytes, &rawHeader); err != nil {
			return nil, nil, nil, nil, -1, fmt.Errorf("failed to decode header: %w", err)
		}

		lastL1Head = &rawHeader

		// we need to load the receipts if the block contains the enqueue tx event
		containsEnqueueTx := rawHeader.Bloom.Test(enqueueTxEventID)
		if containsEnqueueTx {
			logger.Info("Loading receipts", "block", l1Header.NumberU64())
			_, blockReceipts = l1Oracle.ReceiptsByBlockHash(l1Header.Hash())
			if blockReceipts == nil || blockReceipts.Len() != txs.Len() {
				return nil, nil, nil, nil, -1, fmt.Errorf("receipts not found for block %d", blockInfo.NumberU64())
			}
			logger.Info("Loaded receipts", "block", blockInfo.NumberU64(), "receiptCount", blockReceipts.Len())
		}

		for txIndex, tx := range txs {
			if len(tx.BlobHashes()) > 0 {
				blobCounter += len(tx.BlobHashes())
			}

			if tx.To() == nil {
				// ignore contract creation
				continue
			}

			to := *tx.To()
			if to != common.Address(rollupCfg.InboxAddress) && to != common.Address(rollupCfg.SCCAddress) && !containsEnqueueTx {
				// ignore non-batcher tx
				continue
			}

			// check sender for inbox txs
			var from common.Address
			if to == common.Address(rollupCfg.InboxAddress) {
				from, err = signer.Sender(tx)
				if err != nil {
					return nil, nil, nil, nil, -1, fmt.Errorf("failed to recover sender of tx %s: %w", tx.Hash().Hex(), err)
				}

				logger.Info("Processing inbox tx", "tx", tx.Hash().Hex(), "from", from.Hex())

				if from != *txChainBatcher && from != *blobBatcher {
					// ignore invalid inbox txs
					logger.Info("tx is not from tx chain batcher or blob batcher")
					continue
				}
			}

			// lazy load block receipts
			if blockReceipts == nil {
				logger.Info("Loading receipts", "block", blockInfo.NumberU64())
				_, blockReceipts = l1Oracle.ReceiptsByBlockHash(blockInfo.Hash())
				if blockReceipts == nil || blockReceipts.Len() != txs.Len() {
					return nil, nil, nil, nil, -1, fmt.Errorf("receipts not found for block %d", blockInfo.NumberU64())
				}
				logger.Info("Loaded receipts", "block", blockInfo.NumberU64(), "receiptCount", blockReceipts.Len())
			}

			receipt := blockReceipts[txIndex]
			if receipt.Status != ethtypes.ReceiptStatusSuccessful {
				// ignore failed txs
				logger.Warn("Ignoring failed batcher tx", "tx", tx.Hash().Hex())
				continue
			}

			if from == *txChainBatcher && to == common.Address(rollupCfg.InboxAddress) {
				logger.Info("Processing tx chain tx", "tx", tx.Hash().Hex())
				// decode tx chain data
				var txChainData opprog.BatchSubmissionData
				if err := txChainData.Decode(tx.Data()); err != nil {
					logger.Error("Failed to decode tx chain data", "tx", tx.Hash().Hex(), "err", err)
					return nil, nil, nil, nil, -1, fmt.Errorf("failed to decode tx chain data: %w", err)
				}

				if l2StartBlock >= txChainData.PrevTotalElements+txChainData.BatchSize {
					// stop when all blobs are collected
					logger.Info("All tx chain data collected, will exist when all blobs are collected",
						"lastTxChainBatchStart", txChainData.PrevTotalElements, "size", txChainData.BatchSize)
					stopWhenAllBlobsCollected = true
				} else {
					logger.Info("Not reached the start block yet, continue searching",
						"target", l2StartBlock, "batchStart", txChainData.PrevTotalElements, "batchSize", txChainData.BatchSize)
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
			} else if from == *blobBatcher && tx.Type() == ethtypes.BlobTxType && to == common.Address(rollupCfg.InboxAddress) {
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
					return nil, nil, nil, nil, -1, fmt.Errorf("blob tx %s not in the same batch as tx chain", tx.Hash().Hex())
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

				logger.Info("Processed inbox blob tx", "tx", tx.Hash().Hex(), "blobCount", len(blobHashes))

				lastFoundBatch.BlobTransactions = append(lastFoundBatch.BlobTransactions, &opprog.BlobTxInfo{
					BlockRef:   l1BlockRef,
					Tx:         tx,
					BlobHashes: blobHashes,
				})
				if stopWhenAllBlobsCollected && len(lastFoundBatch.BlobTransactions) == int(lastFoundBatch.TotalBlobTxCount) {
					// already collected all batches we need, time to break out from the searching
					logger.Info("All blobs collected, stop searching")
					slices.Reverse(rawBatchInfos)
					return
				}
			} else if to == common.Address(rollupCfg.SCCAddress) && disputedBatchHeader == nil {
				// collect state commitment batch header
				for _, log := range receipt.Logs {
					if log.Address != common.Address(rollupCfg.SCCAddress) || len(log.Topics) < 2 {
						// ignore invalid event
						continue
					}

					eventSignature := common.Hash(stateCommitmentEventID)
					if log.Topics[0] == eventSignature {
						// Decode non-indexed fields (from Data)
						mapData := make(map[string]interface{})
						if err = l1.SCCABI.UnpackIntoMap(mapData, "StateBatchAppended", log.Data); err != nil {
							return nil, nil, nil, nil, -1, fmt.Errorf("failed to unpack StateBatchAppended event: %w", err)
						}

						chainID, ok := mapData["_chainId"].(*big.Int)
						batchRoot, ok := mapData["_batchRoot"].([32]byte)
						batchSize, ok := mapData["_batchSize"].(*big.Int)
						prevTotalElements, ok := mapData["_prevTotalElements"].(*big.Int)
						extraData, ok := mapData["_extraData"].([]byte)
						if !ok {
							return nil, nil, nil, nil, -1, errors.New("failed to decode state batch event")
						}
						if chainID.Cmp(l2Cfg.ChainID) != 0 {
							// ignore non configured L2 chain events
							continue
						}

						stateBatchStartBlock, stateBatchEndBlock := prevTotalElements.Uint64()+1, prevTotalElements.Uint64()+batchSize.Uint64()
						if l2EndBlock == stateBatchEndBlock {
							// need to find the exact batch header for the disputed batch
							disputedBatchHeader = &rollup.BatchHeader{
								BatchRoot:         batchRoot,
								BatchSize:         batchSize,
								PrevTotalElements: prevTotalElements,
								ExtraData:         extraData,
							}

							logger.Info("Found batch header to dispute", "hash", disputedBatchHeader.Hash().Hex())
						} else {
							logger.Info("End block not in state batch range", "start", stateBatchStartBlock, "end", stateBatchEndBlock, "target", l2EndBlock)
						}
					}
				}
			} else if containsEnqueueTx {
				enqueueIndex, err := collectEnqueueTx(logger, rollupCfg, l2Cfg.ChainID, receipt, enqueueTxs)
				if err != nil {
					return nil, nil, nil, nil, -1, fmt.Errorf("failed to collect enqueue tx: %w", err)
				}
				if enqueueIndex >= 0 && (earliestEnqueue < 0 || enqueueIndex < earliestEnqueue) {
					earliestEnqueue = enqueueIndex
				}
			} else {
				// nothing to do
				logger.Warn("Ignoring invalid batcher tx", "tx", tx.Hash().Hex())
			}

			logger.Info("Processed inbox tx", "tx", tx.Hash().Hex())
		}
	}
}

func collectEnqueueTx(logger log.Logger, rollupCfg *chainconfig.RollupConfig, chainId *big.Int,
	receipt *ethtypes.Receipt, enqueueTxs map[uint64]*rollup.Enqueue) (int64, error) {
	// check for the enqueue tx event
	enqueueTxEventID := l1.CTCABI.Events["TransactionEnqueued"].ID().Bytes()
	if !receipt.Bloom.Test(enqueueTxEventID) {
		// ignore non-enqueue tx
		return -1, nil
	}

	firstEnqueue := int64(-1)
	for _, log := range receipt.Logs {
		if log.Address != common.Address(rollupCfg.CTCAddress) || len(log.Topics) != 4 {
			// ignore invalid event
			continue
		}

		eventSignature := common.Hash(enqueueTxEventID)
		if log.Topics[0] == eventSignature {
			// Decode non-indexed fields (from Data)
			mapData := make(map[string]interface{})
			if err := l1.CTCABI.UnpackIntoMap(mapData, "TransactionEnqueued", log.Data); err != nil {
				return -1, fmt.Errorf("failed to unpack TransactionEnqueued event: %w", err)
			}

			queueIndex := log.Topics[3].Big().Uint64()
			chainID, ok := mapData["_chainId"].(*big.Int)
			txData, ok := mapData["_data"].([]byte)
			if !ok {
				return -1, errors.New("failed to decode enqueue event")
			}
			if chainID.Cmp(chainId) != 0 {
				// ignore non configured L2 chain events
				continue
			}

			logger.Info("Processed enqueue tx", "tx", receipt.TxHash.Hex(), "queueIndex", queueIndex)

			dataPtr := hexutil.Bytes(txData)
			enqueueTxs[queueIndex] = &rollup.Enqueue{
				Data:       &dataPtr,
				QueueIndex: &queueIndex,
			}

			if int64(queueIndex) < firstEnqueue {
				firstEnqueue = int64(queueIndex)
			}
		}
	}

	return firstEnqueue, nil
}

func collectMissingEnqueueTx(logger log.Logger, l2ChainID *big.Int, rollupCfg *chainconfig.RollupConfig, lastL1Head *ethtypes.Header, l2Blocks []*types.Block, collectedEarliestEnqueue int64, collectedEnqueueTxs map[uint64]*rollup.Enqueue, l1Oracle l1.Oracle) error {
	earliestEnqueueTx := int64(-1)
	earliestEnqueueTxBlock := uint64(0)

	// find the first enqueue tx in the l2 blocks
	for _, l2Block := range l2Blocks {
		for _, tx := range l2Block.Transactions() {
			if tx.QueueOrigin() == types.QueueOriginL1ToL2 {
				earliestEnqueueTx = int64(*tx.GetMeta().QueueIndex)
				earliestEnqueueTxBlock = tx.GetMeta().L1BlockNumber.Uint64()
				break
			}
		}
	}

	logger.Info("Checking if there is any missing enqueue tx", "batchEarliest", earliestEnqueueTx, "collected", collectedEarliestEnqueue)

	// no collected anything yet, we need to collect at least 1 enqueue tx
	if collectedEarliestEnqueue < 0 {
		collectedEarliestEnqueue = earliestEnqueueTx + 1
	}

	if earliestEnqueueTx < 0 || earliestEnqueueTx >= collectedEarliestEnqueue {
		// no missing enqueue tx
		return nil
	}

	logger.Info("Enqueue tx missing", "from", earliestEnqueueTx, "to", collectedEarliestEnqueue)

	enqueueTxEventID := l1.CTCABI.Events["TransactionEnqueued"].ID().Bytes()

	// otherwise we need to keep traversing back util we have collected all the missing enqueue tx
	for l1Header := l1Oracle.HeaderByBlockHash(lastL1Head.ParentHash); l1Header.NumberU64() >= earliestEnqueueTxBlock; l1Header = l1Oracle.HeaderByBlockHash(l1Header.ParentHash()) {
		logger.Info("Processing L1 block", "block", l1Header.NumberU64())

		// first check whether there is any enqueue tx event
		rawHeaderBytes, err := l1Header.HeaderRLP()
		if err != nil {
			return fmt.Errorf("failed to encode header: %w", err)
		}

		var rawHeader ethtypes.Header
		if err := rlp.DecodeBytes(rawHeaderBytes, &rawHeader); err != nil {
			return fmt.Errorf("failed to decode header: %w", err)
		}

		// we need to load the receipts if the block contains the enqueue tx event
		containsEnqueueTx := rawHeader.Bloom.Test(enqueueTxEventID)
		if !containsEnqueueTx {
			// block does not have any enqueue tx
			continue
		}

		blockInfo, blockReceipts := l1Oracle.ReceiptsByBlockHash(l1Header.Hash())
		if blockReceipts == nil {
			return fmt.Errorf("receipts not found for block %d", blockInfo.NumberU64())
		}
		logger.Info("Loaded receipts", "block", blockInfo.NumberU64(), "receiptCount", blockReceipts.Len())

		for _, receipt := range blockReceipts {
			queueIndex, err := collectEnqueueTx(logger, rollupCfg, l2ChainID, receipt, collectedEnqueueTxs)
			if err != nil {
				return fmt.Errorf("failed to collect enqueue tx: %w", err)
			}

			if queueIndex > 0 {
				logger.Info("Collected enqueue tx", "tx", receipt.TxHash.Hex(), "queueIndex", queueIndex)
			}
		}
	}

	// replace the in-complete enqueue txs with the collected ones
	nextQueueIndex := earliestEnqueueTx
	for blockIndex, l2Block := range l2Blocks {
		for _, tx := range l2Block.Transactions() {
			if tx.QueueOrigin() != types.QueueOriginL1ToL2 {
				continue
			}

			queueIndex := *tx.GetMeta().QueueIndex
			if queueIndex != uint64(nextQueueIndex) {
				logger.Error("We have a gap in the enqueue txs", "expected", nextQueueIndex, "actual", queueIndex)
				return fmt.Errorf("found gap in enqueue tx, expected %d, actual %d", nextQueueIndex, queueIndex)
			}

			enqueue := collectedEnqueueTxs[queueIndex]
			if enqueue == nil {
				return fmt.Errorf("missing enqueue tx %d", queueIndex)
			}

			// complete the transaction with the collected enqueue tx
			tx.SetInput(*enqueue.Data)
			tx.GetMeta().RawTransaction = *enqueue.Data

			nextQueueIndex = int64(queueIndex) + 1
			if nextQueueIndex >= collectedEarliestEnqueue {
				return nil
			}
		}

		// need to rebuild the block, since tx has been changed, tx root will also be changed
		header := &types.Header{
			Number: l2Block.Number(),
			Time:   l2Block.Time(),
			Extra:  l2Block.Extra(),
		}

		l2Blocks[blockIndex] = types.NewBlock(header, l2Block.Transactions(), nil, nil)
	}

	return nil
}

func rebuildL2Blocks(logger log.Logger, l2Cfg *params.ChainConfig, l1Oracle l1.Oracle, rawBatchInfos []*opprog.RawBatchInfo, enqueueTxs map[uint64]*rollup.Enqueue, l2StartBlock, l2EndBlock uint64) ([]*types.Block, error) {
	// start rebuild the batch
	// decode batches and rebuild l2 blocks
	l2Blocks := make([]*types.Block, 0, int(l2EndBlock-l2StartBlock+1))
	framesByChannelId := make(map[derive.ChannelID][]derive.Frame)
	for _, rawBatchInfo := range rawBatchInfos {
		for _, blobTx := range rawBatchInfo.BlobTransactions {
			logger.Info("Processing blob tx data", "tx", blobTx.Tx.Hash().Hex(), "blobCount", len(blobTx.BlobHashes))
			// derive the batch
			for _, indexedBlobHash := range blobTx.BlobHashes {
				logger.Info("Processing blob sidecar", "block", blobTx.BlockRef.Number, "blobIndex", indexedBlobHash.Index, "blobHash", indexedBlobHash.Hash.Hex())
				blob := l1Oracle.GetBlob(blobTx.BlockRef, indexedBlobHash)
				if blob == nil {
					return nil, fmt.Errorf("blob %s not found", indexedBlobHash.Hash.Hex())
				}
				rawData, err := blob.ToData()
				if err != nil {
					return nil, fmt.Errorf("failed to convert blob to data: %w", err)
				}

				// parse frames
				frames, err := derive.ParseFrames(rawData)
				if err != nil {
					return nil, fmt.Errorf("failed to parse frames: %w", err)
				} else if len(frames) == 0 {
					return nil, fmt.Errorf("no frames found in blob")
				}

				framesByChannelId[frames[0].ID] = append(framesByChannelId[frames[0].ID], frames...)

				logger.Info("Processed blob tx", "tx", blobTx.Tx.Hash().Hex(), "blobIndex", indexedBlobHash.Index, "blobHash", indexedBlobHash.Hash.Hex(), "frameCount", len(frames))
			}
		}
	}

	// derive the batch
	for channelId, frames := range framesByChannelId {
		// sort the frames by number
		slices.SortFunc(frames, func(i, j derive.Frame) int {
			return int(i.FrameNumber - j.FrameNumber)
		})
		logger.Info("Processing frame", "channel", channelId.String(), "frameCount", len(frames))
		channel, err := opderive.ProcessFrames(l2Cfg.ChainID, channelId, frames, enqueueTxs)
		if err != nil {
			return nil, fmt.Errorf("failed to process frames: %w", err)
		}

		for _, batch := range channel.Batches {
			spanBatch, ok := batch.AsSpanBatch()
			if !ok {
				return nil, fmt.Errorf("failed to convert batch to span batch: %w", err)
			}

			logger.Info("Deriving span batch", "channel", channelId.String())

			derivedBlocks := spanBatch.DeriveL2Blocks()

			logger.Info("Derived blocks", "channel", channelId.String(), "blockCount", len(derivedBlocks))

			// filter out the blocks that are not in the range
			for _, block := range derivedBlocks {
				// we don't need the l2 start block, it is considered as the "safe" head,
				// we only the blocks right after
				if block.NumberU64() > l2StartBlock && block.NumberU64() <= l2EndBlock {
					l2Blocks = append(l2Blocks, block)
				}
			}
		}
	}

	// the collected channels might not be in order, sort them by block number
	slices.SortFunc(l2Blocks, func(i, j *types.Block) int {
		return int(i.NumberU64() - j.NumberU64())
	})

	return l2Blocks, nil
}

func deriveL2States(logger log.Logger,
	l2Oracle l2.Oracle, l1Oracle l1.Oracle, dtlPreimageOracle dtl.Oracle,
	l2Cfg *params.ChainConfig,
	l2OutputRoot common.Hash, l2Blocks []*types.Block, safeHeadHeader, disputedBatchHeader *rollup.BatchHeader) ([]ethhex.Bytes, error) {
	logger.Info("Building up L2 chain...")
	l2Chain, err := l2.NewOracleBackedL2Chain(logger, l2Oracle, l1Oracle, dtlPreimageOracle, l2Cfg, l2common.Hash(l2OutputRoot), safeHeadHeader)
	if err != nil {
		return nil, fmt.Errorf("failed to build L2 chain: %w", err)
	}

	logger.Info("Created L2 chain, start derivation", "head", l2Chain.CurrentHeader().Number.Uint64(), "headHash", l2Chain.CurrentHeader().Hash().Hex())
	parentHeader := l2Chain.CurrentHeader()

	disputedBatchStartBlock, disputedBatchEndBlock := disputedBatchHeader.PrevTotalElements.Uint64()+1, disputedBatchHeader.PrevTotalElements.Uint64()+disputedBatchHeader.BatchSize.Uint64()

	logger.Info("Derivation range", "start", l2Blocks[0].NumberU64(), "end", l2Blocks[len(l2Blocks)-1].NumberU64())
	intermediateStateRoots := make([]ethhex.Bytes, 0, disputedBatchHeader.BatchSize.Uint64())
	for _, block := range l2Blocks {
		logger.Info("Processing L2 block", "block", block.Number().Uint64())

		logger.Info("Checking parent state availability",
			"block", parentHeader.Number.Uint64(),
			"parentHash", parentHeader.Hash().Hex())
		if !l2Chain.HasBlockAndState(parentHeader.Hash(), parentHeader.Number.Uint64()) {
			logger.Error("missing parent block or state", "block", parentHeader.Number.Uint64())
			return nil, fmt.Errorf("missing parent block %d", parentHeader.Number.Uint64())
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

		logger.Info("Creating intermediate state at", "block", block.Number().Uint64(), "root", parentHeader.Root.Hex())
		state, err := l2Chain.StateAt(parentHeader.Root)
		if err != nil {
			return nil, fmt.Errorf("failed to get state at %d: %w", block.Number().Uint64()-1, err)
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
			return nil, fmt.Errorf("failed to finalize block %d: %w", block.Number().Uint64(), err)
		}

		logger.Info("Block finalized", "block", block.Number().Uint64(),
			"hash", block.Hash().Hex())

		// commit the state
		root, err := state.Commit(l2Chain.Config().IsEIP158(l2Chain.CurrentHeader().Number))
		if err != nil {
			return nil, fmt.Errorf("state write error: %w", err)
		}

		if err := state.Database().TrieDB().Commit(root, true); err != nil {
			return nil, fmt.Errorf("trie write error: %w", err)
		}

		// only append the state roots of the disputed batch
		if block.NumberU64() >= disputedBatchStartBlock && block.NumberU64() <= disputedBatchEndBlock {
			intermediateStateRoots = append(intermediateStateRoots, root.Bytes())
		}
		logger.Info("State commited", "block", block.Number().Uint64(), "root", root.Hex())

		// Set head
		if _, err := l2Chain.SetCanonical(block); err != nil {
			return nil, fmt.Errorf("failed to set canonical block %d: %w", block, err)
		}

		logger.Debug("Canonical block set", "block", block.Number().Uint64())

		// Add block
		l2Chain.InsertBlockWithoutSetHead(block)

		logger.Debug("Block inserted", "block", block.Number().Uint64())

		parentHeader = block.Header()

		logger.Debug("Block processing finished", "block", block.Number().Uint64(), "hash", block.Hash().Hex())
	}

	logger.Info("Derivation finished, validating claim", "head", l2Chain.CurrentHeader().Number.Uint64(),
		"headHash", l2Chain.CurrentHeader().Hash().Hex(),
		"stateRoot", l2Chain.CurrentHeader().Root.Hex())

	return intermediateStateRoots, nil
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
