/* Imports: External */
export interface EventArgsAddressSet {
  _name: string
  _newAddress: string
}

export interface EventArgsTransactionEnqueued {
  _chainId: bigint
  _l1TxOrigin: string
  _target: string
  _gasLimit: bigint
  _data: string
  _queueIndex: bigint
  _timestamp: bigint
}

export interface EventArgsTransactionBatchAppended {
  _chainId: bigint
  _batchIndex: bigint
  _batchRoot: string
  _batchSize: bigint
  _prevTotalElements: bigint
  _extraData: string
}

export interface EventArgsStateBatchAppended {
  _chainId: bigint
  _batchIndex: bigint
  _batchRoot: string
  _batchSize: bigint
  _prevTotalElements: bigint
  _extraData: string
}

export interface EventArgsSequencerBatchAppended {
  _chainId: bigint
  _startingQueueIndex: bigint
  _numQueueElements: bigint
  _totalElements: bigint
}

export interface EventArgsVerifierStake {
  _sender: string
  _chainId: bigint
  _batchIndex: bigint
  _blockNumber: bigint
  _amount: bigint
}

export interface EventArgsAppendBatchElement {
  _chainId: bigint
  _batchIndex: bigint
  _shouldStartAtElement: number
  _totalElementsToAppend: number
  _txBatchSize: bigint
  _txBatchTime: bigint
  _root: string
}
