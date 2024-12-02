// channelBuilder.ts
import { ethers } from 'ethersv6'
import {
  BatchToInboxElement,
  ChannelConfig,
  Frame,
  RollupConfig,
} from './types'
import { SpanChannelOut } from './span-channel-out'
import { L2Block } from '@metis.io/core-utils'
import { ChannelCompressor } from './channel-compressor'
import { CHANNEL_FULL_ERR, MAX_BLOB_SIZE } from './consts'
import { maxDataSize } from './utils'
import { Logger } from '@eth-optimism/common-ts'

export class ChannelBuilder {
  public spanChannelOut: SpanChannelOut
  public blocks: L2Block[] = []
  public latestL1Origin: number
  public oldestL1Origin: number
  public latestL2: number
  public oldestL2: number

  constructor(
    private logger: Logger,
    private cfg: ChannelConfig,
    rollupCfg: RollupConfig,
    private l1Client: ethers.Provider
  ) {
    this.spanChannelOut = new SpanChannelOut(
      this.logger,
      rollupCfg.l2ChainID,
      maxDataSize(cfg.targetNumFrames, MAX_BLOB_SIZE) / 0.6, // hardcode 0.6 for now, adjust later
      new ChannelCompressor(this.logger),
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

    try {
      const epoch = await this.l1Client.getBlock(firstTx.l1BlockNumber)
      this.updateBlockInfo(block, epoch.number)
      const startTime = Date.now()
      await this.spanChannelOut.addBlock(block, epoch.hash)
      const endTime = Date.now()
      this.logger.info(
        `Adding block ${block.blockNumber} took ${endTime - startTime} ms`
      )
      return
    } catch (err) {
      if (err === CHANNEL_FULL_ERR) {
        this.logger.info('Channel full, stop adding new blocks')
        return
      }
      this.logger.error(`Failed to add block to span channel`, {
        err,
      })
      throw err
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
