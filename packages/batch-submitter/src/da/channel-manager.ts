import { ethers } from 'ethersv6'
import { Channel } from './channel'
import {
  BatchToInboxElement,
  ChannelConfig,
  RollupConfig,
  TxData,
} from './types'
import { CHANNEL_FULL_ERR } from './consts'

export class ChannelManager {
  private blocks: BatchToInboxElement[] = []
  private l1OriginLastClosedChannel: bigint
  private tip: string = ethers.ZeroHash
  private currentChannel: Channel | null = null
  private channelQueue: Channel[] = []
  private txChannels: Map<string, Channel> = new Map()

  constructor(
    private cfg: ChannelConfig,
    private rollupCfg: RollupConfig,
    private l1Client: ethers.Provider
  ) {}

  clear(l1OriginLastClosedChannel: bigint): void {
    this.blocks = []
    this.l1OriginLastClosedChannel = l1OriginLastClosedChannel
    this.tip = ethers.ZeroHash
    this.currentChannel = null
    this.channelQueue = []
    this.txChannels.clear()
  }

  async txData(l1Head: bigint): Promise<[TxData, boolean]> {
    const firstWithTxData = this.channelQueue.find((ch) => ch.hasTxData())
    const dataPending = firstWithTxData && firstWithTxData.hasTxData()

    console.log('Requested tx data', {
      l1Head,
      txDataPending: dataPending,
      blocksPending: this.blocks.length,
    })

    if (dataPending) {
      return this.nextTxData(firstWithTxData)
    }

    if (this.blocks.length === 0) {
      throw new Error('No more blocks')
    }

    await this.ensureChannelWithSpace(l1Head)
    await this.processBlocks()

    return this.nextTxData(this.currentChannel)
  }

  private async ensureChannelWithSpace(l1Head: bigint): Promise<void> {
    if (this.currentChannel && !this.currentChannel.isFull()) {
      return
    }

    const cfg = this.cfg
    const newChannel = new Channel(cfg, this.rollupCfg, this.l1Client)

    this.currentChannel = newChannel
    this.channelQueue.push(newChannel)

    console.info('Created channel', {
      id: newChannel.id(),
      l1Head,
      l1OriginLastClosedChannel: this.l1OriginLastClosedChannel,
      blocks_pending: this.blocks.length,
      batch_type: cfg.batchType,
      compression_algo: cfg.compressionAlgo,
      target_num_frames: cfg.targetNumFrames,
      max_frame_size: cfg.maxFrameSize,
      use_blobs: cfg.useBlobs,
    })
  }

  private async processBlocks(): Promise<void> {
    let blocksAdded = 0

    for (const block of this.blocks) {
      try {
        await this.currentChannel!.addBlock(block)
        console.debug('Added block to channel', {
          id: this.currentChannel!.id(),
          block: block.hash,
        })

        blocksAdded += 1

        if (this.currentChannel!.isFull()) {
          break
        }
      } catch (err) {
        if (err === CHANNEL_FULL_ERR) {
          break
        }
        throw err
      }
    }

    this.blocks = this.blocks.slice(blocksAdded)

    console.debug('Added blocks to channel', {
      blocks_added: blocksAdded,
      blocks_pending: this.blocks.length,
      channel_full: this.currentChannel!.isFull(),
      input_bytes: this.currentChannel!.inputBytes(),
      ready_bytes: this.currentChannel!.readyBytes(),
    })
  }

  addL2Block(block: BatchToInboxElement): void {
    if (this.tip !== ethers.ZeroHash && this.tip !== block.parentHash) {
      throw new Error('ErrReorg')
    }

    this.blocks.push(block)
    this.tip = block.hash
  }

  private nextTxData(channel: Channel | null): [TxData, boolean] {
    if (!channel || !channel.hasTxData()) {
      throw new Error('No more data')
    }
    const [tx, end] = channel.nextTxData()
    this.txChannels.set(tx.id, channel)
    return [tx, end]
  }
}
