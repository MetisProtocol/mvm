// span-channel-out.ts
import { ethers, randomBytes } from 'ethersv6'
import RLP from 'rlp'
import { ChannelCompressor } from './channel-compressor'
import { SpanBatch } from './span-batch'
import { SingularBatch } from './singular-batch'
import { BatchToInboxElement, Frame, RollupConfig } from './types'
import {
  CHANNEL_FULL_ERR,
  FRAME_OVERHEAD_SIZE,
  MAX_RLP_BYTES_PER_CHANNEL,
} from './consts'
import { L2Transaction, QueueOrigin } from '@localtest911/core-utils'

export class SpanChannelOut {
  private _id: Uint8Array
  private frame: number
  private rlp: [Uint8Array, Uint8Array]
  private rlpIndex: number
  private lastCompressedRLPSize: number

  private closed: boolean
  private full: Error | null
  private spanBatch: SpanBatch
  private readonly maxBlocksPerSpanBatch: number
  private sealedRLPBytes: number

  constructor(
    chainId: bigint,
    private target: number,
    private compressor: ChannelCompressor,
    opts?: { maxBlocksPerSpanBatch?: number }
  ) {
    this._id = randomBytes(16)
    this.frame = 0
    this.rlp = [new Uint8Array(0), new Uint8Array(0)]
    this.rlpIndex = 0
    this.lastCompressedRLPSize = 0
    this.closed = false
    this.full = null
    this.spanBatch = new SpanBatch(
      new Uint8Array(20),
      new Uint8Array(20),
      chainId,
      [],
      0
    )
    this.maxBlocksPerSpanBatch = opts?.maxBlocksPerSpanBatch ?? 0
    this.sealedRLPBytes = 0
  }

  get id(): Uint8Array {
    return this._id
  }

  reset(): void {
    this.closed = false
    this.full = null
    this.frame = 0
    this.sealedRLPBytes = 0
    this.rlp = [new Uint8Array(0), new Uint8Array(0)]
    this.lastCompressedRLPSize = 0
    this.compressor.reset()
    this.resetSpanBatch()
    this._id = randomBytes(16)
  }

  private resetSpanBatch(): void {
    this.spanBatch = new SpanBatch(
      this.spanBatch.parentCheck,
      this.spanBatch.l1OriginCheck,
      this.spanBatch.chainID,
      [],
      0
    )
  }

  private activeRLP(): Uint8Array {
    return this.rlp[this.rlpIndex]
  }

  private inactiveRLP(): Uint8Array {
    return this.rlp[(this.rlpIndex + 1) % 2]
  }

  private swapRLP(): void {
    this.rlpIndex = (this.rlpIndex + 1) % 2
  }

  async addBlock(block: BatchToInboxElement, epochHash: string) {
    if (this.closed) {
      throw new Error('Channel out already closed')
    }

    if (!block.txs) {
      throw new Error('Block has no transactions')
    }

    const opaqueTxs: L2Transaction[] = []
    for (const tx of block.txs) {
      const l2Tx = ethers.Transaction.from(tx.rawTransaction) as any
      l2Tx.l1BlockNumber = tx.l1BlockNumber
      l2Tx.l1TxOrigin = tx.l1TxOrigin
      l2Tx.queueOrigin = tx.isSequencerTx
        ? QueueOrigin.Sequencer
        : QueueOrigin.L1ToL2
      l2Tx.rawTransaction = tx.rawTransaction
      l2Tx.seqR = tx.seqSign.slice(0, 64)
      l2Tx.seqS = tx.seqSign.slice(64, 128)
      l2Tx.seqV = tx.seqSign.slice(128, 130)
      opaqueTxs.push(l2Tx as L2Transaction)
    }

    const epochNum = block.txs[0].l1BlockNumber

    const singularBatch: SingularBatch = new SingularBatch(
      block.blockNumber,
      block.parentHash,
      epochNum,
      epochHash,
      block.timestamp,
      opaqueTxs
    )

    await this.addSingularBatch(singularBatch)
  }

  async addSingularBatch(batch: SingularBatch): Promise<void> {
    if (this.closed) {
      throw new Error('Channel out already closed')
    }
    if (this.full) {
      throw this.full
    }

    this.ensureOpenSpanBatch()

    await this.spanBatch.appendSingularBatch(batch)
    const rawSpanBatch = this.spanBatch.toRawSpanBatch()

    this.swapRLP()
    const active = this.activeRLP()
    const encoded = rawSpanBatch.encode()
    if (encoded.length > active.length) {
      this.rlp[this.rlpIndex] = new Uint8Array(encoded.length)
    }
    this.rlp[this.rlpIndex].set(encoded)

    if (this.rlp[this.rlpIndex].length > MAX_RLP_BYTES_PER_CHANNEL) {
      throw new Error(
        `ErrTooManyRLPBytes: could not take ${
          this.rlp[this.rlpIndex].length
        } bytes as replacement of channel of ${
          this.inactiveRLP().length
        } bytes, max is ${MAX_RLP_BYTES_PER_CHANNEL}`
      )
    }

    const rlpGrowth =
      this.rlp[this.rlpIndex].length - this.lastCompressedRLPSize
    if (this.compressor.len() + rlpGrowth < this.target) {
      return
    }

    await this.compress()

    if (this.full) {
      if (this.sealedRLPBytes === 0 && this.spanBatch.batches.length === 1) {
        return
      }

      if (this.compressor.len() === this.target) {
        return
      }

      this.swapRLP()
      await this.compress()
      throw this.full
    }
  }

  private ensureOpenSpanBatch(): void {
    if (
      this.maxBlocksPerSpanBatch === 0 ||
      this.spanBatch.batches.length < this.maxBlocksPerSpanBatch
    ) {
      return
    }
    const active = this.activeRLP()
    const inactive = this.inactiveRLP()
    if (inactive.length > active.length) {
      throw new Error('Inactive RLP unexpectedly larger')
    }
    this.sealedRLPBytes = active.length
    inactive.set(active)
    this.resetSpanBatch()
  }

  private async compress(): Promise<void> {
    const rlpBatches = RLP.encode(this.activeRLP())
    this.compressor.reset()
    await this.compressor.write(rlpBatches)
    this.lastCompressedRLPSize = rlpBatches.length
    this.checkFull()
  }

  inputBytes(): number {
    return this.activeRLP().length
  }

  readyBytes(): number {
    if (this.closed || this.full) {
      return this.compressor.len()
    }
    return 0
  }

  private checkFull(): void {
    if (this.full) {
      return
    }
    if (this.compressor.len() >= this.target) {
      this.full = CHANNEL_FULL_ERR
    }
  }

  fullErr(): Error | null {
    return this.full
  }

  async close(): Promise<void> {
    if (this.closed) {
      throw new Error('ErrChannelOutAlreadyClosed')
    }
    this.closed = true
    if (this.full) {
      return
    }
    await this.compress()
  }

  outputFrame(frameSize: number): [Frame, boolean] {
    if (frameSize < FRAME_OVERHEAD_SIZE) {
      throw new Error('Frame size too small')
    }

    const f = this.createEmptyFrame(frameSize)

    this.compressor.read(f.data)

    this.frame += 1
    return [f, f.isLast]
  }

  private createEmptyFrame(maxSize: number): Frame {
    const readyBytes = this.readyBytes()
    const dataSize = Math.min(readyBytes, maxSize - FRAME_OVERHEAD_SIZE)
    return {
      id: this.id,
      frameNumber: this.frame,
      data: new Uint8Array(dataSize),
      isLast: this.closed && dataSize >= readyBytes,
    }
  }
}
