// channel.ts
import { ethers } from 'ethers'
import { ChannelBuilder } from './channel-builder'
import {
  BatchToInboxElement,
  ChannelConfig,
  Frame,
  RollupConfig,
  TxData,
} from './types'
import { Blob } from './blob'

export class Channel {
  private channelBuilder: ChannelBuilder
  private pendingTransactions: Map<string, TxData> = new Map()
  private confirmedTransactions: Map<string, number> = new Map()

  constructor(
    private cfg: ChannelConfig,
    rollupCfg: RollupConfig,
    l1Client: ethers.Provider
  ) {
    this.channelBuilder = new ChannelBuilder(cfg, rollupCfg, l1Client)
  }

  id(): Uint8Array {
    return this.channelBuilder.spanChannelOut.id
  }

  hasTxData(): boolean {
    return this.channelBuilder.hasFrame()
  }

  nextTxData(): [TxData, boolean] {
    const [frame, end] = this.channelBuilder.nextFrame()
    const txData: TxData = {
      frames: [frame],
      asBlob: this.cfg.useBlobs,

      get id(): string {
        let sb = ''
        let curChID = ''
        this.frames.forEach((f) => {
          const chIDStringer = (id: Uint8Array) =>
            Buffer.from(id).toString('hex')
          const frameIdHex = chIDStringer(f.id)
          if (frameIdHex === curChID) {
            sb += `+${frame.frameNumber}`
          } else {
            if (curChID !== '') {
              sb += '|'
            }
            curChID = frameIdHex
            sb += `${chIDStringer(frame.id)}:${frame.frameNumber}`
          }
        })
        return sb
      },

      get blobs(): Blob[] {
        return this.frames.map((f: Frame) => new Blob().fromData(f.data))
      },
    }
    this.pendingTransactions.set(txData.id, txData)
    return [txData, end]
  }

  async addBlock(block: BatchToInboxElement): Promise<void> {
    await this.channelBuilder.addBlock(block)
  }

  isFull(): boolean {
    return this.channelBuilder.isFull()
  }

  fullError(): Error | null {
    return this.channelBuilder.spanChannelOut.fullErr()
  }

  inputBytes(): number {
    return this.channelBuilder.spanChannelOut.inputBytes()
  }

  readyBytes(): number {
    return this.channelBuilder.spanChannelOut.readyBytes()
  }

  pendingFrames(): number {
    return this.channelBuilder.pendingFrames()
  }

  latestL1Origin(): number {
    return this.channelBuilder.latestL1Origin
  }

  oldestL1Origin(): number {
    return this.channelBuilder.oldestL1Origin
  }

  latestL2(): number {
    return this.channelBuilder.latestL2
  }

  oldestL2(): number {
    return this.channelBuilder.oldestL2
  }

  close(): void {
    this.channelBuilder.spanChannelOut.close()
  }

  noneSubmitted(): boolean {
    return (
      this.pendingTransactions.size === 0 &&
      this.confirmedTransactions.size === 0
    )
  }

  txFailed(id: string): void {
    const txData = this.pendingTransactions.get(id)
    if (txData) {
      this.pendingTransactions.delete(id)
      this.channelBuilder
    }
  }

  txConfirmed(id: string, inclusionBlock: number): void {
    this.pendingTransactions.delete(id)
    this.confirmedTransactions.set(id, inclusionBlock)
  }
}
