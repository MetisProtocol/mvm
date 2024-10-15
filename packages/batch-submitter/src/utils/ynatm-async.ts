export class YnatmAsync {
  // GWEI = 1e9
  private GWEI = Math.pow(10, 9)
  private MAX_INT32 = ~(1 << 31)

  toGwei = (x) => x * this.GWEI

  EXPONENTIAL =
    (base = 2, inGwei = true) =>
    ({ x }) => {
      let p = Math.pow(base, x)
      if (inGwei) {
        p = this.toGwei(p)
      }
      return x + p
    }

  LINEAR =
    (slope = 1, inGwei = true) =>
    ({ x, c }) => {
      let p = slope * x
      if (inGwei) {
        p = this.toGwei(p)
      }
      return c + p
    }

  DOUBLES = ({ y }) => {
    return y * 2
  }

  // The default behaviour of an overflow of the timeout value
  // passed to `setTimeout` will result it being set to 1.
  sanitizeTimeout = (timeout) => {
    if (timeout > this.MAX_INT32) {
      console.log(
        `WARNING: Timeout larger than max supported timeout size.
                    ${timeout} set to ${this.MAX_INT32}.
          `
      )
      return this.MAX_INT32
    }
    return timeout
  }

  // Returns a list of gasPrices, based on the scaling function
  getGasPriceVariations = ({
    minGasPrice,
    maxGasPrice,
    gasPriceScalingFunction,
  }) => {
    // Calculates a sequence of gasPrices
    let i = 0
    let curGasPrice = minGasPrice
    let gasPrices = []

    // Warning for the user on their gasPrice if their first
    // Increment is < 1e-6 (because of the GWEI conversion)
    const firstGasPriceDelta =
      gasPriceScalingFunction(minGasPrice, 1) - minGasPrice
    if (firstGasPriceDelta / minGasPrice < 1e-6) {
      console.log(
        `WARNING: GasPrice is scaling very slowly. Might take a while.
                Double check the supplied gasPriceScalingFunction.
                If you're using a custom function, make sure to use toGwei.
      `
      )
    }

    for (;;) {
      if (curGasPrice > maxGasPrice) {
        break
      }
      gasPrices = gasPrices.concat(curGasPrice)
      curGasPrice = gasPriceScalingFunction({
        y: curGasPrice,
        x: ++i,
        c: minGasPrice,
      })
    }

    return gasPrices
  }

  // Immediately rejects the promise if it contains the "revert" keyword
  rejectOnRevert = (e) => {
    return e.toString().toLowerCase().includes('revert')
  }

  /**
   * Gradually keeps trying a transaction with an incremental amount of gas
   * while keeping the same nonce.
   *
   * @param {Function} sendSignedTransactionFunction:
   *   Function that accepts a signedTx, and uses that to send directly
   * @param {Function} signFunction
   *   Function that accepts a gasPrice, and uses that gasPrice to sign a tx
   *   e.g. (gasPrice) => wallet.sendTranscation({ ...tx, gasPrice })
   *        (gasPrice) => web3.eth.sendTransaction(tx, { from: sender, gasPrice })
   * @param {number} minGasPrice:
   *   Minimum gasPrice to start with
   * @param {number} masGasPrice:
   *   Maximum allowed gasPrice
   * @param {number} delay:
   *   Delay before retrying transaction with a higher gasPrice (ms)
   * @param {Function} rejectImmediatelyOnCondition:
   *   If an error occurs and matches some condition. Throws the error immediately
   *   and stops attempting to retry the proceeding transactions.
   *   By default, it'll stop immediately stop if the error contains the string "revert"
   */
  sendAfterSign = async ({
    sendSignedTransactionFunction,
    signFunction,
    minGasPrice,
    maxGasPrice,
    gasPriceScalingFunction = this.LINEAR(5),
    delay = 60000,
    rejectImmediatelyOnCondition = this.rejectOnRevert,
  }): Promise<any> => {
    // Make sure its an int
    minGasPrice = parseInt(minGasPrice)

    // Defaults to 2x minGasPrice
    if (!maxGasPrice) {
      maxGasPrice = 2 * minGasPrice
    } else {
      maxGasPrice = parseInt(maxGasPrice)
    }

    // List of varying gasPrices
    const gasPrices = this.getGasPriceVariations({
      minGasPrice,
      maxGasPrice,
      gasPriceScalingFunction,
    })

    const promise = new Promise<any>((resolve, reject) => {
      // List of timeout Ids
      const timeoutIds = []
      const failedTxs = []

      // After waiting (N + 1) * delay seconds, throw an error
      const finalTimeoutId = setTimeout(() => {
        reject(new Error('Transaction taking too long!'))
      }, this.sanitizeTimeout((gasPrices.length + 1) * delay))
      timeoutIds.push(finalTimeoutId)

      // For each signed transactions
      for (const [i, gasPrice] of gasPrices.entries()) {
        // Async function to wait for transaction
        const waitForTx = async () => {
          try {
            const signedTx = await signFunction(gasPrice)
            const tx = await sendSignedTransactionFunction(signedTx)

            // Clear other timeouts
            for (const tid of timeoutIds) {
              clearTimeout(tid)
            }

            resolve(tx)
          } catch (e) {
            failedTxs.push(e)

            // Reject if either we have retried all possible gasPrices
            // Or if some condition is met
            if (
              failedTxs.length >= gasPrices.length ||
              rejectImmediatelyOnCondition(e)
            ) {
              for (const tid of timeoutIds) {
                clearTimeout(tid)
              }
              reject(e)
            }
          }
        }

        // Attempt to send the signed transaction after <x> delay
        const timeoutId = setTimeout(waitForTx, this.sanitizeTimeout(i * delay))
        timeoutIds.push(timeoutId)
      }
    })

    return promise
  }
}
