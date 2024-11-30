/* External Imports */
import { ethers } from 'ethersv6'

export const DUMMY_BYTES32: string[] = Array.from(
  {
    length: 10,
  },
  (_, i) => {
    return ethers.keccak256(`0x0${i}`)
  }
)
