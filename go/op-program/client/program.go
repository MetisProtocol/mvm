package client

import (
  "errors"
  "fmt"
  "io"
  "os"

  "github.com/ethereum/go-ethereum/log"

  "github.com/MetisProtocol/mvm/l2geth/common"
  "github.com/MetisProtocol/mvm/l2geth/core/types"
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
  l2BlockWithBatchInfo := rollupOracle.L2BlockWithBatchInfo(l2ClaimBlockNum)
  if l2BlockWithBatchInfo == nil {
    return fmt.Errorf("failed to get target l2 block with batch info")
  }

  signer := types.NewEIP155Signer(l2Cfg.ChainID)
  l2StartBlock, err := rollup.BatchedBlockToBlock(l2BlockWithBatchInfo.Block, &signer)
  if err != nil {
    return fmt.Errorf("failed to convert batched block to block: %w", err)
  }

  l2BatchStartBlock := uint64(l2BlockWithBatchInfo.Batch.PrevTotalElements)
  if l2BatchStartBlock > l2ClaimBlockNum {
    return fmt.Errorf("something is wrong, start block is less than the end block: %d vs %d", l2BatchStartBlock, l2ClaimBlockNum)
  }

  l2Chain, err := l2.NewOracleBackedL2Chain(logger, l2Oracle, nil, l2Cfg, l2StartBlock.Hash())
  if err != nil {
    return fmt.Errorf("failed to create oracle-backed L2 chain: %w", err)
  }

  for i := l2BatchStartBlock; i <= l2ClaimBlockNum; i++ {
    blockResp := rollupOracle.L2BlockWithBatchInfo(i)
    if blockResp == nil {
      return fmt.Errorf("failed to get l2 block %d with batch info", i)
    }

    block, err := rollup.BatchedBlockToBlock(blockResp.Block, &signer)
    if err != nil {
      return fmt.Errorf("failed to convert batched block to block: %w", err)
    }

    if !l2Chain.HasBlockAndState(block.ParentHash(), block.Number().Uint64()-1) {
      return fmt.Errorf("missing parent block %d", block.Number().Uint64()-1)
    }

    if l2Chain.GetBlockByHash(block.Hash()) != nil {
      logger.Info("block already inserted, ignore", "block", block.Number().Uint64())
      continue
    }

    if _, err := l2Chain.SetCanonical(block); err != nil {
      return fmt.Errorf("failed to set canonical block %d: %w", i, err)
    }

    if err := l2Chain.InsertBlockWithoutSetHead(block); err != nil {
      return fmt.Errorf("failed to insert block %d: %w", i, err)
    }
  }

  return claim.ValidateClaim(logger, l2ClaimBlockNum, eth.Bytes32(l2Claim), l2Chain)
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
