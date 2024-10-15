'use strict'

import { ethers } from 'ethers'
import { task } from 'hardhat/config'
import * as types from 'hardhat/internal/core/params/argumentTypes'
import { getContractFactory } from '../src/contract-defs'
import { predeploys } from '../src/predeploys'

// Add accounts the the OVM_DeployerWhitelist
// npx hardhat whitelist --address 0x..
task('whitelist')
  .addParam('address', 'Address to whitelist', undefined, types.string)
  .addOptionalParam('transactionGasPrice', 'tx.gasPrice', undefined, types.int)
  .addOptionalParam(
    'useLedger',
    'use a ledger for signing',
    false,
    types.boolean
  )
  .addOptionalParam(
    'ledgerPath',
    'ledger key derivation path',
    ethers.defaultPath,
    types.string
  )
  .addOptionalParam(
    'contractsRpcUrl',
    'Sequencer HTTP Endpoint',
    process.env.CONTRACTS_RPC_URL,
    types.string
  )
  .addOptionalParam(
    'contractsDeployerKey',
    'Private Key',
    process.env.CONTRACTS_DEPLOYER_KEY,
    types.string
  )
  .addOptionalParam(
    'contractAddress',
    'Address of Ownable contract',
    predeploys.OVM_DeployerWhitelist,
    types.string
  )
  .setAction(async (args, hre: any) => {
    const provider = new ethers.JsonRpcProvider(args.contractsRpcUrl)
    let signer: ethers.Signer
    if (!args.useLedger) {
      if (!args.contractsDeployerKey) {
        throw new Error('Must pass --contracts-deployer-key')
      }
      signer = new ethers.Wallet(args.contractsDeployerKey).connect(provider)
    } else {
      // FIXME: not sure how to support hardware wallet in ethers v6 for now, need to figure it out later
      // signer = new LedgerSigner(provider, 'default', args.ledgerPath)
      throw new Error('Ledger not supported in ethers v6')
    }

    const deployerWhitelist = getContractFactory('OVM_DeployerWhitelist')
      .connect(signer)
      .attach(args.contractAddress)

    const addr = await signer.getAddress()
    console.log(`Using signer: ${addr}`)
    const owner = await deployerWhitelist.getFunction('owner').staticCall()
    if (owner === '0x0000000000000000000000000000000000000000') {
      console.log(`Whitelist is disabled. Exiting early.`)
      return
    } else {
      console.log(`OVM_DeployerWhitelist owner: ${owner}`)
    }

    if (addr !== owner) {
      throw new Error(`Incorrect key. Owner ${owner}, Signer ${addr}`)
    }

    const res = await deployerWhitelist
      .getFunction('setWhitelistedDeployer')
      .send(args.address, true, { gasPrice: args.transactionGasPrice })
    await res.wait()
    console.log(`Whitelisted ${args.address}`)
  })
