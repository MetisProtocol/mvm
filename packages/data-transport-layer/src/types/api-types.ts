import {
  EnqueueEntry,
  StateRootBatchEntry,
  StateRootEntry,
  TransactionBatchEntry,
  TransactionEntry,
  VerifierResultEntry,
  VerifierStakeEntry,
  AppendBatchElementEntry,
  BlockEntry,
} from './database-types'

export type EnqueueResponse = EnqueueEntry & {
  ctcIndex: number | null
}

export interface TransactionResponse {
  batch: TransactionBatchEntry
  transaction: TransactionEntry
}

export interface TransactionBatchResponse {
  batch: TransactionBatchEntry
  transactions: TransactionEntry[]
}

export interface BlockResponse {
  batch: TransactionBatchEntry
  block: BlockEntry
}

export interface BlockBatchResponse {
  batch: TransactionBatchEntry
  blocks: BlockEntry[]
}

export interface StateRootResponse {
  batch: StateRootBatchEntry
  stateRoot: StateRootEntry
}

export interface StateRootBatchResponse {
  batch: StateRootBatchEntry
  stateRoots: StateRootEntry[]
}

export interface ContextResponse {
  blockNumber: number
  timestamp: number
  blockHash: string
}

export interface GasPriceResponse {
  gasPrice: string
}

export interface VerifierResultResponse {
  verify: VerifierResultEntry
  batch: StateRootBatchEntry
  stateRoots: string[]
  success: boolean
}

export type SyncingResponse =
  | {
  syncing: true
  highestKnownTransactionIndex: number
  currentTransactionIndex: number
}
  | {
  syncing: false
  currentTransactionIndex: number
}

export interface VerifierStakeResponse {
  verifierStake: VerifierStakeEntry
}

export interface AppendBatchElementResponse {
  batchElement: AppendBatchElementEntry
}

export interface HighestResponse {
  blockNumber: number
}

export interface BlockID {
  hash: string
  number: number
}

export interface L2BlockRef {
  hash: string
  number: number
  parentHash: string
  timestamp: number
  l1origin: BlockID
  sequenceNumber: number
}

export interface L1BlockRef {
  hash: string
  number: number
  parentHash: string
  time: number
}

export interface SyncStatusResponse {
  currentL1: L1BlockRef
  currentL1Finalized?: L1BlockRef // Deprecated by Optimism
  headL1: L1BlockRef
  safeL1: L1BlockRef
  finalizedL1: L1BlockRef
  unsafeL2: L2BlockRef
  safeL2: L2BlockRef
  finalizedL2: L2BlockRef
  pendingSafeL2: L2BlockRef
}
