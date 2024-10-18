import { ethers, toBigInt } from 'ethersv6'

/* Imports: Internal */
import { expect } from '../../../../setup'
import { handleEventsSequencerBatchAppended } from '../../../../../src/services/l1-ingestion/handlers/sequencer-batch-appended'
import { SequencerBatchAppendedExtraData } from '../../../../../src/types'

describe('Event Handlers: CanonicalTransactionChain.SequencerBatchAppended', () => {
  describe('handleEventsSequencerBatchAppended.parseEvent', () => {
    // This tests the behavior of parsing a real mainnet transaction,
    // so it will break if the encoding scheme changes.

    // Transaction and extra data from
    // https://etherscan.io/tx/0x6effe006836b841205ace4d99d7ae1b74ee96aac499a3f358b97fccd32ee9af2
    const exampleExtraData = {
      timestamp: 1614862375,
      blockNumber: 11969713,
      submitter: '0xfd7d4de366850c08ee2cba32d851385a3071ec8d',
      l1TransactionHash:
        '0x6effe006836b841205ace4d99d7ae1b74ee96aac499a3f358b97fccd32ee9af2',
      gasLimit: '548976',
      prevTotalElements: toBigInt(73677),
      batchIndex: toBigInt(743),
      batchSize: toBigInt(101),
      batchRoot:
        '10B99425FB53AD7D40A939205C0F7B35CBB89AB4D67E7AE64BDAC5F1073943B4',
      batchExtraData: '',
    }

    it('should error on malformed transaction data', async () => {
      const input1: [any, SequencerBatchAppendedExtraData, number, any] = [
        {
          args: {
            _startingQueueIndex: toBigInt(0),
            _numQueueElements: toBigInt(0),
            _totalElements: toBigInt(0),
          },
        }, // event
        {
          l1TransactionData: '0x00000',
          blobIndex: 0,
          blobCount: 0,
          ...exampleExtraData,
        }, // extraData
        0, // l2ChainId
        {}, // options
      ]

      expect(() => {
        handleEventsSequencerBatchAppended.parseEvent(...input1)
      }).to.throw(
        `Block ${input1[1].blockNumber} transaction data is invalid for decoding: ${input1[1].l1TransactionData} , ` +
          `converted buffer length is < 12.`
      )
    })
  })
})
