/* Imports: External */
import { ethers } from 'ethers'
import { task } from 'hardhat/config'
import * as types from 'hardhat/internal/core/params/argumentTypes'

import { predeploys } from '../src/predeploys'
import { getContractFactory } from '../src/contract-defs'

task('set-owner')
  .addParam('owner', 'the new oracle address', 0, types.string)
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
    predeploys.OVM_GasPriceOracle,
    types.string
  )
  .setAction(async (args, hre: any, runSuper) => {
    const provider = new ethers.JsonRpcProvider(args.contractsRpcUrl)
    let signer: ethers.Signer
    if (!args.useLedger) {
      signer = new ethers.Wallet(args.contractsDeployerKey).connect(provider)
    } else {
      // TODO: not sure how to support hardware wallet in ethers v6 for now,
      //       need to figure it out later
      // signer = new LedgerSigner(provider, 'default', args.ledgerPath)
      throw new Error('Ledger not supported in ethers v6')
    }

    const Ownable = getContractFactory('Ownable')
      .attach(args.contractAddress)
      .connect(provider)

    const addr = await signer.getAddress()
    console.log(`Using signer ${addr}`)
    const owner = await Ownable.getFunction('owner').staticCall()
    if (owner !== addr) {
      throw new Error(`Incorrect key. Owner ${owner}, Signer ${addr}`)
    }

    console.log(`Owner is currently ${owner.toString()}`)
    console.log(`Setting owner to ${args.owner}`)

    const tx = await Ownable.connect(signer)
      .getFunction('transferOwnership')
      .send(args.owner, {
        gasPrice: args.transactionGasPrice,
      })

    const receipt = await tx.wait()
    console.log(`Success - ${tx.hash}`)
  })
