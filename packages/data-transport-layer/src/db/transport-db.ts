/* Imports: External */
import { LevelUp } from 'levelup'
import level from 'level'
import { BigNumber } from 'ethers'
// 1088 patch only
import patch01 from './patch-01'
/* Imports: Internal */
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
} from '../types/database-types'
import { SimpleDB } from './simple-db'

const TRANSPORT_DB_KEYS = {
  ENQUEUE: `enqueue`,
  ENQUEUE_CTC_INDEX: `ctc:enqueue`,
  TRANSACTION: `transaction`,
  UNCONFIRMED_TRANSACTION: `unconfirmed:transaction`,
  UNCONFIRMED_HIGHEST: `unconfirmed:highest`,
  TRANSACTION_BATCH: `batch:transaction`,
  STATE_ROOT: `stateroot`,
  UNCONFIRMED_STATE_ROOT: `unconfirmed:stateroot`,
  STATE_ROOT_BATCH: `batch:stateroot`,
  STARTING_L1_BLOCK: `l1:starting`,
  STARTING_L1_BATCH_INDEX: `l1:batchindex`,
  HIGHEST_L2_BLOCK: `l2:highest`,
  HIGHEST_SYNCED_BLOCK: `synced:highest`,
  HIGHEST_SYNCED_BATCH_INDEX: `synced:batchindex`,
  VERIFIER_FAILED: `verifier:failed`,
  VERIFIER_SUCCESSFUL: `verifier:successful`,
  MVM_CTC_VERIFIER_STAKE: `mvmctc:verifierstake`,
  MVM_CTC_BATCH_ELEMENT: `mvmctc:batchelement`,
  BLOCK: `block`,
  UNCONFIRMED_BLOCK: `unconfirmed:block`,

  // FDG required keys
  L1_BLOCK_TO_L2_MAPPER_PREFIX: `l1tol2blockmapper:`,
  L2_BLOCK_TO_L1_MAPPER_PREFIX: `l2tol1blockmapper:`,
}

interface Indexed {
  index: number
}

export interface TransportDBMap {}

export class TransportDBMapHolder {
  public dbPath: string
  public dbs: TransportDBMap

  constructor(dbPath: string) {
    this.dbPath = dbPath
    this.dbs = {}
  }

  public async getTransportDbByChainId(chainId): Promise<TransportDB> {
    let db = this.dbs[chainId]
    if (!db) {
      const leveldb = level(this.dbPath + '_' + chainId)
      await leveldb.open()
      db = new TransportDB(leveldb, Number(chainId) === 1088)
      this.dbs[chainId] = db
    }
    return db
  }
}

export class TransportDB {
  public db: SimpleDB
  private enablePatch01: boolean

  constructor(leveldb: LevelUp, enablePatch01: boolean) {
    this.db = new SimpleDB(leveldb)
    this.enablePatch01 = enablePatch01
  }

  public async putEnqueueEntries(entries: EnqueueEntry[]): Promise<void> {
    await this._putEntries(TRANSPORT_DB_KEYS.ENQUEUE, entries)
  }

  public async putBlockEntries(entries: BlockEntry[]): Promise<void> {
    await this._putEntries(TRANSPORT_DB_KEYS.BLOCK, entries)
  }

  public async putUnconfirmedBlockEntries(
    entries: BlockEntry[]
  ): Promise<void> {
    await this._putEntries(TRANSPORT_DB_KEYS.UNCONFIRMED_BLOCK, entries)
  }

  public async putTransactionEntries(
    entries: TransactionEntry[]
  ): Promise<void> {
    await this._putEntries(TRANSPORT_DB_KEYS.TRANSACTION, entries)
  }

  public async putUnconfirmedTransactionEntries(
    entries: TransactionEntry[]
  ): Promise<void> {
    await this._putEntries(TRANSPORT_DB_KEYS.UNCONFIRMED_TRANSACTION, entries)
  }

  public async putTransactionBatchEntries(
    entries: TransactionBatchEntry[]
  ): Promise<void> {
    await this._putEntries(TRANSPORT_DB_KEYS.TRANSACTION_BATCH, entries)
  }

  public async putStateRootEntries(entries: StateRootEntry[]): Promise<void> {
    await this._putEntries(TRANSPORT_DB_KEYS.STATE_ROOT, entries)
  }

  public async putUnconfirmedStateRootEntries(
    entries: StateRootEntry[]
  ): Promise<void> {
    await this._putEntries(TRANSPORT_DB_KEYS.UNCONFIRMED_STATE_ROOT, entries)
  }

  public async putStateRootBatchEntries(
    entries: StateRootBatchEntry[]
  ): Promise<void> {
    await this._putEntries(TRANSPORT_DB_KEYS.STATE_ROOT_BATCH, entries)
  }

  public async putTransactionIndexByQueueIndex(
    queueIndex: number,
    index: number
  ): Promise<void> {
    await this.db.put([
      {
        key: TRANSPORT_DB_KEYS.ENQUEUE_CTC_INDEX,
        index: queueIndex,
        value: index,
      },
    ])
  }

  public async putLastVerifierEntry(
    success: boolean,
    entry: VerifierResultEntry
  ): Promise<void> {
    const key = success
      ? TRANSPORT_DB_KEYS.VERIFIER_SUCCESSFUL
      : TRANSPORT_DB_KEYS.VERIFIER_FAILED
    await this.db.put<VerifierResultEntry>([
      {
        key: `${key}:latest`,
        index: 0,
        value: entry,
      },
    ])
  }

  public async putBatchElementEntries(
    entries: AppendBatchElementEntry[]
  ): Promise<void> {
    await this._putEntries(TRANSPORT_DB_KEYS.MVM_CTC_BATCH_ELEMENT, entries)
  }

  public async putVerifierStakeEntries(
    entries: VerifierStakeEntry[]
  ): Promise<void> {
    await this._putEntries(TRANSPORT_DB_KEYS.MVM_CTC_VERIFIER_STAKE, entries)
    if (entries.length > 0) {
      this._putLatestEntryIndex(
        TRANSPORT_DB_KEYS.MVM_CTC_VERIFIER_STAKE,
        entries[entries.length - 1].index
      )
    }
  }

  public async getLatestVerifierStake(): Promise<VerifierStakeEntry> {
    const index = await this._getLatestEntryIndex(
      TRANSPORT_DB_KEYS.MVM_CTC_VERIFIER_STAKE
    )
    return this._getEntryByIndex(
      TRANSPORT_DB_KEYS.MVM_CTC_VERIFIER_STAKE,
      index
    )
  }

  public async getVerifierStakeByIndex(
    index: number
  ): Promise<VerifierStakeEntry> {
    return this._getEntryByIndex(
      TRANSPORT_DB_KEYS.MVM_CTC_VERIFIER_STAKE,
      index
    )
  }

  public async getBatchElementByIndex(
    index: number
  ): Promise<AppendBatchElementEntry> {
    return this._getEntryByIndex(TRANSPORT_DB_KEYS.MVM_CTC_BATCH_ELEMENT, index)
  }

  public async getLastVerifierEntry(
    success: boolean
  ): Promise<VerifierResultEntry> {
    const key = success
      ? TRANSPORT_DB_KEYS.VERIFIER_SUCCESSFUL
      : TRANSPORT_DB_KEYS.VERIFIER_FAILED
    return this.db.get<VerifierResultEntry>(`${key}:latest`, 0)
  }

  public async getTransactionIndexByQueueIndex(index: number): Promise<number> {
    return this.db.get(TRANSPORT_DB_KEYS.ENQUEUE_CTC_INDEX, index)
  }

  public async getEnqueueByIndex(index: number): Promise<EnqueueEntry> {
    return this._getEntryByIndex(TRANSPORT_DB_KEYS.ENQUEUE, index)
  }

  public async getBlockByIndex(index: number): Promise<BlockEntry> {
    const block = await this._getEntryByIndex<BlockEntry>(
      TRANSPORT_DB_KEYS.BLOCK,
      index
    )
    return this._getFullBlock(block)
  }

  public async getUnconfirmedBlockByIndex(index: number): Promise<BlockEntry> {
    return this._getEntryByIndex(TRANSPORT_DB_KEYS.UNCONFIRMED_BLOCK, index)
  }

  public async getBlocksByIndexRange(
    start: number,
    end: number
  ): Promise<BlockEntry[]> {
    const blocks = await this._getEntries<BlockEntry>(
      TRANSPORT_DB_KEYS.BLOCK,
      start,
      end
    )
    const fullBlocks = []
    for (const block of blocks) {
      const fullBlock = await this._getFullBlock(block)
      fullBlocks.push(fullBlock)
    }
    return fullBlocks
  }

  public async getTransactionByIndex(index: number): Promise<TransactionEntry> {
    return this._getEntryByIndex(TRANSPORT_DB_KEYS.TRANSACTION, index)
  }

  public async getUnconfirmedTransactionByIndex(
    index: number
  ): Promise<TransactionEntry> {
    return this._getEntryByIndex(
      TRANSPORT_DB_KEYS.UNCONFIRMED_TRANSACTION,
      index
    )
  }

  public async getTransactionsByIndexRange(
    start: number,
    end: number
  ): Promise<TransactionEntry[]> {
    return this._getEntries(TRANSPORT_DB_KEYS.TRANSACTION, start, end)
  }

  public async getTransactionBatchByIndex(
    index: number
  ): Promise<TransactionBatchEntry> {
    return this._getEntryByIndex(TRANSPORT_DB_KEYS.TRANSACTION_BATCH, index)
  }

  public async getStateRootByIndex(index: number): Promise<StateRootEntry> {
    return this._getEntryByIndex(TRANSPORT_DB_KEYS.STATE_ROOT, index)
  }

  public async getUnconfirmedStateRootByIndex(
    index: number
  ): Promise<StateRootEntry> {
    return this._getEntryByIndex(
      TRANSPORT_DB_KEYS.UNCONFIRMED_STATE_ROOT,
      index
    )
  }

  public async getStateRootsByIndexRange(
    start: number,
    end: number
  ): Promise<StateRootEntry[]> {
    return this._getEntries(TRANSPORT_DB_KEYS.STATE_ROOT, start, end)
  }

  public async getStateRootBatchByIndex(
    index: number
  ): Promise<StateRootBatchEntry> {
    return this._getEntryByIndex(TRANSPORT_DB_KEYS.STATE_ROOT_BATCH, index)
  }

  public async getLatestEnqueue(): Promise<EnqueueEntry> {
    return this._getLatestEntry(TRANSPORT_DB_KEYS.ENQUEUE)
  }

  public async getLatestTransaction(): Promise<TransactionEntry> {
    return this._getLatestEntry(TRANSPORT_DB_KEYS.TRANSACTION)
  }

  public async getLatestUnconfirmedTransaction(): Promise<TransactionEntry> {
    return this._getLatestEntry(TRANSPORT_DB_KEYS.UNCONFIRMED_TRANSACTION)
  }

  public async getLatestTransactionBatch(): Promise<TransactionBatchEntry> {
    return this._getLatestEntry(TRANSPORT_DB_KEYS.TRANSACTION_BATCH)
  }

  public async getLatestBlock(): Promise<BlockEntry> {
    const block = await this._getLatestEntry<BlockEntry>(
      TRANSPORT_DB_KEYS.BLOCK
    )
    return this._getFullBlock(block)
  }

  public async getLatestUnconfirmedBlock(): Promise<BlockEntry> {
    return this._getLatestEntry(TRANSPORT_DB_KEYS.UNCONFIRMED_BLOCK)
  }

  public async getLatestStateRoot(): Promise<StateRootEntry> {
    return this._getLatestEntry(TRANSPORT_DB_KEYS.STATE_ROOT)
  }

  public async getLatestUnconfirmedStateRoot(): Promise<StateRootEntry> {
    return this._getLatestEntry(TRANSPORT_DB_KEYS.UNCONFIRMED_STATE_ROOT)
  }

  public async getLatestStateRootBatch(): Promise<StateRootBatchEntry> {
    return this._getLatestEntry(TRANSPORT_DB_KEYS.STATE_ROOT_BATCH)
  }

  public async getHighestL2BlockNumber(): Promise<number> {
    return this.db.get<number>(TRANSPORT_DB_KEYS.HIGHEST_L2_BLOCK, 0)
  }

  public async putHighestL2BlockNumber(
    block: number | BigNumber
  ): Promise<void> {
    if (
      BigNumber.from(block).toNumber() <= (await this.getHighestL2BlockNumber())
    ) {
      return
    }

    return this.db.put<number>([
      {
        key: TRANSPORT_DB_KEYS.HIGHEST_L2_BLOCK,
        index: 0,
        value: BigNumber.from(block).toNumber(),
      },
    ])
  }

  public async getHighestSyncedUnconfirmedBlock(): Promise<number> {
    return (
      (await this.db.get<number>(TRANSPORT_DB_KEYS.UNCONFIRMED_HIGHEST, 0)) || 0
    )
  }

  public async setHighestSyncedUnconfirmedBlock(block: number): Promise<void> {
    return this.db.put<number>([
      {
        key: TRANSPORT_DB_KEYS.UNCONFIRMED_HIGHEST,
        index: 0,
        value: block,
      },
    ])
  }

  public async getHighestSyncedL1Block(): Promise<number> {
    return (
      (await this.db.get<number>(TRANSPORT_DB_KEYS.HIGHEST_SYNCED_BLOCK, 0)) ||
      0
    )
  }

  public async setHighestSyncedL1Block(block: number): Promise<void> {
    return this.db.put<number>([
      {
        key: TRANSPORT_DB_KEYS.HIGHEST_SYNCED_BLOCK,
        index: 0,
        value: block,
      },
    ])
  }

  public async getHighestSyncedL1BatchIndex(): Promise<number> {
    return (
      (await this.db.get<number>(
        TRANSPORT_DB_KEYS.HIGHEST_SYNCED_BATCH_INDEX,
        0
      )) || 0
    )
  }

  public async setHighestSyncedL1BatchIndex(batch: number): Promise<void> {
    return this.db.put<number>([
      {
        key: TRANSPORT_DB_KEYS.HIGHEST_SYNCED_BATCH_INDEX,
        index: 0,
        value: batch,
      },
    ])
  }

  public async getLastDerivedL1Block(l2ChainId: number): Promise<number> {
    const derivedL1Blocks = await this.db.rangeKV<number, number>(
      `${TRANSPORT_DB_KEYS.L1_BLOCK_TO_L2_MAPPER_PREFIX}${l2ChainId}`,
      0,
      undefined, // set no limit to upper bound
      true, // reads in reverse order
      true // only needs to latest entry
    )

    return !derivedL1Blocks ? 0 : derivedL1Blocks[0].key
  }

  public async getL1OriginOfL2Block(
    l2block: number,
    l2ChainId: number
  ): Promise<number> {
    const l1Origin = await this.db.get<number>(
      `${TRANSPORT_DB_KEYS.L2_BLOCK_TO_L1_MAPPER_PREFIX}${l2ChainId}`,
      l2block
    )

    return !l1Origin ? 0 : l1Origin
  }

  // getL2SafeHeadFromL1Block returns the latest safe l2 block along with the corresponding l1 block number,
  // the first element of the tuple is the l1 block number that is closet to the given l1 block number,
  // the second element of the tuple is the safe l2 block number.
  public async getL2SafeHeadFromL1Block(
    l1block: number,
    l2ChainId: number
  ): Promise<[number, number]> {
    const l2Blocks = await this.db.rangeKV<number, number>(
      `${TRANSPORT_DB_KEYS.L1_BLOCK_TO_L2_MAPPER_PREFIX}${l2ChainId}`,
      0,
      l1block,
      true, // reads in reverse order
      true // only needs to latest entry
    )

    return !l2Blocks ? [0, 0] : [l2Blocks[0].key, l2Blocks[0].value]
  }

  public async setL1BlockToL2BlockMapping(
    l1block: number,
    l2ChainId: number,
    l2block: number
  ): Promise<void> {
    return this.db.put<number>([
      {
        key: `${TRANSPORT_DB_KEYS.L1_BLOCK_TO_L2_MAPPER_PREFIX}${l2ChainId}`,
        index: l1block,
        value: l2block,
      },
    ])
  }

  public async setL2BlockToL1BlockMapping(
    l1block: number,
    l2ChainId: number,
    l2blocks: number[]
  ): Promise<void> {
    if (!l2blocks) {
      return
    }

    return this.db.put<number>(
      l2blocks.map((l2block) => {
        return {
          key: `${TRANSPORT_DB_KEYS.L2_BLOCK_TO_L1_MAPPER_PREFIX}${l2ChainId}`,
          index: l2block,
          value: l1block,
        }
      })
    )
  }

  public async getStartingL1Block(): Promise<number> {
    return this.db.get<number>(TRANSPORT_DB_KEYS.STARTING_L1_BLOCK, 0)
  }

  public async setStartingL1Block(block: number): Promise<void> {
    return this.db.put<number>([
      {
        key: TRANSPORT_DB_KEYS.STARTING_L1_BLOCK,
        index: 0,
        value: block,
      },
    ])
  }

  public async getStartingL1BatchIndex(): Promise<number> {
    return this.db.get<number>(TRANSPORT_DB_KEYS.STARTING_L1_BATCH_INDEX, 0)
  }

  public async setStartingL1BatchIndex(batch: number): Promise<void> {
    return this.db.put<number>([
      {
        key: TRANSPORT_DB_KEYS.STARTING_L1_BATCH_INDEX,
        index: 0,
        value: batch,
      },
    ])
  }

  // Not sure if this next section belongs in this class.

  public async getFullTransactionByIndex(
    index: number
  ): Promise<TransactionEntry> {
    const transaction = await this.getTransactionByIndex(index)
    if (transaction === null) {
      return null
    }

    if (transaction.queueOrigin === 'l1') {
      const enqueue = await this.getEnqueueByIndex(transaction.queueIndex)
      if (enqueue === null) {
        return null
      }

      return {
        ...transaction,
        ...{
          blockNumber: enqueue.blockNumber,
          timestamp: enqueue.timestamp, //main node will take the dtl time
          gasLimit: enqueue.gasLimit,
          target: enqueue.target,
          origin: enqueue.origin,
          data: enqueue.data,
        },
      }
    } else {
      const txBlockNumber = (transaction.index + 1).toString()
      if (this.enablePatch01 && patch01[txBlockNumber]) {
        transaction.blockNumber = patch01[txBlockNumber][0]
        transaction.timestamp = patch01[txBlockNumber][1]
      }
      return transaction
    }
  }

  public async getLatestFullTransaction(): Promise<TransactionEntry> {
    return this.getFullTransactionByIndex(
      await this._getLatestEntryIndex(TRANSPORT_DB_KEYS.TRANSACTION)
    )
  }

  public async getFullTransactionsByIndexRange(
    start: number,
    end: number
  ): Promise<TransactionEntry[]> {
    const transactions = await this.getTransactionsByIndexRange(start, end)
    if (transactions === null) {
      return null
    }

    const fullTransactions = []
    for (const transaction of transactions) {
      const txBlockNumber = (transaction.index + 1).toString()
      if (this.enablePatch01 && patch01[txBlockNumber]) {
        transaction.blockNumber = patch01[txBlockNumber][0]
        transaction.timestamp = patch01[txBlockNumber][1]
      }
      if (transaction.queueOrigin === 'l1') {
        // Andromeda failed 20397 queue, skip one for verifier batch only
        let queueIndex = transaction.queueIndex
        if (queueIndex >= 20397) {
          queueIndex++
        }
        const enqueue = await this.getEnqueueByIndex(queueIndex)
        if (enqueue === null) {
          return null
        }

        if (this.enablePatch01 && patch01[txBlockNumber]) {
          fullTransactions.push({
            ...transaction,
            ...{
              gasLimit: enqueue.gasLimit,
              target: enqueue.target,
              origin: enqueue.origin,
              data: enqueue.data,
              queueIndex,
            },
          })
        } else {
          fullTransactions.push({
            ...transaction,
            ...{
              blockNumber: enqueue.blockNumber,
              timestamp: transaction.timestamp || enqueue.timestamp, //verifier will take the context time.
              gasLimit: enqueue.gasLimit,
              target: enqueue.target,
              origin: enqueue.origin,
              data: enqueue.data,
              queueIndex,
            },
          })
        }
      } else {
        transaction.origin =
          transaction.origin || '0x0000000000000000000000000000000000000000'
        fullTransactions.push(transaction)
      }
    }

    return fullTransactions
  }

  private async _getFullBlock(block: BlockEntry): Promise<BlockEntry> {
    const fullTransactions = []
    for (const transaction of block.transactions) {
      if (transaction.queueOrigin === 'l1') {
        // Andromeda failed 20397 queue, skip one for verifier batch only
        let queueIndex = transaction.queueIndex
        if (queueIndex >= 20397) {
          queueIndex++
        }
        const enqueue = await this.getEnqueueByIndex(queueIndex)
        if (enqueue === null) {
          return null
        }

        fullTransactions.push({
          ...transaction,
          ...{
            blockNumber: enqueue.blockNumber,
            timestamp: transaction.timestamp || enqueue.timestamp,
            gasLimit: enqueue.gasLimit,
            target: enqueue.target,
            origin: enqueue.origin,
            data: enqueue.data,
            queueIndex,
          },
        })
      } else {
        transaction.origin =
          transaction.origin || '0x0000000000000000000000000000000000000000'
        fullTransactions.push(transaction)
      }
    }
    block.transactions = fullTransactions
    return block
  }

  private async _getLatestEntryIndex(key: string): Promise<number> {
    return this.db.get<number>(`${key}:latest`, 0) || 0
  }

  private async _putLatestEntryIndex(
    key: string,
    index: number
  ): Promise<void> {
    return this.db.put<number>([
      {
        key: `${key}:latest`,
        index: 0,
        value: index,
      },
    ])
  }

  private async _getLatestEntry<TEntry extends Indexed>(
    key: string
  ): Promise<TEntry | null> {
    return this._getEntryByIndex(key, await this._getLatestEntryIndex(key))
  }

  private async _putLatestEntry<TEntry extends Indexed>(
    key: string,
    entry: TEntry
  ): Promise<void> {
    const latest = await this._getLatestEntryIndex(key)
    if (entry.index >= latest) {
      await this._putLatestEntryIndex(key, entry.index)
    }
  }

  private async _putEntries<TEntry extends Indexed>(
    key: string,
    entries: TEntry[]
  ): Promise<void> {
    if (entries.length === 0) {
      return
    }

    await this.db.put<TEntry>(
      entries.map((entry) => {
        return {
          key: `${key}:index`,
          index: entry.index,
          value: entry,
        }
      })
    )

    await this._putLatestEntry(key, entries[entries.length - 1])
  }

  private async _getEntryByIndex<TEntry extends Indexed>(
    key: string,
    index: number
  ): Promise<TEntry | null> {
    if (index === null) {
      return null
    }
    return this.db.get<TEntry>(`${key}:index`, index)
  }

  private async _getEntries<TEntry extends Indexed>(
    key: string,
    startIndex: number,
    endIndex: number
  ): Promise<TEntry[] | []> {
    return this.db.range<TEntry>(`${key}:index`, startIndex, endIndex)
  }
}
