// Only load if not in browser.
import { isNode } from 'browser-or-node'

// eslint-disable-next-line no-var
declare var window: any

const fetch = isNode ? require('node-fetch') : window.fetch

import {
  AppendBatchElementResponse,
  EnqueueResponse,
  StateRootBatchResponse,
  StateRootResponse,
  SyncingResponse,
  TransactionBatchResponse,
  TransactionResponse,
  BlockBatchResponse,
  BlockResponse,
  VerifierResultResponse,
  VerifierStakeResponse,
} from '../types'

export class L1DataTransportClient {
  public _chainId: number
  constructor(private url: string) {
    this._chainId = 0
  }

  public setChainId(chainId: number) {
    this._chainId = chainId
  }

  public async syncing(): Promise<SyncingResponse> {
    return this._get(`/eth/syncing/${this._chainId}`)
  }

  public async highestL1(): Promise<SyncingResponse> {
    return this._get(`/highest/l1`)
  }

  public async getEnqueueByIndex(index: number): Promise<EnqueueResponse> {
    return this._get(`/enqueue/index/${index}/${this._chainId}`)
  }

  public async getLatestEnqueue(): Promise<EnqueueResponse> {
    return this._get(`/enqueue/latest/${this._chainId}`)
  }

  public async getTransactionByIndex(
    index: number
  ): Promise<TransactionResponse> {
    return this._get(`/transaction/index/${index}/${this._chainId}`)
  }

  public async getLatestTransacton(): Promise<TransactionResponse> {
    return this._get(`/transaction/latest/${this._chainId}`)
  }

  public async getTransactionBatchByIndex(
    index: number
  ): Promise<TransactionBatchResponse> {
    return this._get(`/batch/transaction/index/${index}/${this._chainId}`)
  }

  public async getLatestTransactionBatch(): Promise<TransactionBatchResponse> {
    return this._get(`/batch/transaction/latest/${this._chainId}`)
  }

  public async getBlockByIndex(index: number): Promise<BlockResponse> {
    return this._get(`/block/index/${index}/${this._chainId}`)
  }

  public async getLatestBlock(): Promise<BlockResponse> {
    return this._get(`/block/latest/${this._chainId}`)
  }

  public async getBlockBatchByIndex(
    index: number
  ): Promise<BlockBatchResponse> {
    return this._get(`/batch/block/index/${index}/${this._chainId}`)
  }

  public async getLatestBlockBatch(): Promise<BlockBatchResponse> {
    return this._get(`/batch/block/latest/${this._chainId}`)
  }

  public async getStateRootByIndex(index: number): Promise<StateRootResponse> {
    return this._get(`/stateroot/index/${index}/${this._chainId}`)
  }

  public async getLatestStateRoot(): Promise<StateRootResponse> {
    return this._get(`/stateroot/latest/${this._chainId}`)
  }

  public async getStateRootBatchByIndex(
    index: number
  ): Promise<StateRootBatchResponse> {
    return this._get(`/batch/stateroot/index/${index}/${this._chainId}`)
  }

  public async getLatestStateRootBatch(): Promise<StateRootBatchResponse> {
    return this._get(`/batch/stateroot/latest/${this._chainId}`)
  }

  public async getLatestVerifierResult(
    success: boolean
  ): Promise<VerifierResultResponse> {
    return this._get(`/verifier/get/${success}/${this._chainId}`)
  }

  public async setLatestVerifierResult(
    success: boolean,
    index: number,
    stateRoot: string,
    verifierRoot: string
  ): Promise<VerifierResultResponse> {
    return this._get(
      `/verifier/set/${success}/${this._chainId}/${index}/${stateRoot}/${verifierRoot}`
    )
  }

  public async getLatestVerifierStake(): Promise<VerifierStakeResponse> {
    return this._get(`/verifier/stake/latest/${this._chainId}`)
  }

  public async getVerifierStakeByIndex(
    index: number
  ): Promise<VerifierStakeResponse> {
    return this._get(`/verifier/stake/index/${index}/${this._chainId}`)
  }

  public async getBatchElementByIndex(
    index: number
  ): Promise<AppendBatchElementResponse> {
    return this._get(`/batch/element/index/${index}/${this._chainId}`)
  }

  private async _get<TResponse>(endpoint: string): Promise<TResponse> {
    return (await fetch(`${this.url}${endpoint}`)).json()
  }
}
