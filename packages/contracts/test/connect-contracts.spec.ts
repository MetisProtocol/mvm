import { ethers } from 'hardhat'
import { Signer, Contract } from 'ethers'
import {
  connectL1Contracts,
  connectL2Contracts,
} from '../dist/connect-contracts'
import { expect } from './setup'

<<<<<<< HEAD
describe('connectL1Contracts', () => {
=======
// Skipping these tests as the FE work that relies on this logic was never finished.
// Dedicated issue created in https://linear.app/optimism/issue/ENG-1451/decide-what-to-do-with-the-connectl1contracts-tests
describe.skip('connectL1Contracts', () => {
>>>>>>> 2c741af18943321173153180956f4bf84445a337
  let user: Signer
  const l1ContractNames = [
    'addressManager',
    'canonicalTransactionChain',
<<<<<<< HEAD
    'executionManager',
    'fraudVerifier',
    'multiMessageRelayer',
=======
>>>>>>> 2c741af18943321173153180956f4bf84445a337
    'stateCommitmentChain',
    'xDomainMessengerProxy',
    'bondManager',
  ]

  const l2ContractNames = [
    'eth',
    'xDomainMessenger',
    'messagePasser',
    'messageSender',
    'deployerWhiteList',
<<<<<<< HEAD
    'ecdsaContractAccount',
    'sequencerEntrypoint',
    'erc1820Registry',
    'addressManager',
=======
>>>>>>> 2c741af18943321173153180956f4bf84445a337
  ]

  before(async () => {
    ;[user] = await ethers.getSigners()
  })

  it(`connectL1Contracts should throw error if signer or provider isn't provided.`, async () => {
    try {
      await connectL1Contracts(undefined, 'mainnet')
    } catch (err) {
      expect(err.message).to.be.equal('signerOrProvider argument is undefined')
    }
  })

  for (const name of l1ContractNames) {
    it(`connectL1Contracts should return a contract assigned to a field named "${name}"`, async () => {
      const l1Contracts = await connectL1Contracts(user, 'mainnet')
      expect(l1Contracts[name]).to.be.an.instanceOf(Contract)
    })
  }

  for (const name of l2ContractNames) {
    it(`connectL2Contracts should return a contract assigned to a field named "${name}"`, async () => {
      const l2Contracts = await connectL2Contracts(user)
      expect(l2Contracts[name]).to.be.an.instanceOf(Contract)
    })
  }
})
