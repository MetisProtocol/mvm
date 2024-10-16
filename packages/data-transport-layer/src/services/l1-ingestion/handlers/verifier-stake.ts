/* Imports: Internal */
import {
  EventArgsVerifierStake,
  EventHandlerSet,
  VerifierStakeEntry,
} from '../../../types'

/* Imports: External */
import { toNumber } from 'ethers'

export const handleEventsVerifierStake: EventHandlerSet<
  EventArgsVerifierStake,
  null,
  VerifierStakeEntry
> = {
  getExtraData: async () => {
    return null
  },
  parseEvent: async (event) => {
    // console.log(`got event VerifierStake: ${JSON.stringify(event.args)}`)
    const eventBlock = await event.getBlock()
    return {
      index: toNumber(event.args._batchIndex),
      sender: event.args._sender,
      chainId: toNumber(event.args._chainId),
      batchIndex: toNumber(event.args._batchIndex),
      blockNumber: toNumber(event.args._blockNumber),
      amount: event.args._amount.toString(),
      l1BlockNumber: toNumber(event.blockNumber),
      timestamp: eventBlock.timestamp,
    }
  },
  storeEvent: async (entry, db) => {
    // console.log(`start save VerifierStakeEntry ${JSON.stringify(entry)}`)
    if (!entry) {
      return
    }
    await db.putVerifierStakeEntries([entry])
  },
}
