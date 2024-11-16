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
      index: event.args._blockNumber.toNumber(),
      blockNumber: event.args._blockNumber.toNumber(),
      inboxSender: event.args._inboxSender,
    }
  },
  storeEvent: async (entry, db) => {
    if (!entry) {
      return
    }
    await db.putInboxSenderSetEntries([entry])
  },
}
