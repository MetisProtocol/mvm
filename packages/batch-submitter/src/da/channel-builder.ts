// channelBuilder.ts
import { ethers } from 'ethersv6'
import {
  BatchToInboxElement,
  ChannelConfig,
  Frame,
  RollupConfig,
} from './types'
import { SpanChannelOut } from './span-channel-out'
import { L2Block, sleep } from '@localtest911/core-utils'
import { ChannelCompressor } from './channel-compressor'
import { CHANNEL_FULL_ERR } from './consts'

export class ChannelBuilder {
  public spanChannelOut: SpanChannelOut
  public blocks: L2Block[] = []
  public latestL1Origin: number
  public oldestL1Origin: number
  public latestL2: number
  public oldestL2: number

  constructor(
    private cfg: ChannelConfig,
    rollupCfg: RollupConfig,
    private l1Client: ethers.Provider
  ) {
    this.spanChannelOut = new SpanChannelOut(
      rollupCfg.l2ChainID,
      5000, // maxDataSize(cfg.targetNumFrames, MAX_BLOB_SIZE) / 0.6, // hardcode 0.6 for now, adjust later
      new ChannelCompressor(),
      { maxBlocksPerSpanBatch: 0 } // default to 0 - no maximum
    )
    this.latestL1Origin = 0
    this.oldestL1Origin = 0
    this.latestL2 = 0
    this.oldestL2 = 0
  }

  hasFrame(): boolean {
    return this.spanChannelOut.readyBytes() > 0
  }

  nextFrame(): [Frame, boolean] {
    return this.spanChannelOut.outputFrame(this.cfg.maxFrameSize)
  }

  async addBlock(block: BatchToInboxElement): Promise<void> {
    if (this.isFull()) {
      throw this.spanChannelOut.fullErr
    }

    if (!block.txs) {
      throw new Error('Empty block')
    }

    const firstTx = block.txs[0]

    let retry = 3
    let lastErr: Error | null
    while (retry--) {
      try {
        const epoch = await this.l1Client.getBlock(firstTx.l1BlockNumber)
        this.updateBlockInfo(block, epoch.number)
        const startTime = Date.now()
        await this.spanChannelOut.addBlock(block, epoch.hash)
        const endTime = Date.now()
        console.log(
          `Adding block ${block.blockNumber} took ${endTime - startTime} ms`
        )
        return
      } catch (err) {
        if (err === CHANNEL_FULL_ERR) {
          return
        }
        lastErr = err
        console.error(
          `Failed to add block to span channel, remain retry time: ${retry}`
        )
        await sleep(3000)
      }
    }
    if (retry === 0) {
      throw lastErr
    }
  }

  private updateBlockInfo(block: BatchToInboxElement, l1Number: number): void {
    const blockNumber = block.blockNumber
    if (blockNumber > this.latestL2) {
      this.latestL2 = blockNumber
    }
    if (this.oldestL2 === 0 || blockNumber < this.oldestL2) {
      this.oldestL2 = blockNumber
    }

    if (l1Number > this.latestL1Origin) {
      this.latestL1Origin = l1Number
    }
    if (this.oldestL1Origin === 0 || l1Number < this.oldestL1Origin) {
      this.oldestL1Origin = l1Number
    }
  }

  isFull(): boolean {
    return this.spanChannelOut.fullErr() !== null
  }

  pendingFrames(): number {
    return Math.ceil(this.spanChannelOut.readyBytes() / this.cfg.maxFrameSize)
  }
}
