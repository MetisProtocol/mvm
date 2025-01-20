import { ethers, Signer, toBigInt, toNumber } from 'ethersv6'
import * as ynatm from '@eth-optimism/ynatm'

import { YnatmAsync } from '../utils'
import { calcBlobFee } from '../da/eip4844'
import { ceil } from 'lodash'

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

const getGasPriceInWei = async (signer: Signer): Promise<number> => {
  return toNumber((await signer.provider.getFeeData()).gasPrice)
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
    const isEIP1559 =
      !!tx.maxFeePerGas || !!tx.maxPriorityFeePerGas || !!tx.maxFeePerBlobGas
    let fullTx: any
    if (isEIP1559) {
      // to be compatible with EIP-1559, we need to set the gasPrice to the maxPriorityFeePerGas
      const feeScalingFactor = gasPrice
        ? gasPrice / toNumber(tx.maxFeePerGas)
        : 1
      await calculateEIP1559GasPrice(signer.provider, tx, feeScalingFactor)
      fullTx = tx
    } else {
      fullTx = {
        ...tx,
        // in some cases (mostly local testing env) gas price is lower than 1 gwei,
        // so we need to replace it to the current gas price
        gasPrice: gasPrice || (await signer.provider.getFeeData()).gasPrice,
      }
    }

    hooks.beforeSendTransaction(fullTx)
    try {
      const txResponse = await signer.sendTransaction(fullTx)
      hooks.onTransactionResponse(txResponse)
      return signer.provider.waitForTransaction(
        txResponse.hash,
        numConfirmations
      )
    } catch (err) {
      console.error('Error sending transaction:', err)
      throw err
    }
  }

  try {
    const receipt = await ynatm.send({
      sendTransactionFunction: sendTxAndWaitForReceipt,
      minGasPrice: await getGasPriceInWei(signer),
      maxGasPrice: ynatm.toGwei(config.maxGasPriceInGwei),
      gasPriceScalingFunction: ynatm.LINEAR(config.gasRetryIncrement),
      delay: config.resubmissionTimeout,
    })
    return receipt
  } catch (err) {
    console.error('Error submitting transaction:', err)
    throw err
  }
}

export const submitSignedTransactionWithYNATM = async (
  tx: ethers.TransactionRequest,
  signFunction: Function,
  signer: Signer,
  config: ResubmissionConfig,
  numConfirmations: number,
  hooks: TxSubmissionHooks
): Promise<ethers.TransactionReceipt> => {
  try {
    const sendTxAndWaitForReceipt = async (
      signedTx
    ): Promise<ethers.TransactionReceipt> => {
      try {
        hooks.beforeSendTransaction(tx)
        const txResponse = await signer.provider.broadcastTransaction(signedTx)
        hooks.onTransactionResponse(txResponse)
        return signer.provider.waitForTransaction(
          txResponse.hash,
          numConfirmations
        )
      } catch (e) {
        console.error('Error sending transaction:', e.message.substring(0, 100))
        throw e
      }
    }

    const ynatmAsync = new YnatmAsync()
    const receipt = await ynatmAsync.sendAfterSign({
      sendSignedTransactionFunction: sendTxAndWaitForReceipt,
      signFunction,
      minGasPrice: await getGasPriceInWei(signer),
      maxGasPrice: ynatmAsync.toGwei(config.maxGasPriceInGwei),
      gasPriceScalingFunction: ynatm.LINEAR(config.gasRetryIncrement),
      delay: config.resubmissionTimeout,
    })
    return receipt
  } catch (e) {
    console.error('Error submitting transaction:', e)
    throw e
  }
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

export const calculateEIP1559GasPrice = async (
  provider: ethers.Provider,
  tx: ethers.TransactionRequest,
  scalingFactor: number
) => {
  const latestFee = await provider.getFeeData()
  const baseFee = latestFee.maxFeePerGas - latestFee.maxPriorityFeePerGas

  // only scale the priority fee
  tx.maxPriorityFeePerGas = toBigInt(
    ceil(toNumber(tx.maxPriorityFeePerGas) * scalingFactor)
  )
  // use the latest base fee
  tx.maxFeePerGas = baseFee + tx.maxPriorityFeePerGas
  // scale the blob fee as well
  tx.maxFeePerBlobGas = toBigInt(
    ceil(toNumber(tx.maxFeePerBlobGas) * scalingFactor)
  )
}
