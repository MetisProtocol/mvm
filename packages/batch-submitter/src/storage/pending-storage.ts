/* Imports: External */
import * as fs from 'fs/promises'
import * as path from 'path'
import { Logger } from '@eth-optimism/common-ts'
import { toNumber } from 'ethersv6'

const PENDING_TX_FILE_SUFFIX = '_pending.json'

export interface PendingRecordInfo {
  batchIndex: number | bigint
  txHash: string
  nonce: number
  from: string
  maxFeePerGas: number
  maxPriorityFeePerGas: number
  maxFeePerBlobGas: number | null
  submissionTime: number
}

export class PendingStorage {
  public storagePath: string
  private logger: Logger

  constructor(storagePath: string, logger: Logger) {
    this.storagePath = storagePath
    this.logger = logger
  }

  public async recordPendingTx(pending: PendingRecordInfo): Promise<void> {
    const jsonData = {
      from: pending.from,
      batchIndex: toNumber(pending.batchIndex),
      hash: pending.txHash,
      nonce: pending.nonce,
      maxFeePerGas: toNumber(pending.maxFeePerGas),
      maxPriorityFeePerGas: toNumber(pending.maxPriorityFeePerGas),
      maxFeePerBlobGas: pending.maxFeePerBlobGas,
      submissionTime: pending.submissionTime,
    }
    const jsonString = JSON.stringify(jsonData, null, 2)
    const filePath = path.join(
      this.storagePath,
      `${pending.from}${PENDING_TX_FILE_SUFFIX}`
    )

    await fs.writeFile(filePath, jsonString)
    this.logger.info('JSON data has been written to pending tx file', {
      filePath,
    })
  }

  public async clearPendingTx(address: string): Promise<void> {
    const filePath = path.join(
      this.storagePath,
      `${address}${PENDING_TX_FILE_SUFFIX}`
    )
    await fs.rm(filePath, { force: true })
    this.logger.info(`Pending tx of ${address} has been cleared`, {
      filePath,
    })
  }

  public async getPendingTx(
    address: string
  ): Promise<PendingRecordInfo | null> {
    const filePath = path.join(
      this.storagePath,
      `${address}${PENDING_TX_FILE_SUFFIX}`
    )
    if (!(await this.fileExists(filePath))) {
      return null
    }
    const data = await fs.readFile(filePath, 'utf-8')
    if (!data) {
      return null
    }

    const readJsonData = JSON.parse(data)
    return {
      batchIndex: readJsonData.batchIndex,
      txHash: readJsonData.hash,
      from: readJsonData.from,
      nonce: readJsonData.nonce,
      maxFeePerGas: readJsonData.maxFeePerGas,
      maxPriorityFeePerGas: readJsonData.maxPriorityFeePerGas,
      maxFeePerBlobGas: readJsonData.maxFeePerBlobGas
        ? readJsonData.maxFeePerBlobGas
        : null,
      submissionTime: readJsonData.submissionTime,
    }
  }

  private async fileExists(filePath) {
    try {
      await fs.stat(filePath)
      return true
    } catch (error) {
      return false
    }
  }
}
