// types.ts
import { BytesLike, ethers, getBytes } from 'ethers'
import { Blob } from './blob'

export interface RollupConfig {
  l1ChainID: bigint
  l2ChainID: bigint
  batchInboxAddress: string
}

export interface BlockID {
  hash: string
  number: number
}

export interface SystemConfig {
  batcherAddr: string
  overhead: string
  scalar: string
  gasLimit: number
}

export interface ChannelConfig {
  maxFrameSize: number
  targetFrames: number
  maxBlocksPerSpanBatch: number
  targetNumFrames: number
  targetCompressorFactor: number
  compressionAlgo: string
  batchType: number
  useBlobs: boolean
}

export type ChannelId = Uint8Array

export interface Frame {
  id: ChannelId
  frameNumber: number
  data: Uint8Array
  isLast: boolean
}

export interface TxData {
  frames: Frame[]
  asBlob: boolean

  get id(): string
  get blobs(): Blob[]
}

export interface L1BlockInfo {
  number: number
  time: number
  baseFee: bigint
  blockHash: string
  batcherAddr: string
}

export interface L2Client {
  getBlock(blockHashOrBlockTag: string | number): Promise<ethers.Block>
  getBlockNumber(): Promise<number>
}

export interface SingularBatch {
  parentHash: string
  epochNum: number
  epochHash: string
  timestamp: number
  transactions: string[]
}

export interface SpanBatchElement {
  epochNum: number
  timestamp: number
  transactions: string[]
}

export interface SpanBatchTxs {
  totalBlockTxCount: number
  contractCreationBits: bigint
  yParityBits: bigint
  txSigs: SpanBatchSignature[]
  txNonces: number[]
  txGases: number[]
  txTos: string[]
  txDatas: Uint8Array[]
  protectedBits: bigint
  txTypes: number[]
  totalLegacyTxCount: number
}

export interface SpanBatchSignature {
  v: number
  r: bigint
  s: bigint
}

export class Writer {
  private data: number[] = []

  writeBytes(bytes: BytesLike): void {
    this.data.push(...getBytes(bytes))
  }

  writeUint8(value: number): void {
    this.data.push(value)
  }

  writeVarInt(value: number): void {
    const bytes = []
    while (value >= 0x80) {
      bytes.push((value & 0x7f) | 0x80)
      value >>= 7
    }
    bytes.push(value & 0x7f)
    this.data.push(...bytes)
  }

  getData(): Uint8Array {
    return new Uint8Array(this.data)
  }
}

// Move inbox types here to avoid circular dependencies
export interface BatchToInboxRawTx {
  rawTransaction: string | undefined
  seqSign: string | undefined | null
  isSequencerTx: boolean
  l1BlockNumber: number | null
  l1TxOrigin: string | null
  queueIndex: number | null
}

export interface BatchToInboxElement {
  stateRoot: string
  timestamp: number
  blockNumber: number
  hash: string
  parentHash: string
  txs: BatchToInboxRawTx[]
}
export declare type BatchToInbox = BatchToInboxElement[]

export interface InboxBatchParams {
  inputData: string
  batch: BatchToInbox
  blobTxData: TxData[]
}
