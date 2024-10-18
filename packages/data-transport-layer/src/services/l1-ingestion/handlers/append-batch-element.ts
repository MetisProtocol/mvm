/* Imports: Internal */
import {
  AppendBatchElementEntry,
  EventArgsAppendBatchElement,
  EventHandlerSet,
} from '../../../types'

/* Imports: External */
import { toNumber } from 'ethersv6'

export const handleEventsAppendBatchElement: EventHandlerSet<
  EventArgsAppendBatchElement,
  null,
  AppendBatchElementEntry
> = {
  getExtraData: async () => {
    return null
  },
  parseEvent: async (event) => {
    // console.log(`got event AppendBatchElement: ${JSON.stringify(event.args)}`)
    const eventBlock = await event.getBlock()
    return {
      index: toNumber(event.args._batchIndex),
      chainId: toNumber(event.args._chainId),
      batchIndex: toNumber(event.args._batchIndex),
      shouldStartAtElement: event.args._shouldStartAtElement,
      totalElementsToAppend: event.args._totalElementsToAppend,
      txBatchSize: toNumber(event.args._txBatchSize),
      txBatchTime: toNumber(event.args._txBatchTime),
      root: event.args._root,
      l1BlockNumber: toNumber(event.blockNumber),
      timestamp: eventBlock.timestamp,
    }
  },
  storeEvent: async (entry, db) => {
    // console.log(`start save AppendBatchElementEntry ${JSON.stringify(entry)}`)
    if (!entry) {
      return
    }
    await db.putBatchElementEntries([entry])
  },
}
