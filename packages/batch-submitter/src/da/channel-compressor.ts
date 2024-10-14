// compressor.ts

import * as zlib from 'zlib'
import { promisify } from 'util'
import { FRAME_OVERHEAD_SIZE, MAX_BLOB_SIZE } from './consts'

export enum CompressionAlgo {
  Zlib = 'zlib',
  Brotli = 'brotli',
}

const CHANNEL_VERSION_BROTLI: number = 0x01

export interface CompressorConfig {
  targetOutputSize: number
  approxComprRatio: number
  compressionAlgo: CompressionAlgo
}

export class ChannelCompressor {
  private inputBytes: number = 0
  private stream: zlib.BrotliCompress | zlib.Deflate
  private readonly algo: CompressionAlgo

  constructor(
    private config: CompressorConfig = {
      targetOutputSize: maxDataSize(1, MAX_BLOB_SIZE - 1), // default op value
      approxComprRatio: 0.6, // default op value
      compressionAlgo: CompressionAlgo.Brotli, // default value after fjord
    }
  ) {
    this.algo = config.compressionAlgo
    this.createCompressStream()
  }

  private createCompressStream(): void {
    this.stream =
      this.algo === CompressionAlgo.Zlib
        ? zlib.createDeflate({
            level: zlib.constants.Z_BEST_COMPRESSION,
          })
        : zlib.createBrotliCompress({
            params: {
              [zlib.constants.BROTLI_PARAM_MODE]:
                zlib.constants.BROTLI_MODE_TEXT,
              [zlib.constants.BROTLI_PARAM_QUALITY]:
                zlib.constants.BROTLI_MAX_QUALITY,
            },
          })
    if (this.algo === CompressionAlgo.Brotli) {
      this.stream.write(Buffer.from([CHANNEL_VERSION_BROTLI]))
    }
  }

  async write(data: Uint8Array): Promise<number> {
    if (this.fullErr()) {
      throw new Error('Compressor is full')
    }

    this.inputBytes += data.length
    this.stream.write(data)

    return data.length
  }

  read(p: Uint8Array): number {
    const out = this.stream.read(p.length)
    if (!out) {
      return 0
    }
    out.copy(p)
    return out.length
  }

  reset(): void {
    this.inputBytes = 0
    this.createCompressStream()
  }

  len(): number {
    return this.stream.readableLength
  }

  fullErr(): Error | null {
    if (this.inputTargetReached()) {
      return new Error('ErrCompressorFull')
    }
    return null
  }

  private inputThreshold(): number {
    return Math.floor(
      this.config.targetOutputSize / this.config.approxComprRatio
    )
  }

  private inputTargetReached(): boolean {
    return this.inputBytes >= this.inputThreshold()
  }

  getCompressed(): Buffer {
    return this.stream.read() || Buffer.alloc(0)
  }
}

const maxDataSize = (frames: number, maxFrameSize: number) => {
  if (maxFrameSize < FRAME_OVERHEAD_SIZE) {
    throw new Error('Frame size too small')
  }

  return frames * (maxFrameSize - FRAME_OVERHEAD_SIZE)
}
