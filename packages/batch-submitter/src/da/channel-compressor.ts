// compressor.ts

import * as zlib from 'zlib'
import { MAX_BLOB_SIZE } from './consts'
import { maxDataSize } from './utils'

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
  private compressed: Buffer
  private readOffset = 0

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
    this.compressed = Buffer.alloc(0)
    if (this.algo === CompressionAlgo.Brotli) {
      this.compressed = Buffer.concat([
        this.compressed,
        Buffer.from([CHANNEL_VERSION_BROTLI]),
      ])
    }
  }

  write(data: Uint8Array): Promise<number> {
    return new Promise((resolve, reject) => {
      if (this.fullErr()) {
        reject('Compressor is full')
        return
      }

      this.inputBytes += data.length
      this.stream.on('data', (chunk: Buffer) => {
        this.compressed = Buffer.concat([this.compressed, chunk])
      })
      this.stream.on('end', () => {
        resolve(data.length)
      })
      this.stream.write(data)
      this.stream.end()
    })
  }

  read(p: Uint8Array): number {
    const out = this.compressed.subarray(
      this.readOffset,
      this.readOffset + p.length
    )
    if (!out || !out.length) {
      return 0
    }
    out.copy(p)
    this.readOffset += out.length
    return out.length
  }

  reset(): void {
    this.inputBytes = 0
    this.createCompressStream()
  }

  len(): number {
    return this.compressed.length
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
