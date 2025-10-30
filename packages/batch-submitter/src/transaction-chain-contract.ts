/* External Imports */
import {
  BatchContext,
  encodeAppendSequencerBatch,
  EncodeSequencerBatchOptions,
  remove0x,
} from '@metis.io/core-utils'
import {
  Contract,
  keccak256,
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

const APPEND_SEQUENCER_BATCH_METHOD_ID = keccak256(
  Buffer.from('appendSequencerBatchByChainId()', 'utf-8').toString('hex')
).slice(0, 10)

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
  const methodId = APPEND_SEQUENCER_BATCH_METHOD_ID
  const calldata = await encodeAppendSequencerBatch(batch, opts)
  return methodId + encodeHex(batch.chainId, 64) + remove0x(calldata)
}
