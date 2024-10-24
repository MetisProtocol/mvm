import { Writer } from './types'
import { toBeArray, zeroPadValue } from 'ethersv6'
import { FRAME_OVERHEAD_SIZE } from './consts'

export const encodeSpanBatchBits = (
  writer: Writer,
  count: number,
  bits: bigint
): void => {
  const bytes = Math.ceil(count / 8)
  writer.writeBytes(zeroPadValue(toBeArray(bits), bytes))
}

export const maxDataSize = (frames: number, maxFrameSize: number) => {
  if (maxFrameSize < FRAME_OVERHEAD_SIZE) {
    throw new Error('Frame size too small')
  }

  return frames * (maxFrameSize - FRAME_OVERHEAD_SIZE)
}
