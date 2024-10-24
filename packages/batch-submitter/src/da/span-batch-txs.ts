import { getBytes, toBeArray, toBigInt, zeroPadValue } from 'ethersv6'
import { Writer } from './types'
import { newSpanBatchTx } from './span-batch-tx'
import { encodeSpanBatchBits } from './utils'
import { L2Transaction, QueueOrigin } from '@localtest911/core-utils'

export class SpanBatchTxs {
  private totalBlockTxCount: number = 0
  private contractCreationBits: bigint = BigInt(0)
  private yParityBits: bigint = BigInt(0)
  private txSigs: SpanBatchSignature[] = []
  private txNonces: number[] = []
  private txGases: number[] = []
  private txTos: string[] = []
  private txDatas: Uint8Array[] = []
  private protectedBits: bigint = BigInt(0)
  private txTypes: number[] = []
  private totalLegacyTxCount: number = 0

  // metis extra fields
  private queueOriginBits: bigint = BigInt(0) // bitmap to save queue origins, 0 for sequencer, 1 for enqueue
  private l1TxOrigins: string[] = [] // l1 tx origins, only used for enqueue tx
  private txSeqSigs: SpanBatchSequencerSignature[] = []
  private seqYParityBits: bigint = BigInt(0)

  async addTxs(txs: L2Transaction[], chainId: bigint): Promise<void> {
    const offset = this.totalBlockTxCount
    for (let idx = 0; idx < txs.length; idx++) {
      const tx = txs[idx]

      // process legacy tx
      const txType = tx.type ?? 0
      if (!tx.type) {
        const protectedBit = tx.chainId ? BigInt(1) : BigInt(0)
        this.protectedBits |= protectedBit << BigInt(this.totalLegacyTxCount)
        this.totalLegacyTxCount++
      }

      if (tx.chainId && BigInt(tx.chainId) !== chainId) {
        throw new Error(
          `Protected tx has chain ID ${tx.chainId}, but expected chain ID ${chainId}`
        )
      }

      this.txSigs.push({
        r: tx?.signature.r ? BigInt(tx.signature.r) : BigInt(0),
        s: tx?.signature.s ? BigInt(tx.signature.s) : BigInt(0),
      })

      const contractCreationBit = tx.to ? BigInt(0) : BigInt(1)
      this.contractCreationBits |= contractCreationBit << BigInt(idx + offset)

      if (tx.to) {
        this.txTos.push(tx.to)
      }

      const yParityBit = BigInt(
        this.convertVToYParity(tx?.signature.v ?? 0, tx.type)
      )
      this.yParityBits |= yParityBit << BigInt(idx + offset)

      this.txNonces.push(Number(tx.nonce))
      this.txGases.push(Number(tx.gasLimit))
      this.txDatas.push(newSpanBatchTx(tx).marshalBinary())
      this.txTypes.push(txType)

      // append metis extra fields
      this.queueOriginBits |=
        BigInt(tx.queueOrigin === QueueOrigin.Sequencer) << BigInt(idx + offset)
      if (tx.queueOrigin !== QueueOrigin.Sequencer) {
        this.l1TxOrigins.push(tx.l1TxOrigin)
      }
      this.txSeqSigs.push({
        r: tx.seqR ? toBigInt(tx.seqR) : BigInt(0),
        s: tx.seqS ? toBigInt(tx.seqS) : BigInt(0),
      })
      const seqYParityBit = tx.seqV ? toBigInt(tx.seqV) : BigInt(0)
      this.seqYParityBits |= seqYParityBit << BigInt(idx + offset)
    }

    this.totalBlockTxCount += txs.length
  }

  encode(): Uint8Array {
    const writer = new Writer()
    this.encodeContractCreationBits(writer)
    this.encodeYParityBits(writer)
    this.encodeTxSigsRS(writer)
    this.encodeTxTos(writer)
    this.encodeTxDatas(writer)
    this.encodeTxNonces(writer)
    this.encodeTxGases(writer)
    this.encodeProtectedBits(writer)

    // encode metis extra fields
    this.encodeQueueOriginBits(writer)
    this.encodeSeqYParityBits(writer)
    this.encodeTxSeqSigsRS(writer)
    this.encodeL1TxOrigins(writer)
    return writer.getData()
  }

  private convertVToYParity(v: number, txType: number): number {
    switch (txType) {
      case 0: // Legacy
        return this.isProtectedV(v, txType)
          ? Number((v - 35) & 1)
          : Number(v - 27)
      case 1: // AccessList
      case 2: // DynamicFee
        return Number(v)
      default:
        throw new Error(`Invalid tx type: ${txType}`)
    }
  }

  private isProtectedV(v: number, txType: number): boolean {
    return txType === 0 ? v !== 27 && v !== 28 : true
  }

  private encodeQueueOriginBits(writer: Writer): void {
    encodeSpanBatchBits(writer, this.totalBlockTxCount, this.queueOriginBits)
  }

  private encodeContractCreationBits(writer: Writer): void {
    encodeSpanBatchBits(
      writer,
      this.totalBlockTxCount,
      this.contractCreationBits
    )
  }

  private encodeYParityBits(writer: Writer): void {
    encodeSpanBatchBits(writer, this.totalBlockTxCount, this.yParityBits)
  }

  private encodeTxSigsRS(writer: Writer): void {
    for (const txSig of this.txSigs) {
      writer.writeBytes(zeroPadValue(toBeArray(txSig.r), 32))
      writer.writeBytes(zeroPadValue(toBeArray(txSig.s), 32))
    }
  }

  private encodeTxTos(writer: Writer): void {
    for (const txTo of this.txTos) {
      writer.writeBytes(getBytes(txTo))
    }
  }

  private encodeL1TxOrigins(writer: Writer): void {
    for (const l1TxOrigin of this.l1TxOrigins) {
      writer.writeBytes(getBytes(l1TxOrigin))
    }
  }

  private encodeTxDatas(writer: Writer): void {
    for (const txData of this.txDatas) {
      writer.writeBytes(txData)
    }
  }

  private encodeTxNonces(writer: Writer): void {
    for (const txNonce of this.txNonces) {
      writer.writeVarInt(txNonce)
    }
  }

  private encodeTxGases(writer: Writer): void {
    for (const txGas of this.txGases) {
      writer.writeVarInt(txGas)
    }
  }

  private encodeProtectedBits(writer: Writer): void {
    encodeSpanBatchBits(writer, this.totalLegacyTxCount, this.protectedBits)
  }

  private encodeTxSeqSigsRS(writer: Writer): void {
    for (const txSeqSig of this.txSeqSigs) {
      writer.writeBytes(zeroPadValue(toBeArray(txSeqSig.r), 32))
      writer.writeBytes(zeroPadValue(toBeArray(txSeqSig.s), 32))
    }
  }

  private encodeSeqYParityBits(writer: Writer): void {
    encodeSpanBatchBits(writer, this.totalBlockTxCount, this.seqYParityBits)
  }
}

interface SpanBatchSignature {
  r: bigint
  s: bigint
}

interface SpanBatchSequencerSignature {
  r: bigint
  s: bigint
}
