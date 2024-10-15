export interface EventArgsAddressSet {
  _name: string
  _newAddress: string
  _oldAddress: string
}

export interface EventArgsTransactionEnqueued {
  _l1TxOrigin: string
  _target: string
  _gasLimit: bigint
  _data: string
  _queueIndex: bigint
  _timestamp: bigint
}

export interface EventArgsTransactionBatchAppended {
  _batchIndex: bigint
  _batchRoot: string
  _batchSize: bigint
  _prevTotalElements: bigint
  _extraData: string
}

export interface EventArgsStateBatchAppended {
  _batchIndex: bigint
  _batchRoot: string
  _batchSize: bigint
  _prevTotalElements: bigint
  _extraData: string
}

export interface EventArgsSequencerBatchAppended {
  _startingQueueIndex: bigint
  _numQueueElements: bigint
  _totalElements: bigint
}
