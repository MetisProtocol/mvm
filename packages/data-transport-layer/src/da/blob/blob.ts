import { blobToKzgCommitment } from 'c-kzg'
import { createHash } from 'crypto'

const BlobSize = 4096 * 32
const MaxBlobDataSize = (4 * 31 + 3) * 1024 - 4
const EncodingVersion = 0
const VersionOffset = 1
const Rounds = 1024

export class Blob {
  constructor(public readonly data: Uint8Array) {}

  toString(): string {
    return Buffer.from(this.data).toString('hex')
  }

  terminalString(): string {
    return `${Buffer.from(this.data.slice(0, 3)).toString(
      'hex'
    )}..${Buffer.from(this.data.slice(BlobSize - 3)).toString('hex')}`
  }

  toData(): Uint8Array {
    if (this.data[VersionOffset] !== EncodingVersion) {
      throw new Error(
        `Invalid encoding version, expected: ${EncodingVersion}, got: ${this.data[VersionOffset]}`
      )
    }

    const outputLen = (this.data[2] << 16) | (this.data[3] << 8) | this.data[4]
    if (outputLen > MaxBlobDataSize) {
      throw new Error(`Invalid length for blob: ${outputLen}`)
    }

    const output = new Uint8Array(MaxBlobDataSize)
    output.set(this.data.slice(5, 32), 0)

    let opos = 28
    let ipos = 32

    const encodedByte = new Uint8Array(4)
    encodedByte[0] = this.data[0]

    for (let i = 1; i < 4; i++) {
      ;[encodedByte[i], opos, ipos] = this.decodeFieldElement(
        opos,
        ipos,
        output
      )
    }
    opos = this.reassembleBytes(opos, encodedByte, output)

    for (let i = 1; i < Rounds && opos < outputLen; i++) {
      for (let j = 0; j < 4; j++) {
        ;[encodedByte[j], opos, ipos] = this.decodeFieldElement(
          opos,
          ipos,
          output
        )
      }
      opos = this.reassembleBytes(opos, encodedByte, output)
    }

    for (let i = outputLen; i < MaxBlobDataSize; i++) {
      if (output[i] !== 0) {
        throw new Error(`Extraneous data in output at position ${i}`)
      }
    }
    for (; ipos < BlobSize; ipos++) {
      if (this.data[ipos] !== 0) {
        throw new Error(`Extraneous data in blob at position ${ipos}`)
      }
    }

    return output.slice(0, outputLen)
  }

  clear(): void {
    this.data.fill(0)
  }

  versionedHash(): string {
    const hasher = createHash('sha256')
    hasher.update(blobToKzgCommitment(this.data))
    return '0x01' + hasher.digest('hex').substring(2)
  }

  private decodeFieldElement(
    opos: number,
    ipos: number,
    output: Uint8Array
  ): [number, number, number] {
    if (ipos + 32 > BlobSize) {
      throw new Error(`Invalid input position during decoding: ipos=${ipos}`)
    }
    const byteValue = this.data[ipos]
    if (byteValue & 0b1100_0000) {
      throw new Error(`Invalid field element: ${byteValue}`)
    }
    output.set(this.data.slice(ipos + 1, ipos + 32), opos)
    return [byteValue, opos + 32, ipos + 32]
  }

  private reassembleBytes(
    opos: number,
    encodedByte: Uint8Array,
    output: Uint8Array
  ): number {
    opos--
    const x =
      (encodedByte[0] & 0b0011_1111) | ((encodedByte[1] & 0b0011_0000) << 2)
    const y =
      (encodedByte[1] & 0b0000_1111) | ((encodedByte[3] & 0b0000_1111) << 4)
    const z =
      (encodedByte[2] & 0b0011_1111) | ((encodedByte[3] & 0b0011_0000) << 2)

    output[opos - 32] = z
    output[opos - 64] = y
    output[opos - 96] = x

    return opos
  }
}
