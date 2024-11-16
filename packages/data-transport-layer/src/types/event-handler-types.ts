import { TransportDB } from '../db/transport-db'
import {
  BlockEntry,
  StateRootBatchEntry,
  StateRootEntry,
  TransactionBatchEntry,
  TransactionEntry,
} from './database-types'
import { EventLog, Provider } from 'ethersv6'

export type TypedEthersEvent<T> = EventLog & {
  args: T
}

export type GetExtraDataHandler<TEventArgs, TExtraData> = (
  event?: TypedEthersEvent<TEventArgs>,
  l1RpcProvider?: Provider
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

export interface ContextfulData {
  context?: any
}

export interface EventHandlerSet<
  TEventArgs,
  TExtraData extends ContextfulData,
  TParsedEvent
> {
  getExtraData: GetExtraDataHandler<TEventArgs, TExtraData>
  parseEvent: ParseEventHandler<TEventArgs, TExtraData, TParsedEvent>
  storeEvent: StoreEventHandler<TParsedEvent>
}

export type GetExtraDataHandlerAny<TExtraData> = (
  event?: any,
  l1RpcProvider?: Provider
) => Promise<TExtraData>

export interface EventHandlerSetAny<
  TExtraData extends ContextfulData,
  TParsedEvent
> {
  getExtraData: GetExtraDataHandlerAny<TExtraData>
  parseEvent: ParseEventHandler<any, TExtraData, TParsedEvent>
  storeEvent: StoreEventHandler<TParsedEvent>
}

export interface SequencerBatchAppendedExtraData extends ContextfulData {
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
  blobCount: number
}

export interface SequencerBatchAppendedParsedEvent {
  transactionBatchEntry: TransactionBatchEntry
  transactionEntries: TransactionEntry[]
}

export interface SequencerBatchInboxParsedEvent {
  transactionBatchEntry: TransactionBatchEntry
  blockEntries: BlockEntry[]
}

export interface StateBatchAppendedExtraData extends ContextfulData {
  timestamp: number
  blockNumber: number
  submitter: string
  l1TransactionHash: string
  l1TransactionData: string
}

export interface StateBatchAppendedParsedEvent extends ContextfulData {
  stateRootBatchEntry: StateRootBatchEntry
  stateRootEntries: StateRootEntry[]
}
