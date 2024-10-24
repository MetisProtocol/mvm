import cloneDeep from 'lodash/cloneDeep'
import {
  Block,
  BlockParams,
  ethers,
  JsonRpcProvider,
  Network,
  toBigInt,
  toNumber,
  TransactionReceipt,
  TransactionReceiptParams,
  TransactionResponse,
  TransactionResponseParams,
} from 'ethersv6'
import { L2Block, L2Transaction } from './batches'

/**
 * Helper for adding additional L2 context to transactions
 */

export class L2Provider extends JsonRpcProvider {
  toL2Transaction(
    tx: TransactionResponseParams,
    txResponse: TransactionResponse
  ): L2Transaction {
    const anyTx = tx as any
    const txResponseAny = txResponse as any

    txResponseAny.l1BlockNumber = toNumber(anyTx.l1BlockNumber)
    txResponseAny.l1TxOrigin = anyTx.l1TxOrigin
    txResponseAny.queueOrigin = anyTx.queueOrigin
    txResponseAny.rawTransaction = anyTx.rawTransaction
    txResponseAny.seqV = toNumber(anyTx.seqV || 0)
    txResponseAny.seqR = toBigInt(anyTx.seqR || 0)
    txResponseAny.seqS = toBigInt(anyTx.seqS || 0)

    return txResponseAny as L2Transaction
  }

  _wrapBlock(value: BlockParams, network: Network) {
    const originalTxs = value.transactions
    const block = super._wrapBlock(value, network)
    const blockAny = block as any
    blockAny.l2Transactions = new Array<L2Transaction>(
      block.transactions ? block.transactions.length : 0
    )
    blockAny.l2TransactionPromises = originalTxs.map(async (tx, index) => {
      let txResponse: TransactionResponse
      if (typeof tx === 'string') {
        txResponse = await this.getTransaction(tx)
      } else {
        txResponse = this.toL2Transaction(
          tx,
          block.prefetchedTransactions[index]
        )
      }
      blockAny.l2Transactions[index] = txResponse as L2Transaction
      return txResponse
    })

    return blockAny as L2Block
  }

  _wrapTransactionResponse(
    tx: TransactionResponseParams,
    network: Network
  ): TransactionResponse {
    return this.toL2Transaction(tx, super._wrapTransactionResponse(tx, network))
  }

  _wrapTransactionReceipt(
    receipt: TransactionReceiptParams,
    network: Network
  ): TransactionReceipt {
    const txReceipt = super._wrapTransactionReceipt(receipt, network)

    const anyReceipt = receipt as any
    const txReceiptAny = txReceipt as any

    txReceiptAny.l1GasPrice = toBigInt(anyReceipt.l1GasPrice || 0)
    txReceiptAny.l1GasUsed = toBigInt(anyReceipt.l1GasUsed || 0)
    txReceiptAny.l1Fee = toBigInt(anyReceipt.l1Fee || 0)
    txReceiptAny.l1FeeScalar = parseFloat(anyReceipt.l1FeeScalar || 0)

    return txReceiptAny as TransactionReceipt
  }
}
