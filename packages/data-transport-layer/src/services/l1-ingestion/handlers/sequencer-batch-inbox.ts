/* Imports: External */
import { Block, ethers, toBigInt, toNumber, TransactionResponse } from 'ethers'
import { fromHexString, MinioClient, MinioConfig, remove0x, toHexString, zlibDecompress, } from '@metis.io/core-utils'
import { getClient } from '@lodestar/api'
import { networksChainConfig } from '@lodestar/config/networks'
import { ChainConfig } from '@lodestar/config/chainConfig'
import { createChainForkConfig } from '@lodestar/config'
import { chainConfig } from '@lodestar/config/chainConfig/configs/minimal'
import { deneb } from '@lodestar/types'
import * as rlp from 'rlp'
import * as pako from 'pako' // Used for zlib and gzip decompression
// To support Brotli, you need to install the corresponding library, such as iltorb or brotli
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
import {
  Batch,
  BatchType,
  BlobWithMetadata,
  ChannelWithMetadata,
  CompressionAlgo,
  SingularBatch,
  SpanBatch, SpanBatchElement
} from './types'

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
        toBigInt(calldata.slice(offset + 32, offset + 64)) - toBigInt(1)
      batchSubmissionData.batchIndex =
        toBigInt(calldata.slice(offset, offset + 32))
      batchSubmissionData.batchSize =
        toBigInt(calldata.slice(offset + 64, offset + 68))
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
    const da = toNumber(calldata.slice(0, 1))
    const compressType = toNumber(calldata.slice(1, 2))
    let contextData = calldata.slice(70)
    let blobs: BlobWithMetadata[] = []
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
      let config: ChainConfig
      if (options.l2ChainId === 1088) {
        config = networksChainConfig.mainnet
      } else if (options.l2ChainId === 1089) {
        config = networksChainConfig.sepolia
      } else {
        config = chainConfig
      }
      const chainForkConfig = createChainForkConfig(config)
      const api = getClient(
        { baseUrl: options.beaconChainRpcUrl },
        { config: chainForkConfig }
      )

      // TODO: cache this somewhere, no need to retrieve this every time
      const genesis = await api.beacon.getGenesis()

      const l1RpcProvider = new ethers.JsonRpcProvider(options.l1RpcProviderUrl)

      for (let i = 0; i < contextData.length; i += 32) {
        const txHash = contextData.slice(i, i + 32)

        // get block timestamp
        // fetch receipt from el
        const receipt = await l1RpcProvider.getTransactionReceipt(
          `0x${txHash.toString('hex')}`
        )

        if (!receipt) {
          throw new Error(`Receipt of ${txHash} not found`)
        }

        // TODO: cache this somewhere, no need to retrieve this every time
        const block = await l1RpcProvider.getBlock(receipt.blockNumber)

        if (!block) {
          throw new Error(`Block ${receipt.blockNumber} not found`)
        }

        // calculate the slot number
        const slot =
          (block.timestamp / 1000 - genesis.value().genesisTime) /
          chainConfig.SECONDS_PER_SLOT

        const blobIndices = []
        for (let i = extraData.blobIndex; i < extraData.blobIndex + extraData.blobCount; i++) {
          blobIndices.push(i)
        }

        // get blob from cl
        const blobSidecars = await api.beacon.getBlobSidecars({
          blockId: slot.toString(10),
          indices: blobIndices
        })

        const blobSidecarValues = blobSidecars.value()
        if (blobSidecarValues.length < 1) {
          throw new Error(`Invalid number ${blobSidecarValues.length} of blob sidecars for ${txHash}`)
        }

        // TODO: verify kzg commitment with blob data and kzg proof to make sure the data is legit

        blobs.push(...blobSidecarValues.map(blobSidecar => {
          return {
            txHash: txHash.toString('hex'),
            inclusionBlock: receipt.blockNumber,
            timestamp: block.timestamp,
            blockHash: block.hash,
            blob: {
              data: blobSidecar.blob
            },
          }
        }))
      }
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
        const txCount = toNumber(
          contextData.slice(offset, offset + 3)
        )
        offset += 3
        const blockTimestamp = toNumber(
          contextData.slice(offset, offset + 5)
        )
        offset += 5
        const l1BlockNumber = toNumber(
          contextData.slice(offset, offset + 32)
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
          const txType = toNumber(
            contextData.slice(offset, offset + 1)
          )
          offset += 1
          const txDataLen = toNumber(
            contextData.slice(offset, offset + 3)
          )
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
            const txData: Buffer = contextData.slice(offset, offset + txDataLen)
            offset += txDataLen
            const decoded = decodeSequencerBatchTransaction(txData, l2ChainId)
            transactionEntry.data = toHexString(txData)
            transactionEntry.value = decoded.value
            transactionEntry.decoded = decoded
            const signLen = toNumber(
              contextData.slice(offset, offset + 3)
            )
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
      // parse the blob data
      const datas: Uint8Array[] = []
      for (const blob of blobs) {
        const blobData = new Blob(blob.blob.data)
        datas.push(blobData.toData())
      }

      const frames: FrameWithMetadata[] = []
      for (let i = 0; i < blobs.length; i++) {
        const framesPerData = parseFrames(datas[i])
        frames.push(...framesPerData.map((frame) => {
          return {
            txHash: blobs[i].txHash,
            inclusionBlock: blobs[i].inclusionBlock,
            timestamp: blobs[i].timestamp,
            blockHash: blobs[i].blockHash,
            frame,
          }
        }))
      }

      const channelsWithMeta = processFrames(frames)

      for (const channel of channelsWithMeta) {
        for (const batch of channel.batches) {
          const blockEntry: BlockEntry = batch.batchType === BatchType.SingularBatchType ? {
            index: batch.blockNumber,
            batchIndex: batch.transactions[0].batchIndex,
            timestamp: batch.transactions[0].timestamp,
            transactions: batch.transactions,
            confirmed: true,
          } : {
            index: batch.blockNumber,
            batchIndex: batch.transactions[0].batchIndex,
            timestamp: batch.transactions[0].timestamp,
            transactions: batch.transactions,
            confirmed: true,
          }
          blockEntries.push(blockEntry)
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
    numSequencedTransactions: toNumber(
      calldata.slice(offset, offset + 3)
    ),
    numSubsequentQueueTransactions: toNumber(
      calldata.slice(offset + 3, offset + 6)
    ),
    timestamp: toNumber(
      calldata.slice(offset + 6, offset + 11)
    ),
    blockNumber: toNumber(
      calldata.slice(offset + 11, offset + 16)
    ),
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

// translated from optimism go blob struct
export class Blob {
  data: Uint8Array; // Stores the blob data

  constructor(data: Uint8Array) {
    this.data = data;
  }

  // Decodes the blob into raw byte data
  toData(): Uint8Array {
    const VersionOffset = 0;       // Offset for the version byte
    const EncodingVersion = 1;     // Supported encoding version
    const MaxBlobDataSize = 128 * 1024; // Maximum blob data size (e.g., 128KB)

    // Check the encoding version
    if (this.data[VersionOffset] !== EncodingVersion) {
      throw new Error(
        `Invalid encoding version: expected ${EncodingVersion}, got ${this.data[VersionOffset]}`
      );
    }

    // Decode the 3-byte big-endian length value into a 4-byte integer
    const outputLen =
      (this.data[2] << 16) | (this.data[3] << 8) | this.data[4];
    if (outputLen > MaxBlobDataSize) {
      throw new Error(`Invalid blob length: got ${outputLen}`);
    }

    // Special case for round 0: copy only the remaining 27 bytes of the first field element
    const output = new Uint8Array(MaxBlobDataSize);
    output.set(this.data.slice(5, 5 + 27), 0);

    // Now process the remaining 3 field elements to complete round 0
    let opos = 28; // Current position in the output buffer
    let ipos = 32; // Current position in the input blob
    const encodedByte = new Uint8Array(4); // Buffer for the 4 6-bit chunks
    encodedByte[0] = this.data[0]; // Store the first byte for later reassembly

    for (let i = 1; i < 4; i++) {
      [encodedByte[i], opos, ipos] = this.decodeFieldElement(opos, ipos, output);
    }
    opos = this.reassembleBytes(opos, encodedByte, output);

    // In each remaining round, decode 4 field elements (128 bytes) into 127 bytes of output
    const totalFieldElements = this.data.length / 32;
    const Rounds = Math.ceil((totalFieldElements - 4) / 4);

    for (let i = 1; i <= Rounds && opos < outputLen; i++) {
      for (let j = 0; j < 4; j++) {
        [encodedByte[j], opos, ipos] = this.decodeFieldElement(opos, ipos, output);
      }
      opos = this.reassembleBytes(opos, encodedByte, output);
    }

    // Check for any extraneous data in the output buffer
    for (let i = outputLen; i < output.length; i++) {
      if (output[i] !== 0) {
        throw new Error(
          `Extraneous data in output at position ${i}`
        );
      }
    }

    // Truncate the output to the actual length
    const finalOutput = output.slice(0, outputLen);

    // Check for any extraneous data in the input blob
    for (; ipos < this.data.length; ipos++) {
      if (this.data[ipos] !== 0) {
        throw new Error(`Extraneous data in blob at position ${ipos}`);
      }
    }

    return finalOutput;
  }

  // Decodes the next input field element, writing its lower 31 bytes into the output
  private decodeFieldElement(
    opos: number,
    ipos: number,
    output: Uint8Array
  ): [number, number, number] {
    // The two highest-order bits of the first byte should always be 0
    if ((this.data[ipos] & 0b11000000) !== 0) {
      throw new Error(`Invalid field element at position ${ipos}`);
    }
    // Copy the lower 31 bytes of the field element to the output
    output.set(this.data.slice(ipos + 1, ipos + 32), opos);
    const encodedByte = this.data[ipos]; // Get the first byte of the field element
    return [encodedByte, opos + 31, ipos + 32]; // Note: opos increases by 31
  }

  // Reassembles bytes from the 4x6-bit chunks and places them in the appropriate output positions
  private reassembleBytes(
    opos: number,
    encodedByte: Uint8Array,
    output: Uint8Array
  ): number {
    opos--; // Account for the fact that we don't output a 128th byte
    // Compute x, y, z from the encoded bytes
    const x =
      (encodedByte[0] & 0b00111111) |
      ((encodedByte[1] & 0b00110000) << 2);
    const y =
      (encodedByte[1] & 0b00001111) |
      ((encodedByte[3] & 0b00001111) << 4);
    const z =
      (encodedByte[2] & 0b00111111) |
      ((encodedByte[3] & 0b00110000) << 2);

    // Place the reassembled bytes in their appropriate output locations
    output[opos - 31] = z;
    output[opos - 31 * 2] = y;
    output[opos - 31 * 3] = x;

    return opos;
  }
}

// Constants
const DerivationVersion0 = 0; // Assuming version 0 as per the Go code
const MaxFrameLen = 1_000_000; // Maximum frame length (currently 1MB)

// Type definitions
type ChannelID = Uint8Array; // 16-byte array

interface FrameWithMetadata {
  txHash: string;
  inclusionBlock: number;
  timestamp: number;
  blockHash: string;
  frame: Frame;
}

// Frame class definition
class Frame {
  id: ChannelID;        // 16-byte channel ID
  frameNumber: number;  // uint16
  data: Uint8Array;     // Frame data
  isLast: boolean;      // Indicates if this is the last frame

  constructor(
    id: ChannelID,
    frameNumber: number,
    data: Uint8Array,
    isLast: boolean
  ) {
    this.id = id;
    this.frameNumber = frameNumber;
    this.data = data;
    this.isLast = isLast;
  }

  // Static method to unmarshal a frame from a DataView
  static unmarshalBinary(dataView: DataView, offset: number): { frame: Frame; newOffset: number } {
    const initialOffset = offset;
    const totalLength = dataView.byteLength;

    // Read ChannelID (16 bytes)
    if (offset + 16 > totalLength) {
      throw new Error(`Reading channel_id: unexpected EOF`);
    }
    const id = new Uint8Array(dataView.buffer, dataView.byteOffset + offset, 16);
    offset += 16;

    // Read FrameNumber (uint16, big-endian)
    if (offset + 2 > totalLength) {
      throw new Error(`Reading frame_number: unexpected EOF`);
    }
    const frameNumber = dataView.getUint16(offset, false); // Big-endian
    offset += 2;

    // Read frameLength (uint32, big-endian)
    if (offset + 4 > totalLength) {
      throw new Error(`Reading frame_data_length: unexpected EOF`);
    }
    const frameLength = dataView.getUint32(offset, false); // Big-endian
    offset += 4;

    // Cap frame length to MaxFrameLen
    if (frameLength > MaxFrameLen) {
      throw new Error(`frame_data_length is too large: ${frameLength}`);
    }

    // Read frame data
    if (offset + frameLength > totalLength) {
      throw new Error(`Reading frame_data: unexpected EOF`);
    }
    const frameData = new Uint8Array(dataView.buffer, dataView.byteOffset + offset, frameLength);
    offset += frameLength;

    // Read isLast byte
    if (offset + 1 > totalLength) {
      throw new Error(`Reading final byte (is_last): unexpected EOF`);
    }
    const isLastByte = dataView.getUint8(offset);
    offset += 1;

    let isLast: boolean;
    if (isLastByte === 0) {
      isLast = false;
    } else if (isLastByte === 1) {
      isLast = true;
    } else {
      throw new Error(`Invalid byte as is_last: ${isLastByte}`);
    }

    // Create a new Frame instance
    const frame = new Frame(id, frameNumber, frameData, isLast);

    return { frame, newOffset: offset };
  }
}

// Function to parse frames from a Uint8Array
function parseFrames(data: Uint8Array): Frame[] {
  if (data.length === 0) {
    throw new Error("Data array must not be empty");
  }
  if (data[0] !== DerivationVersion0) {
    throw new Error(`Invalid derivation format byte: got ${data[0]}`);
  }

  let offset = 1; // Skip the version byte
  const frames: Frame[] = [];
  const dataView = new DataView(data.buffer, data.byteOffset, data.byteLength);

  while (offset < data.length) {
    try {
      const { frame, newOffset } = Frame.unmarshalBinary(dataView, offset);
      frames.push(frame);
      offset = newOffset;
    } catch (error) {
      throw new Error(`Parsing frame ${frames.length}: ${error.message}`);
    }
  }

  if (offset !== data.length) {
    throw new Error(
      `Did not fully consume data: have ${frames.length} frames and ${data.length - offset} bytes left`
    );
  }
  if (frames.length === 0) {
    throw new Error("Was not able to find any frames");
  }
  return frames;
}

export class Channel {
  id: ChannelID;
  frames: Map<number, Frame>;
  isReady: boolean;

  constructor(id: ChannelID) {
    this.id = id;
    this.frames = new Map();
    this.isReady = false;
  }

  addFrame(frame: Frame): void {
    if (this.frames.has(frame.frameNumber)) {
      throw new Error(`Duplicate frame number ${frame.frameNumber}`);
    }

    this.frames.set(frame.frameNumber, frame);

    if (frame.isLast) {
      this.isReady = true;
    }
  }

  isComplete(): boolean {
    if (!this.isReady) {
      return false;
    }
    const frameNumbers = Array.from(this.frames.keys());
    const maxFrameNumber = Math.max(...frameNumbers);
    for (let i = 0; i <= maxFrameNumber; i++) {
      if (!this.frames.has(i)) {
        return false;
      }
    }
    return true;
  }

  assembleData(): Uint8Array {
    if (!this.isComplete()) {
      throw new Error('Channel is not complete');
    }
    const frameNumbers = Array.from(this.frames.keys()).sort((a, b) => a - b);
    const dataParts: Uint8Array[] = [];
    for (const frameNumber of frameNumbers) {
      const frame = this.frames.get(frameNumber)!;
      dataParts.push(frame.data);
    }
    const totalLength = dataParts.reduce((sum, part) => sum + part.length, 0);
    const assembledData = new Uint8Array(totalLength);
    let offset = 0;
    for (const part of dataParts) {
      assembledData.set(part, offset);
      offset += part.length;
    }
    return assembledData;
  }
}

function processChannelFrames(
  channelID: ChannelID,
  frames: FrameWithMetadata[]
): ChannelWithMetadata {
  // Create a new Channel
  const channel = new Channel(channelID);
  let invalidFrames = false;

  // Sort frames by frameNumber
  frames.sort((a, b) => a.frame.frameNumber - b.frame.frameNumber);

  // Add frames to channel
  for (const frameMeta of frames) {
    try {
      channel.addFrame(frameMeta.frame);
    } catch (err) {
      console.error(`Error adding frame to channel ${uint8ArrayToHex(channelID)}: ${err.message}`);
      invalidFrames = true;
    }
  }

  let batches: (SingularBatch | SpanBatch)[] = [];
  let batchTypes: number[] = [];
  let comprAlgos: CompressionAlgo[] = [];
  let invalidBatches = false;

  if (channel.isComplete()) {
    try {
      const channelData = channel.assembleData()
      // Create BatchReader to read batches from channelData
      const batchReader = new BatchReader(channelData);
      let batchData: BatchData | null;
      while ((batchData = batchReader.nextBatch()) !== null) {
        comprAlgos.push(batchData.comprAlgo);
        const batchType = batchData.getBatchType();
        batchTypes.push(batchType);

        if (batchType === BatchType.SingularBatchType) {
          const singularBatch = getSingularBatch(batchData);
          if (singularBatch) {
            batches.push(singularBatch);
          } else {
            console.error(`Error processing singular batch in channel ${uint8ArrayToHex(channelID)}`);
            invalidBatches = true;
          }
        } else if (batchType === BatchType.SpanBatchType) {
          // TODO: need to provide blockTime, genesisTimestamp, and chainID here...
          const spanBatch = deriveSpanBatch(batchData, /* blockTime */ 2, /* genesisTimestamp */ 0, /* chainID */ BigInt(1));
          if (spanBatch) {
            batches.push(spanBatch);
          } else {
            console.error(`Error processing span batch in channel ${uint8ArrayToHex(channelID)}`);
            invalidBatches = true;
          }
        } else {
          console.error(`Unrecognized batch type ${batchType} in channel ${uint8ArrayToHex(channelID)}`);
        }
      }
    } catch (err) {
      console.error(`Error processing channel ${uint8ArrayToHex(channelID)}: ${err.message}`);
      invalidBatches = true;
    }
  } else {
    console.log(`Channel ${uint8ArrayToHex(channelID)} is not complete`);
  }

  const channelWithMetadata: ChannelWithMetadata = {
    id: channelID,
    isReady: channel.isComplete(),
    invalidFrames,
    invalidBatches,
    frames,
    batches,
    batchTypes,
    comprAlgos,
  };

  return channelWithMetadata;
}

const MaxSpanBatchElementCount = 10_000_000;

// Update BatchData class
export class BatchData {
  comprAlgo: CompressionAlgo;
  data: Uint8Array;

  constructor(comprAlgo: CompressionAlgo, data: Uint8Array) {
    this.comprAlgo = comprAlgo;
    this.data = data;
  }

  getBatchType(): BatchType {
    // The batch type is the first byte of data
    return this.data[0] as BatchType;
  }
}

export class BatchReader {
  data: Uint8Array;
  offset: number;

  constructor(data: Uint8Array) {
    this.data = data;
    this.offset = 0;
  }

  nextBatch(): BatchData | null {
    if (this.offset >= this.data.length) {
      return null;
    }

    const comprAlgoByte = this.data[this.offset];
    const comprAlgo = comprAlgoByte as CompressionAlgo;
    this.offset += 1;

    if (this.offset + 4 > this.data.length) {
      throw new Error('Unexpected end of data when reading batch length');
    }
    const lengthBytes = this.data.slice(this.offset, this.offset + 4);
    const batchLength = (lengthBytes[0] << 24) | (lengthBytes[1] << 16) | (lengthBytes[2] << 8) | lengthBytes[3];
    this.offset += 4;

    if (this.offset + batchLength > this.data.length) {
      throw new Error('Unexpected end of data when reading batch data');
    }
    const compressedData = this.data.slice(this.offset, this.offset + batchLength);
    this.offset += batchLength;

    let decompressedData: Uint8Array;
    switch (comprAlgo) {
      case CompressionAlgo.None:
        decompressedData = compressedData;
        break;
      // FIXME: need to figure out the options for zlib cm8 and cm15
      case CompressionAlgo.ZlibCM8:
        decompressedData = pako.inflate(compressedData);
        break;
      case CompressionAlgo.ZlibCM15:
        decompressedData = pako.inflate(compressedData);
        break;
      case CompressionAlgo.Brotli:
        decompressedData = decompressBrotli(compressedData);
        break;
      default:
        throw new Error(`Unsupported compression algorithm: ${comprAlgo}`);
    }

    return new BatchData(comprAlgo, decompressedData);
  }
}

function getSingularBatch(batchData: BatchData): SingularBatch | null {
  try {
    // Check if the batch type is SingularBatchType
    const batchType = batchData.getBatchType();
    if (batchType !== BatchType.SingularBatchType) {
      throw new Error(`Invalid batch type: expected SingularBatchType (${BatchType.SingularBatchType}), got ${batchType}`);
    }

    // Skip the batch type byte
    const dataWithoutType = batchData.data.slice(1);

    // RLP-decode the remaining data
    const decoded = rlp.decode(dataWithoutType);

    if (!Array.isArray(decoded) || decoded.length !== 5) {
      throw new Error('Invalid singular batch format');
    }

    const [parentHash, epochNumber, epochHash, timestamp, transactions] = decoded;

    // Parse fields
    const batch: SingularBatch = {
      batchType: BatchType.SingularBatchType,
      parentHash: '0x' + Buffer.from(parentHash).toString('hex'),
      epochNumber: toBigInt(epochNumber).toString(),
      epochHash: '0x' + Buffer.from(epochHash).toString('hex'),
      timestamp: toBigInt(timestamp).toString(),
      transactions: (transactions as unknown as Buffer[]).map((tx: Buffer) => tx),
    };

    return batch;
  } catch (err) {
    console.error(`Error decoding singular batch: ${err.message}`);
    return null;
  }
}

function deriveSpanBatch(
  batchData: BatchData,
  blockTime: number,
  genesisTimestamp: number,
  chainID: bigint
): SpanBatch | null {
  try {
    // Check if the batch type is SpanBatchType
    const batchType = batchData.getBatchType();
    if (batchType !== BatchType.SpanBatchType) {
      throw new Error(`Invalid batch type: expected SpanBatchType (${BatchType.SpanBatchType}), got ${batchType}`);
    }

    // Skip the batch type byte
    const dataWithoutType = batchData.data.slice(1);
    let offset = 0;
    const data = dataWithoutType;

    // Read prefix fields
    const { value: relTimestamp, bytesRead: relTimestampBytes } = readUvarint(data, offset);
    offset += relTimestampBytes;

    const { value: l1OriginNum, bytesRead: l1OriginNumBytes } = readUvarint(data, offset);
    offset += l1OriginNumBytes;

    // Read parent_check (20 bytes)
    if (offset + 20 > data.length) {
      throw new Error('Unexpected end of data when reading parent_check');
    }
    const parentCheck = data.slice(offset, offset + 20);
    offset += 20;

    // Read l1_origin_check (20 bytes)
    if (offset + 20 > data.length) {
      throw new Error('Unexpected end of data when reading l1_origin_check');
    }
    const l1OriginCheck = data.slice(offset, offset + 20);
    offset += 20;

    // Read payload fields
    const { value: blockCount, bytesRead: blockCountBytes } = readUvarint(data, offset);
    offset += blockCountBytes;

    if (blockCount === 0) {
      throw new Error('Span batch must not be empty');
    }

    if (blockCount > MaxSpanBatchElementCount) {
      throw new Error('Span batch size limit reached');
    }

    // Read origin_bits (bitlist)
    const originBitsLength = Math.ceil(blockCount / 8);
    if (offset + originBitsLength > data.length) {
      throw new Error('Unexpected end of data when reading origin_bits');
    }
    const originBitsBytes = data.slice(offset, offset + originBitsLength);
    offset += originBitsLength;

    // Convert originBitsBytes to a BigInt
    let originBits = BigInt(0);
    for (let i = 0; i < originBitsBytes.length; i++) {
      originBits |= BigInt(originBitsBytes[i]) << BigInt(8 * i);
    }

    // Read block_tx_counts
    const blockTxCounts: number[] = [];
    let totalBlockTxCount = 0;
    for (let i = 0; i < blockCount; i++) {
      const { value: blockTxCount, bytesRead: blockTxCountBytes } = readUvarint(data, offset);
      offset += blockTxCountBytes;

      if (blockTxCount > MaxSpanBatchElementCount) {
        throw new Error('Block tx count exceeds limit');
      }

      blockTxCounts.push(blockTxCount);
      totalBlockTxCount += blockTxCount;
    }

    if (totalBlockTxCount > MaxSpanBatchElementCount) {
      throw new Error('Total tx count exceeds limit');
    }

    // Read transactions
    const transactions: Uint8Array[] = [];
    let txRead = 0;
    while (txRead < totalBlockTxCount) {
      // Read transaction data
      const { txData, bytesRead } = readTransaction(data, offset);
      offset += bytesRead;
      transactions.push(txData);
      txRead++;
    }

    // Reconstruct block origins
    const blockOriginNums: number[] = new Array(blockCount);
    let l1OriginBlockNumber = l1OriginNum;
    for (let i = blockCount - 1; i >= 0; i--) {
      blockOriginNums[i] = l1OriginBlockNumber;
      const bit = (originBits >> BigInt(i)) & BigInt(1);
      if (bit === BigInt(1) && i > 0) {
        l1OriginBlockNumber--;
      }
    }

    // Build span batch elements
    const batches: SpanBatchElement[] = [];
    let txIndex = 0;
    for (let i = 0; i < blockCount; i++) {
      const batchTxCount = blockTxCounts[i];
      const txsForBatch = transactions.slice(txIndex, txIndex + batchTxCount);
      txIndex += batchTxCount;

      // Compute timestamp for this batch
      const timestamp = genesisTimestamp + relTimestamp + blockTime * i;

      // Build span batch element
      const batchElement: SpanBatchElement = {
        epochNumber: blockOriginNums[i].toString(),
        timestamp: timestamp.toString(),
        transactions: txsForBatch,
      };

      batches.push(batchElement);
    }

    // Construct SpanBatch
    const spanBatch: SpanBatch = {
      batchType: BatchType.SpanBatchType,
      parentCheck: '0x' + Buffer.from(parentCheck).toString('hex'),
      l1OriginCheck: '0x' + Buffer.from(l1OriginCheck).toString('hex'),
      batches,
    };

    return spanBatch;
  } catch (err) {
    console.error(`Error decoding span batch: ${err.message}`);
    return null;
  }
}

function uint8ArrayToHex(array: Uint8Array): string {
  return '0x' + Buffer.from(array).toString('hex');
}

function hexStringToUint8Array(hexString: string): Uint8Array {
  if (hexString.startsWith('0x')) {
    hexString = hexString.slice(2);
  }
  return Uint8Array.from(Buffer.from(hexString, 'hex'));
}

function decompressBrotli(data: Uint8Array): Uint8Array {
  throw new Error('Brotli decompression not implemented');
}

function processFrames(frames: FrameWithMetadata[]): ChannelWithMetadata[] {
  const framesByChannel = groupFramesByChannelID(frames);

  const channels: ChannelWithMetadata[] = [];

  for (const [channelIDHex, frames] of Object.entries(framesByChannel)) {
    const channelID = hexStringToUint8Array(channelIDHex);
    const channelResult = processChannelFrames(channelID, frames);
    channels.push(channelResult);
  }

  return channels;
}

function groupFramesByChannelID(frames: FrameWithMetadata[]): { [channelIDHex: string]: FrameWithMetadata[] } {
  const framesByChannel: { [channelIDHex: string]: FrameWithMetadata[] } = {};
  for (const frame of frames) {
    const channelIDHex = uint8ArrayToHex(frame.frame.id);
    if (!framesByChannel[channelIDHex]) {
      framesByChannel[channelIDHex] = [];
    }
    framesByChannel[channelIDHex].push(frame);
  }
  return framesByChannel;
}

function readUvarint(data: Uint8Array, offset: number): { value: number; bytesRead: number } {
  let x = 0;
  let s = 0;
  let bytesRead = 0;
  for (let i = offset; i < data.length; i++) {
    const b = data[i];
    if (b < 0x80) {
      if (i - offset >= 9 && b > 1) {
        throw new Error('Overflow in readUvarint');
      }
      x |= (b << s) >>> 0;
      bytesRead++;
      return { value: x >>> 0, bytesRead };
    }
    x |= ((b & 0x7f) << s) >>> 0;
    s += 7;
    bytesRead++;
    if (s > 35) {
      throw new Error('Overflow in readUvarint');
    }
  }
  throw new Error('Unexpected end of data in readUvarint');
}

function readTransaction(data: Uint8Array, offset: number): { txData: Uint8Array; bytesRead: number } {
  const initialOffset = offset;

  // Read the first byte to check if it's a transaction type
  const txType = data[offset];
  let isTypedTransaction = false;
  if (txType <= 0x7f) {
    // Typed transaction
    isTypedTransaction = true;
    offset += 1;
  }

  // RLP-decode the transaction
  const result = rlp.decode(data.slice(offset), true);
  const txPayload = result.data as Uint8Array;
  const decodedLength = data.length - offset - result.remainder.length;

  // Reconstruct the transaction data
  const txData = isTypedTransaction
    ? Uint8Array.of(txType, ...txPayload)
    : txPayload;

  offset += decodedLength;

  return { txData, bytesRead: offset - initialOffset };
}
