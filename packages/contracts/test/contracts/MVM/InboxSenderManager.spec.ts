import { expect } from '../../setup'

/* External Imports */
import { ethers } from 'hardhat'
import { Contract, ContractFactory, Signer } from 'ethers'

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
  let defaultInboxBlobSender: string
  let InboxSenderManager: Contract
  beforeEach(async () => {
    defaultInboxSender = ethers.utils.getAddress(
      ethers.utils.hexlify(ethers.utils.randomBytes(20))
    )

    defaultInboxBlobSender = ethers.utils.getAddress(
      ethers.utils.hexlify(ethers.utils.randomBytes(20))
    )

    InboxSenderManager = await Factory__InboxSenderManager.deploy(
      AddressManager.address,
      [
        {
          senderType: 0,
          sender: defaultInboxSender,
        },
        {
          senderType: 1,
          sender: defaultInboxBlobSender,
        },
      ]
    )
    await AddressManager.setAddress('METIS_MANAGER', manager.getAddress())
  })

  describe('Default inbox sender', () => {
    it('should set the initial default inbox sender', async () => {
      expect(await InboxSenderManager.defaultInboxSender(0)).to.equal(
        defaultInboxSender
      )
      expect(await InboxSenderManager.defaultInboxSender(1)).to.equal(
        defaultInboxBlobSender
      )
    })
  })

  describe('Inbox sender', () => {
    it('should revert if set inbox sender called by non-mananger', async () => {
      const blockNumber = 1
      const inboxSender = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )
      const inboxBlobSender = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )
      await expect(
        InboxSenderManager.setInboxSenders(blockNumber, [
          {
            senderType: 0,
            sender: inboxSender,
          },
          {
            senderType: 1,
            sender: inboxBlobSender,
          },
        ])
      ).to.be.revertedWith(
        'MVM_InboxSenderManager: Function can only be called by the METIS_MANAGER.'
      )
    })

    it('should set the inbox address at the first time', async () => {
      const blockNumber = 1
      const inboxSender = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )
      const inboxBlobSender = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )

      await InboxSenderManager.connect(manager).setInboxSenders(blockNumber, [
        {
          senderType: 0,
          sender: inboxSender,
        },
        {
          senderType: 1,
          sender: inboxBlobSender,
        },
      ])

      const storedInboxSender = await InboxSenderManager.getInboxSender(
        blockNumber,
        0
      )
      expect(storedInboxSender).to.equal(inboxSender)

      const storedInboxBlobSender = await InboxSenderManager.getInboxSender(
        blockNumber,
        1
      )
      expect(storedInboxBlobSender).to.equal(inboxBlobSender)
    })

    it('should set inbox address successfully the second time ', async () => {
      const blockNumber1 = 1
      const inboxSender1 = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )
      const inboxBlobSender1 = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )
      const blockNumber2 = 2
      const inboxSender2 = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )
      const inboxBlobSender2 = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )

      await InboxSenderManager.connect(manager).setInboxSenders(blockNumber1, [
        {
          senderType: 0,
          sender: inboxSender1,
        },
        {
          senderType: 1,
          sender: inboxBlobSender1,
        },
      ])
      expect(await InboxSenderManager.getInboxSender(blockNumber1, 0)).to.equal(
        inboxSender1
      )
      expect(await InboxSenderManager.getInboxSender(blockNumber1, 1)).to.equal(
        inboxBlobSender1
      )

      await InboxSenderManager.connect(manager).setInboxSenders(blockNumber2, [
        {
          senderType: 0,
          sender: inboxSender2,
        },
        {
          senderType: 1,
          sender: inboxBlobSender2,
        },
      ])

      expect(await InboxSenderManager.getInboxSender(blockNumber2, 0)).to.equal(
        inboxSender2
      )
      expect(await InboxSenderManager.getInboxSender(blockNumber2, 1)).to.equal(
        inboxBlobSender2
      )

      const blockNumber3 = 3
      const inboxSender3 = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )
      const inboxBlobSender3 = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )

      // set inbox sender separately
      await InboxSenderManager.connect(manager).setInboxSenders(blockNumber3, [
        {
          senderType: 0,
          sender: inboxSender3,
        },
      ])

      expect(await InboxSenderManager.getInboxSender(blockNumber3, 0)).to.equal(
        inboxSender3
      )
      expect(await InboxSenderManager.getInboxSender(blockNumber3, 1)).to.equal(
        inboxBlobSender2
      )

      const blockNumber4 = 4
      await InboxSenderManager.connect(manager).setInboxSenders(blockNumber4, [
        {
          senderType: 1,
          sender: inboxBlobSender3,
        },
      ])

      expect(await InboxSenderManager.getInboxSender(blockNumber4, 0)).to.equal(
        inboxSender3
      )
      expect(await InboxSenderManager.getInboxSender(blockNumber4, 1)).to.equal(
        inboxBlobSender3
      )
    })

    it('should emit InboxSenderSet event when setting the inbox sender', async () => {
      const blockNumber = 1
      const inboxSender = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )

      const inboxBlobSender = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )

      const tx = await InboxSenderManager.connect(manager).setInboxSenders(
        blockNumber,
        [
          {
            senderType: 0,
            sender: inboxSender,
          },
          {
            senderType: 1,
            sender: inboxBlobSender,
          },
        ]
      )
      const receipt = await tx.wait()

      expect(receipt.events.length).to.equal(2)
      expect(receipt.events[0].event).to.equal('InboxSenderSet')
      expect(receipt.events[0].args.blockNumber).to.equal(blockNumber)
      expect(receipt.events[0].args.inboxSender).to.equal(inboxSender)
      expect(receipt.events[1].event).to.equal('InboxSenderSet')
      expect(receipt.events[1].args.blockNumber).to.equal(blockNumber)
      expect(receipt.events[1].args.inboxSender).to.equal(inboxBlobSender)
    })

    it('should return the default inbox sender if no address is set for a block', async () => {
      expect(await InboxSenderManager.getInboxSender(1, 0)).to.equal(
        await InboxSenderManager.defaultInboxSender(0)
      )
      expect(await InboxSenderManager.getInboxSender(1, 1)).to.equal(
        await InboxSenderManager.defaultInboxSender(1)
      )
    })

    it('should return the inbox sender for the block if it is between two set blocks', async () => {
      const blockNumber1 = 1
      const inboxSender1 = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )
      const inboxBlobSender1 = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )
      const blockNumber2 = 3
      const inboxSender2 = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )
      const inboxBlobSender2 = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )
      const queryBlockNumber = 2

      await InboxSenderManager.connect(manager).setInboxSenders(blockNumber1, [
        {
          senderType: 0,
          sender: inboxSender1,
        },
        {
          senderType: 1,
          sender: inboxBlobSender1,
        },
      ])
      await InboxSenderManager.connect(manager).setInboxSenders(blockNumber2, [
        {
          senderType: 0,
          sender: inboxSender2,
        },
        {
          senderType: 1,
          sender: inboxBlobSender2,
        },
      ])

      const storedInboxSender = await InboxSenderManager.getInboxSender(
        queryBlockNumber,
        0
      )
      const storedInboxBlobSender = await InboxSenderManager.getInboxSender(
        queryBlockNumber,
        1
      )
      expect(storedInboxSender).to.equal(inboxSender1)
      expect(storedInboxBlobSender).to.equal(inboxBlobSender1)
    })

    it('should return the latest inbox sender if the block number is greater than the highest set block', async () => {
      const blockNumber1 = 1
      const inboxSender1 = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )
      const inboxBlobSender1 = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )
      const blockNumber2 = 2
      const inboxSender2 = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )
      const inboxBlobSender2 = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )
      const blockNumber3 = 3
      const inboxSender3 = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )
      const inboxBlobSender3 = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )
      const blockNumber4 = 4
      const inboxSender4 = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )
      const inboxBlobSender4 = ethers.utils.getAddress(
        ethers.utils.hexlify(ethers.utils.randomBytes(20))
      )
      const higherBlockNumber = 5
      console.log(
        'inboxSender1',
        inboxSender1,
        'inboxBlobSender1',
        inboxBlobSender1
      )
      console.log(
        'inboxSender2',
        inboxSender2,
        'inboxBlobSender2',
        inboxBlobSender2
      )
      console.log(
        'inboxSender3',
        inboxSender3,
        'inboxBlobSender3',
        inboxBlobSender3
      )
      console.log(
        'inboxSender4',
        inboxSender4,
        'inboxBlobSender4',
        inboxBlobSender4
      )

      await InboxSenderManager.connect(manager).setInboxSenders(blockNumber1, [
        {
          senderType: 0,
          sender: inboxSender1,
        },
        {
          senderType: 1,
          sender: inboxBlobSender1,
        },
      ])
      await InboxSenderManager.connect(manager).setInboxSenders(blockNumber2, [
        {
          senderType: 0,
          sender: inboxSender2,
        },
        {
          senderType: 1,
          sender: inboxBlobSender2,
        },
      ])
      await InboxSenderManager.connect(manager).setInboxSenders(blockNumber3, [
        {
          senderType: 0,
          sender: inboxSender3,
        },
        {
          senderType: 1,
          sender: inboxBlobSender3,
        },
      ])

      const storedInboxSender = await InboxSenderManager.getInboxSender(
        higherBlockNumber,
        0
      )
      const storedInboxBlobSender = await InboxSenderManager.getInboxSender(
        higherBlockNumber,
        1
      )
      expect(storedInboxSender).to.equal(inboxSender3)
      expect(storedInboxBlobSender).to.equal(inboxBlobSender3)

      // overwrite should work
      await InboxSenderManager.connect(manager).overwriteLastInboxSenders(
        blockNumber4,
        [
          {
            senderType: 0,
            sender: inboxSender4,
          },
          {
            senderType: 1,
            sender: inboxBlobSender4,
          },
        ]
      )

      const storedInboxSender4 = await InboxSenderManager.getInboxSender(
        higherBlockNumber,
        0
      )
      const storedInboxBlobSender4 = await InboxSenderManager.getInboxSender(
        higherBlockNumber,
        1
      )
      expect(storedInboxSender4).to.equal(inboxSender4)
      expect(storedInboxBlobSender4).to.equal(inboxBlobSender4)

      // 3 has been overwritten, should return 2
      const storedInboxSender2 = await InboxSenderManager.getInboxSender(
        blockNumber3,
        0
      )
      const storedInboxBlobSender2 = await InboxSenderManager.getInboxSender(
        blockNumber3,
        1
      )
      expect(storedInboxSender2).to.equal(inboxSender2)
      expect(storedInboxBlobSender2).to.equal(inboxBlobSender2)

      // overwrite should revert if the block number is less than the second last block, in current case, which is 2
      await expect(
        InboxSenderManager.connect(manager).overwriteLastInboxSenders(
          blockNumber2,
          [
            {
              senderType: 0,
              sender: inboxSender4,
            },
            {
              senderType: 1,
              sender: inboxBlobSender4,
            },
          ]
        )
      ).to.be.revertedWith(
        'MVM_InboxSenderManager: Block number must be greater than the previous block number.'
      )
    })
  })
})
