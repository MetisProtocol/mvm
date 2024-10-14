// channelBuilder.ts
import { ethers } from 'ethers'
import { ChannelConfig, Frame, RollupConfig } from './types'
import { SpanChannelOut } from './span-channel-out'
import { L2Block } from '@metis.io/core-utils'
import { ChannelCompressor } from './channel-compressor'

export class ChannelBuilder {
  public spanChannelOut: SpanChannelOut
  public blocks: L2Block[] = []
  public latestL1Origin: number
  public oldestL1Origin: number
  public latestL2: number
  public oldestL2: number

  constructor(
    private cfg: ChannelConfig,
    private rollupCfg: RollupConfig,
    private l1Client: ethers.Provider
  ) {
    this.spanChannelOut = new SpanChannelOut(
      rollupCfg.l2ChainID,
      cfg.targetNumFrames,
      new ChannelCompressor(),
      rollupCfg,
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

  async addBlock(block: L2Block): Promise<void> {
    if (this.isFull()) {
      throw this.spanChannelOut.fullErr
    }

    if (!block.transactions) {
      throw new Error('Empty block')
    }

    const firstTx = block.transactions[0]
    const epoch = await this.l1Client.getBlock(firstTx.l1BlockNumber)

    this.updateBlockInfo(block, epoch)

    await this.spanChannelOut.addBlock(this.rollupCfg, block, epoch.hash)
  }

  private updateBlockInfo(block: L2Block, l1Info: ethers.Block): void {
    const blockNumber = Number(block.number)
    if (blockNumber > this.latestL2) {
      this.latestL2 = blockNumber
    }
    if (this.oldestL2 === 0 || blockNumber < this.oldestL2) {
      this.oldestL2 = blockNumber
    }

    if (l1Info.number > this.latestL1Origin) {
      this.latestL1Origin = l1Info.number
    }
    if (this.oldestL1Origin === 0 || l1Info.number < this.oldestL1Origin) {
      this.oldestL1Origin = l1Info.number
    }
  }

  isFull(): boolean {
    return this.spanChannelOut.fullErr() !== null
  }

  pendingFrames(): number {
    return Math.ceil(this.spanChannelOut.readyBytes() / this.cfg.maxFrameSize)
  }
}
