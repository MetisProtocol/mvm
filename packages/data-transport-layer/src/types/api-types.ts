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
