import { EventArgsTransactionEnqueued } from '@metis.io/core-utils'

/* Imports: Internal */
import { EnqueueEntry, EventHandlerSet } from '../../../types'
import { MissingElementError } from './errors'
import { toNumber } from 'ethers'

export const handleEventsTransactionEnqueued: EventHandlerSet<
  EventArgsTransactionEnqueued,
  null,
  EnqueueEntry
> = {
  getExtraData: async () => {
    return null
  },
  parseEvent: async (event) => {
    return {
      index: toNumber(event.args._queueIndex),
      target: event.args._target,
      data: event.args._data,
      gasLimit: event.args._gasLimit.toString(),
      origin: event.args._l1TxOrigin,
      blockNumber: toNumber(event.blockNumber),
      timestamp:
        toNumber(event.blockNumber) >= 14570938
          ? Math.floor(new Date().getTime() / 1000)
          : toNumber(event.args._timestamp),
      ctcIndex: null,
    }
  },
  storeEvent: async (entry, db) => {
    // Defend against situations where we missed an event because the RPC provider
    // (infura/alchemy/whatever) is missing an event.
    if (entry.index > 0) {
      const prevEnqueueEntry = await db.getEnqueueByIndex(entry.index - 1)

      // We should *alwaus* have a previous enqueue entry here.
      if (prevEnqueueEntry === null) {
        throw new MissingElementError('TransactionEnqueued')
      }
    }

    await db.putEnqueueEntries([entry])
  },
}
