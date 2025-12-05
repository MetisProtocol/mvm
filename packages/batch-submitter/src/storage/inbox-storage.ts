/* Imports: External */
import { Logger } from '@eth-optimism/common-ts'
import { BytesLike, toNumber } from 'ethersv6'
import * as fs from 'fs/promises'
import * as path from 'path'

const INBOX_OK_FILE = 'inbox_ok.json'
const INBOX_FAIL_FILE = 'inbox_fail.json'
const STEPS_FILE = 'steps.json'

export interface InboxRecordInfo {
  batchIndex: number | bigint
  blockNumber: number | bigint
  txHash: string
}

export interface InboxSteps {
  input: string // the inbox tx data input
  blobs: Array<Array<BytesLike>> // array of blob tx data
  txHashes: Array<string> // blob tx hashes + inbox tx hash
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
      batchIndex: toNumber(batchIndex),
      errMsg,
    }
    const jsonString = JSON.stringify(jsonData, null, 2)
    const filePath = path.join(this.storagePath, INBOX_FAIL_FILE)
    await fs.writeFile(filePath, jsonString)
    this.logger.info('JSON data has been written to failed tx', { filePath })
    return true
  }

  public async recordConfirmedTx(inbox: InboxRecordInfo): Promise<boolean> {
    const jsonData = {
      batchIndex: toNumber(inbox.batchIndex),
      number: toNumber(inbox.blockNumber),
      hash: inbox.txHash,
    }
    const jsonString = JSON.stringify(jsonData, null, 2)
    const filePath = path.join(this.storagePath, INBOX_OK_FILE)
    await fs.writeFile(filePath, jsonString)
    this.logger.info('JSON data has been written to ok_tx file', { filePath })
    return true
  }

  public async getLatestConfirmedTx(): Promise<InboxRecordInfo> {
    const filePath = path.join(this.storagePath, INBOX_OK_FILE)
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
      blockNumber: readJsonData.number,
      txHash: readJsonData.hash,
    }
  }

  public async insertStep(jsonData: InboxSteps) {
    const data = {
      input: jsonData.input,
      txHashes: jsonData.txHashes,
      blobs: jsonData.blobs.map((blobArray) =>
        blobArray.map((blob) => {
          if (typeof blob === 'string') {
            return blob
          }
          return '0x' + Buffer.from(blob).toString('hex')
        })
      ),
    }
    const jsonString = JSON.stringify(data, null, 2)
    const filePath = path.join(this.storagePath, STEPS_FILE)
    await fs.writeFile(filePath, jsonString)
  }

  public async getStep(): Promise<InboxSteps | null> {
    const filePath = path.join(this.storagePath, STEPS_FILE)
    if (!(await this.fileExists(filePath))) {
      return null
    }
    const data = await fs.readFile(filePath, 'utf-8')
    return JSON.parse(data)
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
