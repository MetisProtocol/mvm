import {
  ethers,
  JsonRpcProvider,
  Provider,
  Signer,
  toBigInt,
  toNumber,
} from 'ethersv6'
import * as ynatm from '@eth-optimism/ynatm'

import { YnatmAsync } from '../utils'
import { Logger } from '@eth-optimism/common-ts'
import { PendingRecordInfo } from '../storage/pending-storage'

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
  beforeSendTransaction: (tx: ethers.TransactionRequest) => Promise<void>
  onTransactionResponse: (
    txResponse: ethers.TransactionResponse
  ) => Promise<void>
  onTxReceipt: (receipt: ethers.TransactionReceipt) => Promise<void>
}

export const setTxEIP1559Fees = async (
  tx: any,
  oldTx: PendingRecordInfo | null,
  l1Provider: Provider,
  resubmissionTimeout: number,
  blobTx: boolean = false
): Promise<void> => {
  const feeData = await l1Provider.getFeeData()
  // check if pending tx exists and has not been confirmed yet,
  // also need to check if the resubmission timeout has passed,
  // will only bump the fees if the timeout has passed
  if (
    oldTx &&
    Date.now() - oldTx.submissionTime > resubmissionTimeout &&
    oldTx.nonce === tx.nonce &&
    !(await l1Provider.getTransactionReceipt(oldTx.txHash))
  ) {
    // pending tx exists, need to bump
    // for blob tx we need to double all fees,
    // for non-blob tx we need to bump maxFeePerGas and maxPriorityFeePerGas by 11% (using 11% instead of 10% to avoid rounding issues).
    const bumpThreshold = blobTx ? 100n : 11n
    const bumpedMaxFeePerGas =
      (toBigInt(oldTx.maxFeePerGas) * (100n + bumpThreshold)) / 100n
    const bumpedMaxPriorityFeePerGas =
      (toBigInt(oldTx.maxPriorityFeePerGas) * (100n + bumpThreshold)) / 100n
    const newMaxFeePerGas = feeData.maxFeePerGas * 2n

    tx.maxFeePerGas =
      bumpedMaxFeePerGas > newMaxFeePerGas
        ? bumpedMaxFeePerGas
        : newMaxFeePerGas
    tx.maxPriorityFeePerGas =
      bumpedMaxPriorityFeePerGas > feeData.maxPriorityFeePerGas
        ? bumpedMaxPriorityFeePerGas
        : feeData.maxPriorityFeePerGas
    if (blobTx) {
      const bumpedMaxFeePerBlobGas = toBigInt(oldTx.maxFeePerBlobGas) * 2n
      const newMaxFeePerBlobGas = (await getBlobBaseFee(l1Provider)) * 2n
      tx.maxFeePerBlobGas =
        newMaxFeePerBlobGas > bumpedMaxFeePerBlobGas
          ? newMaxFeePerBlobGas
          : bumpedMaxFeePerBlobGas
    }
  } else {
    tx.maxFeePerGas = feeData.maxFeePerGas * 2n
    tx.maxPriorityFeePerGas = feeData.maxPriorityFeePerGas
    if (blobTx) {
      tx.maxFeePerBlobGas = (await getBlobBaseFee(l1Provider)) * 2n
    }
  }
}

export const checkGasFee = (
  logger: Logger,
  transactionSubmitter: any,
  tx: any
) => {
  const yntmSubmmiter = transactionSubmitter as YnatmTransactionSubmitter

  const gasCapInWei = ethers.parseUnits(
    yntmSubmmiter.ynatmConfig.maxGasPriceInGwei.toString(10),
    'gwei'
  )
  if (toBigInt(tx.maxFeePerGas) > gasCapInWei) {
    logger.error('Gas price exceeds the cap', {
      max: gasCapInWei,
      current: toNumber(tx.maxFeePerGas),
    })

    throw new Error(
      `Gas price ${tx.maxFeePerGas} exceeds the cap ${yntmSubmmiter.ynatmConfig.maxGasPriceInGwei}`
    )
  }
}

// This function is used to validate the transaction fee before sending it, since MPC sign sometimes takes a long time,
// the signed tx may be sent after the base fee has already increased more than 2 times.
export const validateTxFeeBeforeMPCSend = async (
  tx: any,
  l1Provider: Provider
): Promise<void> => {
  if (!tx.maxFeePerGas || !tx.maxPriorityFeePerGas) {
    throw new Error(
      "Transaction doesn't have maxFeePerGas or maxPriorityFeePerGas"
    )
  }

  const feeData = await l1Provider.getFeeData()

  // Assume the worst case scenario:
  // 1. Gas used in the n-th block is 100% of the gas limit
  // 2. We are sending a transaction in-between blocks, price fetched at block n, but tx send at block n+1
  // In this case, the base fee in the next block will be 12.5% higher than the base fee we fetched.
  // To avoid this situation, we need to make sure the tx's maxFeePerGas & maxFeePerBlobGas is at least 12.5%
  // (let's make it 13%, since we are doing int calc instead float) higher than the base fee we fetched.
  if (tx.maxFeePerGas < (feeData.maxFeePerGas * 113n) / 100n) {
    throw new Error(
      `Transaction maxFeePerGas ${tx.maxFeePerGas} is lower than current maxFeePerGas ${feeData.maxFeePerGas}`
    )
  }

  if (tx.maxPriorityFeePerGas < feeData.maxPriorityFeePerGas) {
    throw new Error(
      `Transaction maxPriorityFeePerGas ${tx.maxPriorityFeePerGas} is lower than current maxPriorityFeePerGas ${feeData.maxPriorityFeePerGas}`
    )
  }

  if (tx.maxFeePerBlobGas) {
    const blobBaseFee = await getBlobBaseFee(l1Provider)
    if (tx.maxFeePerBlobGas < (blobBaseFee * 113n) / 100n) {
      throw new Error(
        `Transaction maxFeePerBlobGas ${tx.maxFeePerBlobGas} is lower than current blob base fee ${blobBaseFee}`
      )
    }
  }
}

export const getBlobBaseFee = async (l1Provider: Provider): Promise<bigint> => {
  return toBigInt(
    await (l1Provider as JsonRpcProvider).send('eth_blobBaseFee', [])
  )
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
      const feeData = await signer.provider.getFeeData()
      fullTx = {
        ...tx,
        maxFeePerGas: feeData.maxFeePerGas,
        maxPriorityFeePerGas: feeData.maxPriorityFeePerGas,
      }
    } else {
      fullTx = {
        ...tx,
        // in some cases (mostly local testing env) gas price is lower than 1 gwei,
        // so we need to replace it to the current gas price
        gasPrice: gasPrice || (await signer.provider.getFeeData()).gasPrice,
      }
    }

    await hooks.beforeSendTransaction(fullTx)
    try {
      const txResponse = await signer.sendTransaction(fullTx)
      await hooks.onTransactionResponse(txResponse)
      const receipt = await signer.provider.waitForTransaction(
        txResponse.hash,
        numConfirmations
      )
      await hooks.onTxReceipt(receipt)
      return receipt
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
        await hooks.beforeSendTransaction(tx)
        const txResponse = await signer.provider.broadcastTransaction(signedTx)
        await hooks.onTransactionResponse(txResponse)
        const txReceipt = await signer.provider.waitForTransaction(
          txResponse.hash,
          numConfirmations
        )
        await hooks.onTxReceipt(txReceipt)
        return txReceipt
      } catch (e) {
        console.error('Error sending transaction:', e.message.substring(0, 200))
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
        onTxReceipt: () => undefined,
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
        onTxReceipt: () => undefined,
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
