/* Imports: External */
import { Contract, ethers, EventLog, toBigInt, toNumber } from 'ethersv6'
import { MerkleTree } from 'merkletreejs'
import { getContractDefinition } from '@metis.io/contracts'
import {
  fromHexString,
  toHexString,
  toRpcHexString,
  EventArgsSequencerBatchAppended,
  MinioClient,
  MinioConfig,
  remove0x,
} from '@localtest911/core-utils'

/* Imports: Internal */
import {
  DecodedSequencerBatchTransaction,
  SequencerBatchAppendedExtraData,
  SequencerBatchAppendedParsedEvent,
  TransactionBatchEntry,
  TransactionEntry,
  EventHandlerSet,
} from '../../../types'
import { SEQUENCER_GAS_LIMIT, parseSignatureVParam } from '../../../utils'
import { MissingElementError } from './errors'

export const handleEventsSequencerBatchAppended: EventHandlerSet<
  EventArgsSequencerBatchAppended,
  SequencerBatchAppendedExtraData,
  SequencerBatchAppendedParsedEvent
> = {
  getExtraData: async (event, l1RpcProvider) => {
    const eventBlock = await l1RpcProvider.getBlock(event.blockNumber, true)
    const l1Transaction = eventBlock.prefetchedTransactions.find(
      (i) => i.hash === event.transactionHash
    )

    // TODO: We need to update our events so that we actually have enough information to parse this
    // batch without having to pull out this extra event. For the meantime, we need to find this
    // "TransactonBatchAppended" event to get the rest of the data.
    const CanonicalTransactionChain = new Contract(
      event.address,
      getContractDefinition('CanonicalTransactionChain').abi,
      l1RpcProvider
    )

    const batchSubmissionEvent = (
      await CanonicalTransactionChain.queryFilter(
        CanonicalTransactionChain.filters.TransactionBatchAppended(),
        eventBlock.number,
        eventBlock.number
      )
    ).find((foundEvent: EventLog) => {
      // We might have more than one event in this block, so we specifically want to find a
      // "TransactonBatchAppended" event emitted immediately before the event in question.
      return (
        foundEvent.transactionHash === event.transactionHash &&
        foundEvent.index === event.index - 1
      )
    })

    if (!batchSubmissionEvent) {
      throw new Error(
        `Well, this really shouldn't happen. A SequencerBatchAppended event doesn't have a corresponding TransactionBatchAppended event.`
      )
    }

    return {
      timestamp: eventBlock.timestamp,
      blockNumber: eventBlock.number,
      submitter: l1Transaction.from,
      l1TransactionHash: l1Transaction.hash,
      l1TransactionData: l1Transaction.data,
      gasLimit: `${SEQUENCER_GAS_LIMIT}`,

      prevTotalElements: event.args._prevTotalElements,
      batchIndex: event.args._batchIndex,
      batchSize: event.args._batchSize,
      batchRoot: event.args._batchRoot,
      batchExtraData: event.args._extraData,

      // blob related, not used in old sequencer batch
      blobIndex: 0,
      blobCount: 0,
    }
  },
  parseEvent: async (event, extraData, l2ChainId, options) => {
    const transactionEntries: TransactionEntry[] = []

    // It's easier to deal with this data if it's a Buffer.
    let calldata = fromHexString(extraData.l1TransactionData)
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
    }

    // chainid + 32, so not [12, 15]
    if (calldata.length < 44) {
      throw new Error(
        `Block ${extraData.blockNumber} transaction data is invalid for decoding: ${extraData.l1TransactionData} , ` +
          `converted buffer length is < 44.`
      )
    }
    const numContexts = toNumber(calldata.slice(44, 47))
    let transactionIndex = 0
    let enqueuedCount = 0
    let nextTxPointer = 47 + 16 * numContexts
    const leafs = []
    let rootFromCalldata = ''
    let fromStorage = false
    const sequencerTxIndex = []
    for (let i = 0; i < numContexts; i++) {
      const contextPointer = 47 + 16 * i
      const context = parseSequencerBatchContext(calldata, contextPointer)

      if (
        i === 0 &&
        context.blockNumber === 0 &&
        context.timestamp === 0 &&
        context.numSubsequentQueueTransactions === 0 &&
        context.numSequencedTransactions === 0
      ) {
        // calldata = timestamp[13] + zero[1]{0} + sizeOfTxData[8]{00000000} + markleRoot[64]
        const storageObject = calldata
          .slice(nextTxPointer)
          .toString('hex')
          .slice(0, 86)
        rootFromCalldata = storageObject.slice(22, 86)
        fromStorage = true
        // console.info('calc storage object name', storageObject)
        const txData = await minioClient.readObject(storageObject, 2)
        // const verified = await minioClient.verifyObject(storageObject, txData, 2)
        // if (!verified) {
        //   throw new Error(`verified calldata from storage error, storage object ${storageObject}`)
        // }
        if (!txData) {
          throw new Error(
            `got calldata from storage error, storage object ${storageObject}`
          )
        }
        console.info('got storage data', storageObject)
        calldata = Buffer.concat([
          calldata.slice(0, nextTxPointer),
          Buffer.from(txData, 'hex'),
        ])
      }

      for (let j = 0; j < context.numSequencedTransactions; j++) {
        const sequencerTransaction = parseSequencerBatchTransaction(
          calldata,
          nextTxPointer
        )

        const decoded = decodeSequencerBatchTransaction(
          sequencerTransaction,
          event.args._chainId
        )

        transactionEntries.push({
          index: toNumber(
            extraData.prevTotalElements + toBigInt(transactionIndex)
          ),
          batchIndex: toNumber(extraData.batchIndex),
          blockNumber: toNumber(context.blockNumber),
          timestamp: toNumber(context.timestamp),
          gasLimit: toNumber(0).toString(),
          target: ethers.ZeroAddress,
          origin: null,
          data: toHexString(sequencerTransaction),
          queueOrigin: 'sequencer',
          value: decoded.value,
          queueIndex: null,
          decoded,
          confirmed: true,
          seqSign: null,
        })
        // block number = index + 1
        leafs.push(
          ethers.keccak256(
            new ethers.AbiCoder().encode(
              ['uint256', 'bytes'],
              [
                toNumber(
                  extraData.prevTotalElements +
                    toBigInt(transactionIndex) +
                    toBigInt(1)
                ),
                parseMerkleLeafFromSequencerBatchTransaction(
                  calldata,
                  nextTxPointer
                ),
              ]
            )
          )
        )
        // push to update sequencer sign
        sequencerTxIndex.push(transactionIndex)
        nextTxPointer += 3 + sequencerTransaction.length
        transactionIndex++
      }

      for (let j = 0; j < context.numSubsequentQueueTransactions; j++) {
        const queueIndex =
          event.args._startingQueueIndex + toBigInt(enqueuedCount)

        // Okay, so. Since events are processed in parallel, we don't know if the Enqueue
        // event associated with this queue element has already been processed. So we'll ask
        // the api to fetch that data for itself later on and we use fake values for some
        // fields. The real TODO here is to make sure we fix this data structure to avoid ugly
        // "dummy" fields. EXCEPT timestamp, which is set to local time when enqueued. this timestamp
        // was submitted to CTC as part of the context. use the timestamp in the context otherwise
        // the batch timestamp will be inconsistent with the main node.
        transactionEntries.push({
          index: toNumber(
            extraData.prevTotalElements + toBigInt(transactionIndex)
          ),
          batchIndex: toNumber(extraData.batchIndex),
          blockNumber: 0,
          timestamp:
            toNumber(
              extraData.prevTotalElements + toBigInt(transactionIndex)
            ) <= 2287472
              ? 0
              : toNumber(context.timestamp), //timestamp needs to be consistent
          gasLimit: toBigInt(0).toString(),
          target: ethers.ZeroAddress,
          origin: ethers.ZeroAddress,
          data: '0x',
          queueOrigin: 'l1',
          value: '0x0',
          queueIndex: toNumber(queueIndex),
          decoded: null,
          confirmed: true,
          seqSign: null, //enqueue always set to null
        })

        enqueuedCount++
        transactionIndex++
      }
    }

    // restore sequencer sign
    // in update period, perhaps some start tx has no sequencer sign (only one batch like this)
    if (nextTxPointer < calldata.length) {
      const cachedSignList = []
      // eslint-disable-next-line @typescript-eslint/prefer-for-of
      for (let j = 0; j < sequencerTxIndex.length; j++) {
        const sequencerSign = parseSequencerBatchTransaction(
          calldata,
          nextTxPointer
        )
        const decodedSign = remove0x(toHexString(sequencerSign))
        // sign length is 64 * 2 + 2, or '000000'
        if (decodedSign && decodedSign === '000000') {
          // transactionEntries[j].seqSign = '0x0,0x0,0x0'
          cachedSignList.push('0x0,0x0,0x0')
        } else if (!decodedSign || decodedSign.length < 130) {
          // transactionEntries[j].seqSign = ''
          cachedSignList.push('')
        } else {
          const seqR = '0x' + removeLeadingZeros(decodedSign.substring(0, 64))
          const seqS = '0x' + removeLeadingZeros(decodedSign.substring(64, 128))
          let seqV = decodedSign.substring(128)
          if (seqV.length > 0) {
            seqV = '0x' + removeLeadingZeros(seqV)
          } else {
            seqV = '0x0'
          }
          // transactionEntries[j].seqSign = `${seqR},${seqS},${seqV}`
          cachedSignList.push(`${seqR},${seqS},${seqV}`)
        }
        nextTxPointer += 3 + sequencerSign.length
        if (nextTxPointer >= calldata.length) {
          break
        }
      }
      const startIndex = sequencerTxIndex.length - cachedSignList.length
      // fill empty seqSign first
      for (let j = 0; j < startIndex; j++) {
        transactionEntries[sequencerTxIndex[j]].seqSign = ''
      }
      for (let j = startIndex; j < sequencerTxIndex.length; j++) {
        transactionEntries[sequencerTxIndex[j]].seqSign =
          cachedSignList[j - startIndex]
      }
    }

    const hash = (el: Buffer | string): Buffer => {
      return Buffer.from(ethers.keccak256(el).slice(2), 'hex')
    }
    const tree = new MerkleTree(leafs, hash)
    let merkleRoot = tree.getHexRoot()
    if (merkleRoot.startsWith('0x')) {
      merkleRoot = merkleRoot.slice(2)
    }
    console.info(
      `root from batch: ${rootFromCalldata}, re-calculate root: ${merkleRoot}, equals: ${
        rootFromCalldata === merkleRoot
      }`
    )
    if (fromStorage && rootFromCalldata !== merkleRoot) {
      throw new Error(
        `verified calldata from storage error, batch index is ${extraData.batchIndex}`
      )
    }

    const transactionBatchEntry: TransactionBatchEntry = {
      index: toNumber(extraData.batchIndex),
      root: extraData.batchRoot,
      size: toNumber(extraData.batchSize),
      prevTotalElements: toNumber(extraData.prevTotalElements),
      extraData: extraData.batchExtraData,
      blockNumber: toNumber(extraData.blockNumber),
      timestamp: toNumber(extraData.timestamp),
      submitter: extraData.submitter,
      l1TransactionHash: extraData.l1TransactionHash,
    }

    return {
      transactionBatchEntry,
      transactionEntries,
    }
  },
  storeEvent: async (entry, db) => {
    // Defend against situations where we missed an event because the RPC provider
    // (infura/alchemy/whatever) is missing an event.
    if (entry.transactionBatchEntry.index > 0) {
      const prevTransactionBatchEntry = await db.getTransactionBatchByIndex(
        entry.transactionBatchEntry.index - 1
      )

      // We should *always* have a previous transaction batch here.
      if (prevTransactionBatchEntry === null) {
        throw new MissingElementError('SequencerBatchAppended')
      }
    }

    await db.putTransactionEntries(entry.transactionEntries)

    // Add an additional field to the enqueued transactions in the database
    // if they have already been confirmed
    for (const transactionEntry of entry.transactionEntries) {
      if (transactionEntry.queueOrigin === 'l1') {
        await db.putTransactionIndexByQueueIndex(
          transactionEntry.queueIndex,
          transactionEntry.index
        )
      }
    }

    await db.putTransactionBatchEntries([entry.transactionBatchEntry])
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
      calldata.slice(offset + 3, offset + 6)
    ),
    timestamp: toNumber(calldata.slice(offset + 6, offset + 11)),
    blockNumber: toNumber(calldata.slice(offset + 11, offset + 16)),
  }
}

const parseMerkleLeafFromSequencerBatchTransaction = (
  calldata: Buffer,
  offset: number
): Buffer => {
  const transactionLength = toNumber(calldata.slice(offset, offset + 3))

  return calldata.slice(offset, offset + 3 + transactionLength)
}

const parseSequencerBatchTransaction = (
  calldata: Buffer,
  offset: number
): Buffer => {
  const transactionLength = toNumber(calldata.slice(offset, offset + 3))

  return calldata.slice(offset + 3, offset + 3 + transactionLength)
}

const decodeSequencerBatchTransaction = (
  transaction: Buffer,
  l2ChainId: number
): DecodedSequencerBatchTransaction => {
  const decodedTx = ethers.Transaction.from(`0x${transaction.toString('hex')}`)

  return {
    nonce: toBigInt(decodedTx.nonce).toString(),
    gasPrice: toBigInt(decodedTx.gasPrice).toString(),
    gasLimit: toBigInt(decodedTx.gasLimit).toString(),
    value: toRpcHexString(decodedTx.value),
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
