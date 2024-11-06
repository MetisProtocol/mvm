import * as http from 'http'
import * as https from 'https'
import { URL } from 'url'
import { ethers, toBigInt, toNumber } from 'ethersv6'
import { randomUUID } from 'crypto'
import '@localtest911/core-utils'
import * as kzg from 'c-kzg'

export class MpcClient {
  protected url: string

  constructor(url: string) {
    this.url = url
  }

  protected httpRequest(
    url: string,
    options: http.RequestOptions | https.RequestOptions,
    data?: any
  ): Promise<string> {
    return new Promise<string>((resolve, reject) => {
      const protocol = url.toLowerCase().startsWith('https') ? https : http
      const req = protocol.request(options, (response) => {
        let responseData = ''

        response.on('data', (chunk) => {
          responseData += chunk
        })

        response.on('end', () => {
          resolve(responseData)
        })
      })

      req.on('error', (error) => {
        reject(error)
      })

      if (data) {
        req.write(JSON.stringify(data))
      }

      req.end()
    })
  }

  public async getLatestMpc(id: string = '0'): Promise<any> {
    const getUrl = new URL(`/mpc/latest/${id}`, this.url)
    const getOptions: http.RequestOptions | https.RequestOptions = {
      method: 'GET',
      hostname: getUrl.hostname,
      port: getUrl.port,
      path: getUrl.pathname,
    }
    const resp = await this.httpRequest(this.url, getOptions)
    console.debug('getLatestMpc resp', resp)
    if (!resp) {
      return null
    }
    const obj = eval('(' + resp + ')')
    if (obj.result && obj.result.mpc_address) {
      return obj.result
    }
    return null
  }

  public async proposeMpcSign(data: any): Promise<any> {
    const postUrl = new URL('/mpc/propose-mpc-sign', this.url)
    const postOptions: http.RequestOptions | https.RequestOptions = {
      method: 'POST',
      hostname: postUrl.hostname,
      port: postUrl.port,
      path: postUrl.pathname,
      headers: {
        'Content-Type': 'application/json',
        'Content-Length': Buffer.byteLength(JSON.stringify(data)),
      },
    }
    const resp = await this.httpRequest(this.url, postOptions, data)
    console.info('proposeMpcSign resp', resp)
    if (!resp) {
      return null
    }
    const obj = eval('(' + resp + ')')
    if (obj.error) {
      return null
    }
    return obj
  }

  public async getMpcSign(id: string): Promise<string> {
    const getUrl = new URL(`/mpc/sign/${id}`, this.url)
    const getOptions: http.RequestOptions | https.RequestOptions = {
      method: 'GET',
      hostname: getUrl.hostname,
      port: getUrl.port,
      path: getUrl.pathname,
    }
    const resp = await this.httpRequest(this.url, getOptions)
    console.info('getMpcSign resp', resp)
    if (!resp) {
      return ''
    }
    const obj = eval('(' + resp + ')')
    if (obj.error) {
      return ''
    }
    if (obj.result && obj.result.signed_tx) {
      return obj.result.signed_tx
    }
    return ''
  }

  public async getMpcSignWithTimeout(
    id: string,
    maxTimeout: number,
    interval: number
  ): Promise<string> {
    const startTime = Date.now()
    return new Promise<any>(async (resolve, reject) => {
      const requestInterval = setInterval(async () => {
        try {
          const signedTx = await this.getMpcSign(id)
          const currentTime = Date.now()
          if (signedTx || currentTime - startTime >= maxTimeout) {
            clearInterval(requestInterval)
            resolve(signedTx)
          }
        } catch (error) {
          clearInterval(requestInterval)
          reject(error)
        }
      }, interval)
    })
  }

  public removeHexLeadingZero(
    hex: string,
    keepOneZero: boolean = false
  ): string {
    let toHex = hex.startsWith('0x') ? hex.substring(2) : hex
    toHex = toHex.replace(/^0+/, '')
    if (!toHex.length && keepOneZero) {
      toHex = '0'
    }
    if (hex.startsWith('0x')) {
      toHex = '0x' + toHex
    }
    return toHex
  }

  public base64ToHex(base64String: string) {
    // Decode Base64 to binary
    const binaryData = Buffer.from(base64String, 'base64')
    return '0x' + binaryData.toString('hex')
  }

  // call this
  public async signTx(tx: any, mpcId: any): Promise<string> {
    // check tx
    if (!tx.gasLimit) {
      throw new Error('tx gasLimit is required')
    }
    if (tx.nonce === undefined || tx.nonce === null) {
      throw new Error('tx nonce is required')
    }
    // call mpc to sign tx
    const unsignedTx: any = {
      data: tx.data,
      nonce: toNumber(tx.nonce),
      to: tx.to,
      value: tx.value ? toBigInt(tx.value) : toBigInt(0),
      gasLimit: toBigInt(tx.gasLimit),
      chainId: tx.chainId,
    }
    if (!tx.type) {
      // populate legacy tx
      if (!tx.gasPrice) {
        throw new Error('gasPrice is required for legacy tx')
      }

      unsignedTx.gasPrice = toBigInt(tx.gasPrice)
    } else {
      // populate typed tx
      const txType = toNumber(tx.type)
      if (txType === 1) {
        // check for access list tx
        if (!tx.gasPrice) {
          throw new Error('gasPrice is required for access list tx')
        }
        unsignedTx.gasPrice = toBigInt(tx.gasPrice)
        unsignedTx.accessList = tx.accessList
      } else if (txType > 1) {
        // check for post-EIP1559 tx
        if (!tx.maxFeePerGas) {
          throw new Error('maxFeePerGas is required for post-EIP1559 tx')
        }
        if (!tx.maxPriorityFeePerGas) {
          throw new Error(
            'maxPriorityFeePerGas is required for post-EIP1559 tx'
          )
        }

        unsignedTx.maxFeePerGas = toBigInt(tx.maxFeePerGas)
        unsignedTx.maxPriorityFeePerGas = toBigInt(tx.maxPriorityFeePerGas)

        if (txType === 3) {
          // extra checks for blob tx
          if (!tx.maxFeePerBlobGas) {
            throw new Error('maxFeePerBlobGas is required for blob tx')
          }

          if (!tx.blobs || !tx.blobVersionedHashes) {
            throw new Error('blobs and their hashes are required for blob tx')
          }

          unsignedTx.maxFeePerBlobGas = toBigInt(tx.maxFeePerBlobGas)
          unsignedTx.blobs = tx.blobs
          unsignedTx.blobVersionedHashes = tx.blobVersionedHashes
          unsignedTx.kzg = kzg
        }
      }
    }

    const signId = randomUUID()
    const postData = {
      sign_id: signId,
      mpc_id: mpcId,
      sign_type: 0,
      sign_data: ethers.Transaction.from(unsignedTx).unsignedSerialized,
      sign_msg: '',
    }
    const signResp = await this.proposeMpcSign(postData)
    if (!signResp) {
      throw new Error(`MPC ${mpcId} propose sign failed`)
    }

    const signedTx = await this.getMpcSignWithTimeout(signId, 60000, 5000)
    if (!signedTx) {
      throw new Error(`MPC ${mpcId} get sign failed`)
    }
    return this.base64ToHex(signedTx)
  }
}
