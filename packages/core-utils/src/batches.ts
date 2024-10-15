/* External Imports */
import { ethers } from 'ethers'

export interface RollupInfo {
  mode: 'sequencer' | 'verifier'
  syncing: boolean
  ethContext: {
    blockNumber: number
    timestamp: number
  }
  rollupContext: {
    index: number
    queueIndex: number
  }
}

export enum QueueOrigin {
  Sequencer = 'sequencer',
  L1ToL2 = 'l1',
}

/**
 * Transaction & Blocks. These are the true data-types we expect
 * from running a batch submitter.
 */
export interface L2Transaction extends ethers.TransactionResponse {
  l1BlockNumber: number
  l1TxOrigin: string
  queueOrigin: string
  rawTransaction: string
  seqR: string | undefined | null
  seqS: string | undefined | null
  seqV: string | undefined | null
}

export interface L2Block extends ethers.Block {
  l2Transactions: L2Transaction[]
}

/**
 * BatchElement & Batch. These are the data-types of the compressed / batched
 * block data we submit to L1.
 */
export interface BatchElement {
  stateRoot: string
  isSequencerTx: boolean
  rawTransaction: undefined | string
  timestamp: number
  blockNumber: number
  seqSign: string | undefined | null
}

export type Batch = BatchElement[]
