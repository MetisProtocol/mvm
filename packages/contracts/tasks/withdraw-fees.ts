'use strict'

import { ethers } from 'ethers'
import { task } from 'hardhat/config'
import * as types from 'hardhat/internal/core/params/argumentTypes'
import { getContractFactory } from '../src/contract-defs'
import { predeploys } from '../src/predeploys'

// Withdraw fees from the FeeVault to L1
// npx hardhat withdraw-fees --dry-run
task('withdraw-fees')
  .addOptionalParam('dryRun', 'simulate withdrawing fees', false, types.boolean)
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
    'privateKey',
    'Private Key',
    process.env.CONTRACTS_DEPLOYER_KEY,
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
    if (args.dryRun) {
      console.log('Performing dry run of fee withdrawal...')
    }

    const l2FeeVault = getContractFactory('OVM_SequencerFeeVault')
      .connect(signer)
      .attach(predeploys.OVM_SequencerFeeVault)

    const signerAddress = await signer.getAddress()
    const signerBalance = await provider.getBalance(signerAddress)
    const signerBalanceInETH = ethers.formatEther(signerBalance)
    console.log(
      `Using L2 signer ${signerAddress} with a balance of ${signerBalanceInETH} ETH`
    )
    const l1FeeWallet = await l2FeeVault.getFunction('l1FeeWallet').staticCall()
    const amount = await provider.getBalance(await l2FeeVault.getAddress())
    const amountInETH = ethers.formatEther(amount)
    console.log(
      `${
        args.dryRun ? '[DRY RUN] ' : ''
      }Withdrawing ${amountInETH} ETH to the L1 address: ${l1FeeWallet}`
    )
    if (args.dryRun) {
      await l2FeeVault.getFunction('withdraw').estimateGas()
      return
    } else {
      const withdrawTx = await l2FeeVault.getFunction('withdraw').send()
      console.log(
        `Withdrawal complete: https://optimistic.etherscan.io/tx/${withdrawTx.hash}`
      )
      console.log(
        `Complete withdrawal in 1 week here: https://optimistic.etherscan.io/address/${predeploys.OVM_SequencerFeeVault}#withdrawaltxs`
      )
    }
  })
