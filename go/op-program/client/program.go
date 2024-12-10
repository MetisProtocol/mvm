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
	l2eth "github.com/MetisProtocol/mvm/l2geth/eth"
	"github.com/MetisProtocol/mvm/l2geth/params"
	"github.com/MetisProtocol/mvm/l2geth/rollup"
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

	l2StartBlock := getBatchBlockFromDTLOracle(logger, l2BatchStartBlock, l2Cfg.ChainID, rollupOracle)
	if l2StartBlock == nil {
		return fmt.Errorf("failed to get start block")
	}

	logger.Info("Starting process L2 blocks", "start", l2StartBlock.Number().Uint64(), "end", l2ClaimBlockNum)

	parentHeader := l2Oracle.BlockHeaderByNumber(uint64(l2Batch.PrevTotalElements))
	parentHeaderJSON, _ := json.Marshal(parentHeader)
	logger.Info("Parent header", "header", string(parentHeaderJSON))

	l2Chain, err := l2.NewOracleBackedL2Chain(logger, l2Oracle, nil, l2Cfg, parentHeader.Hash())
	if err != nil {
		return fmt.Errorf("failed to create oracle-backed L2 chain: %w", err)
	}

	logger.Info("Created L2 chain", "head", l2Chain.CurrentHeader().Number.Uint64(), "headHash", l2Chain.CurrentHeader().Hash().Hex())

	for l2Block := l2StartBlock; l2Block.NumberU64() <= l2ClaimBlockNum; l2Block = getBatchBlockFromDTLOracle(logger, l2Block.NumberU64()+1, l2Cfg.ChainID, rollupOracle) {
		logger.Info("Processing L2 block", "block", l2Block.Number().Uint64())
		logger.Info("Checking parent state availability",
			"block", parentHeader.Number.Uint64(),
			"parentHash", parentHeader.Hash().Hex())
		if !l2Chain.HasBlockAndState(parentHeader.Hash(), parentHeader.Number.Uint64()) {
			logger.Error("missing parent block or state", "block", parentHeader.Number.Uint64())
			return fmt.Errorf("missing parent block %d", l2Block.Number().Uint64()-1)
		}

		consEngine := l2Chain.Engine()
		blockHeader := &types.Header{
			ParentHash: parentHeader.Hash(),
			Number:     l2Block.Number(),
			GasLimit:   l2eth.DefaultConfig.Miner.GasFloor,
			Extra:      l2eth.DefaultConfig.Miner.ExtraData,
			Time:       l2Block.Time(),
			Difficulty: consEngine.CalcDifficulty(l2Chain, l2Block.Time(), parentHeader),
		}
		if err := consEngine.Prepare(l2Chain, blockHeader); err != nil {
			return fmt.Errorf("failed to prepare block %d: %w", l2Block.Number().Uint64(), err)
		}

		state, err := l2Chain.StateAt(parentHeader.Root)
		if err != nil {
			return fmt.Errorf("failed to get state at %d: %w", l2Block.Number().Uint64()-1, err)
		}

		txs := l2Block.Transactions()
		gasUsed := uint64(0)
		emptyAddress := common.Address{}
		receipts := make(types.Receipts, 0, len(txs))
		logs := make([]*types.Log, 0)
		if len(txs) > 0 {
			firstTx := txs[0]
			gp := new(core.GasPool).AddGas(parentHeader.GasLimit)
			for i, tx := range txs {
				tx.SetL1BlockNumber(firstTx.L1BlockNumber().Uint64())
				tx.SetIndex(*firstTx.GetMeta().Index)
				tx.SetL1Timestamp(firstTx.L1Timestamp())

				state.Prepare(tx.Hash(), common.Hash{}, i)
				revid := state.Snapshot()
				receipt, err := core.ApplyTransaction(l2Chain.Config(), l2Chain, &emptyAddress, gp, state, blockHeader, tx, &gasUsed, *l2Chain.GetVMConfig())
				if err != nil {
					// not collecting log for failed tx
					state.RevertToSnapshot(revid)
				} else if receipt.Logs != nil {
					logs = append(logs, receipt.Logs...)
				}
				gasUsed += receipt.GasUsed
				receipts = append(receipts, receipt)
			}
		}

		l2Block, err = consEngine.FinalizeAndAssemble(l2Chain, blockHeader, state, txs, nil, receipts)
		if err != nil {
			return fmt.Errorf("failed to finalize block %d: %w", l2Block.Number().Uint64(), err)
		}

		headerJSON, _ := json.Marshal(l2Block.Header())
		logger.Info("L2 Block header", "block", l2Block.Number().Uint64(), "header", string(headerJSON))

		if err := consEngine.SyncSeal(l2Chain, l2Block); err != nil {
			return fmt.Errorf("failed to sync seal block %d: %w", l2Block.Number().Uint64(), err)
		}

		if _, err := l2Chain.SetCanonical(l2Block); err != nil {
			return fmt.Errorf("failed to set canonical block %d: %w", l2Block, err)
		}

		if err := l2Chain.InsertBlockWithoutSetHead(l2Block); err != nil {
			return fmt.Errorf("failed to insert block %d: %w", l2Block, err)
		}

		parentHeader = l2Block.Header()
	}

	return claim.ValidateClaim(logger, l2ClaimBlockNum, eth.Bytes32(l2Claim), l2Chain)
}

func getBatchBlockFromDTLOracle(logger log.Logger, l2BlockNumber uint64, chainId *big.Int, rollupOracle dtl.Oracle) *types.Block {
	// transmit the data piece by piece to avoid data cutoff by oracle
	l2BlockMeta := rollupOracle.L2BlockMeta(l2BlockNumber)
	if l2BlockMeta == nil {
		logger.Error("failed to get start block meta")
		return nil
	}

	var txs []*rollup.Transaction
	if l2BlockMeta.TransactionCount > 0 {
		for i := uint64(0); i < l2BlockMeta.TransactionCount; i++ {
			tx := rollupOracle.L2BatchTransaction(l2BlockNumber, i)
			if tx == nil {
				logger.Error("failed to get transaction", "index", i, "block", l2BlockNumber)
				return nil
			}
			txs = append(txs, tx)
		}
	}

	signer := types.NewEIP155Signer(chainId)
	l2Block, err := rollup.BatchedBlockToBlock(&rollup.Block{
		Index:        l2BlockMeta.Index,
		BatchIndex:   l2BlockMeta.BatchIndex,
		Timestamp:    l2BlockMeta.Timestamp,
		Transactions: txs,
		Confirmed:    l2BlockMeta.Confirmed,
	}, &signer)
	if err != nil {
		logger.Error("failed to convert batched block to block", "err", err)
		return nil
	}

	return l2Block
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
