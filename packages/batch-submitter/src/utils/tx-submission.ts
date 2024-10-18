import { ethers, Signer } from 'ethersv6'
import * as ynatm from '@eth-optimism/ynatm'

import { YnatmAsync } from '../utils'

export interface ResubmissionConfig {
  resubmissionTimeout: number
  minGasPriceInGwei: number
  maxGasPriceInGwei: number
  gasRetryIncrement: number
}

export type SubmitTransactionFn = (
  tx: ethers.TransactionRequest
) => Promise<ethers.TransactionReceipt>

export interface TxSubmissionHooks {
  beforeSendTransaction: (tx: ethers.TransactionRequest) => void
  onTransactionResponse: (txResponse: ethers.TransactionResponse) => void
}

const getGasPriceInGwei = async (signer: Signer): Promise<number> => {
  return parseInt(
    ethers.formatUnits((await signer.provider.getFeeData()).gasPrice, 'gwei'),
    10
  )
}

export const submitTransactionWithYNATM = async (
  tx: ethers.TransactionRequest,
  signer: Signer,
  config: ResubmissionConfig,
  numConfirmations: number,
  hooks: TxSubmissionHooks
): Promise<ethers.TransactionReceipt> => {
  const sendTxAndWaitForReceipt = async (
    gasPrice
  ): Promise<ethers.TransactionReceipt> => {
    const fullTx = {
      ...tx,
      gasPrice,
    }
    hooks.beforeSendTransaction(fullTx)
    const txResponse = await signer.sendTransaction(fullTx)
    hooks.onTransactionResponse(txResponse)
    return signer.provider.waitForTransaction(txResponse.hash, numConfirmations)
  }

  const minGasPrice = await getGasPriceInGwei(signer)
  const receipt = await ynatm.send({
    sendTransactionFunction: sendTxAndWaitForReceipt,
    minGasPrice: ynatm.toGwei(minGasPrice),
    maxGasPrice: ynatm.toGwei(config.maxGasPriceInGwei),
    gasPriceScalingFunction: ynatm.LINEAR(config.gasRetryIncrement),
    delay: config.resubmissionTimeout,
  })
  return receipt
}

export const submitSignedTransactionWithYNATM = async (
  tx: ethers.TransactionRequest,
  signFunction: Function,
  signer: Signer,
  config: ResubmissionConfig,
  numConfirmations: number,
  hooks: TxSubmissionHooks
): Promise<ethers.TransactionReceipt> => {
  const sendTxAndWaitForReceipt = async (
    signedTx
  ): Promise<ethers.TransactionReceipt> => {
    hooks.beforeSendTransaction(tx)
    const txResponse = await signer.sendTransaction(signedTx)
    hooks.onTransactionResponse(txResponse)
    return signer.provider.waitForTransaction(txResponse.hash, numConfirmations)
  }

  const ynatmAsync = new YnatmAsync()
  const minGasPrice = await getGasPriceInGwei(signer)
  const receipt = await ynatmAsync.sendAfterSign({
    sendSignedTransactionFunction: sendTxAndWaitForReceipt,
    signFunction,
    minGasPrice: ynatmAsync.toGwei(minGasPrice),
    maxGasPrice: ynatmAsync.toGwei(config.maxGasPriceInGwei),
    gasPriceScalingFunction: ynatm.LINEAR(config.gasRetryIncrement),
    delay: config.resubmissionTimeout,
  })
  return receipt
}

export interface TransactionSubmitter {
  submitTransaction(
    tx: ethers.TransactionRequest,
    hooks?: TxSubmissionHooks
  ): Promise<ethers.TransactionReceipt>

  submitSignedTransaction(
    tx: ethers.TransactionRequest,
    signFunction: Function,
    hooks?: TxSubmissionHooks
  ): Promise<ethers.TransactionReceipt>
}

export class YnatmTransactionSubmitter implements TransactionSubmitter {
  constructor(
    readonly signer: Signer,
    readonly ynatmConfig: ResubmissionConfig,
    readonly numConfirmations: number
  ) {}

  public async submitTransaction(
    tx: ethers.TransactionRequest,
    hooks?: TxSubmissionHooks
  ): Promise<ethers.TransactionReceipt> {
    if (!hooks) {
      hooks = {
        beforeSendTransaction: () => undefined,
        onTransactionResponse: () => undefined,
      }
    }
    return submitTransactionWithYNATM(
      tx,
      this.signer,
      this.ynatmConfig,
      this.numConfirmations,
      hooks
    )
  }

  public async submitSignedTransaction(
    tx: ethers.TransactionRequest,
    signFunction: Function,
    hooks?: TxSubmissionHooks
  ): Promise<ethers.TransactionReceipt> {
    if (!hooks) {
      hooks = {
        beforeSendTransaction: () => undefined,
        onTransactionResponse: () => undefined,
      }
    }
    return submitSignedTransactionWithYNATM(
      tx,
      signFunction,
      this.signer,
      this.ynatmConfig,
      this.numConfirmations,
      hooks
    )
  }
}
