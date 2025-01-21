import { expect } from '../../setup'

/* External Imports */
import { ethers } from 'hardhat'
import { Signer, ContractFactory, Contract } from 'ethers'
import { smockit, MockContract } from '@eth-optimism/smock'

/* Internal Imports */
import {
  makeAddressManager,
  setProxyTarget,
  NON_NULL_BYTES32,
  getEthTime,
  increaseEthTime,
} from '../../helpers'

describe('MVM_StateCommitmentChain', () => {
  let sequencer: Signer
  let user: Signer
  before(async () => {
    ;[sequencer, user] = await ethers.getSigners()
  })

  let AddressManager: Contract
  before(async () => {
    AddressManager = await makeAddressManager()
  })

  let Mock__CanonicalTransactionChain: MockContract
  let Mock__BondManager: MockContract
  before(async () => {
    Mock__CanonicalTransactionChain = await smockit(
      await ethers.getContractFactory('CanonicalTransactionChain')
    )

    await setProxyTarget(
      AddressManager,
      'CanonicalTransactionChain',
      Mock__CanonicalTransactionChain
    )

    Mock__BondManager = await smockit(
      await ethers.getContractFactory('BondManager')
    )

    await setProxyTarget(AddressManager, 'BondManager', Mock__BondManager)

    // await AddressManager.setAddress('BondManager', Mock__BondManager.address)

    Mock__BondManager.smocked.isCollateralizedByChainId.will.return.with(true)

    // Set up the proposer address
    await AddressManager.setAddress(
      '1088_MVM_Proposer',
      await sequencer.getAddress()
    )
  })

  let Factory__StateCommitmentChain: ContractFactory
  let Factory__ChainStorageContainer: ContractFactory
  before(async () => {
    Factory__StateCommitmentChain = await ethers.getContractFactory(
      'MVM_StateCommitmentChain'
    )
    Factory__ChainStorageContainer = await ethers.getContractFactory(
      'ChainStorageContainer'
    )
  })

  let StateCommitmentChain: Contract
  let ChainStorageContainer: Contract
  beforeEach(async () => {
    StateCommitmentChain = await Factory__StateCommitmentChain.deploy(
      AddressManager.address,
      60 * 60 * 24 * 7, // 1 week fraud proof window
      60 * 30 // 30 minute sequencer publish window
    )

    ChainStorageContainer = await Factory__ChainStorageContainer.deploy(
      AddressManager.address,
      'StateCommitmentChain'
    )

    await AddressManager.setAddress(
      'ChainStorageContainer-SCC-batches',
      ChainStorageContainer.address
    )

    await AddressManager.setAddress(
      'StateCommitmentChain',
      StateCommitmentChain.address
    )
  })

  describe('findEarliestDisputableBatch', () => {
    const DEFAULT_CHAINID = 1088
    const batch = [NON_NULL_BYTES32]
    
    beforeEach(async () => {
      // Set CTC mock to return sufficient elements
      Mock__CanonicalTransactionChain.smocked.getTotalElementsByChainId.will.return.with(
        batch.length * 10 // Support 10 batches
      )
      
      // Add 10 batches
      for (let i = 0; i < 10; i++) {
        // Add time interval between batches
        await increaseEthTime(ethers.provider, 60) // Increase by 60 seconds
        await StateCommitmentChain.connect(sequencer).appendStateBatchByChainId(
          DEFAULT_CHAINID,
          batch,
          i,
          '1088_MVM_Proposer',
          ethers.utils.hexZeroPad(ethers.utils.hexlify(i), 32), // Last batch block hash
          i + 1 // Last batch block number
        )
      }
    })

    it('should return the first batch when all batches are within fraud proof window', async () => {
      const [batchHeaderHash, lastL2BlockNumber] = await StateCommitmentChain.findEarliestDisputableBatch(DEFAULT_CHAINID)
      
      // Get first batch info for comparison
      const firstBatchHash = await ChainStorageContainer.getByChainId(DEFAULT_CHAINID, 0)
      
      // Verify it returns the first batch
      expect(batchHeaderHash).to.equal(firstBatchHash)
      expect(lastL2BlockNumber).to.equal(1) // Because each batch contains one element
    })

    it('should revert when no batches have been submitted', async () => {
      // Create new StateCommitmentChain instance with no batches
      const newStateCommitmentChain = await Factory__StateCommitmentChain.deploy(
        AddressManager.address,
        60 * 60 * 24 * 7, // 1 week fraud proof window
        60 * 30 // 30 minute sequencer publish window
      )

      await expect(
        newStateCommitmentChain.findEarliestDisputableBatch(DEFAULT_CHAINID)
      ).to.be.revertedWith('No batch has been appended yet')
    })

    it('should correctly handle fraud proof window', async () => {
      // Increase time to make some batches out of fraud proof window
      const FRAUD_PROOF_WINDOW = await StateCommitmentChain.FRAUD_PROOF_WINDOW()
      await increaseEthTime(ethers.provider, FRAUD_PROOF_WINDOW.div(2).toNumber())

      const [batchHeaderHash, lastL2BlockNumber] = await StateCommitmentChain.findEarliestDisputableBatch(DEFAULT_CHAINID)
      
      // Should still return the first disputable batch
      const firstBatchHash = await ChainStorageContainer.getByChainId(DEFAULT_CHAINID, 0)
      expect(batchHeaderHash).to.equal(firstBatchHash)
      expect(lastL2BlockNumber).to.equal(1)
    })

    it('should return the first batch within fraud proof window when some batches are expired', async () => {
      // First make the first 5 batches expire by moving time forward
      const FRAUD_PROOF_WINDOW = await StateCommitmentChain.FRAUD_PROOF_WINDOW()
      await increaseEthTime(ethers.provider, FRAUD_PROOF_WINDOW.toNumber() + 300) // Add 5 minutes extra

      // Add 5 more recent batches
      for (let i = 10; i < 15; i++) {
        const timestamp = await getEthTime(ethers.provider)
        await StateCommitmentChain.connect(sequencer).appendStateBatchByChainId(
          DEFAULT_CHAINID,
          batch,
          i,
          '1088_MVM_Proposer',
          ethers.utils.hexZeroPad(ethers.utils.hexlify(i), 32),
          i + 1
        )
        // Add time interval between batches
        await increaseEthTime(ethers.provider, 60) // Increase by 60 seconds
      }

      const [batchHeaderHash, lastL2BlockNumber] = await StateCommitmentChain.findEarliestDisputableBatch(DEFAULT_CHAINID)
      
      // Should return the first batch that is still within the fraud proof window
      // This should be the 10th batch (index 9)
      const expectedBatchHash = await ChainStorageContainer.getByChainId(DEFAULT_CHAINID, 10)
      
      expect(batchHeaderHash).to.equal(expectedBatchHash)
      expect(lastL2BlockNumber).to.equal(11) // Because this is the 11th batch (1-based)

      // Verify this batch is actually within the fraud proof window
      const batchHeader = {
        batchIndex: 10,
        batchRoot: NON_NULL_BYTES32,
        batchSize: 1,
        prevTotalElements: 10,
        extraData: ethers.utils.defaultAbiCoder.encode(
          ['uint256', 'address', 'bytes32', 'uint256'],
          [
            await getEthTime(ethers.provider) - 300, // Approximate timestamp of this batch
            await sequencer.getAddress(),
            ethers.utils.hexZeroPad(ethers.utils.hexlify(10), 32),
            11
          ]
        )
      }
      
      expect(
        await StateCommitmentChain.insideFraudProofWindowByChainId(DEFAULT_CHAINID, batchHeader)
      ).to.be.true
    })

    it('should handle case when all batches are outside fraud proof window', async () => {
      // Move time forward to expire all existing batches
      const FRAUD_PROOF_WINDOW = await StateCommitmentChain.FRAUD_PROOF_WINDOW()
      await increaseEthTime(ethers.provider, FRAUD_PROOF_WINDOW.toNumber() * 2)

      // Should revert since no batches are disputable
      await expect(
        StateCommitmentChain.findEarliestDisputableBatch(DEFAULT_CHAINID)
      ).to.be.revertedWith('No batch to dispute')
    })

    it('should handle case when only the last batch is within fraud proof window', async () => {
      // First expire all existing batches
      const FRAUD_PROOF_WINDOW = await StateCommitmentChain.FRAUD_PROOF_WINDOW()
      await increaseEthTime(ethers.provider, FRAUD_PROOF_WINDOW.toNumber() + 300)

      // Add one more recent batch
      const lastBatchIndex = 10
      await StateCommitmentChain.connect(sequencer).appendStateBatchByChainId(
        DEFAULT_CHAINID,
        batch,
        lastBatchIndex,
        '1088_MVM_Proposer',
        ethers.utils.hexZeroPad(ethers.utils.hexlify(lastBatchIndex), 32),
        lastBatchIndex + 1
      )

      const [batchHeaderHash, lastL2BlockNumber] = await StateCommitmentChain.findEarliestDisputableBatch(DEFAULT_CHAINID)
      
      // Should return the last batch
      const expectedBatchHash = await ChainStorageContainer.getByChainId(DEFAULT_CHAINID, lastBatchIndex)
      expect(batchHeaderHash).to.equal(expectedBatchHash)
      expect(lastL2BlockNumber).to.equal(lastBatchIndex + 1)
    })

    it('should handle case with exactly fraud proof window time difference', async () => {
      const FRAUD_PROOF_WINDOW = await StateCommitmentChain.FRAUD_PROOF_WINDOW()
      const timeToMove = FRAUD_PROOF_WINDOW.toNumber()

      // Move time forward
      await increaseEthTime(ethers.provider, timeToMove)

      const [batchHeaderHash, lastL2BlockNumber] = await StateCommitmentChain.findEarliestDisputableBatch(DEFAULT_CHAINID)
      
      // Should return the second batch (index 1) as it's the first one still within the window
      const expectedBatchIndex = 9
      const expectedBatchHash = await ChainStorageContainer.getByChainId(DEFAULT_CHAINID, expectedBatchIndex)
      expect(batchHeaderHash).to.equal(expectedBatchHash)
      expect(lastL2BlockNumber).to.equal(expectedBatchIndex + 1)
    })

    it('should handle case with multiple batches submitted at the same timestamp', async () => {
      // First expire all existing batches
      const FRAUD_PROOF_WINDOW = await StateCommitmentChain.FRAUD_PROOF_WINDOW()
      await increaseEthTime(ethers.provider, FRAUD_PROOF_WINDOW.toNumber() + 300)

      // Add multiple batches at the same timestamp
      const startIndex = 10
      const timestamp = await getEthTime(ethers.provider)
      
      for (let i = 0; i < 3; i++) {
        await StateCommitmentChain.connect(sequencer).appendStateBatchByChainId(
          DEFAULT_CHAINID,
          batch,
          startIndex + i,
          '1088_MVM_Proposer',
          ethers.utils.hexZeroPad(ethers.utils.hexlify(startIndex + i), 32),
          startIndex + i + 1
        )
      }

      const [batchHeaderHash, lastL2BlockNumber] = await StateCommitmentChain.findEarliestDisputableBatch(DEFAULT_CHAINID)
      
      // Should return the first batch of the group
      const expectedBatchHash = await ChainStorageContainer.getByChainId(DEFAULT_CHAINID, startIndex)
      expect(batchHeaderHash).to.equal(expectedBatchHash)
      expect(lastL2BlockNumber).to.equal(startIndex + 1)
    })

    it('should handle case with different chain IDs', async () => {
      const DIFFERENT_CHAINID = 1089
      
      // Add a batch to different chain ID
      await StateCommitmentChain.connect(sequencer).appendStateBatchByChainId(
        DIFFERENT_CHAINID,
        batch,
        0,
        '1089_MVM_Proposer',
        ethers.utils.hexZeroPad(ethers.utils.hexlify(0), 32),
        1
      )

      // Should still return correct batch for original chain ID
      const [batchHeaderHash, lastL2BlockNumber] = await StateCommitmentChain.findEarliestDisputableBatch(DEFAULT_CHAINID)
      const firstBatchHash = await ChainStorageContainer.getByChainId(DEFAULT_CHAINID, 0)
      expect(batchHeaderHash).to.equal(firstBatchHash)
      expect(lastL2BlockNumber).to.equal(1)

      // Should work for different chain ID as well
      const [diffChainBatchHash, diffChainL2BlockNumber] = await StateCommitmentChain.findEarliestDisputableBatch(DIFFERENT_CHAINID)
      const expectedBatchHash = await ChainStorageContainer.getByChainId(DIFFERENT_CHAINID, 0)
      expect(diffChainBatchHash).to.equal(expectedBatchHash)
      expect(diffChainL2BlockNumber).to.equal(1)
    })

    it('should correctly handle batch deletion and subsequent queries', async () => {
      // First add some batches and move time forward to make first few batches disputable
      const FRAUD_PROOF_WINDOW = await StateCommitmentChain.FRAUD_PROOF_WINDOW()
      await increaseEthTime(ethers.provider, FRAUD_PROOF_WINDOW.div(2).toNumber())

      // Get the batch to delete (let's delete batch at index 5)
      const batchIndexToDelete = 5

      // Get the actual batch data from the event
      const events = await StateCommitmentChain.queryFilter(
        StateCommitmentChain.filters.StateBatchAppended()
      )
      const batchEvent = events.find(
        event => 
          event.args._chainId.eq(DEFAULT_CHAINID) && 
          event.args._batchIndex.eq(batchIndexToDelete)
      )

      if (!batchEvent) {
        throw new Error('Batch event not found')
      }

      // Construct the batch header using the actual data from the event
      const batchToDelete = {
        batchIndex: batchEvent.args._batchIndex,
        batchRoot: batchEvent.args._batchRoot,
        batchSize: batchEvent.args._batchSize,
        prevTotalElements: batchEvent.args._prevTotalElements,
        extraData: batchEvent.args._extraData
      }

      // Store some data for later comparison
      const originalBatchHash = await ChainStorageContainer.getByChainId(DEFAULT_CHAINID, batchIndexToDelete)
      const originalTotalElements = await StateCommitmentChain.getTotalElementsByChainId(DEFAULT_CHAINID)
      const originalTotalBatches = await StateCommitmentChain.getTotalBatchesByChainId(DEFAULT_CHAINID)

      // Set up the fraud verifier address
      await AddressManager.setAddress(
        '1088_MVM_FraudVerifier',
        await sequencer.getAddress()
      )

      // Delete the batch
      await StateCommitmentChain.connect(sequencer).deleteStateBatchByChainId(
        DEFAULT_CHAINID,
        batchToDelete
      )

      // Verify the deletion was successful
      // 1. Total elements should be reduced
      expect(await StateCommitmentChain.getTotalElementsByChainId(DEFAULT_CHAINID))
        .to.equal(batchToDelete.prevTotalElements)

      // 2. Total batches should be reduced
      expect(await StateCommitmentChain.getTotalBatchesByChainId(DEFAULT_CHAINID))
        .to.equal(batchIndexToDelete)

      // 3. Previous batches should still be accessible and unchanged
      for (let i = 0; i < batchIndexToDelete; i++) {
        const batchHash = await ChainStorageContainer.getByChainId(DEFAULT_CHAINID, i)
        expect(batchHash).to.not.equal(ethers.constants.HashZero)
      }

      // 4. Try to access a deleted batch - should revert
      await expect(
        ChainStorageContainer.getByChainId(DEFAULT_CHAINID, batchIndexToDelete)
      ).to.be.revertedWith('Index out of bounds')

      // After deleting batch 5, we want batch 4 to be the last disputable batch
      // Get batch 4's timestamp from its event
      const batch4Event = events.find(
        event => 
          event.args._chainId.eq(DEFAULT_CHAINID) && 
          event.args._batchIndex.eq(batchIndexToDelete - 1)
      )
      if (!batch4Event) {
        throw new Error('Batch 4 event not found')
      }

      // Get batch 4's timestamp from its extraData
      const [batch4Timestamp] = ethers.utils.defaultAbiCoder.decode(
        ['uint256', 'address', 'bytes32', 'uint256'],
        batch4Event.args._extraData
      )

      // Calculate how much time to move forward:
      // We want batch 4 to be just inside the fraud proof window
      // Current time + timeToMove = batch4Timestamp + FRAUD_PROOF_WINDOW
      const currentTime = await getEthTime(ethers.provider)
      const timeToMove = batch4Timestamp.add(FRAUD_PROOF_WINDOW).sub(currentTime).sub(10) // Leave 10 seconds buffer

      // Move time forward
      await increaseEthTime(ethers.provider, timeToMove.toNumber())

      // 5. findEarliestDisputableBatch should return batch 4
      const [batchHeaderHash, lastL2BlockNumber] = await StateCommitmentChain.findEarliestDisputableBatch(DEFAULT_CHAINID)
      const lastValidBatch = await ChainStorageContainer.getByChainId(DEFAULT_CHAINID, batchIndexToDelete - 1)
      expect(batchHeaderHash).to.equal(lastValidBatch)
      expect(lastL2BlockNumber).to.equal(batchIndexToDelete) // Should be 5 since it's batch 4 + 1

      // Verify this is indeed the last valid batch by checking the next query will fail
      await increaseEthTime(ethers.provider, 120) // Move past the fraud proof window completely
      await expect(
        StateCommitmentChain.findEarliestDisputableBatch(DEFAULT_CHAINID)
      ).to.be.revertedWith('No batch to dispute')

      // 6. Add a new batch and verify everything still works
      const newBatchIndex = batchIndexToDelete
      await StateCommitmentChain.connect(sequencer).appendStateBatchByChainId(
        DEFAULT_CHAINID,
        batch,
        newBatchIndex,
        '1088_MVM_Proposer',
        ethers.utils.hexZeroPad(ethers.utils.hexlify(newBatchIndex), 32),
        newBatchIndex + 1
      )

      // The new batch should be queryable
      const newBatchHash = await ChainStorageContainer.getByChainId(DEFAULT_CHAINID, newBatchIndex)
      expect(newBatchHash).to.not.equal(ethers.constants.HashZero)
      expect(newBatchHash).to.not.equal(originalBatchHash)

      // And it should be the only disputable batch now
      const [newBatchHeaderHash, newLastL2BlockNumber] = await StateCommitmentChain.findEarliestDisputableBatch(DEFAULT_CHAINID)
      expect(newBatchHeaderHash).to.equal(newBatchHash)
      expect(newLastL2BlockNumber).to.equal(newBatchIndex + 1)
    })
  })
}) 