import { createHash } from 'crypto'
import {
  Blob as CBlob,
  blobToKzgCommitment,
  Bytes48,
  verifyBlobKzgProof,
} from 'c-kzg'
import { ethers } from 'ethers'

const BlobSize = 4096 * 32
const MaxBlobDataSize = (4 * 31 + 3) * 1024 - 4
const EncodingVersion = 0
const VersionOffset = 1
const Rounds = 1024

const hexStringToUint8Array = (hexString: string): Uint8Array => {
  hexString = hexString.replace(/^0x/, '').replace(/\s/g, '')

  if (hexString.length % 2 !== 0) {
    throw new Error('Invalid hex string')
  }

  const arrayBuffer = new Uint8Array(hexString.length / 2)

  for (let i = 0; i < hexString.length; i += 2) {
    const byteValue = parseInt(hexString.substring(i, i + 2), 16)
    if (isNaN(byteValue)) {
      throw new Error('Invalid hex string')
    }
    arrayBuffer[i / 2] = byteValue
  }

  return arrayBuffer
}

export class Blob {
  public readonly data: Uint8Array = new Uint8Array(BlobSize)

  static kzgToVersionedHash(commitment: Bytes48): string {
    const hasher = createHash('sha256')
    hasher.update(commitment)
    return hasher.digest('hex')
  }

  static verifyBlobProof(
    blob: Blob,
    commitment: Bytes48,
    proof: Bytes48
  ): boolean {
    return verifyBlobKzgProof(blob.data as CBlob, commitment, proof)
  }

  fromData(data: Uint8Array): Blob {
    if (data.length > MaxBlobDataSize) {
      throw new Error(`Input too large: len=${data.length}`)
    }
    this.clear()

    let readOffset = 0
    const read1 = (): number => {
      if (readOffset >= data.length) {
        return 0
      }
      return data[readOffset++]
    }

    let writeOffset = 0
    const buf31 = new Uint8Array(31)
    const zero31 = new Uint8Array(31)

    const read31 = (): void => {
      if (readOffset >= data.length) {
        buf31.set(zero31)
        return
      }
      const n = Math.min(31, data.length - readOffset)
      buf31.set(data.slice(readOffset, readOffset + n))
      readOffset += n
    }

    const write1 = (v: number): void => {
      if (writeOffset % 32 !== 0) {
        throw new Error(`Invalid byte write offset: ${writeOffset}`)
      }
      if (v & 0b1100_0000) {
        throw new Error(`Invalid 6 bit value: 0b${v.toString(2)}`)
      }
      this.data[writeOffset++] = v
    }

    const write31 = (): void => {
      if (writeOffset % 32 !== 1) {
        throw new Error(`Invalid bytes31 write offset: ${writeOffset}`)
      }
      this.data.set(buf31, writeOffset)
      writeOffset += 31
    }

    for (let round = 0; round < Rounds && readOffset < data.length; round++) {
      if (round === 0) {
        buf31[0] = EncodingVersion
        const ilen = data.length
        buf31[1] = (ilen >> 16) & 0xff
        buf31[2] = (ilen >> 8) & 0xff
        buf31[3] = ilen & 0xff
        read31()
      } else {
        read31()
      }

      const x = read1()
      write1(x & 0b0011_1111)
      write31()

      read31()
      const y = read1()
      write1((y & 0b0000_1111) | ((x & 0b1100_0000) >> 2))
      write31()

      read31()
      const z = read1()
      write1(z & 0b0011_1111)
      write31()

      read31()
      write1(((z & 0b1100_0000) >> 2) | ((y & 0b1111_0000) >> 4))
      write31()
    }

    if (readOffset < data.length) {
      throw new Error(
        `Expected to fit data but failed, read offset: ${readOffset}, data length: ${data.length}`
      )
    }

    return this
  }

  toString(): string {
    return Buffer.from(this.data).toString('hex')
  }

  terminalString(): string {
    return `${Buffer.from(this.data.slice(0, 3)).toString(
      'hex'
    )}..${Buffer.from(this.data.slice(BlobSize - 3)).toString('hex')}`
  }

  async computeKZGCommitment(): Promise<Bytes48> {
    return blobToKzgCommitment(this.data as CBlob)
  }

  clear(): void {
    this.data.fill(0)
  }
}
