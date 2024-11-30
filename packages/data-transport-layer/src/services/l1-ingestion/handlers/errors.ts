export type EventName =
  | 'TransactionEnqueued'
  | 'SequencerBatchAppended'
  | 'SequencerBatchInbox'
  | 'StateBatchAppended'

export class MissingElementError extends Error {
  constructor(public name: EventName) {
    super(`missing event: ${name}`)
  }
}

export class BlobDataExpiredError extends Error {
  constructor(public blobTxHash: string) {
    super(`Blob tx ${blobTxHash} has expired`)
  }
}
