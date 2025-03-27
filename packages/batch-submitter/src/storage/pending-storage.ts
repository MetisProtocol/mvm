/* Imports: External */
import * as fs from 'fs/promises'
import * as path from 'path'
import { Logger } from '@eth-optimism/common-ts'
import { toBigInt, toNumber } from 'ethersv6'

const PENDING_TX_FILE_SUFFIX = '_pending.json'

export interface PendingRecordInfo {
  batchIndex: number | bigint
  txHash: string
  from: string
  maxFeePerGas: number
  maxPriorityFeePerGas: number
  maxFeePerBlobGas: number | null
}

export class PendingStorage {
  public storagePath: string
  private logger: Logger

  constructor(storagePath: string, logger: Logger) {
    this.storagePath = storagePath
    this.logger = logger
  }

  public async recordPendingTx(pending: PendingRecordInfo): Promise<boolean> {
    const jsonData = {
      from: pending.from,
      batchIndex: toNumber(pending.batchIndex),
      hash: pending.txHash,
      maxFeePerGas: toNumber(pending.maxFeePerGas),
      maxPriorityFeePerGas: toNumber(pending.maxPriorityFeePerGas),
      maxFeePerBlobGas: pending.maxFeePerBlobGas,
    }
    const jsonString = JSON.stringify(jsonData, null, 2)
    const filePath = path.join(
      this.storagePath,
      `${pending.from}${PENDING_TX_FILE_SUFFIX}`
    )
    try {
      const fileHandle = await fs.open(filePath, 'w')
      await fileHandle.write(jsonString)
      await fileHandle.close()
      this.logger.info('JSON data has been written to pending tx file', {
        filePath,
      })
      return true
    } catch (writeError) {
      this.logger.error('Error writing to pending tx file:', writeError)
      throw new Error('Error writing to pending tx file file')
    }
  }

  public async clearPendingTx(address: string): Promise<void> {
    const filePath = path.join(
      this.storagePath,
      `${address}${PENDING_TX_FILE_SUFFIX}`
    )
    try {
      await fs.rm(filePath, { force: true })
      this.logger.info(`Pending tx of ${address} has been cleared`, {
        filePath,
      })
    } catch (removeError) {
      this.logger.error('Error removing pending tx file:', removeError)
      throw new Error('Error removing pending tx file file')
    }
  }

  public async getPendingTx(
    address: string
  ): Promise<PendingRecordInfo | null> {
    const filePath = path.join(
      this.storagePath,
      `${address}${PENDING_TX_FILE_SUFFIX}`
    )
    if (!this.fileExists(filePath)) {
      return null
    }
    try {
      const data = await fs.readFile(filePath, 'utf-8')
      if (!data) {
        return null
      }
      const readJsonData = JSON.parse(data)
      return {
        batchIndex: readJsonData.batchIndex,
        txHash: readJsonData.hash,
        from: readJsonData.from,
        maxFeePerGas: readJsonData.maxFeePerGas,
        maxPriorityFeePerGas: readJsonData.maxPriorityFeePerGas,
        maxFeePerBlobGas: readJsonData.maxFeePerBlobGas
          ? readJsonData.maxFeePerBlobGas
          : null,
      }
    } catch (readError) {
      this.logger.debug(`Unable to read pending tx file: ${readError}`)
      this.logger.info(`No pending tx found for ${address}`)
    }
    return null
  }

  private async fileExists(filePath) {
    try {
      await fs.stat(filePath)
      return true
    } catch (error) {
      if (error.code === 'ENOENT') {
        return false
      }
      throw error
    }
  }
}
