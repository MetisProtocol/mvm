// span-channel-out.ts
import { ethers, randomBytes } from 'ethersv6'
import RLP from 'rlp'
import { ChannelCompressor } from './channel-compressor'
import { SpanBatch } from './span-batch'
import { SingularBatch } from './singular-batch'
import { BatchToInboxElement, Frame } from './types'
import {
  CHANNEL_FULL_ERR,
  FRAME_OVERHEAD_SIZE,
  MAX_RLP_BYTES_PER_CHANNEL,
} from './consts'
import { L2Transaction, QueueOrigin } from '@localtest911/core-utils'
import { Logger } from '@eth-optimism/common-ts'

export class SpanChannelOut {
  private _id: Uint8Array
  private frame: number
  private rlp: Uint8Array

  private closed: boolean
  private full: Error | null
  private spanBatch: SpanBatch
  private readonly maxBlocksPerSpanBatch: number
  private sealedRLPBytes: number

  constructor(
    private logger: Logger,
    chainId: bigint,
    private target: number,
    private compressor: ChannelCompressor,
    opts?: { maxBlocksPerSpanBatch?: number }
  ) {
    this._id = randomBytes(16)
    this.frame = 0
    this.rlp = new Uint8Array(0)
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

  async addBlock(block: BatchToInboxElement, epochHash: string) {
    if (this.closed) {
      throw new Error('Channel out already closed')
    }

    if (!block.txs) {
      throw new Error('Block has no transactions')
    }

    const opaqueTxs: L2Transaction[] = []
    for (const tx of block.txs) {
      try {
        const l2Tx = ethers.Transaction.from(tx.rawTransaction) as any
        l2Tx.l1BlockNumber = tx.l1BlockNumber
        l2Tx.l1TxOrigin = tx.l1TxOrigin
        l2Tx.queueOrigin = tx.isSequencerTx
          ? QueueOrigin.Sequencer
          : QueueOrigin.L1ToL2
        l2Tx.rawTransaction = tx.rawTransaction
        if (tx.seqSign) {
          l2Tx.seqR = '0x' + tx.seqSign.slice(0, 64)
          l2Tx.seqS = '0x' + tx.seqSign.slice(64, 128)
          l2Tx.seqV = '0x' + tx.seqSign.slice(128, 130)
        }
        opaqueTxs.push(l2Tx as L2Transaction)
      } catch (err) {
        this.logger.error('Failed to parse tx', { err })
        throw err
      }
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

    // this.rlp = await rawSpanBatch.encode()
    this.logger.info('Appended singular batch', {
      l2Block: batch.blockNumber,
    })

    // if (this.rlp.length > MAX_RLP_BYTES_PER_CHANNEL) {
    //   this.logger.error(`Channel is too large: ${this.rlp.length}`)
    //   throw new Error(
    //     `ErrTooManyRLPBytes: could not take ${this.rlp.length} bytes, max is ${MAX_RLP_BYTES_PER_CHANNEL}`
    //   )
    // }
    //
    // // TODO: might need to optimize this, no need to compress the data everytime.
    // await this.compress(this.rlp)
    //
    // if (this.full) {
    //   if (this.sealedRLPBytes === 0 && this.spanBatch.batches.length === 1) {
    //     return
    //   }
    //
    //   if (this.compressor.len() === this.target) {
    //     return
    //   }
    //
    //   this.logger.info('channel is full')
    //   throw this.full
    // }
  }

  private ensureOpenSpanBatch(): void {
    if (
      this.maxBlocksPerSpanBatch === 0 ||
      this.spanBatch.batches.length < this.maxBlocksPerSpanBatch
    ) {
      return
    }
    this.sealedRLPBytes = this.rlp.length
  }

  private async compress(data: Uint8Array): Promise<void> {
    this.logger.info('Compressing new data', { dataLength: data.length })
    const startTime = Date.now()
    // rlp encode the data first
    const rlpBatches = RLP.encode(data)
    this.logger.info('RLP encoded batch data', { rlpLength: rlpBatches.length })
    this.compressor.reset()
    // write the rlp encoded data to the compressor
    await this.compressor.write(rlpBatches)
    this.checkFull()
    const endTime = Date.now()
    this.logger.info(`Compress done`, {
      spend: `${endTime - startTime}ms`,
      inputLength: rlpBatches.length,
      compressedLength: this.compressor.len(),
      compressionRate: `${
        (1 - this.compressor.len() / rlpBatches.length) * 100
      }%`,
    })
  }

  inputBytes(): number {
    return this.rlp.length
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
    await this.compress(await this.spanBatch.toRawSpanBatch().encode())
  }

  outputFrame(frameSize: number): [Frame, boolean] {
    if (frameSize < FRAME_OVERHEAD_SIZE) {
      throw new Error('Frame size too small')
    }

    const f = this.createEmptyFrame(frameSize)
    this.frame += 1
    return [f, f.isLast]
  }

  private createEmptyFrame(maxSize: number): Frame {
    const readyBytes = this.readyBytes()
    const dataSize = Math.min(readyBytes, maxSize - FRAME_OVERHEAD_SIZE)
    this.logger.info('Creating frame', { dataSize, readyBytes, maxSize })
    return {
      id: this.id,
      frameNumber: this.frame,
      data: this.compressor.read(dataSize),
      isLast: this.closed && dataSize >= readyBytes,
    }
  }
}
