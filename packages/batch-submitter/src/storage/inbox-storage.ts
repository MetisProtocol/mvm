/* Imports: External */
import * as fs from 'fs/promises'
import * as path from 'path'
import { Logger } from '@eth-optimism/common-ts'
import { toBigInt } from 'ethersv6'

const INBOX_OK_FILE = 'inbox_ok.json'
const INBOX_FAIL_FILE = 'inbox_fail.json'

export interface InboxRecordInfo {
  batchIndex: number | bigint
  blockNumber: number | bigint
  txHash: string
}

export class InboxStorage {
  public storagePath: string
  private logger: Logger

  constructor(storagePath: string, logger: Logger) {
    this.storagePath = storagePath
    this.logger = logger
  }

  public async recordFailedTx(
    batchIndex: number | bigint,
    errMsg: string
  ): Promise<boolean> {
    const jsonData = {
      batchIndex: toBigInt(batchIndex),
      errMsg,
    }
    const jsonString = JSON.stringify(jsonData, null, 2)
    const filePath = path.join(this.storagePath, INBOX_FAIL_FILE)
    try {
      const fileHandle = await fs.open(filePath, 'w')
      await fileHandle.write(jsonString)
      await fileHandle.close()
      this.logger.info('JSON data has been written to failed tx', { filePath })
      return true
    } catch (writeError) {
      this.logger.error('Error writing to failed tx file:', writeError)
      throw new Error('Error writing to failed tx file')
    }
  }

  public async recordConfirmedTx(inbox: InboxRecordInfo): Promise<boolean> {
    const jsonData = {
      batchIndex: toBigInt(inbox.batchIndex),
      number: toBigInt(inbox.blockNumber),
      hash: inbox.txHash,
    }
    const jsonString = JSON.stringify(jsonData, null, 2)
    const filePath = path.join(this.storagePath, INBOX_OK_FILE)
    try {
      const fileHandle = await fs.open(filePath, 'w')
      await fileHandle.write(jsonString)
      await fileHandle.close()
      this.logger.info('JSON data has been written to ok_tx file', { filePath })
      return true
    } catch (writeError) {
      this.logger.error('Error writing to ok_tx file:', writeError)
      throw new Error('Error writing to ok_tx file')
    }
  }

  public async getLatestConfirmedTx(): Promise<InboxRecordInfo> {
    const filePath = path.join(this.storagePath, INBOX_OK_FILE)
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
        blockNumber: readJsonData.number,
        txHash: readJsonData.hash,
      }
    } catch (readError) {
      this.logger.error('Error reading ok_tx file:', readError)
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
