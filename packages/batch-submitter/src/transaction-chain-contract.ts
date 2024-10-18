/* External Imports */
import {
  Contract,
  toBeHex,
  toBigInt,
  TransactionRequest,
  TransactionResponse,
} from 'ethersv6'
import { keccak256 } from 'ethers/lib/utils'
import {
  BatchContext,
  encodeAppendSequencerBatch,
  EncodeSequencerBatchOptions,
  remove0x,
} from '@localtest911/core-utils'
import { Promise } from 'bluebird'

interface AppendSequencerBatchParams {
  chainId: number
  shouldStartAtElement: number
  totalElementsToAppend: number
  contexts: BatchContext[]
  transactions: string[]
  blockNumbers: number[]
  seqSigns: string[] // de-sequencer block sign, length equals sequencerTx
}

export { encodeAppendSequencerBatch, BatchContext, AppendSequencerBatchParams }

/**********************
 * Internal Functions *
 *********************/

const APPEND_SEQUENCER_BATCH_METHOD_ID = keccak256(
  Buffer.from('appendSequencerBatchByChainId()')
).slice(2, 10)

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
  return (
    '0x' +
    remove0x(methodId) +
    encodeHex(batch.chainId, 64) +
    remove0x(calldata)
  )
}
