import { ServiceError, credentials } from '@grpc/grpc-js'

import { disperser } from './generated/disperser'
import { Wallet, ethers, utils } from 'ethers'
import { IDisperserClient, DisperseClientConfig } from './types'
import { fromHexString } from './../../common'

const {
  DisperserClient,
  BlobStatusRequest,
  DisperseBlobRequest,
  RetrieveBlobRequest,
  AuthenticatedRequest,
  AuthenticationData,
} = disperser

export const createDisperserClient = (
  config: DisperseClientConfig,
  wallet: Wallet
): IDisperserClient => {
  const { hostname, port, useSecureGrpc } = config
  const addr = `${hostname}:${port}`
  const client = new DisperserClient(
    addr,
    useSecureGrpc ? credentials.createSsl() : credentials.createInsecure()
  )

  const disperseBlob = (
    data: Uint8Array,
    customQuorums: number[]
  ): Promise<disperser.BlobStatus> => {
    return new Promise(async (resolve, reject) => {
      const request = new DisperseBlobRequest({
        data,
        custom_quorum_numbers: customQuorums,
        account_id: wallet.publicKey,
      })
      client.DisperseBlob(
        request,
        (error: ServiceError | null, response: disperser.DisperseBlobReply) => {
          if (error) {
            reject(error)
          } else {
            resolve(response.result)
          }
        }
      )
    })
  }

  const disperseBlobAuthenticated = (
    data: Uint8Array,
    quorums: number[]
  ): Promise<disperser.DisperseBlobReply> => {
    return new Promise<disperser.DisperseBlobReply>(async (resolve, reject) => {
      const stream = client.DisperseBlobAuthenticated()
      stream.write(
        new AuthenticatedRequest({
          disperse_request: new DisperseBlobRequest({
            data,
            custom_quorum_numbers: quorums,
            account_id: wallet.publicKey,
          }),
        })
      )

      stream.on('error', (error: ServiceError) => {
        console.error('stream error', error)
        reject(error)
      })

      stream.on('data', async (response: disperser.AuthenticatedReply) => {
        if (response.blob_auth_header) {
          const nonceBytes = utils.arrayify(
            response.blob_auth_header.challenge_parameter
          )
          const hash = ethers.utils.arrayify(ethers.utils.keccak256(nonceBytes))
          const seckey = Uint8Array.from(fromHexString(wallet.privateKey))
          const signingKey = new ethers.utils.SigningKey(seckey)
          const signature = signingKey.signDigest(hash)
          const authData = new Uint8Array([
            ...ethers.utils.arrayify(signature.r),
            ...ethers.utils.arrayify(signature.s),
            signature.recoveryParam,
          ])
          stream.write(
            new AuthenticatedRequest({
              authentication_data: new AuthenticationData({
                authentication_data: authData,
              }),
            })
          )
        } else if (response.disperse_reply) {
          resolve(response.disperse_reply)
        } else {
          reject(new Error('Unexpected response'))
        }
      })
    })
  }

  const getBlobStatus = (
    requestId: Uint8Array
  ): Promise<disperser.BlobStatusReply> => {
    return new Promise((resolve, reject) => {
      const request = new BlobStatusRequest({
        request_id: requestId,
      })

      client.GetBlobStatus(
        request,
        (error: ServiceError | null, response: disperser.BlobStatusReply) => {
          if (error) {
            reject(error)
          } else {
            resolve(response)
          }
        }
      )
    })
  }

  const retrieveBlob = (
    batchHeaderHash: Uint8Array,
    blobIndex: number
  ): Promise<Uint8Array> => {
    return new Promise<Uint8Array>((resolve, reject) => {
      const request = new RetrieveBlobRequest({
        batch_header_hash: batchHeaderHash,
        blob_index: blobIndex,
      })
      client.RetrieveBlob(
        request,
        (error: ServiceError | null, response: disperser.RetrieveBlobReply) => {
          if (error) {
            reject(error)
          } else {
            resolve(response.data)
          }
        }
      )
    })
  }

  return {
    disperseBlob,
    disperseBlobAuthenticated,
    getBlobStatus,
    retrieveBlob,
  }
}
