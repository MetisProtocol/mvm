import { disperser } from './generated/disperser'
import { EigenDAClientConfig, IEigenDAClient } from './types'
import { createDisperserClient } from './disperser-client'
import { ethers } from 'ethers'

const { BlobStatus } = disperser

export const createEigenDAClient = (
  config: EigenDAClientConfig
): IEigenDAClient => {
  const [hostname, port] = config.rpc.split(':')
  const disperserClient = createDisperserClient(
    {
      hostname,
      port,
      useSecureGrpc: !config.disableTLS,
    },
    new ethers.Wallet(config.signerPrivateKey)
  )
  const getBlob = async (
    batchHeaderHash: Uint8Array,
    blobIndex: number
  ): Promise<Uint8Array> => {
    return disperserClient.retrieveBlob(batchHeaderHash, blobIndex)
  }

  const putBlob = async (data: Uint8Array): Promise<disperser.BlobInfo> => {
    const customQuorumNumbers = config.customQuorumIDs.map((e) => e as number)
    const disperseReply = await disperserClient.disperseBlobAuthenticated(
      data,
      customQuorumNumbers
    )
    const blobStatus = disperseReply.result
    const requestId = disperseReply.request_id

    if (blobStatus === BlobStatus.FAILED) {
      console.error('Unable to disperse blob to EigenDA, aborting', data)
      return
    }
    const timeoutPromise = new Promise<never>((_, reject) => {
      setTimeout(() => {
        reject(
          new Error(
            `timed out waiting for EigenDA blob to confirm blob with request id=${requestId}`
          )
        )
      }, config.statusQueryTimeout)
    })

    let resultResolve: (value: disperser.BlobInfo) => void
    const resultPromise = new Promise<disperser.BlobInfo>((resolve) => {
      resultResolve = resolve
    })

    const ticker = setInterval(async () => {
      try {
        const statusRes = await disperserClient.getBlobStatus(requestId)

        switch (statusRes.status) {
          case disperser.BlobStatus.PROCESSING:
          case disperser.BlobStatus.DISPERSING:
            console.log(
              'Blob submitted, waiting for dispersal from EigenDA',
              'requestID',
              requestId
            )
            break
          case disperser.BlobStatus.FAILED:
            console.error(
              'EigenDA blob dispersal failed in processing',
              'requestID',
              requestId,
              'err',
              statusRes.info
            )
            clearInterval(ticker)
            throw new Error(
              `EigenDA blob dispersal failed in processing, requestID=${requestId}`
            )
          case disperser.BlobStatus.INSUFFICIENT_SIGNATURES:
            console.error(
              'EigenDA blob dispersal failed in processing with insufficient signatures',
              'requestID',
              requestId
            )
            clearInterval(ticker)
            throw new Error(
              `EigenDA blob dispersal failed in processing with insufficient signatures, requestID=${requestId}`
            )
          case disperser.BlobStatus.CONFIRMED:
            if (config.waitForFinalization) {
              console.log(
                'EigenDA blob confirmed, waiting for finalization',
                'requestID',
                requestId
              )
            } else {
              console.log('EigenDA blob confirmed', 'requestID', requestId)
              clearInterval(ticker)
              resultResolve(statusRes.info)
              return
            }
            break
          case disperser.BlobStatus.FINALIZED:
            console.log(
              'Successfully dispersed blob to EigenDA',
              'requestID',
              requestId,
              'batchHeaderHash',
              statusRes.info.blob_verification_proof.batch_metadata
                .batch_header_hash
            )
            clearInterval(ticker)
            resultResolve(statusRes.info)
            return
          default:
            clearInterval(ticker)
            throw new Error(
              `EigenDA blob dispersal failed in processing with reply status ${statusRes.status}`
            )
        }
      } catch (error) {
        console.error(
          'Unable to retrieve blob dispersal status, will retry',
          'requestID',
          requestId,
          'err',
          error
        )
      }
    }, config.statusQueryRetryInterval)

    try {
      const result = await Promise.race([timeoutPromise, resultPromise])
      clearInterval(ticker)
      return result
    } catch (error) {
      clearInterval(ticker)
      throw error
    }
  }

  return {
    getBlob,
    putBlob,
  }
}
