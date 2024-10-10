import { ethers } from 'ethers'
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
export const fetchBatches = async (
  fetchConf: FetchBatchesConfig,
) => {
  console.log('Fetching batches with config:', fetchConf)

  const l1RpcProvider = new ethers.JsonRpcProvider(fetchConf.l1Rpc)
  const l1BeaconProvider = new L1BeaconClient(fetchConf.l1Beacon)

  // TODO: fetch batches concurrently
  const txsMetadata = []
  const channelsMetadata = []
  for (const blobTxHash of fetchConf.blobTxHashes) {
    console.warn(`Fetching blob tx: ${blobTxHash}`)
    try {
      // fetch tx and receipt from el
      const receipt = await l1RpcProvider.getTransactionReceipt(blobTxHash)
      if (!receipt) {
        throw new Error(`Tx or receipt of ${blobTxHash} not found`)
      }

      // TODO: We might be able to cache this somewhere, no need to retrieve this every time.
      //       But due to potential chain reorgs, just retrieve the data everytime for now.
      //       Might need to think of a better solution in the future.
      const block = await l1RpcProvider.getBlock(receipt.blockNumber)
      if (!block) {
        throw new Error(`Block ${receipt.blockNumber} not found`)
      }

      // Even we got the hash of the blob tx, we still need to traverse through the blocks
      // since we need to count the blob index in the block
      let blobIndex = 0
      for (const tx of block.prefetchedTransactions) {
        if (!tx) {
          console.log(`Skipping empty transaction in block: ${block.number}`)
          continue
        }

        // only process the blob tx hash recorded in the commitment
        if (blobTxHash.toLowerCase() === tx.hash.toLowerCase()) {
          console.log(`Processing transaction: ${tx.hash}`)
          const sender = tx.from
          if (!fetchConf.batchSenders.includes(sender.toLowerCase())) {
            console.warn(
              `Invalid sender (${sender}) for transaction: ${tx.hash}`
            )
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
              console.warn(
                `Transaction ${tx.hash} is a batch but has no blob hashes`
              )
              continue
            }

            // get blob hashes and indices
            const hashes = tx.blobVersionedHashes.map((hash, index) => ({
              index: blobIndex + index,
              hash,
            }))
            blobIndex += hashes.length

            console.log(`Fetching blobs for transaction: ${tx.hash}`)

            // fetch blob data from beacon chain
            const blobs = await l1BeaconProvider.getBlobs(
              block.timestamp,
              hashes.map((h) => h.index)
            )

            for (const blob of blobs) {
              datas.push(blob.data)
            }
          }

          console.log(`Parsing frames for transaction: ${tx.hash}`)
          let frames: Frame[] = []
          for (const data of datas) {
            try {
              // parse the frames from the blob data
              const parsedFrames = parseFrames(data, block.number)
              frames = frames.concat(parsedFrames)
            } catch (err) {
              console.error(
                `Failed to parse frames for transaction ${tx.hash}:`,
                err
              )
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
    } catch (err) {
      console.error(`Something goes wrong here when fetching batches:`, err)
    }
  }

  const channelMap: { [channelId: string]: Channel } = {}

  // process downloaded tx metadata
  for (const txMetadata of txsMetadata) {
    console.log(`Processing tx metadata of ${txMetadata.tx.hash}`)
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
        console.log(`Creating new channel for ID: ${channelId}`)
        channelMap[channelId] = new Channel(channelId, frame.inclusionBlock)
      }

      try {
        channelMap[channelId].addFrame(frame)
      } catch (err) {
        console.error(`Failed to add frame to channel ${channelId}:`, err)
      }
    }

    for (const channelId in channelMap) {
      if (!channelMap.hasOwnProperty(channelId)) {
        // ignore object prototype properties
        continue
      }

      console.log(`Processing channel: ${channelId}`)
      const channel = channelMap[channelId]

      if (!channel.isReady()) {
        console.warn(`Channel ${channelId} is not ready.`)
        continue
      }

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
              txMetadata.blockTime,
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
        console.error(`Error reading batches for channel ${channelId}:`, err)
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
