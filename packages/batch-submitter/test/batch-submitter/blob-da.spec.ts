import { ethers, hexlify } from 'ethersv6'
import fs from 'fs'
import { ChannelManager } from '../../src/da/channel-manager'
import { BatchToInboxElement } from '../../src/da/types'
import { CompressionAlgo } from '../../src/da/channel-compressor'
import { L2Block, L2Provider } from '@localtest911/core-utils'
import { loadTrustedSetup } from 'c-kzg'
import { Logger } from '@eth-optimism/common-ts'

loadTrustedSetup(0)

describe('ChannelManager', function () {
  this.timeout(600000)

  const l1RpcUrl = '{l1-rpc-url}'
  const l1Provider = new ethers.JsonRpcProvider(l1RpcUrl)
  const l2RpcUrl = '{l2-rpc-url}'
  const l2Provider = new L2Provider(l2RpcUrl)
  const channelConfig = {
    maxFrameSize: 120000,
    targetFrames: 6,
    maxBlocksPerSpanBatch: 0,
    targetNumFrames: 1,
    targetCompressorFactor: 0.6,
    compressionAlgo: CompressionAlgo.Brotli,
    batchType: 1,
    useBlobs: true,
  }
  const rollupConfig = {
    l1ChainID: BigInt(1),
    l2ChainID: BigInt(1088),
    batchInboxAddress: '0xFakeInboxAddress',
  }

  it('should fetch L2 blocks, add to channel, and dump txData to JSON', async () => {
    const channelManager = new ChannelManager(
      new Logger({ name: 'ChannelManager', level: 'debug' }),
      channelConfig,
      rollupConfig,
      l1Provider
    )
    const latestBlock = await l2Provider.getBlockNumber()

    const blockPromises: Promise<BatchToInboxElement>[] = []
    const getBatchElement = async (blockNum: number) => {
      const block = (await l2Provider.getBlock(blockNum, true)) as L2Block
      await Promise.all(block.l2TransactionPromises)
      const batchElement: BatchToInboxElement = {
        stateRoot: block.stateRoot,
        timestamp: block.timestamp,
        blockNumber: block.number,
        hash: block.hash,
        parentHash: block.parentHash,
        txs: block.l2Transactions.map((tx) => ({
          rawTransaction: tx.rawTransaction,
          isSequencerTx: tx.queueOrigin === 'sequencer',
          seqSign:
            tx.seqR && tx.seqS && tx.seqV
              ? `${tx.seqR}${tx.seqS}${tx.seqV}`
              : null,
          l1BlockNumber: block.number,
          l1TxOrigin: tx.queueOrigin,
          queueIndex: tx.nonce,
        })),
      }

      console.log(
        `Fetched block ${blockNum}, tx count: ${block.transactions.length}`
      )
      return batchElement
    }

    for (let i = latestBlock - 10000; i < latestBlock; i++) {
      blockPromises.push(getBatchElement(i))
    }

    const blocks = await Promise.all(blockPromises)
    blocks.sort((a, b) => a.blockNumber - b.blockNumber)

    for (const block of blocks) {
      channelManager.addL2Block(block)
      console.log(`Added block ${block.blockNumber} to channel`)
    }

    const txDataArray = []

    while (true) {
      const [txData, end] = await channelManager.txData(BigInt(10000))
      const blobs = txData.blobs.map((blob) => hexlify(blob.data))
      txDataArray.push(blobs)
      if (end) {
        break
      }
    }

    fs.writeFileSync('txData.json', JSON.stringify(txDataArray, null, 2))
  })
})
