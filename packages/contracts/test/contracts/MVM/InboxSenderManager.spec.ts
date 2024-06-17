import { expect } from '../../setup'

/* External Imports */
import { ethers } from 'hardhat'
import { Signer, ContractFactory, Contract } from 'ethers'

/* Internal Imports */
import { makeAddressManager } from '../../helpers'

describe('InboxSenderManager', () => {
  let deployer: Signer
  let manager: Signer
  before(async () => {
    ;[deployer, manager] = await ethers.getSigners()
  })

  let AddressManager: Contract
  before(async () => {
    AddressManager = await makeAddressManager()
  })

  let Factory__InboxSenderManager: ContractFactory
  before(async () => {
    Factory__InboxSenderManager = await ethers.getContractFactory(
      'MVM_InboxSenderManager'
    )
  })

  let defaultInboxSender: string
  let InboxSenderManager: Contract
  beforeEach(async () => {
    defaultInboxSender = ethers.utils.getAddress(
      ethers.utils.hexlify(ethers.utils.randomBytes(20))
    )
    InboxSenderManager = await Factory__InboxSenderManager.deploy(
      AddressManager.address,
      defaultInboxSender
    )
    await AddressManager.setAddress('METIS_MANAGER', manager.getAddress())
  })

  describe('Default inbox sender', () => {
    it('should set the initial default inbox sender', async () => {
      expect(await InboxSenderManager.defaultInboxSender()).to.equal(
        defaultInboxSender
      )
    })
  })

  describe('Inbox sender', () => {
    it('should revert if set inbox sender called by non-mananger', async () => {
      const blockNumber = 1
      const inboxSender = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )
      await expect(
        InboxSenderManager.setInboxSender(blockNumber, inboxSender)
      ).to.be.revertedWith(
        'MVM_InboxSenderManager: Function can only be called by the METIS_MANAGER.'
      )
    })

    it('should set the inbox address at the first time', async () => {
      const blockNumber = 1
      const inboxSender = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )

      await InboxSenderManager.connect(manager).setInboxSender(
        blockNumber,
        inboxSender
      )

      const storedInboxSender = await InboxSenderManager.getInboxSender(
        blockNumber
      )
      expect(storedInboxSender).to.equal(inboxSender)
    })

    it('should set inbox address successfully the second time ', async () => {
      const blockNumber1 = 1
      const inboxSender1 = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )
      const blockNumber2 = 2
      const inboxSender2 = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )

      await InboxSenderManager.connect(manager).setInboxSender(
        blockNumber1,
        inboxSender1
      )
      expect(await InboxSenderManager.getInboxSender(blockNumber1)).to.equal(
        inboxSender1
      )

      await InboxSenderManager.connect(manager).setInboxSender(
        blockNumber2,
        inboxSender2
      )

      expect(await InboxSenderManager.getInboxSender(blockNumber2)).to.equal(
        inboxSender2
      )
    })

    it('should emit InboxSenderSet event when setting the inbox sender', async () => {
      const blockNumber = 1
      const inboxSender = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )

      const tx = await InboxSenderManager.connect(manager).setInboxSender(
        blockNumber,
        inboxSender
      )
      const receipt = await tx.wait()

      expect(receipt.events.length).to.equal(1)
      expect(receipt.events[0].event).to.equal('InboxSenderSet')
      expect(receipt.events[0].args.blockNumber).to.equal(blockNumber)
      expect(receipt.events[0].args.inboxSender).to.equal(inboxSender)
    })

    it('should return the default inbox sender if no address is set for a block', async () => {
      const blockNumber = 1
      const defaultInboxSender = await InboxSenderManager.defaultInboxSender()

      const storedInboxSender = await InboxSenderManager.getInboxSender(
        blockNumber
      )
      expect(storedInboxSender).to.equal(defaultInboxSender)
    })

    it('should return the inbox sender for the block if it is between two set blocks', async () => {
      const blockNumber1 = 1
      const inboxSender1 = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )
      const blockNumber2 = 3
      const inboxSender2 = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )
      const queryBlockNumber = 2

      await InboxSenderManager.connect(manager).setInboxSender(
        blockNumber1,
        inboxSender1
      )
      await InboxSenderManager.connect(manager).setInboxSender(
        blockNumber2,
        inboxSender2
      )

      const storedInboxSender = await InboxSenderManager.getInboxSender(
        queryBlockNumber
      )
      expect(storedInboxSender).to.equal(inboxSender1)
    })

    it('should return the latest inbox sender if the block number is greater than the highest set block', async () => {
      const blockNumber1 = 1
      const inboxSender1 = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )
      const blockNumber2 = 2
      const inboxSender2 = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )
      const blockNumber3 = 3
      const inboxSender3 = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )
      const higherBlockNumber = 4

      await InboxSenderManager.connect(manager).setInboxSender(
        blockNumber1,
        inboxSender1
      )
      await InboxSenderManager.connect(manager).setInboxSender(
        blockNumber2,
        inboxSender2
      )
      await InboxSenderManager.connect(manager).setInboxSender(
        blockNumber3,
        inboxSender3
      )

      const storedInboxSender = await InboxSenderManager.getInboxSender(
        higherBlockNumber
      )
      expect(storedInboxSender).to.equal(inboxSender3)
    })
  })
})
