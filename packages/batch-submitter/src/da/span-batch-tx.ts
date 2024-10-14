import { ethers, toBigInt } from 'ethers'
import { Writer } from './types'
import RLP from 'rlp'
import { L2Transaction } from '@metis.io/core-utils'
import { AccessList } from '@ethersproject/transactions'

interface SpanBatchTxData {
  txType(): number
  marshalBinary(): Uint8Array
}

export class SpanBatchTx {
  inner: SpanBatchTxData

  constructor(inner: SpanBatchTxData) {
    this.inner = inner
  }

  type(): number {
    return this.inner.txType()
  }

  marshalBinary(): Uint8Array {
    if (this.type() === 0) {
      // for legacy tx, we don't need to encode the type
      return this.inner.marshalBinary()
    }

    // for access list and dynamic fee tx, we need to encode the type first
    const writer = new Writer()
    writer.writeUint8(this.type())
    writer.writeBytes(this.inner.marshalBinary())
    return writer.getData()
  }
}

class SpanBatchLegacyTxData implements SpanBatchTxData {
  value: bigint
  gasPrice: bigint
  data: string

  constructor(tx: L2Transaction) {
    this.value = tx.value.toBigInt()
    this.gasPrice = tx.gasPrice!.toBigInt()
    this.data = tx.data
  }

  txType(): number {
    return 0
  }

  marshalBinary(): Uint8Array {
    return RLP.encode([this.value, this.gasPrice, this.data])
  }
}

class SpanBatchAccessListTxData implements SpanBatchTxData {
  value: bigint
  gasPrice: bigint
  data: string
  accessList: AccessList

  constructor(tx: L2Transaction) {
    this.value = tx.value.toBigInt()
    this.gasPrice = tx.gasPrice!.toBigInt()
    this.data = tx.data
    this.accessList = tx.accessList!
  }

  txType(): number {
    return 1
  }

  marshalBinary(): Uint8Array {
    return RLP.encode([
      this.value,
      this.gasPrice,
      this.data,
      this.accessList.map((al) => [al.address, al.storageKeys]),
    ])
  }
}

class SpanBatchDynamicFeeTxData implements SpanBatchTxData {
  value: bigint
  maxPriorityFeePerGas: bigint
  maxFeePerGas: bigint
  data: string
  accessList: AccessList

  constructor(tx: L2Transaction) {
    this.value = tx.value.toBigInt()
    this.maxPriorityFeePerGas = tx.maxPriorityFeePerGas!.toBigInt()
    this.maxFeePerGas = tx.maxFeePerGas!.toBigInt()
    this.data = tx.data
    this.accessList = tx.accessList!
  }

  txType(): number {
    return 2
  }

  marshalBinary(): Uint8Array {
    return RLP.encode([
      this.value,
      this.maxPriorityFeePerGas,
      this.maxFeePerGas,
      this.data,
      this.accessList.map((al) => [al.address, al.storageKeys]),
    ])
  }
}

export const newSpanBatchTx = (tx: L2Transaction): SpanBatchTx => {
  switch (tx.type) {
    case 0:
      return new SpanBatchTx(new SpanBatchLegacyTxData(tx))
    case 1:
      return new SpanBatchTx(new SpanBatchAccessListTxData(tx))
    case 2:
      return new SpanBatchTx(new SpanBatchDynamicFeeTxData(tx))
    default:
      throw new Error(`Invalid tx type: ${tx.type}`)
  }
}
