package main

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/ethclient"
	"github.com/MetisProtocol/mvm/l2geth/rollup"
)

func main() {
	targetBlockRange := []int{1454290, 0}
	workers := 10
	l2gethURL := "http://127.0.0.1:8545"
	l1dtlURL := "http://127.0.0.1:7878"

	// Connect to L2 Geth
	l2gethClient, err := ethclient.Dial(l2gethURL)
	if err != nil {
		panic(err)
	}

	chainID, err := l2gethClient.ChainID(context.Background())
	if err != nil {
		panic(err)
	}

	l1DTLClient := rollup.NewClient(l1dtlURL, chainID)

	startBlock := targetBlockRange[0]
	endBlock := targetBlockRange[1]

	if endBlock == 0 {
		latestDTLBlock, err := l1DTLClient.GetLatestBlock(rollup.BackendL1)
		if err != nil {
			panic(err)
		}

		endBlock = int(latestDTLBlock.NumberU64()) - 100
	}

	startBatch, err := getBlockBatchInfo(uint64(startBlock), l1DTLClient)
	if err != nil {
		panic(err)
	}

	endBatch, err := getBlockBatchInfo(uint64(endBlock), l1DTLClient)
	if err != nil {
		panic(err)
	}

	validCount := uint64(0)
	inValidCount := uint64(0)

	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Recovered from panic: ", r)
		}

		fmt.Printf("🚀 Total batches: %d\n", validCount+inValidCount)
		fmt.Printf("✅ Total valid block batches: %d\n", validCount)
		fmt.Printf("❌ Total invalid block batches: %d\n", inValidCount)
	}()

	jobCh := make(chan uint64, workers)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		go func(workerId int) {
			for {
				select {
				case <-ctx.Done():
					fmt.Printf("👋 Worker %d done\n", workerId)
					return
				case batchIndex := <-jobCh:
					var batch *rollup.Batch
					for batch == nil {
						batch, _, err = l1DTLClient.GetBlockBatch(batchIndex)
						if err != nil {
							fmt.Println("Error getting block batch: ", err)
							continue
						}
						break
					}

					batchStartBlock := int(batch.PrevTotalElements + 1)
					batchEndBlock := int(batch.PrevTotalElements + 1 + batch.Size)
					if isBlockBlockBatchValid(batchStartBlock, batchEndBlock, l2gethClient, l1DTLClient) {
						fmt.Printf("✅ Block batch %d is valid, start block: %d, end block: %d\n", batch.Index, batchStartBlock, batchEndBlock)
						atomic.AddUint64(&validCount, 1)
					} else {
						fmt.Printf("❌ Block batch is %d invalid, start block: %d, end block: %d\n", batch.Index, batchStartBlock, batchEndBlock)
						atomic.AddUint64(&inValidCount, 1)
					}
					wg.Done()
				}
			}
		}(i)
	}

	for i := startBatch.Index; i <= endBatch.Index; i++ {
		jobCh <- i
		wg.Add(1)
	}

	wg.Wait()
}

func getBlockBatchInfo(block uint64, l1DTLClient *rollup.Client) (*rollup.Batch, error) {
	blockResp, err := l1DTLClient.GetRawBlock(block-1, rollup.BackendL1)
	if err != nil {
		return nil, err
	}

	return blockResp.Batch, nil
}

func isBlockBlockBatchValid(startBlock, endBlock int, l2gethClient *ethclient.Client, l1DTLClient *rollup.Client) bool {
	for i := startBlock; i <= endBlock; i++ {
		targetBlock := int64(i)
		targetIndex := uint64(i - 1)

		var block, block0 *types.Block
		var err error

		for block == nil {
			block, err = l1DTLClient.GetBlock(targetIndex, rollup.BackendL1)
			if err == nil {
				break
			}
			fmt.Printf("Error getting block %d from DTL: %s\n", targetBlock, err)
		}

		for block0 == nil {
			block0, err = l2gethClient.BlockByNumber(context.TODO(), big.NewInt(targetBlock))
			if err == nil {
				break
			}
			fmt.Printf("Error getting block %d from l2geth: %s\n", targetBlock, err)
		}

		block0TxRoot := block0.TxHash()

		zero := uint64(0)
		if block.Header().Number.Uint64() != block0.NumberU64() {
			fmt.Println(fmt.Errorf("Block number mismatch: %d != %d", block.Header().Number.Uint64(), block0.NumberU64()))
			return false
		}
		if block.Transactions().Len() != block0.Transactions().Len() {
			fmt.Println(fmt.Errorf("Transaction count mismatch: %d != %d", block.Transactions().Len(), block0.Transactions().Len()))
			return false
		}

		for i, tx := range block.Transactions() {
			tx0 := block0.Transactions()[i]

			if tx0.Hash() != tx.Hash() {
				fmt.Println(fmt.Errorf("Transaction hash mismatch: %s != %s", tx0.Hash().Hex(), tx.Hash().Hex()))
				return false
			}

			// compare every field of transaction meta
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

			meta2 := tx0.GetMeta()

			//fmt.Printf("tx %d: %s\n", i, tx.Hash().Hex())

			if *meta.QueueIndex != *meta2.QueueIndex {
				fmt.Println(fmt.Errorf("QueueIndex mismatch: %d != %d", *meta.QueueIndex, *meta2.QueueIndex))
				return false
			}
			if meta.L1BlockNumber.Cmp(meta2.L1BlockNumber) != 0 {
				fmt.Println(fmt.Errorf("L1BlockNumber mismatch: %d != %d", meta.L1BlockNumber, meta2.L1BlockNumber))
				return false
			}
			if meta.L1Timestamp != meta2.L1Timestamp {
				fmt.Println(fmt.Errorf("L1Timestamp mismatch: %d != %d", meta.L1Timestamp, meta2.L1Timestamp))
				return false
			}
			if meta.L1MessageSender.Hex() != meta.L1MessageSender.Hex() {
				fmt.Println(fmt.Errorf("L1MessageSender mismatch: %s != %s", meta.L1MessageSender.Hex(), meta2.L1MessageSender.Hex()))
				return false
			}
			if meta.QueueOrigin != meta2.QueueOrigin {
				fmt.Println(fmt.Errorf("QueueOrigin mismatch: %d != %d", meta.QueueOrigin, meta2.QueueOrigin))
				return false
			}
			if *meta.Index != *meta2.Index {
				fmt.Println(fmt.Errorf("Index mismatch: %d != %d", *meta.Index, *meta2.Index))
				return false
			}
			if bytes.Compare(meta.RawTransaction, meta2.RawTransaction) != 0 {
				fmt.Println(fmt.Errorf("RawTransaction mismatch: %s != %s", meta.RawTransaction, meta2.RawTransaction))
				return false
			}
			if meta.R.Cmp(meta2.R) != 0 {
				fmt.Println(fmt.Errorf("R mismatch: %s != %s", meta.R.String(), meta2.R.String()))
				return false
			}
			if meta.S.Cmp(meta2.S) != 0 {
				fmt.Println(fmt.Errorf("S mismatch: %s != %s", meta.S.String(), meta2.S.String()))
				return false
			}
			if meta.V.Cmp(meta2.V) != 0 {
				fmt.Println(fmt.Errorf("V mismatch: %s != %s", meta.V.String(), meta2.V.String()))
				return false
			}
		}

		root := types.DeriveSha(block.Transactions())

		if block0TxRoot != root {
			fmt.Println(fmt.Errorf("Transaction root mismatch: %s != %s", block0TxRoot.Hex(), root.Hex()))
			return false
		}
	}

	return true
}
