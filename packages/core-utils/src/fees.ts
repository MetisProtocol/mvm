/**
 * Fee related serialization and deserialization
 */

import { remove0x } from './common'
import { toBigInt } from 'ethers'

const txDataZeroGas = 4
const txDataNonZeroGasEIP2028 = 16
const big10 = toBigInt(10)

export const scaleDecimals = (
  value: number | bigint,
  decimals: number | bigint
): bigint => {
  value = toBigInt(value)
  decimals = toBigInt(decimals)
  // 10**decimals
  const divisor = big10 ** decimals
  return value / divisor
}

// data is the RLP encoded unsigned transaction
export const calculateL1GasUsed = (
  data: string | Buffer,
  overhead: number | bigint
): bigint => {
  const [zeroes, ones] = zeroesAndOnes(data)
  const zeroesCost = zeroes * txDataZeroGas
  // Add a buffer to account for the signature
  const onesCost = (ones + 68) * txDataNonZeroGasEIP2028
  return toBigInt(onesCost) + toBigInt(zeroesCost) + toBigInt(overhead)
}

export const calculateL1Fee = (
  data: string | Buffer,
  overhead: number | bigint,
  l1GasPrice: number | bigint,
  scalar: number | bigint,
  decimals: number | bigint
): bigint => {
  const l1GasUsed = calculateL1GasUsed(data, overhead)
  const l1Fee = l1GasUsed * toBigInt(l1GasPrice)
  const scaled = l1Fee * toBigInt(scalar)
  const result = scaleDecimals(scaled, decimals)
  return result
}

// Count the number of zero bytes and non zero bytes in a buffer
export const zeroesAndOnes = (data: Buffer | string): Array<number> => {
  if (typeof data === 'string') {
    data = Buffer.from(remove0x(data), 'hex')
  }
  let zeros = 0
  let ones = 0
  for (const byte of data) {
    if (byte === 0) {
      zeros++
    } else {
      ones++
    }
  }
  return [zeros, ones]
}
