import { BytesLike, getBytes } from 'ethers'
import { SpanBatchTxs } from './span-batch-txs'
import { SingularBatch } from './singular-batch'
import { encodeSpanBatchBits } from './utils'
import { Writer } from './types'
import { L2Transaction } from '@metis.io/core-utils'

export class SpanBatch {
  public originBits: bigint
  public blockTxCounts: number[]
  public sbtxs: SpanBatchTxs

  constructor(
    public parentCheck: Uint8Array,
    public l1OriginCheck: Uint8Array,
    public chainID: bigint,
    public batches: SpanBatchElement[]
  ) {
    this.originBits = BigInt(0)
    this.blockTxCounts = []
    this.sbtxs = new SpanBatchTxs()
  }

  get batchType(): number {
    return 1 // SpanBatchType
  }

  get timestamp(): number {
    return this.batches[0]?.timestamp || 0
  }

  async appendSingularBatch(singularBatch: SingularBatch): Promise<void> {
    if (
      this.batches.length > 0 &&
      this.peek(0).timestamp > singularBatch.timestamp
    ) {
      throw new Error('span batch is not ordered')
    }

    this.batches.push(this.singularBatchToElement(singularBatch))

    this.l1OriginCheck = getBytes(singularBatch.epochHash).slice(0, 20)

    let epochBit = BigInt(0)
    if (this.batches.length === 1) {
      epochBit = BigInt(1)
      this.parentCheck = getBytes(singularBatch.parentHash).slice(0, 20)
    } else {
      if (this.peek(1).epochNum < this.peek(0).epochNum) {
        epochBit = BigInt(1)
      }
    }

    this.originBits |= epochBit << BigInt(this.batches.length - 1)

    this.blockTxCounts.push(this.peek(0).transactions.length)

    const newTxs = this.peek(0).transactions
    await this.sbtxs.addTxs(newTxs, this.chainID)
  }

  toRawSpanBatch(): RawSpanBatch {
    if (this.batches.length === 0) {
      throw new Error('cannot merge empty singularBatch list')
    }
    const spanStart = this.batches[0]
    const spanEnd = this.batches[this.batches.length - 1]

    return new RawSpanBatch(
      spanStart.timestamp,
      spanEnd.epochNum,
      this.parentCheck,
      this.l1OriginCheck,
      this.batches.length,
      this.originBits,
      this.blockTxCounts,
      this.sbtxs
    )
  }

  private peek(n: number): SpanBatchElement {
    return this.batches[this.batches.length - 1 - n]
  }

  private singularBatchToElement(
    singularBatch: SingularBatch
  ): SpanBatchElement {
    return {
      epochNum: singularBatch.epochNum,
      timestamp: singularBatch.timestamp,
      transactions: singularBatch.transactions,
    }
  }
}

export interface SpanBatchElement {
  epochNum: number
  timestamp: number
  transactions: L2Transaction[]
}

export class RawSpanBatch {
  constructor(
    public l1Timestamp: number, // for here we use referenced l1 timestamp instead of the relative time
    public l1OriginNum: number,
    public parentCheck: Uint8Array,
    public l1OriginCheck: Uint8Array,
    public blockCount: number,
    public originBits: bigint,
    public blockTxCounts: number[],
    public txs: SpanBatchTxs
  ) {}

  encode(): Uint8Array {
    const writer = new Writer()
    this.encodePrefix(writer)
    this.encodePayload(writer)
    return writer.getData()
  }

  private encodePrefix(writer: Writer): void {
    writer.writeVarInt(this.l1Timestamp)
    writer.writeVarInt(this.l1OriginNum)
    writer.writeBytes(this.parentCheck)
    writer.writeBytes(this.l1OriginCheck)
  }

  private encodePayload(writer: Writer): void {
    writer.writeVarInt(this.blockCount)
    encodeSpanBatchBits(writer, this.blockCount, this.originBits)
    for (const count of this.blockTxCounts) {
      writer.writeVarInt(count)
    }
    writer.writeBytes(this.txs.encode())
  }
}
