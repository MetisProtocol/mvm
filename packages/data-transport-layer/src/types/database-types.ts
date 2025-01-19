export interface DecodedSequencerBatchTransaction {
  sig: {
    r: string
    s: string
    v: number
  }
  value: string
  gasLimit: string
  gasPrice: string
  nonce: string
  target: string
  data: string
}

export interface EnqueueEntry {
  index: number
  target: string
  data: string
  gasLimit: string
  origin: string
  blockNumber: number
  timestamp: number
}

export interface TransactionEntry {
  index: number
  batchIndex: number
  data: string
  blockNumber: number
  timestamp: number
  gasLimit: string
  target: string
  origin: string
  value: string
  queueOrigin: 'sequencer' | 'l1'
  queueIndex: number | null
  decoded: DecodedSequencerBatchTransaction | null
  confirmed: boolean
  seqSign: string | null
}

interface BatchEntry {
  index: number
  blockNumber: number
  timestamp: number
  submitter: string
  size: number
  root: string
  prevTotalElements: number
  extraData: string
  l1TransactionHash: string
}

export type TransactionBatchEntry = BatchEntry
export type StateRootBatchEntry = BatchEntry

export interface StateRootEntry {
  index: number
  batchIndex: number
  value: string
  confirmed: boolean
}

export interface VerifierResultEntry {
  index: number // match StateRootEntry's index
  stateRoot: string
  verifierRoot: string
  timestamp: number
}

export interface VerifierStakeEntry {
  index: number
  sender: string
  chainId: number
  batchIndex: number
  blockNumber: number
  amount: string
  l1BlockNumber: number
  timestamp: number
}

export interface AppendBatchElementEntry {
  index: number
  chainId: number
  batchIndex: number
  shouldStartAtElement: number
  totalElementsToAppend: number
  txBatchSize: number
  txBatchTime: number
  root: string
  l1BlockNumber: number
  timestamp: number
}

export interface BlockEntry {
  index: number
  batchIndex: number
  timestamp: number
  transactions: TransactionEntry[]
  confirmed: boolean
  extraData?: string
}

export enum SenderType {
  Batch = 0,
  Blob = 1,
}

export interface InboxSenderSetEntry {
  index: number
  blockNumber: number
  inboxSender: string
  senderType: SenderType
}

export interface Upgrades {
  fpUpgraded: boolean
}
