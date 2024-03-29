/* External Imports */
import { Promise as bPromise } from 'bluebird'
import { Signer, ethers, Contract, providers, BigNumber } from 'ethers'
import { TransactionReceipt } from '@ethersproject/abstract-provider'
import {
  getContractInterface,
  getContractFactory,
  getContractInterface as getNewContractInterface,
} from '@metis.io/contracts'

import {
  L2Block,
  RollupInfo,
  BatchElement,
  Batch,
  QueueOrigin,
  EncodeSequencerBatchOptions,
  MinioClient,
  MinioConfig,
  remove0x,
  toHexString,
} from '@metis.io/core-utils'
import { Logger, Metrics } from '@eth-optimism/common-ts'

/* Internal Imports */
import {
  CanonicalTransactionChainContract,
  encodeAppendSequencerBatch,
  BatchContext,
  AppendSequencerBatchParams,
} from '../transaction-chain-contract'

import { BlockRange, BatchSubmitter, TransactionBatchSubmitterInbox } from '.'
import { TransactionSubmitter, MpcClient, sequencerSetABI } from '../utils'
import { InboxStorage, InboxRecordInfo } from '../storage'

export interface AutoFixBatchOptions {
  fixDoublePlayedDeposits: boolean
  fixMonotonicity: boolean
  fixSkippedDeposits: boolean
}

export class TransactionBatchSubmitter extends BatchSubmitter {
  protected chainContract: CanonicalTransactionChainContract
  protected mvmCtcContract: CanonicalTransactionChainContract
  protected seqsetContract: Contract
  protected l2ChainId: number
  protected syncing: boolean
  private autoFixBatchOptions: AutoFixBatchOptions
  private validateBatch: boolean
  private transactionSubmitter: TransactionSubmitter
  private gasThresholdInGwei: number
  private useMinio: boolean
  private minioConfig: MinioConfig
  private encodeSequencerBatchOptions?: EncodeSequencerBatchOptions
  private mpcUrl: string
  private inboxStorage: InboxStorage
  private inboxAddress: string
  private inboxStartIndex: string
  private inboxSubmitter: TransactionBatchSubmitterInbox
  private seqsetValidHeight: number
  private seqsetContractAddress: string
  private seqsetUpgradeOnly: boolean

  constructor(
    signer: Signer,
    l2Provider: providers.StaticJsonRpcProvider,
    minTxSize: number,
    maxTxSize: number,
    maxBatchSize: number,
    maxBatchSubmissionTime: number,
    numConfirmations: number,
    resubmissionTimeout: number,
    addressManagerAddress: string,
    minBalanceEther: number,
    gasThresholdInGwei: number,
    transactionSubmitter: TransactionSubmitter,
    blockOffset: number,
    validateBatch: boolean,
    logger: Logger,
    metrics: Metrics,
    autoFixBatchOptions: AutoFixBatchOptions = {
      fixDoublePlayedDeposits: false,
      fixMonotonicity: false,
      fixSkippedDeposits: false,
    }, // TODO: Remove this
    useMinio: boolean,
    minioConfig: MinioConfig,
    mpcUrl: string,
    batchInboxAddress: string,
    batchInboxStartIndex: string,
    batchInboxStoragePath: string,
    seqsetValidHeight: number,
    seqsetContractAddress: string,
    seqsetUpgradeOnly: number
  ) {
    super(
      signer,
      l2Provider,
      minTxSize,
      maxTxSize,
      maxBatchSize,
      maxBatchSubmissionTime,
      numConfirmations,
      resubmissionTimeout,
      0, // Supply dummy value because it is not used.
      addressManagerAddress,
      minBalanceEther,
      blockOffset,
      logger,
      metrics,
      mpcUrl.length > 0
    )
    this.validateBatch = validateBatch
    this.autoFixBatchOptions = autoFixBatchOptions
    this.gasThresholdInGwei = gasThresholdInGwei
    this.transactionSubmitter = transactionSubmitter
    this.useMinio = useMinio
    this.minioConfig = minioConfig
    this.mpcUrl = mpcUrl

    this.inboxAddress = batchInboxAddress
    this.inboxStartIndex = batchInboxStartIndex
    this.inboxStorage = new InboxStorage(batchInboxStoragePath, logger)
    this.inboxSubmitter = new TransactionBatchSubmitterInbox(
      this.inboxStorage,
      this.inboxAddress,
      this.l2Provider,
      this.logger,
      this.maxTxSize,
      useMinio,
      minioConfig
    )
    this.seqsetValidHeight = seqsetValidHeight
    this.seqsetContractAddress = seqsetContractAddress
    this.seqsetUpgradeOnly = seqsetUpgradeOnly === 1

    this.logger.info('Batch validation options', {
      autoFixBatchOptions,
      validateBatch,
    })
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
    const addrs = await this._getChainAddresses()
    const ctcAddress = addrs.ctcAddress
    const mvmCtcAddress = addrs.mvmCtcAddress

    if (mvmCtcAddress === ethers.constants.AddressZero) {
      this.logger.error('MVM_CanonicalTransaction contract load failed')
      process.exit(1)
    }

    if (
      typeof this.chainContract !== 'undefined' &&
      ctcAddress === this.chainContract.address &&
      typeof this.mvmCtcContract !== 'undefined' &&
      mvmCtcAddress === this.mvmCtcContract.address
    ) {
      this.logger.debug('Chain contract already initialized', {
        ctcAddress,
        mvmCtcAddress,
        seqsetAddress: this.seqsetContractAddress,
      })
      return
    }

    const unwrapped_OVM_CanonicalTransactionChain = (
      await getContractFactory('CanonicalTransactionChain', this.signer)
    ).attach(ctcAddress)

    this.chainContract = new CanonicalTransactionChainContract(
      unwrapped_OVM_CanonicalTransactionChain.address,
      getContractInterface('CanonicalTransactionChain'),
      this.signer
    )
    this.logger.info('Initialized new CTC', {
      address: this.chainContract.address,
    })

    const unwrapped_MVM_CanonicalTransaction = (
      await getContractFactory('MVM_CanonicalTransaction', this.signer)
    ).attach(mvmCtcAddress)

    this.mvmCtcContract = new CanonicalTransactionChainContract(
      unwrapped_MVM_CanonicalTransaction.address,
      getContractInterface('MVM_CanonicalTransaction'),
      this.signer // to be replaced
    )
    this.logger.info('Initialized new mvmCTC', {
      address: this.mvmCtcContract.address,
    })

    if (this.seqsetValidHeight > 0 && this.seqsetContractAddress) {
      this.seqsetContract = new ethers.Contract(
        this.seqsetContractAddress,
        sequencerSetABI,
        this.l2Provider
      )

      this.logger.info('Connected L2 Seqset contracts', {
        seqsetContract: this.seqsetContract.address,
      })
    }
    return
  }

  public async _onSync(): Promise<TransactionReceipt> {
    const pendingQueueElements =
      await this.chainContract.getNumPendingQueueElements()
    this.logger.debug('Got number of pending queue elements', {
      pendingQueueElements,
    })

    if (pendingQueueElements !== 0) {
      this.logger.info(
        'Syncing mode enabled! Skipping batch submission and clearing queue elements',
        { pendingQueueElements }
      )
    }

    this.logger.info('Syncing mode enabled but queue is empty. Skipping...')
    return
  }

  public async _getBatchStartAndEnd(): Promise<BlockRange> {
    this.logger.info(
      'Getting batch start and end for transaction batch submitter...'
    )

    let startBlock =
      (
        await this.chainContract.getTotalElementsByChainId(this.l2ChainId)
      ).toNumber() + this.blockOffset
    this.logger.info('Retrieved start block number from CTC', {
      startBlock,
    })

    // batch index start from config
    const batchIndexStart = BigNumber.from(this.inboxStartIndex).toNumber()
    // current batch index from CTC contract
    const batchIndexCtcNext = (
      await this.chainContract.getTotalBatchesByChainId(this.l2ChainId)
    ).toNumber()
    const localInboxRecord = await this.inboxStorage.getLatestConfirmedTx()
    const useBatchInbox =
      this.inboxAddress &&
      this.inboxAddress.length === 42 &&
      this.inboxAddress.startsWith('0x') &&
      batchIndexStart <= batchIndexCtcNext
    // read next batch index from local storage and inbox tx hash
    let batchIndexNext = batchIndexCtcNext
    if (localInboxRecord) {
      const localBatchIndex = BigNumber.from(
        localInboxRecord.batchIndex
      ).toNumber()
      if (localBatchIndex >= batchIndexNext) {
        batchIndexNext = localBatchIndex + 1

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
          const inboxTxStartBlock = BigNumber.from(
            '0x' + inboxTx.data.substring(70, 134)
          ).toNumber()
          const inboxTxTotal = BigNumber.from(
            '0x' + inboxTx.data.substring(134, 142)
          ).toNumber()
          startBlock = inboxTxStartBlock + inboxTxTotal

          this.logger.info('Retrieved start block number from BatchInbox tx', {
            inboxTxStartBlock,
            inboxTxTotal,
            startBlock,
            txHash: localInboxRecord.txHash,
          })
        }
      }
    }
    this.logger.info('Retrieved batch index info', {
      configStartIndex: batchIndexStart,
      ctcIndexNext: batchIndexCtcNext,
      useBatchInbox,
      localInboxRecord,
    })

    let endBlock =
      Math.min(
        startBlock + this.maxBatchSize,
        await this.l2Provider.getBlockNumber()
      ) + 1 // +1 because the `endBlock` is *exclusive*
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
    // confirmation block
    if (
      !this.seqsetUpgradeOnly &&
      this.seqsetContractAddress &&
      this.seqsetValidHeight > 0 &&
      endBlock > this.seqsetValidHeight
    ) {
      // seqsetContract should ready
      const l2FinalizeBlock = await this.seqsetContract.finalizedBlock()
      const l2FinalizeBlockNum = BigNumber.from(l2FinalizeBlock).toNumber()
      if (l2FinalizeBlockNum > 0) {
        endBlock = Math.min(endBlock, l2FinalizeBlockNum + 1)
      }
    }

    this.logger.info('Retrieved end block number from L2 sequencer', {
      startBlock,
      endBlock,
      l2chainId: this.l2ChainId,
    })

    if (startBlock >= endBlock) {
      if (startBlock > endBlock) {
        this.logger
          .error(`More chain elements in L1 (${startBlock}) than in the L2 node (${endBlock}).
                   This shouldn't happen because we don't submit batches if the sequencer is syncing.`)
      }
      this.logger.info('No txs to submit. Skipping batch submission...')
      return
    }
    return {
      start: startBlock,
      end: endBlock,
      useInbox: useBatchInbox,
      nextBatchIndex: batchIndexNext,
    }
  }

  public async _submitBatch(
    startBlock: number,
    endBlock: number,
    useInbox?: boolean,
    nextBatchIndex?: number
  ): Promise<TransactionReceipt> {
    // Do not submit batch if gas price above threshold
    const gasPriceInGwei = parseInt(
      ethers.utils.formatUnits(await this.signer.getGasPrice(), 'gwei'),
      10
    )
    if (gasPriceInGwei > this.gasThresholdInGwei) {
      this.logger.warn(
        'Gas price is higher than gas price threshold; aborting batch submission',
        {
          gasPriceInGwei,
          gasThresholdInGwei: this.gasThresholdInGwei,
        }
      )
      return
    }

    if (useInbox) {
      this.logger.debug('Will submit batch to inbox address', {
        startBlock,
        endBlock,
        nextBatchIndex,
      })
      return this.inboxSubmitter.submitBatchToInbox(
        startBlock,
        endBlock,
        nextBatchIndex,
        this.metrics,
        this.signer,
        this.mpcUrl,
        (sizeInBytes: number): boolean => {
          return this._shouldSubmitBatch(sizeInBytes)
        },
        this.transactionSubmitter,
        this._makeHooks('sendBatchToInbox'),
        (
          submitTransaction: () => Promise<TransactionReceipt>,
          successMessage: string,
          callback?: (
            receipt: TransactionReceipt | null,
            err: any
          ) => Promise<boolean>
        ): Promise<TransactionReceipt> => {
          return this._submitAndLogTx(
            submitTransaction,
            successMessage,
            callback
          )
        }
      )
    }

    const params = await this._generateSequencerBatchParams(
      startBlock,
      endBlock
    )
    if (!params) {
      throw new Error(
        `Cannot create sequencer batch with params start ${startBlock} and end ${endBlock}`
      )
    }

    const [batchParams, wasBatchTruncated] = params
    // encodeBatch of calldata for _shouldSubmitBatch
    const encodeBatch = await encodeAppendSequencerBatch(batchParams, null)
    const batchSizeInBytes = encodeBatch.length / 2
    this.logger.debug('Sequencer batch generated', {
      batchSizeInBytes,
    })

    // Only submit batch if one of the following is true:
    // 1. it was truncated
    // 2. it is large enough
    // 3. enough time has passed since last submission
    if (
      this.seqsetUpgradeOnly &&
      this.seqsetValidHeight > 0 &&
      endBlock >= this.seqsetValidHeight - 1
    ) {
      // force submit upgrade last batch
      this.logger.info('Force submit tx when upgrade.', {
        endBlock,
        seqsetValidHeight: this.seqsetValidHeight,
      })
    } else if (
      !wasBatchTruncated &&
      !this._shouldSubmitBatch(batchSizeInBytes)
    ) {
      return
    }
    this.metrics.numTxPerBatch.observe(endBlock - startBlock)
    const l1tipHeight = await this.signer.provider.getBlockNumber()
    this.logger.debug('Submitting batch.', {
      calldata: batchParams,
      l1tipHeight,
    })

    return this.submitAppendSequencerBatch(batchParams)
  }

  public async _mpcBalanceCheck(): Promise<boolean> {
    if (!this.useMpc) {
      return true
    }
    this.logger.info('MPC model balance check of tx batch submitter...')
    const mpcClient = new MpcClient(this.mpcUrl)
    const mpcInfo = await mpcClient.getLatestMpc()
    if (!mpcInfo || !mpcInfo.mpc_address) {
      this.logger.error('MPC 0 info get failed')
      return false
    }
    return this._hasEnoughETHToCoverGasCosts(mpcInfo.mpc_address)
  }

  /*********************
   * Private Functions *
   ********************/

  private async getEncodeAppendSequencerBatchOptions() {
    if (!this.encodeSequencerBatchOptions) {
      if (!this.l2ChainId) {
        this.l2ChainId = await this._getL2ChainId()
      }
      if (this.minioConfig) {
        this.minioConfig.l2ChainId = this.l2ChainId
      }

      this.encodeSequencerBatchOptions = {
        useMinio: this.useMinio,
        minioClient: this.minioConfig
          ? new MinioClient(this.minioConfig)
          : null,
      }
    }
  }

  private async submitAppendSequencerBatch(
    batchParams: AppendSequencerBatchParams
  ): Promise<TransactionReceipt> {
    await this.getEncodeAppendSequencerBatchOptions()
    // if (this.encodeSequencerBatchOptions?.useMinio) {
    //   this.logger.info('encode batch options minioClient if null: ' + (this.encodeSequencerBatchOptions?.minioClient == null).toString())
    // }
    // const tx =
    //   await this.chainContract.customPopulateTransaction.appendSequencerBatch(
    //     batchParams,
    //     this.encodeSequencerBatchOptions
    //   )
    // unsigned tx
    const tx = this.useMinio
      ? await this.mvmCtcContract.customPopulateTransaction.appendSequencerBatch(
          batchParams,
          this.encodeSequencerBatchOptions
        )
      : await this.chainContract.customPopulateTransaction.appendSequencerBatch(
          batchParams,
          this.encodeSequencerBatchOptions
        )

    // MPC enabled: prepare nonce, gasPrice
    if (this.mpcUrl) {
      this.logger.info('submitter with mpc', { url: this.mpcUrl })
      const mpcClient = new MpcClient(this.mpcUrl)
      const mpcInfo = await mpcClient.getLatestMpc()
      if (!mpcInfo || !mpcInfo.mpc_address) {
        throw new Error('MPC info get failed')
      }
      const mpcAddress = mpcInfo.mpc_address
      tx.nonce = await this.signer.provider.getTransactionCount(mpcAddress)
      tx.gasLimit = await this.signer.provider.estimateGas({
        to: tx.to,
        from: mpcAddress,
        data: tx.data,
      })
      tx.value = ethers.utils.parseEther('0')
      tx.chainId = (await this.signer.provider.getNetwork()).chainId
      // mpc model can use ynatm
      // tx.gasPrice = gasPrice
      // mpcInfo.mpc_id

      const submitSignedTransaction = (): Promise<TransactionReceipt> => {
        return this.transactionSubmitter.submitSignedTransaction(
          tx,
          async (gasPrice) => {
            tx.gasPrice = gasPrice
            return mpcClient.signTx(tx, mpcInfo.mpc_id)
          },
          this._makeHooks('appendSequencerBatch')
        )
      }
      return this._submitAndLogTx(
        submitSignedTransaction,
        'Submitted batch with MPC!'
      )
    } else {
      tx.gasLimit = await this.signer.provider.estimateGas({
        //estimate gas
        to: tx.to,
        from: await this.signer.getAddress(), //mpc address
        data: tx.data,
      })
    }

    const submitTransaction = (): Promise<TransactionReceipt> => {
      return this.transactionSubmitter.submitTransaction(
        tx,
        this._makeHooks('appendSequencerBatch')
      )
    }
    return this._submitAndLogTx(submitTransaction, 'Submitted batch!')
  }

  private async _generateSequencerBatchParams(
    startBlock: number,
    endBlock: number
  ): Promise<[AppendSequencerBatchParams, boolean]> {
    // Get all L2 BatchElements for the given range
    const blockRange = endBlock - startBlock
    let batch: Batch = await bPromise.map(
      [...Array(blockRange).keys()],
      (i) => {
        this.logger.debug('Fetching L2BatchElement', {
          blockNo: startBlock + i,
        })
        return this._getL2BatchElement(startBlock + i)
      },
      { concurrency: 100 }
    )

    // fix max batch size with env and mvmCtc
    const mvmMaxBatchSize = await this.mvmCtcContract.getTxBatchSize()
    const fixedMaxTxSize = Math.min(this.maxTxSize, mvmMaxBatchSize)

    // Fix our batches if we are configured to. This will not
    // modify the batch unless an autoFixBatchOption is set
    batch = await this._fixBatch(batch)
    if (this.validateBatch) {
      this.logger.info('Validating batch')
      if (!(await this._validateBatch(batch))) {
        this.metrics.malformedBatches.inc()
        return
      }
    }

    let sequencerBatchParams = await this._getSequencerBatchParams(
      startBlock,
      batch
    )
    let wasBatchTruncated = false
    // This method checks encoded length without options anyway
    // it will set raw calldata to CTC if needs fraud proof
    let encoded = await encodeAppendSequencerBatch(sequencerBatchParams, null)
    while (encoded.length / 2 > fixedMaxTxSize) {
      this.logger.info('Splicing batch...', {
        batchSizeInBytes: encoded.length / 2,
      })
      batch.splice(Math.ceil((batch.length * 2) / 3)) // Delete 1/3rd of all of the batch elements
      sequencerBatchParams = await this._getSequencerBatchParams(
        startBlock,
        batch
      )
      encoded = await encodeAppendSequencerBatch(sequencerBatchParams, null)
      //  This is to prevent against the case where a batch is oversized,
      //  but then gets truncated to the point where it is under the minimum size.
      //  In this case, we want to submit regardless of the batch's size.
      wasBatchTruncated = true
    }
    return [sequencerBatchParams, wasBatchTruncated]
  }

  /**
   * Returns true if the batch is valid.
   */
  protected async _validateBatch(batch: Batch): Promise<boolean> {
    // Verify all of the queue elements are what we expect
    let nextQueueIndex = await this.chainContract.getNextQueueIndexByChainId(
      this.l2ChainId
    )
    for (const ele of batch) {
      this.logger.info('Verifying batch element', { ele })
      if (!ele.isSequencerTx) {
        this.logger.info('Checking queue equality against L1 queue index', {
          nextQueueIndex,
        })
        if (!(await this._doesQueueElementMatchL1(nextQueueIndex, ele))) {
          return false
        }
        nextQueueIndex++
      }
    }

    // Verify all of the batch elements are monotonic
    let lastTimestamp: number
    let lastBlockNumber: number
    for (const [idx, ele] of batch.entries()) {
      if (ele.timestamp < lastTimestamp) {
        this.logger.error('Timestamp monotonicity violated! Element', {
          idx,
          ele,
        })
        return false
      }
      if (ele.blockNumber < lastBlockNumber) {
        this.logger.error('Block Number monotonicity violated! Element', {
          idx,
          ele,
        })
        return false
      }
      lastTimestamp = ele.timestamp
      lastBlockNumber = ele.blockNumber
    }
    return true
  }

  private async _doesQueueElementMatchL1(
    queueIndex: number,
    queueElement: BatchElement
  ): Promise<boolean> {
    const logEqualityError = (name, index, expected, got) => {
      this.logger.error('Observed mismatched values', {
        index,
        expected,
        got,
      })
    }

    let isEqual = true
    const [queueEleHash, timestamp, blockNumber] =
      await this.chainContract.getQueueElementByChainId(
        this.l2ChainId,
        queueIndex
      )

    // TODO: Verify queue element hash equality. The queue element hash can be computed with:
    // keccak256( abi.encode( msg.sender, _target, _gasLimit, _data))

    // Check timestamp & blockNumber equality
    if (timestamp !== queueElement.timestamp) {
      isEqual = false
      logEqualityError(
        'Timestamp',
        queueIndex,
        timestamp,
        queueElement.timestamp
      )
    }
    if (blockNumber !== queueElement.blockNumber) {
      isEqual = false
      logEqualityError(
        'Block Number',
        queueIndex,
        blockNumber,
        queueElement.blockNumber
      )
    }
    return isEqual
  }

  /**
   * Takes in a batch which is potentially malformed & returns corrected version.
   * Current fixes that are supported:
   * - Double played deposits.
   */
  private async _fixBatch(batch: Batch): Promise<Batch> {
    const fixDoublePlayedDeposits = async (b: Batch): Promise<Batch> => {
      let nextQueueIndex = await this.chainContract.getNextQueueIndexByChainId(
        this.l2ChainId
      )
      const fixedBatch: Batch = []
      for (const ele of b) {
        if (!ele.isSequencerTx) {
          if (!(await this._doesQueueElementMatchL1(nextQueueIndex, ele))) {
            this.logger.warn('Fixing double played queue element.', {
              nextQueueIndex,
            })
            fixedBatch.push(
              await this._fixDoublePlayedDepositQueueElement(
                nextQueueIndex,
                ele
              )
            )
            continue
          }
          nextQueueIndex++
        }
        fixedBatch.push(ele)
      }
      return fixedBatch
    }

    const fixSkippedDeposits = async (b: Batch): Promise<Batch> => {
      this.logger.debug('Fixing skipped deposits...')
      let nextQueueIndex = await this.chainContract.getNextQueueIndex()
      const fixedBatch: Batch = []
      for (const ele of b) {
        // Look for skipped deposits
        while (true) {
          const pendingQueueElements =
            await this.chainContract.getNumPendingQueueElementsByChainId(
              this.l2ChainId
            )
          const nextRemoteQueueElements =
            await this.chainContract.getNextQueueIndexByChainId(this.l2ChainId)
          const totalQueueElements =
            pendingQueueElements + nextRemoteQueueElements
          // No more queue elements so we clearly haven't skipped anything
          if (nextQueueIndex >= totalQueueElements) {
            break
          }
          const [queueEleHash, timestamp, blockNumber] =
            await this.chainContract.getQueueElementByChainId(
              this.l2ChainId,
              nextQueueIndex
            )

          if (timestamp < ele.timestamp || blockNumber < ele.blockNumber) {
            this.logger.warn('Fixing skipped deposit', {
              badTimestamp: ele.timestamp,
              skippedQueueTimestamp: timestamp,
              badBlockNumber: ele.blockNumber,
              skippedQueueBlockNumber: blockNumber,
            })
            // Push a dummy queue element
            fixedBatch.push({
              stateRoot: ele.stateRoot,
              isSequencerTx: false,
              rawTransaction: undefined,
              timestamp,
              blockNumber,
              seqSign: null,
            })
            nextQueueIndex++
          } else {
            // The next queue element's timestamp is after this batch element so
            // we must not have skipped anything.
            break
          }
        }
        fixedBatch.push(ele)
        if (!ele.isSequencerTx) {
          nextQueueIndex++
        }
      }
      return fixedBatch
    }

    // TODO: Remove this super complex logic and rely on Geth to actually supply correct block data.
    const fixMonotonicity = async (b: Batch): Promise<Batch> => {
      this.logger.debug('Fixing monotonicity...')
      // The earliest allowed timestamp/blockNumber is the last timestamp submitted on chain.
      const { lastTimestamp, lastBlockNumber } =
        await this._getLastTimestampAndBlockNumber()
      let earliestTimestamp = lastTimestamp
      let earliestBlockNumber = lastBlockNumber
      this.logger.debug('Determined earliest timestamp and blockNumber', {
        earliestTimestamp,
        earliestBlockNumber,
      })

      // The latest allowed timestamp/blockNumber is the next queue element!
      let nextQueueIndex = await this.chainContract.getNextQueueIndexByChainId(
        this.l2ChainId
      )
      let latestTimestamp: number
      let latestBlockNumber: number

      // updateLatestTimestampAndBlockNumber is a helper which updates
      // the latest timestamp and block number based on the pending queue elements.
      const updateLatestTimestampAndBlockNumber = async () => {
        const pendingQueueElements =
          await this.chainContract.getNumPendingQueueElementsByChainId(
            this.l2ChainId
          )
        const nextRemoteQueueElements =
          await this.chainContract.getNextQueueIndexByChainId(this.l2ChainId)
        const totalQueueElements =
          pendingQueueElements + nextRemoteQueueElements
        if (nextQueueIndex < totalQueueElements) {
          const [queueEleHash, queueTimestamp, queueBlockNumber] =
            await this.chainContract.getQueueElementByChainId(
              this.l2ChainId,
              nextQueueIndex
            )
          latestTimestamp = queueTimestamp
          latestBlockNumber = queueBlockNumber
        } else {
          // If there are no queue elements left then just allow any timestamp/blocknumber
          latestTimestamp = Number.MAX_SAFE_INTEGER
          latestBlockNumber = Number.MAX_SAFE_INTEGER
        }
      }
      // Actually update the latest timestamp and block number
      await updateLatestTimestampAndBlockNumber()
      this.logger.debug('Determined latest timestamp and blockNumber', {
        latestTimestamp,
        latestBlockNumber,
      })

      // Now go through our batch and fix the timestamps and block numbers
      // to automatically enforce monotonicity.
      const fixedBatch: Batch = []
      for (const ele of b) {
        if (!ele.isSequencerTx) {
          // Set the earliest allowed timestamp to the old latest and set the new latest
          // to the next queue element's timestamp / blockNumber
          earliestTimestamp = latestTimestamp
          earliestBlockNumber = latestBlockNumber
          nextQueueIndex++
          await updateLatestTimestampAndBlockNumber()
        }
        // Fix the element if its timestammp/blockNumber is too small
        if (
          ele.timestamp < earliestTimestamp ||
          ele.blockNumber < earliestBlockNumber
        ) {
          this.logger.warn('Fixing timestamp/blockNumber too small', {
            oldTimestamp: ele.timestamp,
            newTimestamp: earliestTimestamp,
            oldBlockNumber: ele.blockNumber,
            newBlockNumber: earliestBlockNumber,
          })
          ele.timestamp = earliestTimestamp
          ele.blockNumber = earliestBlockNumber
        }
        // Fix the element if its timestammp/blockNumber is too large
        if (
          ele.timestamp > latestTimestamp ||
          ele.blockNumber > latestBlockNumber
        ) {
          this.logger.warn('Fixing timestamp/blockNumber too large.', {
            oldTimestamp: ele.timestamp,
            newTimestamp: latestTimestamp,
            oldBlockNumber: ele.blockNumber,
            newBlockNumber: latestBlockNumber,
          })
          ele.timestamp = latestTimestamp
          ele.blockNumber = latestBlockNumber
        }
        earliestTimestamp = ele.timestamp
        earliestBlockNumber = ele.blockNumber
        fixedBatch.push(ele)
      }
      return fixedBatch
    }

    // NOTE: It is unsafe to combine multiple autoFix options.
    // If you must combine them, manually verify the output before proceeding.
    if (this.autoFixBatchOptions.fixDoublePlayedDeposits) {
      this.logger.info('Fixing double played deposits')
      batch = await fixDoublePlayedDeposits(batch)
    }
    if (this.autoFixBatchOptions.fixMonotonicity) {
      this.logger.info('Fixing monotonicity')
      batch = await fixMonotonicity(batch)
    }
    if (this.autoFixBatchOptions.fixSkippedDeposits) {
      this.logger.info('Fixing skipped deposits')
      batch = await fixSkippedDeposits(batch)
    }
    return batch
  }

  private async _getLastTimestampAndBlockNumber(): Promise<{
    lastTimestamp: number
    lastBlockNumber: number
  }> {
    const manager = new Contract(
      this.addressManagerAddress,
      getNewContractInterface('Lib_AddressManager'),
      this.signer.provider
    )

    const addr = await manager.getAddress('ChainStorageContainer-CTC-batches')

    const container = new Contract(
      addr,
      getNewContractInterface('IChainStorageContainer'),
      this.signer.provider
    )

    let meta = await container.getGlobalMetadata()
    // remove 0x
    meta = meta.slice(2)
    // convert to bytes27
    meta = meta.slice(10)

    const totalElements = meta.slice(-10)
    const nextQueueIndex = meta.slice(-20, -10)
    const lastTimestamp = parseInt(meta.slice(-30, -20), 16)
    const lastBlockNumber = parseInt(meta.slice(-40, -30), 16)
    this.logger.debug('Retrieved timestamp and block number from CTC', {
      lastTimestamp,
      lastBlockNumber,
    })

    return { lastTimestamp, lastBlockNumber }
  }

  private async _fixDoublePlayedDepositQueueElement(
    queueIndex: number,
    queueElement: BatchElement
  ): Promise<BatchElement> {
    const [queueEleHash, timestamp, blockNumber] =
      await this.chainContract.getQueueElementByChainId(
        this.l2ChainId,
        queueIndex
      )

    if (
      timestamp > queueElement.timestamp &&
      blockNumber > queueElement.blockNumber
    ) {
      this.logger.warn(
        'Double deposit detected. Fixing by skipping the deposit & replacing with a dummy tx.',
        {
          timestamp,
          blockNumber,
          queueElementTimestamp: queueElement.timestamp,
          queueElementBlockNumber: queueElement.blockNumber,
        }
      )
      const dummyTx: string = '0x1234'
      return {
        stateRoot: queueElement.stateRoot,
        isSequencerTx: true,
        rawTransaction: dummyTx,
        timestamp: queueElement.timestamp,
        blockNumber: queueElement.blockNumber,
        seqSign: null, // NOTE dummyTx without sequencer sign, need compare in l2geth?
      }
    }
    if (
      timestamp < queueElement.timestamp &&
      blockNumber < queueElement.blockNumber
    ) {
      this.logger.error('A deposit seems to have been skipped!')
      throw new Error('Skipped deposit?!')
    }
    throw new Error('Unable to fix queue element!')
  }

  private async _getSequencerBatchParams(
    shouldStartAtIndex: number,
    blocks: Batch
  ): Promise<AppendSequencerBatchParams> {
    const totalElementsToAppend = blocks.length

    // Generate contexts
    const contexts: BatchContext[] = []
    let lastBlockIsSequencerTx = false
    let lastTimestamp = 0
    let lastBlockNumber = 0
    const groupedBlocks: Array<{
      sequenced: BatchElement[]
      queued: BatchElement[]
    }> = []
    for (const block of blocks) {
      // if (
      //   (lastBlockIsSequencerTx === false && block.isSequencerTx === true) ||
      //   groupedBlocks.length === 0 ||
      //   (block.timestamp !== lastTimestamp && block.isSequencerTx === true) ||
      //   (block.blockNumber !== lastBlockNumber && block.isSequencerTx === true)
      // ) {
      //   groupedBlocks.push({
      //     sequenced: [],
      //     queued: [],
      //   })
      // }
      if (groupedBlocks.length === 0) {
        groupedBlocks.push({
          sequenced: [],
          queued: [],
        })
      } else if (block.isSequencerTx !== lastBlockIsSequencerTx) {
        groupedBlocks.push({
          sequenced: [],
          queued: [],
        })
      } else if (
        block.timestamp !== lastTimestamp ||
        block.blockNumber !== lastBlockNumber
      ) {
        groupedBlocks.push({
          sequenced: [],
          queued: [],
        })
      }
      const cur = groupedBlocks.length - 1
      block.isSequencerTx
        ? groupedBlocks[cur].sequenced.push(block)
        : groupedBlocks[cur].queued.push(block)
      lastBlockIsSequencerTx = block.isSequencerTx
      lastTimestamp = block.timestamp
      lastBlockNumber = block.blockNumber
    }
    for (const groupedBlock of groupedBlocks) {
      if (
        groupedBlock.sequenced.length === 0 &&
        groupedBlock.queued.length === 0
      ) {
        throw new Error(
          'Attempted to generate batch context with 0 queued and 0 sequenced txs!'
        )
      }
      this.logger.warn('Fetched L2 block', {
        seqLen: groupedBlock.sequenced.length,
        queLen: groupedBlock.queued.length,
      })
      contexts.push({
        numSequencedTransactions: groupedBlock.sequenced.length,
        numSubsequentQueueTransactions: groupedBlock.queued.length,
        timestamp:
          groupedBlock.sequenced.length > 0
            ? groupedBlock.sequenced[0].timestamp
            : groupedBlock.queued[0].timestamp,
        blockNumber:
          groupedBlock.sequenced.length > 0
            ? groupedBlock.sequenced[0].blockNumber
            : groupedBlock.queued[0].blockNumber,
      })
    }

    // Generate sequencer transactions
    const transactions: string[] = []
    const blockNumbers: number[] = []
    const seqSigns: string[] = []
    let l2BlockNumber = shouldStartAtIndex
    for (const block of blocks) {
      if (!block.isSequencerTx) {
        l2BlockNumber++
        continue
      }
      transactions.push(block.rawTransaction)
      blockNumbers.push(l2BlockNumber)
      seqSigns.push(block.seqSign)
      l2BlockNumber++
    }

    return {
      chainId: this.l2ChainId,
      shouldStartAtElement: shouldStartAtIndex - this.blockOffset,
      totalElementsToAppend,
      contexts,
      transactions,
      blockNumbers,
      seqSigns,
    }
  }

  private async _getL2BatchElement(blockNumber: number): Promise<BatchElement> {
    const block = await this._getBlock(blockNumber)
    this.logger.debug('Fetched L2 block', {
      block,
    })

    const batchElement = {
      stateRoot: block.stateRoot,
      timestamp: block.timestamp,
      blockNumber: block.transactions[0].l1BlockNumber,
      isSequencerTx: false,
      rawTransaction: undefined,
      seqSign: null,
    }

    if (this._isSequencerTx(block)) {
      if (block.transactions.length > 1) {
        throw new Error(
          `Not allowed, block ${block.number} has more than 1 transaction, if the block is right, please use inbox submitter by config BATCH_INBOX_START_INDEX`
        )
      }
      batchElement.isSequencerTx = true
      batchElement.rawTransaction = block.transactions[0].rawTransaction
      if (!block.transactions[0].seqR) {
        batchElement.seqSign = ''
      } else {
        let r = remove0x(block.transactions[0].seqR)
        let s = remove0x(block.transactions[0].seqS)
        let v = remove0x(block.transactions[0].seqV)
        if (r === '0') {
          r = '00'
        } else {
          r = this.padZerosToLeft(r)
        }
        if (s === '0') {
          s = '00'
        } else {
          s = this.padZerosToLeft(s)
        }
        if (v.length % 2 === 1) {
          v = `0${v}`
        }
        // restore: '' has no sign, '000000' is zero sign, `{64}{64}{n}` if n is 00, seqV is 0x0
        batchElement.seqSign = `${r}${s}${v}`
      }
    }

    return batchElement
  }

  private async _getBlock(blockNumber: number): Promise<L2Block> {
    // const p = this.l2Provider.getBlockWithTransactions(blockNumber)
    const p = this.l2Provider.send('eth_getBlockByNumber', [
      this.toRpcHexString(blockNumber),
      true,
    ])
    return p as Promise<L2Block>
  }

  private _isSequencerTx(block: L2Block): boolean {
    return block.transactions[0].queueOrigin === QueueOrigin.Sequencer
  }

  private toRpcHexString(n: number): string {
    if (n === 0) {
      return '0x0'
    } else {
      // prettier-ignore
      return '0x' + toHexString(n).slice(2).replace(/^0+/, '')
    }
  }

  private padZerosToLeft(inputString: string): string {
    const targetLength = 64
    if (inputString.length >= targetLength) {
      return inputString
    }

    const zerosToPad = targetLength - inputString.length
    const paddedString = '0'.repeat(zerosToPad) + inputString
    return paddedString
  }
}
