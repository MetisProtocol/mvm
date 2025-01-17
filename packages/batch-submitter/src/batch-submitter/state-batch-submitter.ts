/* External Imports */
import { Promise as bPromise } from 'bluebird'
import {
  Contract,
  ContractTransaction,
  ethers,
  Signer,
  toNumber,
  TransactionReceipt,
} from 'ethersv6'
import { getContractDefinition } from '@localtest911/contracts'
import { Bytes32, L2Block, remove0x, RollupInfo } from '@metis.io/core-utils'
import { Logger, Metrics } from '@eth-optimism/common-ts'

/* Internal Imports */
import { BatchSubmitter, BlockRange } from '.'
import { MpcClient, TransactionSubmitter } from '../utils'
import { InboxStorage } from '../storage'

export class StateBatchSubmitter extends BatchSubmitter {
  // TODO: Change this so that we calculate start = scc.totalElements() and end = ctc.totalElements()!
  // Not based on the length of the L2 chain -- that is only used in the batch submitter
  // Note this means we've got to change the state / end calc logic

  protected l2ChainId: number
  protected syncing: boolean
  protected ctcContract: Contract
  private fraudSubmissionAddress: string
  private transactionSubmitter: TransactionSubmitter
  private mpcUrl: string
  private inboxAddress: string
  private inboxStorage: InboxStorage
  private seqsetValidHeight: number
  private seqsetUpgradeOnly: boolean
  private fpValidHeight: number
  private fpChainContract: Contract

  constructor(
    signer: Signer,
    l1Provider: ethers.JsonRpcProvider,
    l2Provider: ethers.JsonRpcProvider,
    minTxSize: number,
    maxTxSize: number,
    maxBatchSize: number,
    maxBatchSubmissionTime: number,
    numConfirmations: number,
    resubmissionTimeout: number,
    finalityConfirmations: number,
    addressManagerAddress: string,
    minBalanceEther: number,
    transactionSubmitter: TransactionSubmitter,
    blockOffset: number,
    logger: Logger,
    metrics: Metrics,
    fraudSubmissionAddress: string,
    mpcUrl: string,
    batchInboxAddress: string,
    batchInboxStoragePath: string,
    seqsetValidHeight: number,
    seqsetUpgradeOnly: number,
    fpValidHeight: number
  ) {
    super(
      signer,
      null, // state batcher does not need blob signer
      l1Provider,
      l2Provider,
      minTxSize,
      maxTxSize,
      maxBatchSize,
      maxBatchSubmissionTime,
      numConfirmations,
      resubmissionTimeout,
      finalityConfirmations,
      addressManagerAddress,
      minBalanceEther,
      blockOffset,
      logger,
      metrics,
      mpcUrl.length > 0,
      fpValidHeight
    )
    this.fraudSubmissionAddress = fraudSubmissionAddress
    this.transactionSubmitter = transactionSubmitter
    this.mpcUrl = mpcUrl
    this.inboxAddress = batchInboxAddress
    this.inboxStorage = new InboxStorage(batchInboxStoragePath, logger)
    this.seqsetValidHeight = seqsetValidHeight
    this.seqsetUpgradeOnly = seqsetUpgradeOnly === 1
  }

  /*****************************
   * Batch Submitter Overrides *
   ****************************/

  public async _updateChainInfo(): Promise<void> {
    const info: RollupInfo = await this._getRollupInfo()
    if (info.mode === 'verifier') {
      this.logger.error(
        'Verifier mode enabled! Batch submitter only compatible with sequencer mode'
      )
      process.exit(1)
    }
    this.syncing = info.syncing
    const { sccAddress, ctcAddress, mvmSccAddress } =
      await this._getChainAddresses()

    if (
      typeof this.chainContract !== 'undefined' &&
      sccAddress === (await this.chainContract.getAddress()) &&
      ctcAddress === (await this.ctcContract.getAddress())
    ) {
      if (!this.fpUpgraded) {
        this.logger.debug('Chain contract already initialized', {
          sccAddress,
          ctcAddress,
        })
        return
      } else if (
        this.fpUpgraded &&
        this.fpChainContract &&
        mvmSccAddress === (await this.fpChainContract.getAddress())
      ) {
        this.logger.debug('Chain contract already initialized', {
          sccAddress,
          ctcAddress,
          mvmSccAddress,
        })
        return
      }
    }

    this.chainContract = new Contract(
      sccAddress,
      getContractDefinition('IStateCommitmentChain').abi,
      this.signer
    )

    this.ctcContract = new Contract(
      ctcAddress,
      getContractDefinition('CanonicalTransactionChain').abi,
      this.signer
    )

    if (this.fpUpgraded) {
      this.fpChainContract = new Contract(
        mvmSccAddress,
        getContractDefinition('IMVMStateCommitmentChain').abi,
        this.signer
      )
    }

    this.logger.info('Connected Optimism contracts', {
      stateCommitmentChain: await this.chainContract.getAddress(),
      canonicalTransactionChain: await this.ctcContract.getAddress(),
      mvmStateCommitmentChain: this.fpUpgraded
        ? await this.fpChainContract.getAddress()
        : ethers.ZeroAddress,
    })
    return
  }

  public async _onSync(): Promise<TransactionReceipt> {
    this.logger.info('Syncing mode enabled! Skipping state batch submission...')
    return
  }

  public async _getBatchStartAndEnd(): Promise<BlockRange> {
    this.logger.info('Getting batch start and end for state batch submitter...')
    const sccContract = this.fpUpgraded
      ? this.fpChainContract
      : this.chainContract

    const startBlock: number =
      toNumber(await sccContract.getTotalElementsByChainId(this.l2ChainId)) +
      this.blockOffset
    this.logger.info('Retrieved start block number from SCC', {
      startBlock,
    })

    // We will submit state roots for txs which have been in the tx chain for a while.
    let totalElements: number =
      toNumber(
        await this.ctcContract.getTotalElementsByChainId(this.l2ChainId)
      ) + this.blockOffset
    const useBatchInbox =
      this.inboxAddress &&
      this.inboxAddress.length === 42 &&
      this.inboxAddress.startsWith('0x')
    const localInboxRecord = await this.inboxStorage.getLatestConfirmedTx()
    if (useBatchInbox && localInboxRecord) {
      // read total elements
      const inboxTx = await this.signer.provider.getTransaction(
        localInboxRecord.txHash
      )
      if (
        inboxTx.blockNumber &&
        inboxTx.blockNumber > 0 &&
        inboxTx.data &&
        inboxTx.data !== '0x' &&
        inboxTx.data.length > 142
      ) {
        // set start block from raw data
        // 0x[2: DA type] [2: compress type] [64: batch index] [64: L2 start] [8: total blocks]
        //  > 142 ( 2 + 2 + 2 + 64 + 64 + 8 )
        const inboxTxStartBlock = toNumber(
          '0x' + inboxTx.data.substring(70, 134)
        )
        const inboxTxTotal = toNumber('0x' + inboxTx.data.substring(134, 142))
        totalElements = inboxTxStartBlock + inboxTxTotal

        this.logger.info('Retrieved total elements from BatchInbox', {
          totalElements,
        })
      } else {
        this.logger.info('Retrieved total elements from CTC', {
          totalElements,
        })
      }
    } else {
      this.logger.info('Retrieved total elements from CTC', {
        totalElements,
      })
    }

    let endBlock: number = Math.min(
      startBlock + this.maxBatchSize,
      totalElements
    )

    // if for seqset upgrade only, end block should less than or equal with seqsetValidHeight-1,
    // this perhaps cause data size to low, force submit
    if (
      this.seqsetUpgradeOnly &&
      this.seqsetValidHeight > 0 &&
      endBlock > this.seqsetValidHeight
    ) {
      this.logger.info(
        `Set end block to ${this.seqsetValidHeight} when seqset upgrade only`
      )
      endBlock = this.seqsetValidHeight
    }

    if (startBlock >= endBlock) {
      if (startBlock > endBlock) {
        this.logger.error(
          'State commitment chain is larger than transaction chain. This should never happen!'
        )
      }
      this.logger.info(
        'No state commitments to submit. Skipping batch submission...'
      )
      return
    }
    return {
      start: startBlock,
      end: endBlock,
    }
  }

  public async _submitBatch(
    startBlock: number,
    endBlock: number
  ): Promise<TransactionReceipt> {
    // eslint-disable-next-line radix
    const proposer = parseInt(this.l2ChainId.toString()) + '_MVM_Proposer'
    const batch = await this._generateStateCommitmentBatch(startBlock, endBlock)
    const sccContract = this.fpUpgraded
      ? this.fpChainContract
      : this.chainContract
    let args = [this.l2ChainId, batch.stateRoots, startBlock, proposer]
    if (this.fpUpgraded) {
      args.push(batch.blockHash)
      args.push(batch.blockNumber)
    }

    const calldata = sccContract.interface.encodeFunctionData(
      'appendStateBatchByChainId',
      args
    )

    const batchSizeInBytes = remove0x(calldata).length / 2
    this.logger.debug('State batch generated', {
      batchSizeInBytes,
      calldata,
    })

    if (
      this.seqsetUpgradeOnly &&
      this.seqsetValidHeight > 0 &&
      endBlock >= this.seqsetValidHeight - 1
    ) {
      // force submit upgrade last batch
      this.logger.info('Force submit state when upgrade.', {
        endBlock,
        seqsetValidHeight: this.seqsetValidHeight,
      })
    } else if (!this._shouldSubmitBatch(batchSizeInBytes)) {
      return
    }

    const offsetStartsAtIndex = startBlock - this.blockOffset
    this.logger.debug('Submitting batch.', { calldata })

    // Generate the transaction we will repeatedly submit
    const nonce = await this.signer.getNonce() //mpc address , 2 mpc addresses
    // state ctc are different signer addresses.
    args = [this.l2ChainId, batch.stateRoots, offsetStartsAtIndex, proposer]
    if (this.fpUpgraded) {
      args.push(batch.blockHash)
      args.push(batch.blockNumber)
    }
    const tx = await sccContract
      .getFunction('appendStateBatchByChainId')
      .populateTransaction(...args, { nonce })

    // MPC enabled: prepare nonce, gasPrice
    if (this.mpcUrl) {
      this.logger.info('submitter state with mpc', { url: this.mpcUrl })
      const mpcClient = new MpcClient(this.mpcUrl)
      const mpcInfo = await mpcClient.getLatestMpc('1')
      if (!mpcInfo || !mpcInfo.mpc_address) {
        throw new Error('MPC 1 info get failed')
      }
      const txUnsign: ContractTransaction = {
        to: tx.to,
        data: tx.data,
        value: ethers.parseEther('0'),
      }
      const mpcAddress = mpcInfo.mpc_address
      txUnsign.nonce = await this.signer.provider.getTransactionCount(
        mpcAddress
      )
      txUnsign.gasLimit = await this.signer.provider.estimateGas({
        to: tx.to,
        from: mpcAddress,
        data: tx.data,
      })
      txUnsign.chainId = (await this.signer.provider.getNetwork()).chainId
      // mpc model can use ynatm
      // tx.gasPrice = gasPrice

      this.logger.info('submitting with mpc address', { mpcAddress, txUnsign })

      const submitSignedTransaction = (): Promise<TransactionReceipt> => {
        return this.transactionSubmitter.submitSignedTransaction(
          txUnsign,
          async (gasPrice) => {
            try {
              txUnsign.gasPrice =
                gasPrice || (await this.signer.provider.getFeeData()).gasPrice
              const signedTx = await mpcClient.signTx(txUnsign, mpcInfo.mpc_id)
              return signedTx
            } catch (e) {
              this.logger.error('MPC sign tx failed', { error: e })
              throw e
            }
          },
          this._makeHooks('appendSequencerBatch')
        )
      }
      return this._submitAndLogTx(
        submitSignedTransaction,
        'Submitted state root batch with MPC!'
      )
    }

    this.logger.info('Submitting batch.', {
      chainId: this.l2ChainId,
      proposer,
    })
    const submitTransaction = (): Promise<TransactionReceipt> => {
      return this.transactionSubmitter.submitTransaction(
        tx,
        this._makeHooks('appendStateBatch')
      )
    }
    return this._submitAndLogTx(
      submitTransaction,
      'Submitted state root batch!'
    )
  }

  public async _mpcBalanceCheck(): Promise<boolean> {
    if (!this.useMpc) {
      return true
    }
    this.logger.info('MPC model balance check of state batch submitter...')
    const mpcClient = new MpcClient(this.mpcUrl)
    const mpcInfo = await mpcClient.getLatestMpc('1')
    if (!mpcInfo || !mpcInfo.mpc_address) {
      this.logger.error('MPC 1 info get failed')
      return false
    }
    return this._hasEnoughETHToCoverGasCosts(mpcInfo.mpc_address)
  }

  /*********************
   * Private Functions *
   ********************/

  private async _generateStateCommitmentBatch(
    startBlock: number,
    endBlock: number
  ): Promise<{
    stateRoots: Bytes32[]
    blockHash: Bytes32
    blockNumber: number
  }> {
    const blockRange = endBlock - startBlock
    const batch: {
      stateRoot: Bytes32
      blockHash: Bytes32
      blockNumber: number
    }[] = await bPromise.map(
      [...Array(blockRange).keys()],
      async (i: number) => {
        this.logger.debug('Fetching L2BatchElement', {
          blockNo: startBlock + i,
        })
        const block = (await this.l2Provider.getBlock(
          startBlock + i,
          true
        )) as L2Block
        const blockTx = await block.getTransaction(0)
        if (
          blockTx.from.toLowerCase() ===
          this.fraudSubmissionAddress.toLowerCase()
        ) {
          this.logger.warn('Found transaction from fraud submission address', {
            txHash: blockTx.hash,
            fraudSubmissionAddress: this.fraudSubmissionAddress,
          })
          this.fraudSubmissionAddress = 'no fraud'
          return {
            stateRoot:
              '0xbad1bad1bad1bad1bad1bad1bad1bad1bad1bad1bad1bad1bad1bad1bad1bad1',
            blockHash:
              '0xbad1bad1bad1bad1bad1bad1bad1bad1bad1bad1bad1bad1bad1bad1bad1bad1',
            blockNumber: startBlock + i,
          }
        }
        return {
          stateRoot: block.stateRoot,
          blockHash: block.hash,
          blockNumber: startBlock + i,
        }
      },
      { concurrency: 100 }
    )

    const proposer = this.l2ChainId + '_MVM_Proposer'
    let stateRoots = batch.map((b) => b.stateRoot)
    let tx = this.chainContract.interface.encodeFunctionData(
      'appendStateBatchByChainId',
      [
        this.l2ChainId,
        stateRoots,
        startBlock,
        proposer,
        batch[batch.length - 1].blockHash,
        batch[batch.length - 1].blockNumber,
      ]
    )
    while (remove0x(tx).length / 2 > this.maxTxSize) {
      batch.splice(Math.ceil((batch.length * 2) / 3)) // Delete 1/3rd of all of the batch elements
      this.logger.debug('Splicing batch...', {
        batchSizeInBytes: tx.length / 2,
      })
      stateRoots = batch.map((b) => b.stateRoot)
      tx = this.chainContract.interface.encodeFunctionData(
        'appendStateBatchByChainId',
        [
          this.l2ChainId,
          stateRoots,
          startBlock,
          proposer,
          batch[batch.length - 1].blockHash,
          batch[batch.length - 1].blockNumber,
        ]
      )
    }

    this.logger.info('Generated state commitment batch', {
      stateRoots, // list of stateRoots
    })
    return {
      stateRoots,
      blockHash: batch[batch.length - 1].blockHash,
      blockNumber: batch[batch.length - 1].blockNumber,
    }
  }
}
