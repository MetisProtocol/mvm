import { encodeHex, sleep } from '../common'
import * as Minio from 'minio'
import crypto from 'crypto'

export interface MinioConfig {
  options: Minio.ClientOptions
  l2ChainId: number,
  bucket: string,
}

export class MinioClient {
  protected client: Minio.Client
  protected options: Minio.ClientOptions
  protected l2ChainId: number
  protected bucket: string

  constructor(config: MinioConfig) {
    this.client = new Minio.Client(config.options)
    // this.client.setRequestOptions({timeout: 2*60*1000})
    this.l2ChainId = config.l2ChainId
    this.options = config.options
    this.bucket = config.bucket
  }

  protected async ensureBucket(): Promise<string> {
    // const bucketName = `metis-${this.l2ChainId}-tx`
    const bucketName = this.bucket
    try {
      const hasBucket = await this.client.bucketExists(bucketName)
      if (!hasBucket) {
        await this.client.makeBucket(bucketName, this.options.region || 'us-east-1')
      }
    }
    catch(x) {
      console.log('bucket exists check', x.message)
    }
    return bucketName
  }

  public sha256Hash(plain): string {
    return crypto.createHash('sha256').update(plain).digest('hex')
  }

  public async writeObject(
    startAtElement: number,
    totalElements: number,
    encodedTransactionData: string,
    tryCount: number): Promise<string> {
      console.info('start write object', startAtElement, totalElements, 'len ' + encodedTransactionData.length)
      if (!encodedTransactionData || startAtElement < 0 || totalElements <= 0) {
        console.info('return with nothing to write')
        return ''
      }
      const bucketName = await this.ensureBucket()
      const calcHash = [startAtElement, totalElements, new Date().getTime(), encodedTransactionData]
      const metaData = {
          'Content-Type': 'application/octet-stream',
          'x-metis-meta-tx-start': startAtElement,
          'x-metis-meta-tx-total': totalElements,
          'x-metis-meta-tx-timestamp': calcHash[2]
      }
      // object key is timestamp[13] + 00000 + sha256(metaData+txData)
      let objectKey = `${encodeHex(calcHash[2], 13)}00000${this.sha256Hash(calcHash.join('_'))}`
      try {
        await this.client.putObject(bucketName, objectKey, encodedTransactionData, null, metaData)
        console.info('write object successfully', objectKey)
      }
      catch(x) {
        console.error('write object err', x.message)
        if (tryCount <= 0) {
          return ''
        }
        tryCount--
        await sleep(1000)
        objectKey = await this.writeObject(startAtElement, totalElements, encodedTransactionData, tryCount)
      }
      return objectKey
  }

  public async readObject(objectName: string, tryCount: number): Promise<string> {
    if (!objectName) {
      return ''
    }
    const bucketName = await this.ensureBucket()
    let data = ''
    try {
      let self = this
      data = await new Promise(function(resolve, reject){
        let chunks = ''
        self.client.getObject(bucketName, objectName, function(err, dataStream) {
          if (err) {
            reject(err)
            return
          }
          dataStream.on('data', function(chunk) {
           chunks += chunk
          })
          dataStream.on('end', function() {
            resolve(chunks)
          })
          dataStream.on('error', function(err) {
            console.log(err)
            reject(err)
          })
        })
      })
      if (!data || data.length === 0) {
        throw 'getObject err: readable.read'
      }
    }
    catch(x) {
      console.error('read object err', x.message)
      if (tryCount <= 0) {
        return ''
      }
      tryCount--
      await sleep(1000)
      data = await this.readObject(objectName, tryCount)
    }
    return data
  }

  public async verifyObject(objectName: string, data: string, tryCount: number) : Promise<boolean> {
    if (!objectName || !data || objectName.length <= 18) {
      return false
    }
    const bucketName = await this.ensureBucket()
    let verified = false
    let meta = null
    try {
      const stat = await this.client.statObject(bucketName, objectName)
      if (!stat) {
        throw 'statObject failed'
      }
      meta = stat.metaData
      if (meta['x-metis-meta-tx-start'] == 'undefined' || meta['x-metis-meta-tx-total'] == 'undefined'
      || !meta['x-metis-meta-tx-timestamp']) {
        return false
      }
      // to verfiy
      const calcHash = [meta['x-metis-meta-tx-start'], meta['x-metis-meta-tx-total'], meta['x-metis-meta-tx-timestamp'], data]
      // hash from name
      const hashFromName = objectName.substr(18)
      const hashFromCalc = this.sha256Hash(calcHash.join('_'))
      verified = hashFromName === hashFromCalc
      if (!verified) {
        console.info('compare hash', 'from name', hashFromName, 'from calc', hashFromCalc, 'data len', data.length)
      }
      return verified
    }
    catch(x) {
      console.error('stat object err', x.message)
      if (tryCount <= 0) {
        return false
      }
      tryCount--
      await sleep(1000)
      verified = await this.verifyObject(objectName, data, tryCount)
    }
    return verified
  }
}