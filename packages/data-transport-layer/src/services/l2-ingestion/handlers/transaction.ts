/* Imports: External */
import { ethers, toNumber } from 'ethers'

/* Imports: Internal */
import { TransportDB } from '../../../db/transport-db'
import {
  DecodedSequencerBatchTransaction,
  StateRootEntry,
  TransactionEntry,
} from '../../../types'
import { padHexString, parseSignatureVParam } from '../../../utils'

export const handleSequencerBlock = {
  parseBlock: async (
    block: any,
    chainId: number
  ): Promise<{
    transactionEntry: TransactionEntry
    stateRootEntry: StateRootEntry
  }> => {
    const transaction = block.transactions[0]
    const transactionIndex =
      transaction.index === null || transaction.index === undefined
        ? toNumber(transaction.blockNumber) - 1
        : toNumber(transaction.index)

    let transactionEntry: Partial<TransactionEntry> = {
      // Legacy support.
      index: transactionIndex,
      value: transaction.value,
      batchIndex: null,
      blockNumber: toNumber(transaction.l1BlockNumber),
      timestamp: toNumber(transaction.l1Timestamp),
      queueOrigin: transaction.queueOrigin,
      confirmed: false,
    }

    if (transaction.queueOrigin === 'sequencer') {
      const decodedTransaction: DecodedSequencerBatchTransaction = {
        sig: {
          v: parseSignatureVParam(transaction.v, chainId),
          r: padHexString(transaction.r, 32),
          s: padHexString(transaction.s, 32),
        },
        value: transaction.value,
        gasLimit: toNumber(transaction.gas).toString(),
        gasPrice: toNumber(transaction.gasPrice).toString(),
        nonce: toNumber(transaction.nonce).toString(),
        target: transaction.to,
        data: transaction.input,
      }

      transactionEntry = {
        ...transactionEntry,
        gasLimit: toNumber(0).toString(),
        target: ethers.ZeroAddress,
        origin: null,
        data: ethers.Transaction.from({
          value: transaction.value,
          gasLimit: transaction.gas,
          gasPrice: transaction.gasPrice,
          nonce: transaction.nonce,
          to: transaction.to,
          data: transaction.input,
          chainId,
          signature: {
            v: toNumber(transaction.v),
            r: padHexString(transaction.r, 32),
            s: padHexString(transaction.s, 32),
          },
        }).serialized,
        decoded: decodedTransaction,
        queueIndex: null,
      }
      // l2 de-seq sign
      if (transaction.seqR) {
        transactionEntry.seqSign = `${transaction.seqR},${transaction.seqS},${transaction.seqV}`
      }
    } else {
      transactionEntry = {
        ...transactionEntry,
        gasLimit: toNumber(transaction.gas).toString(),
        target: ethers.getAddress(transaction.to),
        origin: ethers.getAddress(transaction.l1TxOrigin),
        data: transaction.input,
        decoded: null,
        queueIndex:
          transaction.queueIndex === null ||
          transaction.queueIndex === undefined
            ? toNumber(transaction.nonce)
            : toNumber(transaction.queueIndex),
      }
    }

    const stateRootEntry: StateRootEntry = {
      index: transactionIndex,
      batchIndex: null,
      value: block.stateRoot,
      confirmed: false,
    }

    return {
      transactionEntry: transactionEntry as TransactionEntry, // Not the cleanest thing in the world. Could be improved.
      stateRootEntry,
    }
  },
  storeBlock: async (
    entry: {
      transactionEntry: TransactionEntry
      stateRootEntry: StateRootEntry
    },
    db: TransportDB
  ): Promise<void> => {
    // Having separate indices for confirmed/unconfirmed means we never have to worry about
    // accidentally overwriting a confirmed transaction with an unconfirmed one. Unconfirmed
    // transactions are purely extra information.
    await db.putUnconfirmedTransactionEntries([entry.transactionEntry])
    await db.putUnconfirmedStateRootEntries([entry.stateRootEntry])
  },
}
