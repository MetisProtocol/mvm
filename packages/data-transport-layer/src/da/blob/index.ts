import { ethers } from 'ethersv6'
import { Frame, parseFrames } from './frame'
import { L1BeaconClient } from './l1-beacon-client'
import {
  BatchData,
  batchReader,
  BlobTxType,
  Channel,
  RawSpanBatch,
  SpanBatchType,
} from './channel'
import { BlobDataExpiredError } from '../../services/l1-ingestion/handlers/errors'

const blobExpireBlocks = 4096 * 32

interface FetchBatchesConfig {
  blobTxHashes: string[] // blob transaction hashes
  chainId: number // l1 chain id
  batchInbox: string // batch inbox address
  batchSenders: string[] // batch sender address
  concurrentRequests: number // concurrent requests number
  l2ChainId: number // l2 chain id

  l1Rpc: string // l1 rpc url
  l1Beacon: string // l1 beacon chain url
}

// fetch l2 batches from l1 chain
export const fetchBatches = async (fetchConf: FetchBatchesConfig) => {
  const l1RpcProvider = new ethers.JsonRpcProvider(fetchConf.l1Rpc)
  const l1BeaconProvider = new L1BeaconClient(fetchConf.l1Beacon)
  try {
    await l1BeaconProvider.checkVersion()
  } catch (e) {
    throw new Error('Failed to ping beacon chain, connection error')
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

    // TODO: We might be able to cache this somewhere, no need to retrieve this every time.
    //       But due to potential chain reorgs, just retrieve the data everytime for now.
    //       Might need to think of a better solution in the future.
    const block = await l1RpcProvider.getBlock(receipt.blockNumber, true)
    if (!block) {
      throw new Error(`Block ${receipt.blockNumber} not found`)
    }

    const txs = block.prefetchedTransactions

    // Even we got the hash of the blob tx, we still need to traverse through the blocks
    // since we need to count the blob index in the block
    let blobIndex = 0
    for (const tx of txs) {
      if (!tx) {
        continue
      }

      // only process the blob tx hash recorded in the commitment
      if (blobTxHash.toLowerCase() === tx.hash.toLowerCase()) {
        const sender = tx.from
        if (!fetchConf.batchSenders.includes(sender.toLowerCase())) {
          continue
        }

        const datas: Uint8Array[] = []
        if (tx.type !== BlobTxType) {
          // We are not processing old transactions those are using call data,
          // this should not happen.
          throw new Error(
            `Found inbox transaction ${tx.hash} that is not using blob, ignore`
          )
        } else {
          if (!tx.blobVersionedHashes) {
            // no blob in this blob tx, ignore
            continue
          }

          // get blob hashes and indices
          const hashes = tx.blobVersionedHashes.map((hash, index) => ({
            index: blobIndex + index,
            hash,
          }))
          blobIndex += hashes.length

          // fetch blob data from beacon chain
          const blobs = await l1BeaconProvider.getBlobs(
            block.timestamp,
            hashes.map((h) => h.index)
          )

          for (const blob of blobs) {
            datas.push(blob.data)
          }
        }

        let frames: Frame[] = []
        for (const data of datas) {
          try {
            // parse the frames from the blob data
            const parsedFrames = parseFrames(data, block.number)
            frames = frames.concat(parsedFrames)
          } catch (err) {
            // invalid frame data in the blob, stop and throw error
            throw new Error(`Failed to parse frames: ${err}`)
          }
        }

        const txMetadata = {
          txIndex: tx.index,
          inboxAddr: tx.to,
          blockNumber: block.number,
          blockHash: block.hash,
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
      } else {
        blobIndex += tx.blobVersionedHashes?.length || 0
      }
    }
  }

  const channelMap: { [channelId: string]: Channel } = {}
  const frameDataValidity: { [channelId: string]: boolean } = {}

  // process downloaded tx metadata
  for (const txMetadata of txsMetadata) {
    const framesData = txMetadata.frames

    const invalidFrames = false
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
      try {
        channelMap[channelId].addFrame(frame)
        frameDataValidity[channelId] = true
      } catch (e) {
        frameDataValidity[channelId] = false
      }
    }

    if (invalidFrames) {
      continue
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

      // short circuit if frame data is invalid
      if (!frameDataValidity[channelId]) {
        channelsMetadata.push({
          id: channelId,
          isReady: channel.isReady(),
          invalidFrames: true,
          invalidBatches: false,
          frames: framesMetadata,
          batches: [],
          batchTypes: [],
          comprAlgos: [],
        })
        continue
      }

      if (!channel || !channel.isReady()) {
        continue
      }

      // Read batches from channel
      const reader = channel.reader()

      const batches = []
      const batchTypes = []
      const comprAlgos = []
      let invalidBatches = false

      try {
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
      } catch (err) {
        // mark batches as invalid
        console.log(`Failed to read batches: ${err}`)
        invalidBatches = true
      }

      const channelMetadata = {
        id: channelId,
        isReady: channel.isReady(),
        invalidFrames: false,
        invalidBatches,
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
