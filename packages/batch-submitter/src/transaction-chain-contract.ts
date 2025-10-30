/* External Imports */
import {
  BatchContext,
  encodeAppendSequencerBatch,
  EncodeSequencerBatchOptions,
  remove0x,
} from '@metis.io/core-utils'
import {
  Contract,
  toBeHex,
  toBigInt,
  TransactionRequest,
  TransactionResponse,
} from 'ethersv6'

interface AppendSequencerBatchParams {
  chainId: number
  shouldStartAtElement: number
  totalElementsToAppend: number
  contexts: BatchContext[]
  transactions: string[]
  blockNumbers: number[]
  seqSigns: string[] // de-sequencer block sign, length equals sequencerTx
}

export { AppendSequencerBatchParams, BatchContext, encodeAppendSequencerBatch }

/**********************
 * Internal Functions *
 *********************/

const appendSequencerBatch = async (
  CanonicalTransactionChain: Contract,
  batch: AppendSequencerBatchParams,
  options?: TransactionRequest,
  opts?: EncodeSequencerBatchOptions
): Promise<TransactionResponse> => {
  return CanonicalTransactionChain.runner.sendTransaction({
    to: await CanonicalTransactionChain.getAddress(),
    data: await getEncodedCalldata(batch, opts),
    ...options,
  })
}
const encodeHex = (val: any, len: number) =>
  remove0x(toBeHex(toBigInt(val), len))
export const getEncodedCalldata = async (
  batch: AppendSequencerBatchParams,
  opts?: EncodeSequencerBatchOptions
): Promise<string> => {
  const methodId = '0xa8cda37b' // appendSequencerBatchByChainId()
  const calldata = await encodeAppendSequencerBatch(batch, opts)
  return methodId + encodeHex(batch.chainId, 64) + remove0x(calldata)
}
