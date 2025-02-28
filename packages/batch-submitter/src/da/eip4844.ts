// Constants
const BlobTxMinBlobGasprice = BigInt(1)
const minBlobGasPrice = BlobTxMinBlobGasprice

const fractions = {
  cancun: BigInt(3338477),
  pectra: BigInt(5007716),
}

// CalcBlobFee calculates the blobfee from the header's excess blob gas field.
export const calcBlobFee = (
  excessBlobGas: bigint,
  fraction: 'cancun' | 'pectra'
): bigint =>
  fakeExponential(minBlobGasPrice, excessBlobGas, fractions[fraction])

// fakeExponential approximates factor * e ** (numerator / denominator) using
// Taylor expansion.
export const fakeExponential = (
  factor: bigint,
  numerator: bigint,
  denominator: bigint
): bigint => {
  let output = BigInt(0)
  let accum = factor * denominator

  for (let i = BigInt(1); accum > BigInt(0); i++) {
    output += accum

    accum = (accum * numerator) / denominator / i
  }

  return output / denominator
}
