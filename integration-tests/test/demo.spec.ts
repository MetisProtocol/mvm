import { expect } from 'chai'
import assert = require('assert')
import { JsonRpcProvider, TransactionResponse } from '@ethersproject/providers'
import { BigNumber, Contract, Wallet, utils } from 'ethers'
import { predeploys, getContractInterface } from '@metis.io/contracts'
import { Watcher } from '@eth-optimism/core-utils'
import dotenv = require('dotenv')
import * as path from 'path';

export const getEnvironment = async (): Promise<{
    l1Provider: JsonRpcProvider,
    l2Provider: JsonRpcProvider,
    l2PeerProvider: JsonRpcProvider,
    l1Wallet: Wallet,
    l2Wallet: Wallet,
    l2PeerWallet: Wallet
}> => {
    l1Provider = new JsonRpcProvider("http://localhost:9545")
    l2Provider = new JsonRpcProvider("http://localhost:8545")
    l2PeerProvider = new JsonRpcProvider("http://localhost:10545")
    
    l1Wallet = new Wallet("0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80", l1Provider)
    l2Wallet = new Wallet("0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80", l2Provider)
    l2PeerWallet = new Wallet("0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80", l2PeerProvider)


    return {
        l1Provider,
        l2Provider,
        l2PeerProvider,
        l1Wallet,
        l2Wallet,
        l2PeerWallet
    }
}


const PROXY_SEQUENCER_ENTRYPOINT_ADDRESS = '0x4200000000000000000000000000000000000004'
const MVM_Coinbase_Address = '0xDeadDeAddeAddEAddeadDEaDDEAdDeaDDeAD0000'
const OVM_ETH_Address = '0x420000000000000000000000000000000000000A'
const TAX_ADDRESS = '0x1234123412341234123412341234123412341234'

const NON_NULL_BYTES32 =
  '0x1111111111111111111111111111111111111111111111111111111111111111'
const NON_ZERO_ADDRESS = '0x1111111111111111111111111111111111111111'
const INITIAL_TOTAL_L1_SUPPLY = 5000
const FINALIZATION_GAS = 3_200_000

var depositAmount

let l1Provider: JsonRpcProvider
let l2Provider: JsonRpcProvider
let l2PeerProvider: JsonRpcProvider
let l1Wallet: Wallet
let l2Wallet: Wallet
let l2PeerWallet: Wallet
let AddressManager: Contract
let watcher: Watcher

describe('Fee Payment Integration Tests', async () => {
  const envPath = path.join(__dirname, '/.env');
  dotenv.config({ path: envPath })

  let OVM_L1ETHGateway: Contract
  let OVM_ETH: Contract
  let MVM_Coinbase: Contract
  let MVM_CoinbasePeer: Contract
  let OVM_L2CrossDomainMessenger: Contract

  const getBalances = async ():
    Promise<{
      l1UserBalance: BigNumber,
      l2UserBalance: BigNumber,
      l1GatewayBalance: BigNumber,
      sequencerBalance: BigNumber,
    }> => {
      const l1UserBalance = await l1Wallet.getBalance()
      const l2UserBalance = await MVM_Coinbase.balanceOf(l2Wallet.address)
      const sequencerBalance = await MVM_Coinbase.balanceOf(PROXY_SEQUENCER_ENTRYPOINT_ADDRESS)
      const l1GatewayBalance = await MVM_Coinbase.balanceOf(predeploys.OVM_SequencerFeeVault)
      return {
        l1UserBalance,
        l2UserBalance,
        l1GatewayBalance,
        sequencerBalance
      }
    }

  before(async () => {
    const system = await getEnvironment()
    l1Provider = system.l1Provider
    l2Provider = system.l2Provider
    l2PeerProvider = system.l2Provider
    l1Wallet = system.l1Wallet
    l2Wallet = system.l2Wallet
    l2PeerWallet = system.l2Wallet

    const addressManagerAddress = "0x5FbDB2315678afecb367f032d93F642f64180aa3"
    const addressManagerInterface = getContractInterface('Lib_AddressManager')
    AddressManager = new Contract(addressManagerAddress, addressManagerInterface, l1Provider)
    const mvmAddressManagerAddress = "0xe7f1725E7734CE288F8367e1Bb143E90bb3F0512"
    const mvmAddressManagerInterface = getContractInterface('MVM_AddressManager')
    const MvmAddressManager = new Contract(mvmAddressManagerAddress, mvmAddressManagerInterface, l1Provider)
    
    
    OVM_ETH = new Contract(
      OVM_ETH_Address,
      getContractInterface('OVM_ETH'),
      l2Wallet
    )
    MVM_Coinbase = new Contract(
      MVM_Coinbase_Address,
      getContractInterface('MVM_Coinbase'),
      l2Wallet
    )
    MVM_CoinbasePeer = new Contract(
      MVM_Coinbase_Address,
      getContractInterface('MVM_Coinbase'),
      l2PeerWallet
    )
    console.log(
      await l1Wallet.address,
      await l2Wallet.address,
      await AddressManager.getAddress('MVM_AddressManager'),
      await MvmAddressManager.getAddress('429_MVM_Sequencer'),
      await AddressManager.getAddress('Proxy__OVM_L1StandardBridge'),
      await AddressManager.getAddress('OVM_L2BatchMessageRelayer'))
    const l1StandardBridgeInterface = getContractInterface('IL1StandardBridge')

    OVM_L1ETHGateway = new Contract(
      await AddressManager.getAddress('Proxy__OVM_L1StandardBridge'),
      l1StandardBridgeInterface,
      l1Wallet
    )
    OVM_L2CrossDomainMessenger = new Contract(
      predeploys.L2StandardBridge,
      getContractInterface('L2StandardBridge'),
      l2Wallet
    )
    
    depositAmount = utils.parseEther('100')
    console.log("test:"+depositAmount)
    // let postBalances = await getBalances()
    // console.log(postBalances.l1UserBalance + "," + postBalances.l2UserBalance + "," + postBalances.l1GatewayBalance + "," + postBalances.sequencerBalance)


  })
  

  beforeEach(async () => {
    
    console.log("input balance:"+await l1Wallet.getBalance()+","+depositAmount);
    // postBalances = await getBalances()
    // console.log(postBalances.l1UserBalance + "," + postBalances.l2UserBalance + "," + postBalances.l1GatewayBalance + "," + postBalances.sequencerBalance)
  })

  it('abi',async()=>{
    for(var n in MVM_Coinbase.functions){
      console.log(n,utils.hexDataSlice(utils.keccak256(utils.toUtf8Bytes(n)),0,4))
    }
  })
  it('deposit', async () => {
    // const preBalances = await getBalances()

    // const gasPrice = BigNumber.from(15000000)
    // const gasLimit = BigNumber.from(167060000)
    // //await l2Wallet.sendTransaction({to:PROXY_SEQUENCER_ENTRYPOINT_ADDRESS,value:1000000000000000,gasPrice,gasLimit})
    // // transfer with 0 value to easily pay a gas fee
    // const res: TransactionResponse = await MVM_Coinbase.transfer(
    //   PROXY_SEQUENCER_ENTRYPOINT_ADDRESS,
    //   100,
    //   {
    //     gasPrice,
    //     gasLimit
    //   }
    // )
    //await res.wait()
    // const postBalances = await getBalances()
    // console.log("l1 wallet balance:" + postBalances.l1UserBalance + ",l2 wallet balance" + postBalances.l2UserBalance + ",l1gateway balance" + postBalances.l1GatewayBalance + ",seq balance" + postBalances.sequencerBalance)
    const tx2 = await OVM_L1ETHGateway.depositETHByChainId(
      1088, 
      FINALIZATION_GAS,
      '0xFFFF',
      {
      value: depositAmount
      }
    )
    const res = await tx2.wait()
    //console.log("test",res)
    const taxBalance = await MVM_Coinbase.balanceOf(l2Wallet.address)
    const taxBalancePeer = await OVM_ETH.balanceOf(l2PeerWallet.address)
    
    console.log("test")
    console.log("tax balance:"+taxBalance+","+taxBalancePeer)
    // res = await MVM_Coinbase.withdraw(
    //   1000,
    //   {
    //     gasPrice,
    //     gasLimit
    //   }
    // )
    // await res.wait()
    // var postBalances = await getBalances()
    // console.log("l1 wallet balance:"+postBalances.l1UserBalance+",l2 wallet balance"+postBalances.l2UserBalance+",l1gateway balance"+postBalances.l1GatewayBalance+",seq balance"+postBalances.sequencerBalance)

    // // make sure stored and served correctly by geth
    // expect(res.gasPrice.eq(gasPrice)).to.be.true
    // expect(res.gasLimit.eq(gasLimit)).to.be.true

    // postBalances = await getBalances()
    // const feePaid = preBalances.l2UserBalance.sub(
    //   postBalances.l2UserBalance
    // )

    // expect(
    //   feePaid.
    //     eq(
    //       gasLimit.mul(gasPrice)
    //     )
    // ).to.be.true
  })
  it.skip('withdraw', async () => {
    await OVM_L2CrossDomainMessenger.withdraw(
      predeploys.MVM_Coinbase,
      100, 
      0,
      '0xFFFF',
      { gasPrice: 0,gasLimit:1000000 }
    )
    const taxBalance = await MVM_Coinbase.balanceOf(l2Wallet.address)
    const taxBalancePeer = await MVM_CoinbasePeer.balanceOf(l2PeerWallet.address)
    
    console.log("test")
    console.log("tax balance:"+taxBalance+","+taxBalancePeer)
  })
  
  it.skip('sequencer rejects transaction with a non-multiple-of-1M gasPrice', async () => {
    const gasPrice = BigNumber.from(0)
    const gasLimit = BigNumber.from('0x100000')

    let err: string
    try {
      const res = await MVM_Coinbase.transfer(
        '0x1234123412341234123412341234123412341234',
        0,
        {
          gasPrice,
          gasLimit
        }
      )
      await res.wait()
    } catch (e) {
      err = e.body
    }

    if (err === undefined) {
      throw new Error('Transaction did not throw as expected')
    }

    expect(
      err.includes('Gas price must be a multiple of 1,000,000 wei')
    ).to.be.true
  })
})
