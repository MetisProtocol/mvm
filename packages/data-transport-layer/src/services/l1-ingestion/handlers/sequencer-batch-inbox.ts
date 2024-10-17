/* Imports: External */
import { Block, ethers, toBigInt, toNumber, TransactionResponse } from 'ethers'
import {
  fromHexString,
  L2Transaction,
  MinioClient,
  MinioConfig,
  QueueOrigin,
  remove0x,
  toHexString,
  zlibDecompress,
} from '@metis.io/core-utils'

/* Imports: Internal */
import {
  BlockEntry,
  DecodedSequencerBatchTransaction,
  EventHandlerSetAny,
  SequencerBatchAppendedExtraData,
  SequencerBatchInboxParsedEvent,
  TransactionBatchEntry,
  TransactionEntry,
} from '../../../types'
import { parseSignatureVParam, SEQUENCER_GAS_LIMIT } from '../../../utils'
import { MissingElementError } from './errors'
import { fetchBatches } from '../../../da/blob'
import { batchReader, Channel, RawSpanBatch } from '../../../da/blob/channel'

const l2ToL1ChainId = {
  1088: 1, // for metis andromeda
  59902: 11155111, // for metis sepolia
}

export const handleEventsSequencerBatchInbox: EventHandlerSetAny<
  SequencerBatchAppendedExtraData,
  SequencerBatchInboxParsedEvent
> = {
  getExtraData: async (event, l1RpcProvider) => {
    const l1Transaction = event.transaction as TransactionResponse
    const eventBlock = event.block as Block

    const batchSubmissionData: any = {}
    let batchSubmissionVerified = false

    // [1: DA type] [1: compress type] [32: batch index] [32: L2 start] [4: total blocks, max 65535] [<DATA> { [3: txs count] [5 block timestamp = l1 timestamp of txs] [32 l1BlockNumber of txs, get it from tx0] [1: TX type 0-sequencer 1-enqueue] [3 tx data length] [raw tx data] [3 sign length *sequencerTx*] [sign data] [20 l1Origin *enqueue*] [32 queueIndex *enqueue*].. } ...]
    const calldata = fromHexString(l1Transaction.data)
    if (calldata.length > 70) {
      const offset = 2
      // l2 block number - 1, in order to keep same as CTC
      batchSubmissionData.prevTotalElements =
        toBigInt(calldata.subarray(offset + 32, offset + 64)) - toBigInt(1)
      batchSubmissionData.batchIndex = toBigInt(
        calldata.subarray(offset, offset + 32)
      )
      batchSubmissionData.batchSize = toBigInt(
        calldata.subarray(offset + 64, offset + 68)
      )
      batchSubmissionVerified = true
    }

    if (!batchSubmissionVerified) {
      throw new Error(
        `Well, this really shouldn't happen. A SequencerBatchInbox data doesn't have a corresponding TransactionBatchAppended event.`
      )
    }

    return {
      timestamp: eventBlock.timestamp,
      blockNumber: eventBlock.number,
      submitter: l1Transaction.from,
      l1TransactionHash: l1Transaction.hash,
      l1TransactionData: l1Transaction.data,
      gasLimit: `${SEQUENCER_GAS_LIMIT}`,

      prevTotalElements: batchSubmissionData.prevTotalElements,
      batchIndex: batchSubmissionData.batchIndex,
      batchSize: batchSubmissionData.batchSize,
      batchRoot: eventBlock.parentHash,
      batchExtraData: '',

      blobIndex: event.blobIndex,
      blobCount: l1Transaction.blobVersionedHashes.length,
    }
  },
  parseEvent: async (event, extraData, l2ChainId, options) => {
    const blockEntries: BlockEntry[] = []

    // [1: DA type] [1: compress type] [32: batch index] [32: L2 start] [4: total blocks, max 65535] [<DATA> { [3: txs count] [5 block timestamp = l1 timestamp of txs] [32 l1BlockNumber of txs, get it from tx0] [1: TX type 0-sequencer 1-enqueue] [3 tx data length] [raw tx data] [3 sign length *sequencerTx*] [sign data] [20 l1Origin *enqueue*] [32 queueIndex *enqueue*].. } ...]
    const calldata = fromHexString(extraData.l1TransactionData)
    if (calldata.length < 70) {
      throw new Error(
        `Block ${extraData.blockNumber} transaction data of inbox ${extraData.l1TransactionHash} is invalid for decoding: ${extraData.l1TransactionData} , ` +
          `converted buffer length is < 70.`
      )
    }
    // DA: 0 - L1, 1 - memo, 2 - celestia, 3 - blob
    // current DA is 0
    // Compress Type: 0 - none, 11 - zlib
    const da = toNumber(calldata.subarray(0, 1))
    const compressType = toNumber(calldata.subarray(1, 2))
    let contextData = calldata.subarray(70)
    let channels: Channel[] = []
    // da first
    if (da === 1) {
      const storageObject = remove0x(toHexString(contextData))
      let minioClient: MinioClient = null
      if (
        options.minioBucket &&
        options.minioAccessKey &&
        options.minioSecretKey &&
        options.minioEndpoint &&
        options.minioPort
      ) {
        const minioConfig: MinioConfig = {
          options: {
            endPoint: options.minioEndpoint,
            port: options.minioPort,
            useSSL: options.minioUseSsl,
            accessKey: options.minioAccessKey,
            secretKey: options.minioSecretKey,
          },
          l2ChainId,
          bucket: options.minioBucket,
        }
        minioClient = new MinioClient(minioConfig)
      } else {
        throw new Error(`Missing minio config for DA type is 1`)
      }
      const daData = await minioClient.readObject(storageObject, 2)
      if (!daData) {
        throw new Error(
          `Read data from minio failed, object is ${storageObject}`
        )
      }
      contextData = Buffer.from(daData, 'hex')
    } else if (da === 3) {
      if (contextData.length % 32 !== 0) {
        throw new Error(
          `Blob tx hashes length is not multiple of 32, data: ${contextData}`
        )
      }

      // fetch blobs from cl
      // default to local devnet
      const l1ChainId = l2ToL1ChainId[options.l2ChainId] || 108800

      const blobTxHashes = []
      for (let i = 0; i < contextData.length; i += 32) {
        blobTxHashes.push(ethers.hexlify(contextData.subarray(i, i + 32)))
      }

      channels = channels.concat(
        await fetchBatches({
          blobTxHashes,
          chainId: l1ChainId,
          batchInbox: options.batchInboxAddress,
          batchSenders: [options.batchInboxSender],
          concurrentRequests: 0,
          l1Rpc: options.l1PrcProvider,
          l1Beacon: options.l1BeaconProvider,
          l2ChainId: options.l2ChainId,
        })
      )
    }
    if (compressType === 11) {
      contextData = await zlibDecompress(contextData)
    }

    // when using blob data, the context data is not in the old Metis format,
    // it is chunked by optimism frames, so we need to parse it differently
    if (da !== 3) {
      let offset = 0
      let blockIndex = 0
      const l2Start = toNumber(calldata.slice(2 + 32, 2 + 64))
      let pointerEnd = false
      while (!pointerEnd) {
        const txCount = toNumber(contextData.subarray(offset, offset + 3))
        offset += 3
        const blockTimestamp = toNumber(
          contextData.subarray(offset, offset + 5)
        )
        offset += 5
        const l1BlockNumber = toNumber(
          contextData.subarray(offset, offset + 32)
        )
        offset += 32

        const blockEntry: BlockEntry = {
          index: l2Start + blockIndex - 1, // keep same rule as single tx index
          batchIndex: Number(extraData.batchIndex),
          timestamp: blockTimestamp,
          transactions: [],
          confirmed: true,
        }
        blockIndex++

        for (let i = 0; i < txCount; i++) {
          const txType = toNumber(contextData.subarray(offset, offset + 1))
          offset += 1
          const txDataLen = toNumber(contextData.subarray(offset, offset + 3))
          offset += 3

          const transactionEntry: TransactionEntry = {
            index: blockEntry.index,
            batchIndex: Number(extraData.batchIndex),
            blockNumber: l1BlockNumber,
            timestamp: blockTimestamp,
            gasLimit: Number(0).toString(),
            target: ethers.ZeroAddress,
            origin: null,
            data: '0x',
            queueOrigin: 'sequencer',
            value: '0x0',
            queueIndex: null,
            decoded: null,
            confirmed: true,
            seqSign: null,
          }
          let signData = null
          if (txType === 0) {
            const txData: Buffer = contextData.subarray(
              offset,
              offset + txDataLen
            )
            offset += txDataLen
            const decoded = decodeSequencerBatchTransaction(txData, l2ChainId)
            transactionEntry.data = toHexString(txData)
            transactionEntry.value = decoded.value
            transactionEntry.decoded = decoded
            const signLen = toNumber(contextData.subarray(offset, offset + 3))
            offset += 3
            if (signLen > 0) {
              const decodedSign = remove0x(
                toHexString(contextData.subarray(offset, offset + signLen))
              )
              offset += signLen

              // sign length is 64 * 2 + 2, or '000000'
              if (decodedSign && decodedSign === '000000') {
                // transactionEntries[i].seqSign = '0x0,0x0,0x0'
                signData = '0x0,0x0,0x0'
              } else if (!decodedSign || decodedSign.length < 130) {
                // transactionEntries[i].seqSign = ''
                signData = ''
              } else {
                const seqR =
                  '0x' + removeLeadingZeros(decodedSign.substring(0, 64))
                const seqS =
                  '0x' + removeLeadingZeros(decodedSign.substring(64, 128))
                let seqV = decodedSign.substring(128)
                if (seqV.length > 0) {
                  seqV = '0x' + removeLeadingZeros(seqV)
                } else {
                  seqV = '0x0'
                }
                // transactionEntries[i].seqSign = `${seqR},${seqS},${seqV}`
                signData = `${seqR},${seqS},${seqV}`
              }

              transactionEntry.seqSign = signData
            }
          } else {
            const l1Origin = toHexString(
              contextData.subarray(offset, offset + 20)
            )
            offset += 20
            const queueIndex = toHexString(
              contextData.subarray(offset, offset + 16)
            )
            offset += 16
            transactionEntry.origin = l1Origin
            transactionEntry.queueIndex = toNumber(queueIndex)
            transactionEntry.queueOrigin = 'l1'
          }
          blockEntry.transactions.push(transactionEntry)
          blockEntries.push(blockEntry)
        }

        if (offset >= contextData.length) {
          pointerEnd = true
        }
      }

      const transactionBatchEntry: TransactionBatchEntry = {
        index: Number(extraData.batchIndex),
        root: extraData.batchRoot,
        size: Number(extraData.batchSize),
        prevTotalElements: Number(extraData.prevTotalElements),
        extraData: extraData.batchExtraData,
        blockNumber: extraData.blockNumber,
        timestamp: extraData.timestamp,
        submitter: extraData.submitter,
        l1TransactionHash: extraData.l1TransactionHash,
      }

      return {
        transactionBatchEntry,
        blockEntries,
      }
    } else {
      // TODO: async parse the channels
      for (const channel of channels) {
        const readBatch = await batchReader(channel.reader())
        const batchData = await readBatch()
        // since currently we can only handle span batch,
        // so we can just skip the singular batch
        const rawSpanBatch = batchData.inner as RawSpanBatch
        const spanBatch = await rawSpanBatch.derive(toBigInt(options.l2ChainId))

        for (let i = 0; i < spanBatch.batches.length; i++) {
          const batch = spanBatch.batches[i]
          const l2BlockNumber = spanBatch.l2StartBlock + i
          blockEntries.push({
            index: l2BlockNumber,
            batchIndex: Number(extraData.batchIndex),
            timestamp: batch.timestamp,
            transactions: batch.transactions.map((tx: L2Transaction) => {
              // decode raw tx
              return {
                index: l2BlockNumber,
                batchIndex: Number(extraData.batchIndex),
                blockNumber: ethers.toNumber(batch.epochNum),
                timestamp: batch.timestamp,
                gasLimit: tx.gasLimit.toString(10),
                target: ethers.ZeroAddress,
                origin: tx.queueOrigin,
                data: tx.data,
                queueOrigin:
                  tx.queueOrigin === QueueOrigin.Sequencer ? 'sequencer' : 'l1',
                value: tx.value.toString(10),
                queueIndex: tx.nonce,
                decoded: decodeSequencerBatchTransaction(
                  Buffer.from(remove0x(tx.rawTransaction), 'hex'),
                  l2ChainId
                ),
                confirmed: true,
                seqSign:
                  tx.queueOrigin === QueueOrigin.Sequencer
                    ? `0x${tx.seqR},0x${tx.seqS},0x${tx.seqV}`
                    : '0x0,0x0,0x0',
              }
            }),
            confirmed: true,
          })
        }
      }
    }
  },
  storeEvent: async (entry, db, options) => {
    // Defend against situations where we missed an event because the RPC provider
    // (infura/alchemy/whatever) is missing an event.
    if (entry.transactionBatchEntry.index > 0) {
      const prevTransactionBatchEntry = await db.getTransactionBatchByIndex(
        entry.transactionBatchEntry.index - 1
      )

      // We should *always* have a previous transaction batch here.
      if (prevTransactionBatchEntry === null) {
        throw new MissingElementError('SequencerBatchInbox')
      }
    }
    // await db.putBlockEntries(entry.blockEntries)

    // Add an additional field to the enqueued transactions in the database
    // if they have already been confirmed
    for (const block of entry.blockEntries) {
      if (options.deSeqBlock > 0 && block.index + 1 >= options.deSeqBlock) {
        await db.putBlockEntries([block])
      } else {
        await db.putTransactionEntries(block.transactions)
      }
      for (const transactionEntry of block.transactions) {
        if (transactionEntry.queueOrigin === 'l1') {
          await db.putTransactionIndexByQueueIndex(
            transactionEntry.queueIndex,
            transactionEntry.index
          )
        }
      }
    }

    await db.setL2BlockToL1BlockMapping(
      entry.transactionBatchEntry.blockNumber,
      options.l2ChainId,
      entry.blockEntries.map((block) => block.index)
    )

    await db.putTransactionBatchEntries([entry.transactionBatchEntry])

    // save the mapping of L1 block number to L2 block number
    await db.setL1BlockToL2BlockMapping(
      entry.transactionBatchEntry.blockNumber,
      options.l2ChainId,
      entry.transactionBatchEntry.prevTotalElements +
        entry.transactionBatchEntry.size
    )
  },
}

interface SequencerBatchContext {
  numSequencedTransactions: number
  numSubsequentQueueTransactions: number
  timestamp: number
  blockNumber: number
}

const parseSequencerBatchContext = (
  calldata: Buffer,
  offset: number
): SequencerBatchContext => {
  return {
    numSequencedTransactions: toNumber(calldata.slice(offset, offset + 3)),
    numSubsequentQueueTransactions: toNumber(
      calldata.subarray(offset + 3, offset + 6)
    ),
    timestamp: toNumber(calldata.subarray(offset + 6, offset + 11)),
    blockNumber: toNumber(calldata.subarray(offset + 11, offset + 16)),
  }
}

const decodeSequencerBatchTransaction = (
  transaction: Buffer,
  l2ChainId: number
): DecodedSequencerBatchTransaction => {
  const decodedTx = ethers.Transaction.from(`0x${transaction.toString('hex')}`)

  return {
    nonce: decodedTx.nonce.toString(),
    gasPrice: decodedTx.gasPrice.toString(),
    gasLimit: decodedTx.gasLimit.toString(),
    value: decodedTx.value.toString(),
    target: decodedTx.to ? toHexString(decodedTx.to) : null,
    data: toHexString(decodedTx.data),
    sig: {
      v: parseSignatureVParam(decodedTx.signature.v, l2ChainId),
      r: toHexString(decodedTx.signature.r),
      s: toHexString(decodedTx.signature.s),
    },
  }
}

const removeLeadingZeros = (inputString: string): string => {
  const trimmedString = inputString.replace(/^0+/, '')
  return trimmedString || '0'
}
