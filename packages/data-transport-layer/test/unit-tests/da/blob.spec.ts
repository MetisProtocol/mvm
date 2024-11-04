import { parseFrames } from '../../../src/da/blob/frame'
import { l1BlobData } from '../examples/l1-data'
import { remove0x } from '@localtest911/core-utils'
import {
  BatchData,
  batchReader,
  Channel,
  RawSpanBatch,
  SpanBatchType,
} from '../../../src/da/blob/channel'
import { Blob } from '../../../src/da/blob/blob'
import { ethers, hexlify } from 'ethersv6'

describe('Decode Blob Transaction', function () {
  this.timeout(60000)

  it('should decode blob data and restore transactions', async () => {
    const blob = new Blob(l1BlobData)
    const frames = parseFrames(blob.toData(), 0)

    const channel = new Channel(hexlify(frames[0].id), frames[0].inclusionBlock)
    for (const item of frames) {
      channel.addFrame(item)
    }

    const readBatch = await batchReader(channel.reader())

    const batches = []
    const batchTypes = []
    const comprAlgos = []

    let batchData: BatchData | null
    while ((batchData = await readBatch())) {
      if (!batchData) {
        console.log('No more batch data')
        break
      }
      console.log(`Read new batch, type: ${batchData.batchType}`)
      if (batchData.batchType !== SpanBatchType) {
        throw new Error(`Unsupported batch type ${batchData.batchType}`)
      }
      const spanBatch = batchData.inner as RawSpanBatch
      batchData.inner = await spanBatch.derive(ethers.toBigInt(1088))
      batches.push(batchData.inner)
      batchTypes.push(batchData.batchType)
      comprAlgos.push(batchData.comprAlgo)
    }

    for (let i = 0; i < batches.length; i++) {
      console.log(
        `Batch ${i}, type: ${batchTypes[i]}, compression algo: ${comprAlgos[i]}`
      )
      console.log(
        JSON.stringify(
          batches[i],
          (key, value) => {
            if (typeof value === 'bigint') {
              return value.toString()
            } else if (value instanceof Buffer || value instanceof Uint8Array) {
              return hexlify(value)
            }
            return value
          },
          2
        )
      )
    }
  })
})
