import { ServiceError, credentials } from '@grpc/grpc-js'

import { disperser } from './generated/disperser'
import { Wallet, utils } from 'ethers'
import { IDisperserClient, DisperseClientConfig } from './types'

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
        account_id: await wallet.getAddress(),
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
            account_id: await wallet.getAddress(),
          }),
        })
      )

      stream.on('error', (error: ServiceError) => {
        reject(error)
      })

      stream.on('data', async (response: disperser.AuthenticatedReply) => {
        if (response.blob_auth_header) {
          const nonceBytes = utils.arrayify(
            utils.hexlify(response.blob_auth_header.challenge_parameter)
          )
          const hash = utils.keccak256(nonceBytes)
          const signature = await wallet.signMessage(hash)
          const authData = utils.arrayify(signature)
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
