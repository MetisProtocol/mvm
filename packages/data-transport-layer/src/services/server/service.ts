/* Imports: External */
import { BaseService, Logger, Metrics } from '@eth-optimism/common-ts'
import express, { Request, Response } from 'express'
import promBundle from 'express-prom-bundle'
import cors from 'cors'
import { LevelUp } from 'levelup'
import * as Sentry from '@sentry/node'
import * as Tracing from '@sentry/tracing'
import { toBigInt, toNumber, JsonRpcProvider, Block } from 'ethers'

/* Imports: Internal */
import { TransportDB, TransportDBMapHolder } from '../../db/transport-db'
import {
  ContextResponse,
  GasPriceResponse,
  EnqueueResponse,
  StateRootBatchResponse,
  StateRootResponse,
  SyncingResponse,
  TransactionBatchResponse,
  TransactionResponse,
  BlockBatchResponse,
  BlockResponse,
  VerifierResultResponse,
  VerifierResultEntry,
  VerifierStakeResponse,
  AppendBatchElementResponse,
  HighestResponse,
  SyncStatusResponse,
  L1BlockRef,
  L2BlockRef,
} from '../../types'
import { validators } from '../../utils'
import { L1DataTransportServiceOptions } from '../main/service'

export interface L1TransportServerOptions
  extends L1DataTransportServiceOptions {
  db: LevelUp
  metrics: Metrics
  dbs: TransportDBMapHolder
}

const optionSettings = {
  db: {
    validate: validators.isLevelUP,
  },
  port: {
    default: 7878,
    validate: validators.isInteger,
  },
  hostname: {
    default: 'localhost',
    validate: validators.isString,
  },
  confirmations: {
    validate: validators.isInteger,
  },
  l1RpcProvider: {
    validate: (val: any) => {
      return validators.isUrl(val) || validators.isJsonRpcProvider(val)
    },
  },
  l2RpcProvider: {
    validate: (val: unknown) => {
      return validators.isUrl(val) || validators.isJsonRpcProvider(val)
    },
  },
  defaultBackend: {
    default: 'l1',
    validate: (val: string) => {
      return val === 'l1' || val === 'l2'
    },
  },
  l1GasPriceBackend: {
    default: 'l1',
    validate: (val: string) => {
      return val === 'l1' || val === 'l2'
    },
  },
}

export class L1TransportServer extends BaseService<L1TransportServerOptions> {
  constructor(options: L1TransportServerOptions) {
    super('L1_Transport_Server', options, optionSettings)
  }

  private state: {
    app: express.Express
    server: any
    db: TransportDB
    l1RpcProvider: JsonRpcProvider
    l2RpcProvider: JsonRpcProvider
  } = {} as any

  protected async _init(): Promise<void> {
    if (!this.options.db.isOpen()) {
      await this.options.db.open()
    }

    this.state.db = new TransportDB(
      this.options.db,
      this.options.l2ChainId === 1088
    )
    this.state.l1RpcProvider =
      typeof this.options.l1RpcProvider === 'string'
        ? new JsonRpcProvider(this.options.l1RpcProvider)
        : this.options.l1RpcProvider

    this.state.l2RpcProvider =
      typeof this.options.l2RpcProvider === 'string'
        ? new JsonRpcProvider(this.options.l2RpcProvider)
        : this.options.l2RpcProvider

    this._initializeApp()
  }

  protected async _start(): Promise<void> {
    this.state.server = this.state.app.listen(
      this.options.port,
      this.options.hostname
    )
    this.logger.info('Server started and listening', {
      host: this.options.hostname,
      port: this.options.port,
    })
  }

  protected async _stop(): Promise<void> {
    this.state.server.close()
  }

  /**
   * Initializes the server application.
   * Do any sort of initialization here that you want. Mostly just important that
   * `_registerAllRoutes` is called at the end.
   */
  private _initializeApp() {
    // TODO: Maybe pass this in as a parameter instead of creating it here?
    this.state.app = express()

    if (this.options.useSentry) {
      this._initSentry()
    }

    this.state.app.use(cors())

    // Add prometheus middleware to express BEFORE route registering
    this.state.app.use(
      // This also serves metrics on port 3000 at /metrics
      promBundle({
        // Provide metrics registry that other metrics uses
        promRegistry: this.metrics.registry,
        includeMethod: true,
        includePath: true,
      })
    )

    this._registerAllRoutes()

    // Sentry error handling must be after all controllers
    // and before other error middleware
    if (this.options.useSentry) {
      this.state.app.use(Sentry.Handlers.errorHandler())
    }

    this.logger.info('HTTP Server Options', {
      defaultBackend: this.options.defaultBackend,
      l1GasPriceBackend: this.options.l1GasPriceBackend,
    })

    if (this.state.l1RpcProvider) {
      this.logger.info('HTTP Server L1 RPC Provider initialized', {
        url: this.state.l1RpcProvider._getConnection().url,
      })
    } else {
      this.logger.warn('HTTP Server L1 RPC Provider not initialized')
    }
    if (this.state.l2RpcProvider) {
      this.logger.info('HTTP Server L2 RPC Provider initialized', {
        url: this.state.l2RpcProvider._getConnection().url,
      })
    } else {
      this.logger.warn('HTTP Server L2 RPC Provider not initialized')
    }
  }

  /**
   * Initialize Sentry and related middleware
   */
  private _initSentry() {
    const sentryOptions = {
      dsn: this.options.sentryDsn,
      release: this.options.release,
      environment: this.options.ethNetworkName,
    }
    this.logger = new Logger({
      name: this.name,
      sentryOptions,
    })
    Sentry.init({
      ...sentryOptions,
      integrations: [
        new Sentry.Integrations.Http({ tracing: true }),
        new Tracing.Integrations.Express({
          app: this.state.app,
        }),
      ],
      tracesSampleRate: this.options.sentryTraceRate,
    })
    this.state.app.use(Sentry.Handlers.requestHandler())
    this.state.app.use(Sentry.Handlers.tracingHandler())
  }

  /**
   * Registers a route on the server.
   *
   * @param method Http method type.
   * @param route Route to register.
   * @param handler Handler called and is expected to return a JSON response.
   */
  private _registerRoute(
    method: 'get', // Just handle GET for now, but could extend this with whatever.
    route: string,
    handler: (req?: Request, res?: Response) => Promise<any>
  ): void {
    // TODO: Better typing on the return value of the handler function.
    // TODO: Check for route collisions.
    // TODO: Add a different function to allow for removing routes.

    this.state.app[method](route, async (req, res) => {
      const start = Date.now()
      try {
        const json = await handler(req, res)
        this.logger.debug('Response body', {
          method: req.method,
          url: req.url,
          body: json,
        })
        return res.json(json)
      } catch (e) {
        const elapsed = Date.now() - start
        this.logger.error('Failed HTTP Request', {
          method: req.method,
          url: req.url,
          elapsed,
          msg: e.toString(),
          stack: e.stack,
        })
        return res.status(400).json({
          error: e.toString(),
        })
      }
    })
  }

  private async _getDb(chainId): Promise<TransportDB> {
    let db = this.state.db
    if (chainId) {
      db = await this.options.dbs.getTransportDbByChainId(chainId)
    }
    return db
  }

  private useBatchInbox(batchIndex: number): boolean {
    const inboxAddress = this.options.batchInboxAddress
    const inboxBatchStart = this.options.batchInboxStartIndex
    const inboxSender = this.options.batchInboxSender
    const useBatchInbox =
      inboxAddress &&
      inboxAddress.length === 42 &&
      inboxAddress.startsWith('0x') &&
      inboxSender &&
      inboxSender.length === 42 &&
      inboxSender.startsWith('0x') &&
      inboxBatchStart > 0 &&
      batchIndex > 0 &&
      inboxBatchStart <= batchIndex
    return useBatchInbox
  }

  /**
   * Registers all of the server routes we want to expose.
   * TODO: Link to our API spec.
   */
  private _registerAllRoutes(): void {
    this._registerRoute(
      'get',
      '/eth/syncing/:chainId',
      async (req): Promise<SyncingResponse> => {
        const chainId = toNumber(req.params.chainId)
        const db = await this._getDb(chainId)

        const backend = req.query.backend || this.options.defaultBackend

        let currentL2Block
        let highestL2BlockNumber
        switch (backend) {
          case 'l1':
            currentL2Block = await this.state.db.getLatestTransaction()
            highestL2BlockNumber = await db.getHighestL2BlockNumber()
            break
          case 'l2':
            currentL2Block = await db.getLatestUnconfirmedTransaction()
            highestL2BlockNumber =
              (await db.getHighestSyncedUnconfirmedBlock()) - 1
            break
          default:
            throw new Error(`Unknown transaction backend ${backend}`)
        }

        if (currentL2Block === null) {
          if (highestL2BlockNumber === null) {
            return {
              syncing: false,
              currentTransactionIndex: 0,
            }
          } else {
            return {
              syncing: true,
              highestKnownTransactionIndex: highestL2BlockNumber,
              currentTransactionIndex: 0,
            }
          }
        }

        if (highestL2BlockNumber > currentL2Block.index) {
          return {
            syncing: true,
            highestKnownTransactionIndex: highestL2BlockNumber,
            currentTransactionIndex: currentL2Block.index,
          }
        } else {
          return {
            syncing: false,
            currentTransactionIndex: currentL2Block.index,
          }
        }
      }
    )

    this._registerRoute(
      'get',
      '/highest/l1',
      async (req): Promise<HighestResponse> => {
        const db = await this._getDb(null)
        const blockNumber = await db.getHighestSyncedL1Block()
        return {
          blockNumber,
        }
      }
    )

    this._registerRoute(
      'get',
      '/eth/gasprice',
      async (req): Promise<GasPriceResponse> => {
        const backend = req.query.backend || this.options.l1GasPriceBackend
        let gasPrice: bigint

        if (backend === 'l1') {
          gasPrice = (await this.state.l1RpcProvider.getFeeData()).gasPrice
        } else if (backend === 'l2') {
          const response = await this.state.l2RpcProvider.send(
            'rollup_gasPrices',
            []
          )
          gasPrice = toBigInt(response.l1GasPrice)
        } else {
          throw new Error(`Unknown L1 gas price backend: ${backend}`)
        }

        return {
          gasPrice: gasPrice.toString(),
        }
      }
    )

    this._registerRoute(
      'get',
      '/eth/context/latest',
      async (): Promise<ContextResponse> => {
        const tip = await this.state.l1RpcProvider.getBlockNumber()
        const blockNumber = Math.max(0, tip - this.options.confirmations)

        const block = await this.state.l1RpcProvider.getBlock(blockNumber)

        return {
          blockNumber: block.number,
          timestamp: Math.floor(new Date().getTime() / 1000), // block.timestamp,
          blockHash: block.hash,
        }
      }
    )

    this._registerRoute(
      'get',
      '/eth/context/blocknumber/:number',
      async (req): Promise<ContextResponse> => {
        const number = toNumber(req.params.number)
        const tip = await this.state.l1RpcProvider.getBlockNumber()
        const blockNumber = Math.max(0, tip - this.options.confirmations)

        if (number > blockNumber) {
          return {
            blockNumber: null,
            timestamp: null,
            blockHash: null,
          }
        }

        const block = await this.state.l1RpcProvider.getBlock(number)
        return {
          blockNumber: block.number,
          timestamp: block.timestamp,
          blockHash: block.hash,
        }
      }
    )

    this._registerRoute(
      'get',
      '/enqueue/latest/:chainId',
      async (req): Promise<EnqueueResponse> => {
        const chainId = toNumber(req.params.chainId)
        const db = await this._getDb(chainId)
        const enqueue = await db.getLatestEnqueue()

        if (enqueue === null) {
          return {
            index: null,
            target: null,
            data: null,
            gasLimit: null,
            origin: null,
            blockNumber: null,
            timestamp: null,
            ctcIndex: null,
          }
        }

        const ctcIndex = await db.getTransactionIndexByQueueIndex(enqueue.index)

        return {
          ...enqueue,
          ctcIndex,
        }
      }
    )

    this._registerRoute(
      'get',
      '/enqueue/index/:index/:chainId',
      async (req): Promise<EnqueueResponse> => {
        const chainId = toNumber(req.params.chainId)
        const db = await this._getDb(chainId)
        const enqueue = await db.getEnqueueByIndex(toNumber(req.params.index))

        if (enqueue === null) {
          return {
            index: null,
            target: null,
            data: null,
            gasLimit: null,
            origin: null,
            blockNumber: null,
            timestamp: null,
            ctcIndex: null,
          }
        }

        const ctcIndex = await db.getTransactionIndexByQueueIndex(enqueue.index)

        return {
          ...enqueue,
          ctcIndex,
        }
      }
    )

    this._registerRoute(
      'get',
      '/transaction/latest/:chainId',
      async (req): Promise<TransactionResponse> => {
        const chainId = toNumber(req.params.chainId)
        const db = await this._getDb(chainId)

        const backend = req.query.backend || this.options.defaultBackend
        let transaction = null

        switch (backend) {
          case 'l1':
            transaction = await db.getLatestFullTransaction()
            break
          case 'l2':
            transaction = await db.getLatestUnconfirmedTransaction()
            break
          default:
            throw new Error(`Unknown transaction backend ${backend}`)
        }

        if (transaction === null) {
          return {
            transaction: null,
            batch: null,
          }
        }

        const batch = await db.getTransactionBatchByIndex(
          transaction.batchIndex
        )

        return {
          transaction,
          batch,
        }
      }
    )

    this._registerRoute(
      'get',
      '/transaction/index/:index/:chainId',
      async (req): Promise<TransactionResponse> => {
        const chainId = toNumber(req.params.chainId)
        const db = await this._getDb(chainId)

        const backend = req.query.backend || this.options.defaultBackend
        let transaction = null

        switch (backend) {
          case 'l1':
            transaction = await db.getFullTransactionByIndex(
              toNumber(req.params.index)
            )
            break
          case 'l2':
            transaction = await db.getUnconfirmedTransactionByIndex(
              toNumber(req.params.index)
            )
            break
          default:
            throw new Error(`Unknown transaction backend ${backend}`)
        }

        if (transaction === null) {
          return {
            transaction: null,
            batch: null,
          }
        }

        const batch = await db.getTransactionBatchByIndex(
          transaction.batchIndex
        )

        return {
          transaction,
          batch,
        }
      }
    )

    this._registerRoute(
      'get',
      '/batch/transaction/latest/:chainId',
      async (req): Promise<TransactionBatchResponse> => {
        const chainId = toNumber(req.params.chainId)
        const db = await this._getDb(chainId)
        const batch = await db.getLatestTransactionBatch()

        if (batch === null) {
          return {
            batch: null,
            transactions: [],
          }
        }

        if (this.useBatchInbox(batch.index)) {
          throw new Error('USE_INBOX_BATCH_INDEX')
        }

        const transactions = await db.getFullTransactionsByIndexRange(
          toNumber(batch.prevTotalElements),
          toNumber(batch.prevTotalElements) + toNumber(batch.size)
        )

        return {
          batch,
          transactions,
        }
      }
    )

    this._registerRoute(
      'get',
      '/batch/transaction/index/:index/:chainId',
      async (req): Promise<TransactionBatchResponse> => {
        const chainId = toNumber(req.params.chainId)
        const db = await this._getDb(chainId)

        if (this.useBatchInbox(toNumber(req.params.index))) {
          throw new Error('USE_INBOX_BATCH_INDEX')
        }

        const batch = await db.getTransactionBatchByIndex(
          toNumber(req.params.index)
        )

        if (batch === null) {
          return {
            batch: null,
            transactions: [],
          }
        }

        const transactions = await db.getFullTransactionsByIndexRange(
          toNumber(batch.prevTotalElements),
          toNumber(batch.prevTotalElements) + toNumber(batch.size)
        )

        return {
          batch,
          transactions,
        }
      }
    )

    this._registerRoute(
      'get',
      '/block/latest/:chainId',
      async (req): Promise<BlockResponse> => {
        const chainId = toNumber(req.params.chainId)
        const db = await this._getDb(chainId)

        const backend = req.query.backend || this.options.defaultBackend
        let block = null

        switch (backend) {
          case 'l1':
            block = await db.getLatestBlock()
            break
          case 'l2':
            block = await db.getLatestUnconfirmedBlock()
            break
          default:
            throw new Error(`Unknown transaction backend ${backend}`)
        }

        if (block === null) {
          return {
            block: null,
            batch: null,
          }
        }

        const batch = await db.getTransactionBatchByIndex(block.batchIndex)

        return {
          block,
          batch,
        }
      }
    )

    this._registerRoute(
      'get',
      '/block/index/:index/:chainId',
      async (req): Promise<BlockResponse> => {
        const chainId = toNumber(req.params.chainId)
        const db = await this._getDb(chainId)

        const backend = req.query.backend || this.options.defaultBackend
        let block = null

        switch (backend) {
          case 'l1':
            block = await db.getBlockByIndex(toNumber(req.params.index))
            break
          case 'l2':
            block = await db.getUnconfirmedBlockByIndex(
              toNumber(req.params.index)
            )
            break
          default:
            throw new Error(`Unknown block backend ${backend}`)
        }

        if (block === null) {
          return {
            block: null,
            batch: null,
          }
        }

        const batch = await db.getTransactionBatchByIndex(block.batchIndex)

        return {
          block,
          batch,
        }
      }
    )

    this._registerRoute(
      'get',
      '/batch/block/latest/:chainId',
      async (req): Promise<BlockBatchResponse> => {
        const chainId = toNumber(req.params.chainId)
        const db = await this._getDb(chainId)
        const batch = await db.getLatestTransactionBatch()

        if (batch === null) {
          return {
            batch: null,
            blocks: [],
          }
        }

        if (!this.useBatchInbox(batch.index)) {
          throw new Error('BELOW_INBOX_BATCH_INDEX')
        }

        const blocks = await db.getBlocksByIndexRange(
          toNumber(batch.prevTotalElements),
          toNumber(batch.prevTotalElements) + toNumber(batch.size)
        )

        return {
          batch,
          blocks,
        }
      }
    )

    this._registerRoute(
      'get',
      '/batch/block/index/:index/:chainId',
      async (req): Promise<BlockBatchResponse> => {
        const chainId = toNumber(req.params.chainId)
        const db = await this._getDb(chainId)

        if (!this.useBatchInbox(toNumber(req.params.index))) {
          throw new Error('BELOW_INBOX_BATCH_INDEX')
        }

        const batch = await db.getTransactionBatchByIndex(
          toNumber(req.params.index)
        )

        if (batch === null) {
          return {
            batch: null,
            blocks: [],
          }
        }

        const blocks = await db.getBlocksByIndexRange(
          toNumber(batch.prevTotalElements),
          toNumber(batch.prevTotalElements) + toNumber(batch.size)
        )

        return {
          batch,
          blocks,
        }
      }
    )

    this._registerRoute(
      'get',
      '/stateroot/latest/:chainId',
      async (req): Promise<StateRootResponse> => {
        const chainId = toNumber(req.params.chainId)
        const db = await this._getDb(chainId)

        const backend = req.query.backend || this.options.defaultBackend
        let stateRoot = null

        switch (backend) {
          case 'l1':
            stateRoot = await db.getLatestStateRoot()
            break
          case 'l2':
            stateRoot = await db.getLatestUnconfirmedStateRoot()
            break
          default:
            throw new Error(`Unknown transaction backend ${backend}`)
        }

        if (stateRoot === null) {
          return {
            stateRoot: null,
            batch: null,
          }
        }

        const batch = await db.getStateRootBatchByIndex(stateRoot.batchIndex)

        return {
          stateRoot,
          batch,
        }
      }
    )

    this._registerRoute(
      'get',
      '/stateroot/index/:index/:chainId',
      async (req): Promise<StateRootResponse> => {
        const chainId = toNumber(req.params.chainId)
        const db = await this._getDb(chainId)

        const backend = req.query.backend || this.options.defaultBackend
        let stateRoot = null

        switch (backend) {
          case 'l1':
            stateRoot = await db.getStateRootByIndex(toNumber(req.params.index))
            break
          case 'l2':
            stateRoot = await db.getUnconfirmedStateRootByIndex(
              toNumber(req.params.index)
            )
            break
          default:
            throw new Error(`Unknown transaction backend ${backend}`)
        }

        if (stateRoot === null) {
          return {
            stateRoot: null,
            batch: null,
          }
        }

        const batch = await db.getStateRootBatchByIndex(stateRoot.batchIndex)

        return {
          stateRoot,
          batch,
        }
      }
    )

    this._registerRoute(
      'get',
      '/batch/stateroot/latest/:chainId',
      async (req): Promise<StateRootBatchResponse> => {
        const chainId = toNumber(req.params.chainId)
        const db = await this._getDb(chainId)
        const batch = await db.getLatestStateRootBatch()

        if (batch === null) {
          return {
            batch: null,
            stateRoots: [],
          }
        }

        const stateRoots = await db.getStateRootsByIndexRange(
          toNumber(batch.prevTotalElements),
          toNumber(batch.prevTotalElements) + toNumber(batch.size)
        )

        return {
          batch,
          stateRoots,
        }
      }
    )

    this._registerRoute(
      'get',
      '/batch/stateroot/index/:index/:chainId',
      async (req): Promise<StateRootBatchResponse> => {
        const chainId = toNumber(req.params.chainId)
        const db = await this._getDb(chainId)
        const batch = await db.getStateRootBatchByIndex(
          toNumber(req.params.index)
        )

        if (batch === null) {
          return {
            batch: null,
            stateRoots: [],
          }
        }

        const stateRoots = await db.getStateRootsByIndexRange(
          toNumber(batch.prevTotalElements),
          toNumber(batch.prevTotalElements) + toNumber(batch.size)
        )

        return {
          batch,
          stateRoots,
        }
      }
    )

    this._registerRoute(
      'get',
      '/verifier/get/:success/:chainId',
      async (req): Promise<VerifierResultResponse> => {
        const chainId = toNumber(req.params.chainId)
        const success =
          req.params.success === 'true' || req.params.success === '1'
        const db = await this._getDb(chainId)
        const result = await db.getLastVerifierEntry(success)

        if (result === null) {
          return {
            verify: null,
            batch: null,
            stateRoots: [],
            success,
          }
        }

        const resp: VerifierResultResponse = {
          verify: result,
          batch: null,
          stateRoots: [],
          success,
        }

        const stateRoot = await db.getStateRootByIndex(result.index)
        if (stateRoot) {
          const batch = await db.getStateRootBatchByIndex(stateRoot.batchIndex)
          resp.batch = batch
          const stateRoots = await db.getStateRootsByIndexRange(
            toNumber(batch.prevTotalElements),
            toNumber(batch.prevTotalElements) + toNumber(batch.size)
          )
          const rootsArray: string[] = []
          if (stateRoots && stateRoots.length > 0) {
            stateRoots.forEach((v) => {
              if (v.index === result.index) {
                rootsArray.push(result.verifierRoot)
              } else {
                rootsArray.push(v.value)
              }
            })
          }
          resp.stateRoots = rootsArray
        }

        return resp
      }
    )

    this._registerRoute(
      'get',
      '/verifier/set/:success/:chainId/:index/:stateRoot/:verifierRoot',
      async (req): Promise<VerifierResultResponse> => {
        const chainId = toNumber(req.params.chainId)
        const success =
          req.params.success === 'true' || req.params.success === '1'
        const entry: VerifierResultEntry = {
          index: toNumber(req.params.index),
          stateRoot: req.params.stateRoot,
          verifierRoot: req.params.verifierRoot,
          timestamp: new Date().getTime(),
        }
        const db = await this._getDb(chainId)
        await db.putLastVerifierEntry(success, entry)

        return {
          verify: entry,
          batch: null,
          stateRoots: [],
          success,
        }
      }
    )

    this._registerRoute(
      'get',
      '/verifier/stake/latest/:chainId',
      async (req): Promise<VerifierStakeResponse> => {
        const chainId = toNumber(req.params.chainId)
        const db = await this._getDb(chainId)
        const result = await db.getLatestVerifierStake()

        if (result === null) {
          return {
            verifierStake: null,
          }
        }

        return {
          verifierStake: result,
        }
      }
    )

    this._registerRoute(
      'get',
      '/verifier/stake/index/:index/:chainId',
      async (req): Promise<VerifierStakeResponse> => {
        const chainId = toNumber(req.params.chainId)
        const index = toNumber(req.params.index)
        const db = await this._getDb(chainId)
        const result = await db.getVerifierStakeByIndex(index)

        if (result === null) {
          return {
            verifierStake: null,
          }
        }

        return {
          verifierStake: result,
        }
      }
    )

    this._registerRoute(
      'get',
      '/batch/element/index/:index/:chainId',
      async (req): Promise<AppendBatchElementResponse> => {
        const chainId = toNumber(req.params.chainId)
        const index = toNumber(req.params.index)
        const db = await this._getDb(chainId)
        const result = await db.getBatchElementByIndex(index)

        if (result === null) {
          return {
            batchElement: null,
          }
        }

        return {
          batchElement: result,
        }
      }
    )

    this._registerRoute(
      'get',
      '/rollup/safe-head/:chainId/:l1bock',
      async (req): Promise<L2BlockRef> => {
        const l1BlockNumber = toNumber(req.params.l1block)
        const chainId = toNumber(req.params.chainId)
        const [derivedFromL1Block, safeL2Block] =
          await this.state.db.getL2SafeHeadFromL1Block(l1BlockNumber, chainId)

        // get l1 block and l2 block async
        const derivedFromL1BlockPromise = this.state.l1RpcProvider.getBlock(
          `0x${derivedFromL1Block.toString(16)}`
        )
        const safeL2BlockPromise = this.state.l2RpcProvider.getBlock(
          `0x${safeL2Block.toString(16)}`
        )
        const [l1Block, l2Block] = await Promise.all<Block>([
          derivedFromL1BlockPromise,
          safeL2BlockPromise,
        ])

        return {
          hash: l2Block.hash,
          number: safeL2Block,
          parentHash: l2Block.parentHash,
          timestamp: l2Block.timestamp,
          l1origin: {
            hash: l1Block.hash,
            number: l1Block.number,
          },
          sequenceNumber: 0,
        }
      }
    )

    this._registerRoute(
      'get',
      '/rollup/l1origin/:chainId/:l2block',
      async (req): Promise<L1BlockRef> => {
        const l2BlockNumber = toNumber(req.params.l2block)
        const chainId = toNumber(req.params.chainId)
        const l1BlockNumber = await this.state.db.getL1OriginOfL2Block(
          l2BlockNumber,
          chainId
        )

        const l1Block = await this.state.l1RpcProvider.getBlock(
          `0x${l1BlockNumber.toString(16)}`
        )

        return {
          hash: l1Block.hash,
          number: l1Block.number,
          parentHash: l1Block.parentHash,
          time: l1Block.timestamp,
        }
      }
    )

    this._registerRoute(
      'get',
      '/rollup/sync-status/:chainId',
      async (req): Promise<SyncStatusResponse> => {
        const chainId = toNumber(req.params.chainId)

        // convert eth block to block ref
        const buildL1BlockRef = (block: Block): L1BlockRef => {
          return {
            hash: block.hash,
            number: block.number,
            parentHash: block.parentHash,
            time: block.timestamp,
          }
        }

        // retrieve required L1 blocks
        const lastDerivedL1Block = await this.state.db.getLastDerivedL1Block(
          chainId
        )

        const lastDerivedL1Promise = this.state.l1RpcProvider.getBlock(
          `0x${lastDerivedL1Block.toString(16)}`
        )

        // query required blocks from L1 asynchronously
        const headL1Promise = this.state.l1RpcProvider.getBlock('latest')
        const safeL1Promise = this.state.l1RpcProvider.getBlock('safe')
        const finalizedL1Promise =
          this.state.l1RpcProvider.getBlock('finalized')
        const [lastDerivedL1, headL1, safeL1, finalizedL1] = (
          await Promise.all<Block>([
            lastDerivedL1Promise,
            headL1Promise,
            safeL1Promise,
            finalizedL1Promise,
          ])
        ).map(buildL1BlockRef)

        // build L2 block ref from L2 block and L1 block ref
        const buildL2BlockRef = (
          l2Block: Block,
          l1Block: L1BlockRef | undefined
        ): L2BlockRef => {
          return {
            hash: l2Block.hash,
            number: l2Block.number,
            parentHash: l2Block.parentHash,
            timestamp: l2Block.timestamp,
            l1origin: !l1Block
              ? undefined
              : {
                  hash: l1Block.hash,
                  number: l1Block.number,
                },
            // TODO: currently we are not using this for FDG, leave this as 0 for now
            sequenceNumber: 0,
          }
        }

        // retrieve required L2 blocks
        const [, safeL2Block] = await this.state.db.getL2SafeHeadFromL1Block(
          lastDerivedL1.number,
          chainId
        )
        const safeL2 = buildL2BlockRef(
          await this.state.l2RpcProvider.getBlock(
            `0x${safeL2Block.toString(16)}`
          ),
          lastDerivedL1
        )
        const unsafeL2 = buildL2BlockRef(
          await this.state.l2RpcProvider.getBlock('latest'),
          undefined
        )

        // find the latest finalized L2 block
        const [derivedFromL1Block, finalizedL2Block] =
          await this.state.db.getL2SafeHeadFromL1Block(
            finalizedL1.number,
            chainId
          )
        const currentL1Finalized = buildL1BlockRef(
          await this.state.l1RpcProvider.getBlock(
            `0x${derivedFromL1Block.toString(16)}`
          )
        )
        const finalizedL2 = buildL2BlockRef(
          await this.state.l2RpcProvider.getBlock(
            `0x${finalizedL2Block.toString(16)}`
          ),
          currentL1Finalized
        )

        return {
          currentL1: lastDerivedL1,
          headL1,
          safeL1,
          finalizedL1,
          unsafeL2,
          safeL2,
          finalizedL2,
          // FIXME: use safe l2 as pending safe l2 for now,
          //  since currently we don't have a way to get pending safe l2.
          pendingSafeL2: safeL2,
        }
      }
    )
  }
}
