/* Imports: External */
import { ethers, toBigInt, toNumber } from 'ethers'

export const parseSignatureVParam = (
  v: number | bigint,
  chainId: number
): number => {
  const vNumber = toBigInt(v)
  // for non-eip155 transactions
  if (vNumber === toBigInt(27) || vNumber === toBigInt(28)) {
    return toNumber(vNumber)
  }
  return toNumber(vNumber) - 2 * chainId - 35
}
