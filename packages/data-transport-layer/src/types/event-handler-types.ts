import { BaseProvider } from '@ethersproject/providers'
import { Event } from '@ethersproject/contracts'

import { TransportDB } from '../db/transport-db'
import {
  TransactionBatchEntry,
  TransactionEntry,
  StateRootBatchEntry,
  StateRootEntry,
  VerifierStakeEntry,
  AppendBatchElementEntry,
  BlockEntry,
} from './database-types'

export type TypedEthersEvent<T> = Event & {
  args: T
}

export type GetExtraDataHandler<TEventArgs, TExtraData> = (
  event?: TypedEthersEvent<TEventArgs>,
  l1RpcProvider?: BaseProvider
) => Promise<TExtraData>

export type ParseEventHandler<TEventArgs, TExtraData, TParsedEvent> = (
  event: TypedEthersEvent<TEventArgs>,
  extraData: TExtraData,
  l2ChainId: number,
  options: any
) => Promise<TParsedEvent>

export type StoreEventHandler<TParsedEvent> = (
  parsedEvent: TParsedEvent,
  db: TransportDB,
  options?: any
) => Promise<void>

export interface EventHandlerSet<TEventArgs, TExtraData, TParsedEvent> {
  getExtraData: GetExtraDataHandler<TEventArgs, TExtraData>
  parseEvent: ParseEventHandler<TEventArgs, TExtraData, TParsedEvent>
  storeEvent: StoreEventHandler<TParsedEvent>
}

export type GetExtraDataHandlerAny<TExtraData> = (
  event?: any,
  l1RpcProvider?: BaseProvider
) => Promise<TExtraData>

export interface EventHandlerSetAny<TExtraData, TParsedEvent> {
  getExtraData: GetExtraDataHandlerAny<TExtraData>
  parseEvent: ParseEventHandler<any, TExtraData, TParsedEvent>
  storeEvent: StoreEventHandler<TParsedEvent>
}

export interface SequencerBatchAppendedExtraData {
  timestamp: number
  blockNumber: number
  submitter: string
  l1TransactionData: string
  l1TransactionHash: string
  gasLimit: string

  // Stuff from TransactionBatchAppended.
  prevTotalElements: bigint
  batchIndex: bigint
  batchSize: bigint
  batchRoot: string
  batchExtraData: string

  // blob related
  blobIndex: number
}

export interface SequencerBatchAppendedParsedEvent {
  transactionBatchEntry: TransactionBatchEntry
  transactionEntries: TransactionEntry[]
}

export interface SequencerBatchInboxParsedEvent {
  transactionBatchEntry: TransactionBatchEntry
  blockEntries: BlockEntry[]
}

export interface StateBatchAppendedExtraData {
  timestamp: number
  blockNumber: number
  submitter: string
  l1TransactionHash: string
  l1TransactionData: string
}

export interface StateBatchAppendedParsedEvent {
  stateRootBatchEntry: StateRootBatchEntry
  stateRootEntries: StateRootEntry[]
}
