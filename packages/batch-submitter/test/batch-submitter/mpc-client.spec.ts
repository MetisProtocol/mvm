import { expect } from '../setup'
import { MpcClient } from '../../src/utils/index'
import { randomUUID, sign } from 'crypto'
import { ethers, TransactionRequest } from 'ethers'

describe('MpcClient Test', async () => {
  let mpcClient: MpcClient
  let signId: string
  let mpcId: string
  let mpcAddress: string

  before(async () => {
    mpcClient = new MpcClient('')
    signId = randomUUID()
  })

  it.skip('should match base64 and hex', () => {
    const b64 =
      '+QESAoUG/COsAIMEk+CUklfZ1Hj7cbmMwtGGaxqMUEqLZMeAuKqozaN7AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAlcAAAAAjwAABgAABQAAAAAAAAAAAAAAAAAAAAAAAAEAAAAAY0evSAAAdmvhAAABAAAAAGNHtJ0AAHZsOwAAAgAAAABjR751AAB2bOAAAAIAAAAAY0e/KgAAdmzsABg9EDaSsAACaKq7LrY+sI5LrhedV+E1iI3S5hmnaBtfWF2d9uRBC5rvFIL1EKDIoI2RMaMI0FOzf1fBlsyMTW1S0Wq4SDKyeggYBXJNAaBLxlQ+chp9byiJa3Yl18TuDi02srQtohOmbiQMncmBpg=='
    const hex =
      '0xf90112028506fc23ac00830493e0949257d9d478fb71b98cc2d1866b1a8c504a8b64c780b8aaa8cda37b0000000000000000000000000000000000000000000000000000000000000257000000008f00000600000500000000000000000000000000000000000001000000006347af480000766be1000001000000006347b49d0000766c3b000002000000006347be750000766ce0000002000000006347bf2a0000766cec00183d103692b0000268aabb2eb63eb08e4bae179d57e135888dd2e619a7681b5f585d9df6e4410b9aef1482f510a0c8a08d9131a308d053b37f57c196cc8c4d6d52d16ab84832b27a081805724d01a04bc6543e721a7d6f28896b7625d7c4ee0e2d36b2b42da213a66e240c9dc981a6'

    const decodeHex = mpcClient.base64ToHex(b64)
    // console.log('old and new hex', hex, decodeHex)
    expect(decodeHex).to.equal(hex)

    const transaction = ethers.Transaction.from(hex)

    console.log('Nonce:', transaction.nonce)
    console.log('Gas Price:', transaction.gasPrice.toString())
    console.log('Gas Limit:', transaction.gasLimit.toString())
    console.log('To:', transaction.to)
    console.log('Value:', transaction.value.toString())
    console.log('Data:', transaction.data)
    console.log('V:', transaction.signature.v)
    console.log('R:', transaction.signature.r)
    console.log('S:', transaction.signature.s)
  })

  it.skip('should get mpc info', async () => {
    const mpcInfo = await mpcClient.getLatestMpc()
    console.log('mpc info = ', mpcInfo)
    expect(mpcInfo).to.not.equal(null)
    mpcId = mpcInfo.mpc_id
    mpcAddress = mpcInfo.mpc_address
  })

  it.skip('should post data', async () => {
    // make a tx,
    const nonce: number = 3
    const transaction = {
      nonce,
      gasPrice: ethers.parseUnits('30', 'gwei'),
      gasLimit: ethers.parseUnits('300000', 'wei'),
      to: '0x9257d9d478fb71B98Cc2d1866B1A8C504a8B64C7',
      value: ethers.parseEther('0'),
      data: '0xa8cda37b0000000000000000000000000000000000000000000000000000000000000257000000008f00000600000500000000000000000000000000000000000001000000006347af480000766be1000001000000006347b49d0000766c3b000002000000006347be750000766ce0000002000000006347bf2a0000766cec00183d103692b0000268aabb2eb63eb08e4bae179d57e135888dd2e619a7681b5f585d9df6e4410b9aef14',
      chainId: 17000,
    }

    // const transactionToJson = {
    //   nonce: mpcClient.removeHexLeadingZero(ethers.utils.hexlify(nonce)),
    //   gasPrice: mpcClient.removeHexLeadingZero(
    //     ethers.utils.parseUnits('30', 'gwei').toHexString()
    //   ),
    //   gasLimit: mpcClient.removeHexLeadingZero(
    //     ethers.utils.parseUnits('300000', 'wei').toHexString()
    //   ),
    //   to: '0x9257d9d478fb71B98Cc2d1866B1A8C504a8B64C7',
    //   value: mpcClient.removeHexLeadingZero(
    //     ethers.utils.parseEther('0').toHexString(),
    //     true
    //   ),
    //   data: '0xa8cda37b0000000000000000000000000000000000000000000000000000000000000257000000008f00000600000500000000000000000000000000000000000001000000006347af480000766be1000001000000006347b49d0000766c3b000002000000006347be750000766ce0000002000000006347bf2a0000766cec00183d103692b0000268aabb2eb63eb08e4bae179d57e135888dd2e619a7681b5f585d9df6e4410b9aef14',
    // }

    try {
      const jsonTx = ethers.Transaction.from(transaction).unsignedSerialized
      // const transactionHash = ethers.utils.keccak256(
      //   ethers.utils.defaultAbiCoder.encode(
      //     ['address', 'uint256', 'uint256', 'uint256', 'uint256', 'bytes'],
      //     [
      //       transaction.to,
      //       transaction.value.toHexString(),
      //       transaction.nonce,
      //       transaction.gasPrice.toHexString(),
      //       transaction.gasLimit.toHexString(),
      //       transaction.data || '0x',
      //     ]
      //   )
      // )

      const postData = {
        sign_id: signId,
        mpc_id: mpcId,
        sign_type: 0,
        sign_data: jsonTx,
        sign_msg: '',
        // "chain_id": "themis-kRsLrv"
      }
      console.log('sign data prepare = ', JSON.stringify(postData))
      const signResp = await mpcClient.proposeMpcSign(postData)
      console.log('sign resp = ', signResp)
      expect(signResp).to.not.equal(null)
    } catch (x) {
      console.error(x)
    }
  })

  it.skip('should get signed tx', async () => {
    const signedTx = await mpcClient.getMpcSignWithTimeout(
      signId,
      300 * 1000,
      15 * 1000
    )
    const signedTxHex = mpcClient.base64ToHex(signedTx)
    console.log('signed tx = ', signedTxHex)
    expect(signedTxHex).to.not.equal('')
  })
})
