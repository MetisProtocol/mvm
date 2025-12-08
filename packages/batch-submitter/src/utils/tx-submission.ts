import { ethers, JsonRpcProvider, Provider, Signer, toBigInt } from 'ethersv6'

import { PendingRecordInfo } from '../storage/pending-storage'

export interface ResubmissionConfig {
  resubmissionTimeout: number
  minGasPriceInGwei: number
  maxGasPriceInGwei: number
  maxBlobGasPriceInGwei: number
  gasRetryIncrement: number
  numConfirmations: number
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
  tx: ethers.TransactionRequest,
  oldTx: PendingRecordInfo | null,
  l1Provider: Provider,
  resubmissionTimeout: number
): Promise<boolean> => {
  const feeData = await l1Provider.getFeeData()

  // Use 1Gwei as the tip fee for blob tx
  const newMaxFeePerGas =
    tx.type === 3
      ? feeData.maxFeePerGas * 2n + BigInt(1e9)
      : feeData.maxFeePerGas * 2n + BigInt(1e7)
  const newMaxPriorityFeePerGas =
    tx.type === 3 && feeData.maxPriorityFeePerGas < BigInt(1e9)
      ? BigInt(1e9)
      : feeData.maxPriorityFeePerGas < BigInt(1e7)
      ? BigInt(1e7)
      : feeData.maxPriorityFeePerGas

  const newBlobBaseFeePerGas =
    tx.type === 3 ? 2n * (await getBlobBaseFee(l1Provider)) : 0n

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
    const bumpThreshold = tx.type === 3 ? 100n : 11n
    const bumpedMaxFeePerGas =
      (toBigInt(oldTx.maxFeePerGas) * (100n + bumpThreshold)) / 100n
    const bumpedMaxPriorityFeePerGas =
      (toBigInt(oldTx.maxPriorityFeePerGas) * (100n + bumpThreshold)) / 100n

    tx.maxFeePerGas =
      bumpedMaxFeePerGas > newMaxFeePerGas
        ? bumpedMaxFeePerGas
        : newMaxFeePerGas
    tx.maxPriorityFeePerGas =
      bumpedMaxPriorityFeePerGas > newMaxPriorityFeePerGas
        ? bumpedMaxPriorityFeePerGas
        : newMaxPriorityFeePerGas
    if (tx.type === 3) {
      const bumpedMaxFeePerBlobGas = toBigInt(oldTx.maxFeePerBlobGas) * 2n
      const newMaxFeePerBlobGas = newBlobBaseFeePerGas
      tx.maxFeePerBlobGas =
        newMaxFeePerBlobGas > bumpedMaxFeePerBlobGas
          ? newMaxFeePerBlobGas
          : bumpedMaxFeePerBlobGas
    }
    return true
  }

  tx.maxFeePerGas = newMaxFeePerGas
  tx.maxPriorityFeePerGas = newMaxPriorityFeePerGas
  if (tx.type === 3) {
    tx.maxFeePerBlobGas = newBlobBaseFeePerGas
  }
  return false
}

const checkGasFee = (
  tx: ethers.TransactionRequest,
  config: ResubmissionConfig
) => {
  if (tx.gasPrice) {
    if (
      toBigInt(tx.gasPrice) >
      toBigInt(config.maxGasPriceInGwei) * BigInt(1e9)
    ) {
      throw new Error(
        `Gas price ${tx.gasPrice} exceeds the cap ${config.maxGasPriceInGwei}`
      )
    }

    if (
      config.minGasPriceInGwei &&
      toBigInt(tx.gasPrice) < toBigInt(config.minGasPriceInGwei) * BigInt(1e9)
    ) {
      throw new Error(
        `Gas price ${tx.gasPrice} is below the minimum ${config.minGasPriceInGwei}`
      )
    }
  }

  if (tx.maxFeePerGas) {
    if (
      toBigInt(tx.maxFeePerGas) >
      toBigInt(config.maxGasPriceInGwei) * BigInt(1e9)
    ) {
      throw new Error(
        `Gas price ${tx.maxFeePerGas} exceeds the cap ${config.maxGasPriceInGwei}`
      )
    }
    if (
      config.minGasPriceInGwei &&
      toBigInt(tx.maxFeePerGas) <
        toBigInt(config.minGasPriceInGwei) * BigInt(1e9)
    ) {
      throw new Error(
        `Gas price ${tx.maxFeePerGas} is below the minimum ${config.minGasPriceInGwei}`
      )
    }
  }

  if (
    tx.maxFeePerBlobGas &&
    toBigInt(tx.maxFeePerBlobGas) >
      toBigInt(config.maxBlobGasPriceInGwei) * BigInt(1e9)
  ) {
    throw new Error(
      `Blob gas price ${tx.maxFeePerBlobGas} exceeds the cap ${config.maxBlobGasPriceInGwei}`
    )
  }
}

// This function is used to validate the transaction fee before sending it, since MPC sign sometimes takes a long time,
// the signed tx may be sent after the base fee has already increased more than 2 times.
const validateTxFeeBeforeMPCSend = async (
  tx: ethers.TransactionRequest,
  l1Provider: Provider
): Promise<void> => {
  if (!tx.maxFeePerGas || !tx.maxPriorityFeePerGas) {
    return
  }

  const feeData = await l1Provider.getFeeData()

  // Assume the worst case scenario:
  // 1. Gas used in the n-th block is 100% of the gas limit
  // 2. We are sending a transaction in-between blocks, price fetched at block n, but tx send at block n+1
  // In this case, the base fee in the next block will be 12.5% higher than the base fee we fetched.
  // To avoid this situation, we need to make sure the tx's maxFeePerGas & maxFeePerBlobGas is at least 12.5%
  // (let's make it 13%, since we are doing int calc instead float) higher than the base fee we fetched.
  if (toBigInt(tx.maxFeePerGas) < (feeData.maxFeePerGas * 113n) / 100n) {
    throw new Error(
      `Transaction maxFeePerGas ${tx.maxFeePerGas} is lower than current maxFeePerGas ${feeData.maxFeePerGas}`
    )
  }

  if (toBigInt(tx.maxPriorityFeePerGas) < feeData.maxPriorityFeePerGas) {
    throw new Error(
      `Transaction maxPriorityFeePerGas ${tx.maxPriorityFeePerGas} is lower than current maxPriorityFeePerGas ${feeData.maxPriorityFeePerGas}`
    )
  }

  if (tx.maxFeePerBlobGas) {
    const blobBaseFee = await getBlobBaseFee(l1Provider)
    if (toBigInt(tx.maxFeePerBlobGas) < (blobBaseFee * 113n) / 100n) {
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

const submitTransactionWithYNATM = async (
  tx: ethers.TransactionRequest,
  signer: Signer,
  config: ResubmissionConfig,
  hooks: TxSubmissionHooks
): Promise<ethers.TransactionReceipt> => {
  const isEIP1559 =
    !!tx.maxFeePerGas || !!tx.maxPriorityFeePerGas || !!tx.maxFeePerBlobGas
  let fullTx: any
  const feeData = await signer.provider.getFeeData()
  // to be compatible with EIP-1559, we need to set the gasPrice to the maxPriorityFeePerGas
  if (isEIP1559) {
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
      gasPrice: feeData.gasPrice,
    }
  }

  checkGasFee(fullTx, config)
  await validateTxFeeBeforeMPCSend(tx, signer.provider)
  await hooks.beforeSendTransaction(fullTx)
  const txResponse = await signer.sendTransaction(fullTx)
  await hooks.onTransactionResponse(txResponse)
  const receipt = await signer.provider.waitForTransaction(
    txResponse.hash,
    config.numConfirmations,
    config.resubmissionTimeout
  )
  await hooks.onTxReceipt(receipt)
  return receipt
}

const submitSignedTransactionWithYNATM = async (
  tx: ethers.TransactionRequest,
  signFunction: () => Promise<string>,
  signer: Signer,
  config: ResubmissionConfig,
  hooks: TxSubmissionHooks
): Promise<ethers.TransactionReceipt> => {
  checkGasFee(tx, config)
  await validateTxFeeBeforeMPCSend(tx, signer.provider)
  await hooks.beforeSendTransaction(tx)
  const txResponse = await signer.provider.broadcastTransaction(
    await signFunction()
  )
  await hooks.onTransactionResponse(txResponse)
  const txReceipt = await signer.provider.waitForTransaction(
    txResponse.hash,
    config.numConfirmations,
    config.resubmissionTimeout
  )
  await hooks.onTxReceipt(txReceipt)
  return txReceipt
}

export interface TransactionSubmitter {
  submitTransaction(
    tx: ethers.TransactionRequest,
    hooks?: TxSubmissionHooks
  ): Promise<ethers.TransactionReceipt>

  submitSignedTransaction(
    tx: ethers.TransactionRequest,
    signFunction: () => Promise<string>,
    hooks?: TxSubmissionHooks
  ): Promise<ethers.TransactionReceipt>
}

export class YnatmTransactionSubmitter implements TransactionSubmitter {
  constructor(
    readonly signer: Signer,
    readonly ynatmConfig: ResubmissionConfig
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
    return submitTransactionWithYNATM(tx, this.signer, this.ynatmConfig, hooks)
  }

  public async submitSignedTransaction(
    tx: ethers.TransactionRequest,
    signFunction: () => Promise<string>,
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
      hooks
    )
  }
}
