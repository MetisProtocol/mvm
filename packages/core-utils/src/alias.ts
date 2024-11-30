import { ethers, toBeHex, toBigInt } from 'ethersv6'

export const L1_TO_L2_ALIAS_OFFSET =
  '0x1111000000000000000000000000000000001111'

export const bnToAddress = (bn: bigint | number): string => {
  bn = ethers.toBigInt(bn)
  if (bn < 0) {
    bn +=
      ethers.toBigInt('0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF') +
      toBigInt(1)
  }

  return toBeHex(bn, 40)
}

export const applyL1ToL2Alias = (address: string): string => {
  if (!ethers.isAddress(address)) {
    throw new Error(`not a valid address: ${address}`)
  }

  return bnToAddress(ethers.toBigInt(address) + toBigInt(L1_TO_L2_ALIAS_OFFSET))
}

export const undoL1ToL2Alias = (address: string): string => {
  if (!ethers.isAddress(address)) {
    throw new Error(`not a valid address: ${address}`)
  }

  return bnToAddress(ethers.toBigInt(address) - toBigInt(L1_TO_L2_ALIAS_OFFSET))
}
