import { ethers } from 'ethersv6'
import { BlobDataExpiredError } from '../../services/l1-ingestion/handlers/errors'
import {
  BatchData,
  batchReader,
  BlobTxType,
  Channel,
  RawSpanBatch,
  SpanBatchType,
} from './channel'
import { Frame, parseFrames } from './frame'
import { L1BeaconClient } from './l1-beacon-client'

const blobExpireBlocks = 4096 * 32

interface FetchBatchesConfig {
  blobTxHashes: string[] // blob transaction hashes
  chainId: number // l1 chain id
  batchInbox: string // batch inbox address
  batchSenders: string[] // batch sender address
  concurrentRequests: number // concurrent requests number
  l2ChainId: number // l2 chain id

  l1RpcProvider: ethers.JsonRpcProvider
  l1BeaconProvider: L1BeaconClient
}

let chainIdHasChecked = false // whether chain id has been checked

// fetch l2 batches from l1 chain
export const fetchBatches = async (fetchConf: FetchBatchesConfig) => {
  const { l1RpcProvider, l1BeaconProvider } = fetchConf
  if (!chainIdHasChecked) {
    const checkId = await l1BeaconProvider.getChainId()
    if (Number(checkId) !== fetchConf.chainId) {
      throw new Error(
        `Chain ID mismatch: Beacon ${checkId} !== Expected ${fetchConf.chainId}`
      )
    }
    chainIdHasChecked = true
  }

  const latestBlock = await l1RpcProvider.getBlockNumber()

  // TODO: fetch batches concurrently
  const txsMetadata = []
  const channelsMetadata = []
  for (const blobTxHash of fetchConf.blobTxHashes) {
    // fetch tx and receipt from el
    const receipt = await l1RpcProvider.getTransactionReceipt(blobTxHash)
    if (!receipt) {
      throw new Error(`Tx or receipt of ${blobTxHash} not found`)
    }

    // checks whether blob data has expired
    if (latestBlock - receipt.blockNumber > blobExpireBlocks) {
      throw new BlobDataExpiredError(blobTxHash)
    }

    const block = await l1RpcProvider.getBlock(receipt.blockHash, true)
    if (!block) {
      throw new Error(`Block ${receipt.blockNumber} not found`)
    }

    const tx = block.prefetchedTransactions[receipt.index]
    if (!tx || tx.hash !== blobTxHash) {
      throw new Error(`Transaction ${blobTxHash} not found in block`)
    }
    if (tx.type !== BlobTxType) {
      // We are not processing old transactions those are using call data,
      // this should not happen.
      throw new Error(
        `Found inbox transaction ${tx.hash} that is not using blob, ignore`
      )
    }
    // no blob in this blob tx
    if (!tx.blobVersionedHashes || tx.blobVersionedHashes.length === 0) {
      throw new Error(`No blobVersionedHashes found in transaction ${tx.hash}`)
    }

    // only process the blob tx hash recorded in the commitment
    const sender = tx.from
    if (!fetchConf.batchSenders.includes(sender.toLowerCase())) {
      continue
    }

    const frames: Frame[] = []
    {
      // fetch blob data from beacon chain
      const blobs = await l1BeaconProvider.getBlobs(
        block.timestamp,
        tx.blobVersionedHashes
      )
      if (blobs.length !== tx.blobVersionedHashes.length) {
        throw new Error(
          `Blob count mismatch in tx ${tx.hash}: expected ${tx.blobVersionedHashes.length}, got ${blobs.length}`
        )
      }
      for (const blob of blobs) {
        frames.push(...parseFrames(blob, receipt.blockNumber))
      }
    }

    const txMetadata = {
      txIndex: tx.index,
      inboxAddr: tx.to,
      blockNumber: receipt.blockNumber,
      blockHash: receipt.blockHash,
      blockTime: block.timestamp,
      chainId: fetchConf.chainId,
      sender,
      validSender: true,
      tx,
      frames: frames.map((frame) => ({
        id: Buffer.from(frame.id).toString('hex'),
        data: frame.data,
        isLast: frame.isLast,
        frameNumber: frame.frameNumber,
        inclusionBlock: frame.inclusionBlock,
      })),
    }

    txsMetadata.push(txMetadata)
  }

  const channelMap: { [channelId: string]: Channel } = {}

  // process downloaded tx metadata
  for (const txMetadata of txsMetadata) {
    const framesData = txMetadata.frames

    for (const frameData of framesData) {
      const frame: Frame = {
        id: Buffer.from(frameData.id, 'hex'),
        frameNumber: frameData.frameNumber,
        data: frameData.data,
        isLast: frameData.isLast,
        inclusionBlock: frameData.inclusionBlock,
      }
      const channelId = frameData.id

      if (!channelMap[channelId]) {
        channelMap[channelId] = new Channel(channelId, frame.inclusionBlock)
      }

      // add frames to channel
      channelMap[channelId].addFrame(frame)
    }

    for (const channelId in channelMap) {
      if (!channelMap.hasOwnProperty(channelId)) {
        // ignore object prototype properties
        continue
      }

      const channel = channelMap[channelId]

      // Collect frames metadata
      const framesMetadata = Array.from(channel.inputs.values()).map(
        (frame) => {
          return {
            id: Buffer.from(frame.id).toString('hex'),
            frameNumber: frame.frameNumber,
            inclusionBlock: frame.inclusionBlock,
            isLast: frame.isLast,
            data: Buffer.from(frame.data).toString('base64'),
          }
        }
      )

      if (!channel || !channel.isReady()) {
        continue
      }

      // Read batches from channel
      const reader = channel.reader()

      const batches = []
      const batchTypes = []
      const comprAlgos = []

      // By default, this is after fjord, since we are directly upgrade to fjord,
      // so no need to keep compatibility for old op versions
      const readBatch = await batchReader(reader)
      let batchData: BatchData | null
      while ((batchData = await readBatch())) {
        if (batchData.batchType === SpanBatchType) {
          const spanBatch = batchData.inner as RawSpanBatch
          batchData.inner = await spanBatch.derive(
            ethers.toBigInt(fetchConf.l2ChainId)
          )
        }
        batches.push(batchData.inner)
        batchTypes.push(batchData.batchType)
        if (batchData.comprAlgo) {
          comprAlgos.push(batchData.comprAlgo)
        }
      }

      const channelMetadata = {
        id: channelId,
        isReady: channel.isReady(),
        frames: framesMetadata,
        batches,
        batchTypes,
        comprAlgos,
      }

      channelsMetadata.push(channelMetadata)
    }
  }

  return channelsMetadata
}
