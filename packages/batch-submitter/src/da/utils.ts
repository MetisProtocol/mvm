import { Writer } from './types'
import { toBeArray, zeroPadValue } from 'ethersv6'

export const encodeSpanBatchBits = (
  writer: Writer,
  count: number,
  bits: bigint
): void => {
  const bytes = Math.ceil(count / 8)
  writer.writeBytes(zeroPadValue(toBeArray(bits), bytes))
}
