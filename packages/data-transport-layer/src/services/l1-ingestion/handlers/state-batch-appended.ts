/* Imports: External */
import { getContractDefinition } from '@localtest911/contracts'
import { Interface, toNumber, ethers } from 'ethersv6'

/* Imports: Internal */
import {
  EventArgsStateBatchAppended,
  StateRootBatchEntry,
  StateBatchAppendedExtraData,
  StateBatchAppendedParsedEvent,
  StateRootEntry,
  EventHandlerSet,
} from '../../../types'
import { MissingElementError } from './errors'

export const handleEventsStateBatchAppended: EventHandlerSet<
  EventArgsStateBatchAppended,
  StateBatchAppendedExtraData,
  StateBatchAppendedParsedEvent
> = {
  getExtraData: async (event, l1RpcProvider) => {
    const eventBlock = await l1RpcProvider.getBlock(event.blockNumber, true)
    const l1TransactionHash = eventBlock.transactions.find(
      (i) => i === event.transactionHash
    )

    if (!l1TransactionHash) {
      throw new Error(`Could not find L1 transaction: ${event.transactionHash}`)
    }

    const l1Transaction = await l1RpcProvider.getTransaction(l1TransactionHash)

    return {
      timestamp: eventBlock.timestamp,
      blockNumber: eventBlock.number,
      submitter: l1Transaction.from,
      l1TransactionHash: l1Transaction.hash,
      l1TransactionData: l1Transaction.data,
    }
  },
  parseEvent: async (event, extraData) => {
    const stateRoots = new Interface(
      getContractDefinition('IMVMStateCommitmentChain').abi
    ).decodeFunctionData(
      'appendStateBatchByChainId',
      extraData.l1TransactionData
    )[1]

    const stateRootEntries: StateRootEntry[] = []
    for (let i = 0; i < stateRoots.length; i++) {
      stateRootEntries.push({
        index: toNumber(event.args._prevTotalElements) + i,
        batchIndex: toNumber(event.args._batchIndex),
        value: stateRoots[i],
        confirmed: true,
      })
    }

    // Using .toNumber() here and in other places because I want to move everything to use
    // BigNumber + hex, but that'll take a lot of work. This makes it easier in the future.
    const stateRootBatchEntry: StateRootBatchEntry = {
      index: toNumber(event.args._batchIndex),
      blockNumber: toNumber(extraData.blockNumber),
      timestamp: toNumber(extraData.timestamp),
      submitter: extraData.submitter,
      size: toNumber(event.args._batchSize),
      root: event.args._batchRoot,
      prevTotalElements: toNumber(event.args._prevTotalElements),
      extraData: event.args._extraData,
      l1TransactionHash: extraData.l1TransactionHash,
    }

    return {
      stateRootBatchEntry,
      stateRootEntries,
    }
  },
  storeEvent: async (entry, db) => {
    // Defend against situations where we missed an event because the RPC provider
    // (infura/alchemy/whatever) is missing an event.
    if (entry.stateRootBatchEntry.index > 0) {
      const prevStateRootBatchEntry = await db.getStateRootBatchByIndex(
        entry.stateRootBatchEntry.index - 1
      )

      // We should *always* have a previous batch entry here.
      if (prevStateRootBatchEntry === null) {
        throw new MissingElementError('StateBatchAppended')
      }
    }

    const packedHeader = ethers.AbiCoder.defaultAbiCoder().encode(
      ['bytes32', 'uint256', 'uint256', 'bytes'],
      [
        entry.stateRootBatchEntry.root,
        entry.stateRootBatchEntry.size,
        entry.stateRootBatchEntry.prevTotalElements,
        entry.stateRootBatchEntry.extraData,
      ]
    )

    const headerHash = ethers.keccak256(packedHeader)

    await db.putStateRootBatchEntries([entry.stateRootBatchEntry])
    await db.putStateRootEntries(entry.stateRootEntries)
    await db.putStateRootBatchHeaderHashIndex(
      headerHash,
      entry.stateRootBatchEntry.index
    )
  },
}
