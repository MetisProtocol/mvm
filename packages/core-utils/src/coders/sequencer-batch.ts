import { add0x, encodeHex, remove0x } from '../common'
import { ethers, toBeHex, toBigInt } from 'ethersv6'
import { MinioClient } from './minio-client'
import { MerkleTree } from 'merkletreejs'

export interface BatchContext {
  numSequencedTransactions: number
  numSubsequentQueueTransactions: number
  timestamp: number
  blockNumber: number
}

export interface AppendSequencerBatchParams {
  shouldStartAtElement: number // 5 bytes -- starts at batch
  totalElementsToAppend: number // 3 bytes -- total_elements_to_append
  contexts: BatchContext[] // total_elements[fixed_size[]]
  transactions: string[] // total_size_bytes[],total_size_bytes[]
  blockNumbers: number[]
  seqSigns: string[] // de-sequencer block sign, length equals sequencerTx
}

export interface EncodeSequencerBatchOptions {
  useMinio?: boolean
  minioClient?: MinioClient
}

const APPEND_SEQUENCER_BATCH_METHOD_ID = 'appendSequencerBatch()'

export const encodeAppendSequencerBatch = async (
  b: AppendSequencerBatchParams,
  opts?: EncodeSequencerBatchOptions
): Promise<string> => {
  const encodeShouldStartAtElement = encodeHex(b.shouldStartAtElement, 10)
  const encodedTotalElementsToAppend = encodeHex(b.totalElementsToAppend, 6)
  const contexts = b.contexts.slice()

  // const encodedContextsHeader = encodeHex(b.contexts.length, 6)
  // const encodedContexts =
  //   encodedContextsHeader +
  //   b.contexts.reduce((acc, cur) => acc + encodeBatchContext(cur), '')

  let encodedTransactionData = b.transactions.reduce((acc, cur) => {
    if (cur.length % 2 !== 0) {
      throw new Error('Unexpected uneven hex string value!')
    }
    const encodedTxDataHeader = remove0x(
      toBeHex(toBigInt(remove0x(cur).length / 2), 3)
    )
    return acc + encodedTxDataHeader + remove0x(cur)
  }, '')
  // encode sequencer signs, append to encodedTransactionData
  if (b.seqSigns && b.seqSigns.length > 0) {
    const encodedSeqSignData = b.seqSigns.reduce((acc, cur) => {
      if (cur.length % 2 !== 0) {
        throw new Error('Unexpected uneven hex string value! cur:' + cur)
      }
      const encodedSignDataHeader = remove0x(
        toBeHex(toBigInt(remove0x(cur).length / 2), 3)
      )
      return acc + encodedSignDataHeader + remove0x(cur)
    }, '')
    encodedTransactionData += encodedSeqSignData
  }

  console.info(
    'input data',
    b.shouldStartAtElement,
    b.totalElementsToAppend,
    encodedTransactionData.length
  )

  if (opts?.useMinio && opts?.minioClient) {
    // generate merkle root
    const hash = (el: Buffer | string): Buffer => {
      return Buffer.from(remove0x(ethers.keccak256(el)), 'hex')
    }
    const fromHexString = (hexString) =>
      new Uint8Array(
        hexString.match(/.{1,2}/g).map((byte) => parseInt(byte, 16))
      )
    const leafs = []
    for (let i = 0; i < b.transactions.length; i++) {
      const _blockNumber = b.blockNumbers[i]
      const _cur = b.transactions[i]
      const _encodedTxDataHeader = remove0x(
        toBeHex(toBigInt(remove0x(_cur).length / 2), 3)
      )
      const _encodedTxData = _encodedTxDataHeader + remove0x(_cur)
      leafs.push(
        ethers.keccak256(
          ethers.solidityPacked(
            ['uint256', 'bytes'],
            [_blockNumber, fromHexString(_encodedTxData)]
          )
        )
      )
    }
    const tree = new MerkleTree(leafs, hash)
    const batchRoot = remove0x(tree.getHexRoot())

    const storagedObject = await opts?.minioClient?.writeObject(
      batchRoot,
      b.shouldStartAtElement,
      b.totalElementsToAppend,
      encodedTransactionData,
      3
    )
    console.info(
      'storage tx data to minio',
      storagedObject,
      'context length',
      contexts.length
    )

    // the following 2 conditions except empty encodedTransactionData
    if (
      !storagedObject &&
      encodedTransactionData &&
      b.shouldStartAtElement >= 0 &&
      b.totalElementsToAppend > 0
    ) {
      throw new Error('Storage encoded transaction data failed!')
    }

    if (storagedObject && contexts.length > 0) {
      encodedTransactionData = storagedObject
      contexts.unshift({
        numSequencedTransactions: 0,
        numSubsequentQueueTransactions: 0,
        timestamp: 0,
        blockNumber: 0,
      })
    }
  }

  const encodedContextsHeader = encodeHex(contexts.length, 6)
  const encodedContexts =
    encodedContextsHeader +
    contexts.reduce((acc, cur) => acc + encodeBatchContext(cur), '')

  console.info(
    'sequencer batch result',
    encodeShouldStartAtElement,
    encodedTotalElementsToAppend,
    encodedContexts,
    encodedTransactionData
  )

  return (
    encodeShouldStartAtElement +
    encodedTotalElementsToAppend +
    encodedContexts +
    encodedTransactionData
  )
}

const encodeBatchContext = (context: BatchContext): string => {
  return (
    encodeHex(context.numSequencedTransactions, 6) +
    encodeHex(context.numSubsequentQueueTransactions, 6) +
    encodeHex(context.timestamp, 10) +
    encodeHex(context.blockNumber, 10)
  )
}

export const decodeAppendSequencerBatch = async (
  b: string,
  opts?: EncodeSequencerBatchOptions
): Promise<AppendSequencerBatchParams> => {
  b = remove0x(b)

  const shouldStartAtElement = b.slice(0, 10)
  const totalElementsToAppend = b.slice(10, 16)
  const contextHeader = b.slice(16, 22)
  const contextCount = parseInt(contextHeader, 16)

  let offset = 22
  let contexts = []
  for (let i = 0; i < contextCount; i++) {
    const numSequencedTransactions = b.slice(offset, offset + 6)
    offset += 6
    const numSubsequentQueueTransactions = b.slice(offset, offset + 6)
    offset += 6
    const timestamp = b.slice(offset, offset + 10)
    offset += 10
    const blockNumber = b.slice(offset, offset + 10)
    offset += 10
    contexts.push({
      numSequencedTransactions: parseInt(numSequencedTransactions, 16),
      numSubsequentQueueTransactions: parseInt(
        numSubsequentQueueTransactions,
        16
      ),
      timestamp: parseInt(timestamp, 16),
      blockNumber: parseInt(blockNumber, 16),
    })
  }

  if (contexts.length > 0) {
    const context = contexts[0]
    if (context.blockNumber === 0) {
      switch (context.timestamp) {
        case 0: {
          const storageObject = b.slice(offset)
          const txData = await opts?.minioClient?.readObject(storageObject, 5)
          b = b.slice(0, offset) + txData
          break
        }
      }
      // remove the dummy context
      contexts = contexts.slice(1)
    }
  }

  const transactions = []
  const seqSigns = []
  for (const context of contexts) {
    for (let j = 0; j < context.numSequencedTransactions; j++) {
      const size = b.slice(offset, offset + 6)
      offset += 6
      const raw = b.slice(offset, offset + parseInt(size, 16) * 2)
      transactions.push(add0x(raw))
      offset += raw.length
    }
    // decode sequencer signs
    if (offset < b.length) {
      for (let j = 0; j < context.numSequencedTransactions; j++) {
        const size = b.slice(offset, offset + 6)
        offset += 6
        const signLen = parseInt(size, 16) * 2
        if (signLen === 0) {
          seqSigns.push('')
        } else {
          const raw = b.slice(offset, offset + signLen)
          seqSigns.push(raw)
          offset += raw.length
        }
      }
    }
  }

  return {
    shouldStartAtElement: parseInt(shouldStartAtElement, 16),
    totalElementsToAppend: parseInt(totalElementsToAppend, 16),
    contexts,
    transactions,
    blockNumbers: [],
    seqSigns,
  }
}

export const sequencerBatch = {
  encode: async (
    b: AppendSequencerBatchParams,
    opts?: EncodeSequencerBatchOptions
  ) => {
    const encodedParams = await encodeAppendSequencerBatch(b, opts)
    return (
      ethers.id(APPEND_SEQUENCER_BATCH_METHOD_ID).slice(0, 10) + encodedParams
    )
  },
  decode: async (
    b: string,
    opts?: EncodeSequencerBatchOptions
  ): Promise<AppendSequencerBatchParams> => {
    b = remove0x(b)
    const functionSelector = b.slice(0, 8)
    if (
      functionSelector !==
      ethers.id(APPEND_SEQUENCER_BATCH_METHOD_ID).slice(2, 10)
    ) {
      throw new Error('Incorrect function signature')
    }
    return decodeAppendSequencerBatch(b.slice(8), opts)
  },
}
