/* Imports: Internal */
import { EventArgsInboxSenderSet } from '@localtest911/core-utils'
import {
  EventHandlerSet,
  InboxSenderSetEntry,
  SenderType,
} from '../../../types'
import { toNumber } from 'ethersv6'

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
      index: toNumber(event.args._blockNumber),
      blockNumber: toNumber(event.args.blockNumber),
      inboxSender: event.args._inboxSender,
      senderType: event.args._senderType,
    }
  },
  storeEvent: async (entry, db) => {
    if (!entry) {
      return
    }
    await db.putInboxSenderSetEntries([entry])
  },
}
