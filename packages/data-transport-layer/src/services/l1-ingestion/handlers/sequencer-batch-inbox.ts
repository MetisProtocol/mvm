/* Imports: External */
import { BigNumber, ethers, constants } from 'ethers'
import {
  BlockWithTransactions,
  TransactionResponse,
} from '@ethersproject/abstract-provider'
import { MerkleTree } from 'merkletreejs'
import { getContractFactory } from '@metis.io/contracts'
import {
  fromHexString,
  toHexString,
  toRpcHexString,
  EventArgsSequencerBatchAppended,
  MinioClient,
  MinioConfig,
  remove0x,
  zlibDecompress,
} from '@metis.io/core-utils'

/* Imports: Internal */
import {
  DecodedSequencerBatchTransaction,
  SequencerBatchAppendedExtraData,
  SequencerBatchInboxParsedEvent,
  TransactionBatchEntry,
  TransactionEntry,
  EventHandlerSetAny,
  BlockEntry,
} from '../../../types'
import { SEQUENCER_GAS_LIMIT, parseSignatureVParam } from '../../../utils'
import { MissingElementError } from './errors'

export const handleEventsSequencerBatchInbox: EventHandlerSetAny<
  SequencerBatchAppendedExtraData,
  SequencerBatchInboxParsedEvent
> = {
  getExtraData: async (event, l1RpcProvider) => {
    const l1Transaction = event.transaction as TransactionResponse
    const eventBlock = event.block as BlockWithTransactions

    const batchSubmissionData: any = {}
    let batchSubmissionVerified = false

    // [1: DA type] [1: compress type] [32: batch index] [32: L2 start] [4: total blocks, max 65535] [<DATA> { [3: txs count] [5 block timestamp = l1 timestamp of txs] [32 l1BlockNumber of txs, get it from tx0] [1: TX type 0-sequencer 1-enqueue] [3 tx data length] [raw tx data] [3 sign length *sequencerTx*] [sign data] [20 l1Origin *enqueue*] [32 queueIndex *enqueue*].. } ...]
    const calldata = fromHexString(l1Transaction.data)
    if (calldata.length > 70) {
      const offset = 2
      // l2 block number - 1, in order to keep same as CTC
      batchSubmissionData.prevTotalElements = BigNumber.from(
        calldata.slice(offset + 32, offset + 64)
      ).sub(1)
      batchSubmissionData.batchIndex = BigNumber.from(
        calldata.slice(offset, offset + 32)
      )
      batchSubmissionData.batchSize = BigNumber.from(
        calldata.slice(offset + 64, offset + 68)
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
    // DA: 0 - L1, 1 - memo, 2 - celestia
    // current DA is 0
    // Compress Type: 0 - none, 11 - zlib
    const da = BigNumber.from(calldata.slice(0, 1)).toNumber()
    const compressType = BigNumber.from(calldata.slice(1, 2)).toNumber()
    let contextData = calldata.slice(70)
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
    }
    if (compressType === 11) {
      contextData = await zlibDecompress(contextData)
    }
    let offset = 0
    let blockIndex = 0
    const l2Start = BigNumber.from(calldata.slice(2 + 32, 2 + 64)).toNumber()
    let pointerEnd = false
    while (!pointerEnd) {
      const txCount = BigNumber.from(
        contextData.slice(offset, offset + 3)
      ).toNumber()
      offset += 3
      const blockTimestamp = BigNumber.from(
        contextData.slice(offset, offset + 5)
      ).toNumber()
      offset += 5
      const l1BlockNumber = BigNumber.from(
        contextData.slice(offset, offset + 32)
      ).toNumber()
      offset += 32

      const blockEntry: BlockEntry = {
        index: l2Start + blockIndex - 1, // keep same rule as single tx index
        batchIndex: extraData.batchIndex.toNumber(),
        timestamp: blockTimestamp,
        transactions: [],
        confirmed: true,
      }
      blockIndex++

      for (let i = 0; i < txCount; i++) {
        const txType = BigNumber.from(
          contextData.slice(offset, offset + 1)
        ).toNumber()
        offset += 1
        const txDataLen = BigNumber.from(
          contextData.slice(offset, offset + 3)
        ).toNumber()
        offset += 3

        const transactionEntry: TransactionEntry = {
          index: blockEntry.index,
          batchIndex: extraData.batchIndex.toNumber(),
          blockNumber: l1BlockNumber,
          timestamp: blockTimestamp,
          gasLimit: BigNumber.from(0).toString(),
          target: constants.AddressZero,
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
          const txData: Buffer = contextData.slice(offset, offset + txDataLen)
          offset += txDataLen
          const decoded = decodeSequencerBatchTransaction(txData, l2ChainId)
          transactionEntry.data = toHexString(txData)
          transactionEntry.value = decoded.value
          transactionEntry.decoded = decoded
          const signLen = BigNumber.from(
            contextData.slice(offset, offset + 3)
          ).toNumber()
          offset += 3
          if (signLen > 0) {
            const decodedSign = remove0x(
              toHexString(contextData.slice(offset, offset + signLen))
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
          const l1Origin = toHexString(contextData.slice(offset, offset + 20))
          offset += 20
          const queueIndex = toHexString(contextData.slice(offset, offset + 16))
          offset += 16
          transactionEntry.origin = l1Origin
          transactionEntry.queueIndex = BigNumber.from(queueIndex).toNumber()
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
      index: extraData.batchIndex.toNumber(),
      root: extraData.batchRoot,
      size: extraData.batchSize.toNumber(),
      prevTotalElements: extraData.prevTotalElements.toNumber(),
      extraData: extraData.batchExtraData,
      blockNumber: BigNumber.from(extraData.blockNumber).toNumber(),
      timestamp: BigNumber.from(extraData.timestamp).toNumber(),
      submitter: extraData.submitter,
      l1TransactionHash: extraData.l1TransactionHash,
    }

    return {
      transactionBatchEntry,
      blockEntries,
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
    numSequencedTransactions: BigNumber.from(
      calldata.slice(offset, offset + 3)
    ).toNumber(),
    numSubsequentQueueTransactions: BigNumber.from(
      calldata.slice(offset + 3, offset + 6)
    ).toNumber(),
    timestamp: BigNumber.from(
      calldata.slice(offset + 6, offset + 11)
    ).toNumber(),
    blockNumber: BigNumber.from(
      calldata.slice(offset + 11, offset + 16)
    ).toNumber(),
  }
}

const decodeSequencerBatchTransaction = (
  transaction: Buffer,
  l2ChainId: number
): DecodedSequencerBatchTransaction => {
  const decodedTx = ethers.utils.parseTransaction(transaction)

  return {
    nonce: BigNumber.from(decodedTx.nonce).toString(),
    gasPrice: BigNumber.from(decodedTx.gasPrice).toString(),
    gasLimit: BigNumber.from(decodedTx.gasLimit).toString(),
    value: toRpcHexString(decodedTx.value),
    target: decodedTx.to ? toHexString(decodedTx.to) : null,
    data: toHexString(decodedTx.data),
    sig: {
      v: parseSignatureVParam(decodedTx.v, l2ChainId),
      r: toHexString(decodedTx.r),
      s: toHexString(decodedTx.s),
    },
  }
}

const removeLeadingZeros = (inputString: string): string => {
  const trimmedString = inputString.replace(/^0+/, '')
  return trimmedString || '0'
}
