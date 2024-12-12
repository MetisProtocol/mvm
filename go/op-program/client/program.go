package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/log"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/core"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/params"
	"github.com/MetisProtocol/mvm/l2geth/rollup/rcfg"
	dtl "github.com/ethereum-optimism/optimism/go/op-program/client/rollup"

	"github.com/ethereum-optimism/optimism/op-service/eth"

	preimage "github.com/ethereum-optimism/optimism/go/op-preimage"
	"github.com/ethereum-optimism/optimism/go/op-program/client/claim"
	"github.com/ethereum-optimism/optimism/go/op-program/client/l2"
)

// Main executes the client program in a detached context and exits the current process.
// The client runtime environment must be preset before calling this function.
func Main(logger log.Logger) {
	logger.Info("Starting fault proof program client")
	preimageOracle := CreatePreimageChannel()
	preimageHinter := CreateHinterChannel()
	if err := RunProgram(logger, preimageOracle, preimageHinter); errors.Is(err, claim.ErrClaimNotValid) {
		logger.Error("Claim is invalid", "err", err)
		os.Exit(1)
	} else if err != nil {
		logger.Error("Program failed", "err", err)
		os.Exit(2)
	} else {
		logger.Info("Claim successfully verified")
		os.Exit(0)
	}
}

// RunProgram executes the Program, while attached to an IO based pre-image oracle, to be served by a host.
func RunProgram(logger log.Logger, preimageOracle io.ReadWriter, preimageHinter io.ReadWriter) error {
	logger.Info("Starting program...")
	pClient := preimage.NewOracleClient(preimageOracle)
	hClient := preimage.NewHintWriter(preimageHinter)
	l2PreimageOracle := l2.NewCachingOracle(l2.NewPreimageOracle(pClient, hClient))
	rollupPreimageOracle := dtl.NewCachingOracle(dtl.NewPreimageOracle(pClient, hClient))
	logger.Info("Created preimage oracles and hint writer")

	bootInfo := NewBootstrapClient(pClient).BootInfo()
	logger.Info("Program Bootstrapped", "bootInfo", bootInfo)

	// force ovm to true
	rcfg.UsingOVM = true

	return runDerivation(
		logger,
		bootInfo.L2ChainConfig,
		bootInfo.L2Claim,
		bootInfo.L2ClaimBlockNumber,
		l2PreimageOracle,
		rollupPreimageOracle,
	)
}

// runDerivation executes the L2 state transition, given a minimal interface to retrieve data.
func runDerivation(logger log.Logger, l2Cfg *params.ChainConfig, l2Claim common.Hash, l2ClaimBlockNum uint64, l2Oracle l2.Oracle, rollupOracle dtl.Oracle) error {
	// retrieve the start l2 block of current batch
	l2Batch := rollupOracle.L2BatchOfBlock(l2ClaimBlockNum)
	if l2Batch == nil {
		logger.Error("failed to get target l2 block with batch info")
		return fmt.Errorf("failed to get target l2 block with batch info")
	}

	l2BatchStartBlock := uint64(l2Batch.PrevTotalElements + 1)
	if l2BatchStartBlock > l2ClaimBlockNum {
		logger.Error("something is wrong, start block is less than the end block", "start", l2BatchStartBlock, "end", l2ClaimBlockNum)
		return fmt.Errorf("something is wrong, start block is less than the end block: %d vs %d", l2BatchStartBlock, l2ClaimBlockNum)
	}

	l2StartBlock := getBatchBlockFromDTLOracle(logger, l2BatchStartBlock, rollupOracle)
	if l2StartBlock == nil {
		return fmt.Errorf("failed to get start block")
	}

	logger.Info("Starting process L2 blocks", "start", l2StartBlock.Number().Uint64(), "end", l2ClaimBlockNum)

	parentHeader := l2Oracle.BlockHeaderByNumber(l2StartBlock.Number().Uint64() - 1)
	parentHeaderJSON, _ := json.Marshal(parentHeader)
	logger.Info("Parent header", "header", string(parentHeaderJSON))

	l2Chain, err := l2.NewOracleBackedL2Chain(logger, l2Oracle, nil, l2Cfg, parentHeader.Hash())
	if err != nil {
		return fmt.Errorf("failed to create oracle-backed L2 chain: %w", err)
	}

	logger.Info("Created L2 chain", "head", l2Chain.CurrentHeader().Number.Uint64(), "headHash", l2Chain.CurrentHeader().Hash().Hex())

	for l2Block := l2StartBlock; l2Block.NumberU64() <= l2ClaimBlockNum; l2Block = getBatchBlockFromDTLOracle(logger, l2Block.NumberU64()+1, rollupOracle) {
		logger.Info("Processing L2 block", "block", l2Block.Number().Uint64())
		logger.Info("Checking parent state availability",
			"block", parentHeader.Number.Uint64(),
			"parentHash", parentHeader.Hash().Hex())
		if !l2Chain.HasBlockAndState(parentHeader.Hash(), parentHeader.Number.Uint64()) {
			logger.Error("missing parent block or state", "block", parentHeader.Number.Uint64())
			return fmt.Errorf("missing parent block %d", l2Block.Number().Uint64()-1)
		} else {
			logger.Debug("Parent block and state available", "block", parentHeader.Number.Uint64())
		}

		// Note(@dumdumgoose): We will retrieve the data from l1 DTL as much as possible, but in some cases,
		// to avoid large state, we will need to retrieve the l2 block header from the l2 oracle,
		// since the most important to us are the transaction and state correctness.
		l2Header := l2Oracle.BlockHeaderByNumber(l2Block.Number().Uint64())

		if l2Block.Header().TxHash != l2Header.TxHash {
			logger.Error("l2 block header tx hash mismatch", "block", l2Block.Number().Uint64(), "l2geth", l2Header.TxHash, "dtl", l2Block.Header().TxHash)
			panic("l2 block header tx hash mismatch")
		}

		consEngine := l2Chain.Engine()
		blockHeader := &types.Header{
			ParentHash: parentHeader.Hash(),
			Number:     l2Block.Number(),
			GasLimit:   parentHeader.GasLimit,
			Extra:      l2Header.Extra,
			Time:       l2Block.Time(),
			// Note(@dumdumgoose): Our difficulty is always 2, hardcode the difficulty here, because
			// if we calculate the difficulty with clique, it will traverse the blocks back at most an epoch,
			// which will make our state very large.
			Difficulty: big.NewInt(2),
			Coinbase:   l2Header.Coinbase,
			Nonce:      l2Header.Nonce,
			MixDigest:  l2Header.MixDigest,
		}

		logger.Debug("Block header created", "block", l2Block.Number().Uint64(), "header", blockHeader)
		state, err := l2Chain.StateAt(parentHeader.Root)
		if err != nil {
			return fmt.Errorf("failed to get state at %d: %w", l2Block.Number().Uint64()-1, err)
		}

		logger.Debug("State retrieved", "block", l2Block.Number().Uint64())

		txs := l2Block.Transactions()

		logger.Debug("Transactions retrieved", "block", l2Block.Number().Uint64(), "txs", len(txs))

		emptyAddress := common.Address{}
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

				state.Prepare(tx.Hash(), common.Hash{}, i)

				logger.Debug("Applying transaction", "block", l2Block.Number().Uint64(), "tx", i, "hash", tx.Hash())

				revid := state.Snapshot()
				receipt, err := core.ApplyTransaction(l2Chain.Config(), l2Chain, &emptyAddress, gp, state, blockHeader, tx, &blockHeader.GasUsed, *l2Chain.GetVMConfig())
				if err != nil {
					// not collecting log for failed tx
					logger.Error("Failed to apply transaction", "block", l2Block.Number().Uint64(), "tx", i, "hash", tx.Hash(), "err", err)
					state.RevertToSnapshot(revid)
				} else if receipt.Logs != nil {
					logs = append(logs, receipt.Logs...)
				}
				receipts = append(receipts, receipt)

				receiptJSON, _ := json.Marshal(receipt)

				logger.Debug("Transaction applied", "block", l2Block.Number().Uint64(), "tx", i, "hash", tx.Hash(), "receipt", string(receiptJSON))
			}
		}

		logger.Debug("Transactions applied", "block", l2Block.Number().Uint64(), "gasUsed", blockHeader.GasUsed)

		// Finalize the block
		l2Block, err = consEngine.FinalizeAndAssemble(l2Chain, blockHeader, state, txs, nil, receipts)
		if err != nil {
			return fmt.Errorf("failed to finalize block %d: %w", l2Block.Number().Uint64(), err)
		}

		logger.Debug("Block finalized", "block", l2Block.Number().Uint64())

		// Set head
		if _, err := l2Chain.SetCanonical(l2Block); err != nil {
			return fmt.Errorf("failed to set canonical block %d: %w", l2Block, err)
		}

		logger.Debug("Canonical block set", "block", l2Block.Number().Uint64())

		// Add block
		l2Chain.InsertBlockWithoutSetHead(l2Block)

		logger.Debug("Block inserted", "block", l2Block.Number().Uint64())

		// commit the state
		root, err := state.Commit(l2Chain.Config().IsEIP158(l2Chain.CurrentHeader().Number))
		if err != nil {
			return fmt.Errorf("state write error: %w", err)
		}

		if err := state.Database().TrieDB().Commit(root, true); err != nil {
			return fmt.Errorf("trie write error: %w", err)
		}

		logger.Debug("State commited", "block", l2Block.Number().Uint64())

		parentHeader = l2Block.Header()

		logger.Debug("Block processing finished", "block", l2Block.Number().Uint64(), "hash", l2Block.Hash().Hex())
	}

	logger.Info("Finished processing L2 blocks", "start", l2StartBlock.Number().Uint64(), "end", l2ClaimBlockNum)
	return claim.ValidateClaim(logger, l2ClaimBlockNum, eth.Bytes32(l2Claim), l2Chain)
}

func getBatchBlockFromDTLOracle(logger log.Logger, l2BlockNumber uint64, rollupOracle dtl.Oracle) *types.Block {
	// transmit the data piece by piece to avoid data cutoff by oracle
	l2BlockMeta := rollupOracle.L2BlockMeta(l2BlockNumber)
	if l2BlockMeta == nil {
		logger.Error("failed to get start block meta")
		return nil
	}

	txs := rollupOracle.L2BatchTransactions(l2BlockNumber)
	header := &types.Header{
		Time:   l2BlockMeta.Timestamp,
		Number: big.NewInt(int64(l2BlockMeta.Index + 1)),
	}

	return types.NewBlock(header, txs, nil, nil)
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
