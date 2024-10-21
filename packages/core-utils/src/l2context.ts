import cloneDeep from 'lodash/cloneDeep'
import {
  Block,
  ethers,
  formatBlock,
  formatTransactionReceipt,
  formatTransactionResponse,
  toBigInt,
  toNumber,
  TransactionResponseParams,
} from 'ethersv6'
import { L2Block, L2Transaction } from './batches'

/**
 * Helper for adding additional L2 context to transactions
 */
export const injectL2Context = (l1Provider: ethers.JsonRpcProvider) => {
  const provider = cloneDeep(l1Provider)

  const toL2Transaction = (tx: TransactionResponseParams) => {
    const formattedTx = formatTransactionResponse(tx)
    const txResponse = new ethers.TransactionResponse(formattedTx, provider)

    const anyTx = tx as any
    const txResponseAny = txResponse as any

    txResponseAny.l1BlockNumber = toNumber(anyTx.l1BlockNumber)
    txResponseAny.l1TxOrigin = anyTx.l1TxOrigin
    txResponseAny.queueOrigin = anyTx.queueOrigin
    txResponseAny.rawTransaction = anyTx.rawTransaction
    txResponseAny.seqV = toNumber(anyTx.seqV)
    txResponseAny.seqR = toBigInt(anyTx.seqR)
    txResponseAny.seqS = toBigInt(anyTx.seqS)

    return txResponseAny as L2Transaction
  }

  provider._wrapBlock = (blockParams, network): L2Block => {
    const formattedBlock = formatBlock(blockParams)
    formattedBlock.stateRoot = blockParams.stateRoot

    const block = new Block(formattedBlock, provider)
    if (!block.transactions) {
      // tx not retrieved
      return block as L2Block
    }

    const anyBlock = block as any

    anyBlock.l2Transactions = blockParams.transactions.map((tx) => {
      if (typeof tx === 'string') {
        return block
          .getTransaction(tx)
          .then((txResponseParam) => toL2Transaction(txResponseParam))
      } else {
        return Promise.resolve(toL2Transaction(tx))
      }
    })

    return anyBlock as L2Block
  }

  provider._wrapTransactionResponse = (tx, network) => {
    return toL2Transaction(tx)
  }

  provider._wrapTransactionReceipt = (receipt, network) => {
    const formattedReceipt = formatTransactionReceipt(receipt)
    const txReceipt = new ethers.TransactionReceipt(formattedReceipt, provider)

    const anyReceipt = receipt as any
    const txReceiptAny = txReceipt as any

    txReceiptAny.l1GasPrice = toBigInt(anyReceipt.l1GasPrice)
    txReceiptAny.l1GasUsed = toBigInt(anyReceipt.l1GasUsed)
    txReceiptAny.l1Fee = toBigInt(anyReceipt.l1Fee)
    txReceiptAny.l1FeeScalar = parseFloat(anyReceipt.l1FeeScalar)

    return txReceipt
  }

  return provider
}
