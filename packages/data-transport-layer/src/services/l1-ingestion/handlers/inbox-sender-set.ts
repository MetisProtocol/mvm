/* Imports: Internal */
import { EventArgsInboxSenderSet } from '@metis.io/core-utils'
import { EventHandlerSet, InboxSenderSetEntry } from '../../../types'

export const handleInboxSenderSet: EventHandlerSet<
  EventArgsInboxSenderSet,
  null,
  InboxSenderSetEntry
> = {
  getExtraData: async () => {
    return null
  },
  parseEvent: async (event) => {
    return {
      index: event.args.blockNumber.toNumber(),
      blockNumber: event.args.blockNumber.toNumber(),
      inboxSender: event.args.inboxSender,
    }
  },
  storeEvent: async (entry, db) => {
    if (!entry) {
      return
    }
    await db.putInboxSenderSetEntries([entry])
  },
}
