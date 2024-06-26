import { disperser } from './generated/disperser'

export interface IDisperserClient {
  disperseBlob(
    data: Uint8Array,
    customQuorums: number[]
  ): Promise<disperser.BlobStatus>

  disperseBlobAuthenticated(
    data: Uint8Array,
    quorums: number[]
  ): Promise<disperser.DisperseBlobReply>

  getBlobStatus(requestId: Uint8Array): Promise<disperser.BlobStatusReply>

  retrieveBlob(
    batchHeaderHash: Uint8Array,
    blobIndex: number
  ): Promise<Uint8Array>
}

export interface IEigenDAClient {
  getBlob(batchHeaderHash: Uint8Array, blobIndex: number): Promise<Uint8Array>
  putBlob(txData: Uint8Array): Promise<disperser.BlobInfo>
}

export interface EigenDAClientConfig {
  rpc: string
  statusQueryTimeout: number
  statusQueryRetryInterval: number
  responseTimeout: number
  customQuorumIDs: number[]
  signerPrivateKey: string
  disableTLS: boolean
  putBlobEncodingVersion: Uint8Array
  disablePointVerificationMode: boolean
}

export interface DisperseClientConfig {
  hostname: string
  port: string
  useSecureGrpc: boolean
}
