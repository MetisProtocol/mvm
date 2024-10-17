// Constants
const BlobTxMinBlobGasprice = 1n
const BlobTxBlobGaspriceUpdateFraction = 3338477n

const minBlobGasPrice = BlobTxMinBlobGasprice
const blobGaspriceUpdateFraction = BlobTxBlobGaspriceUpdateFraction

// CalcBlobFee calculates the blobfee from the header's excess blob gas field.
export const calcBlobFee = (excessBlobGas: bigint): bigint =>
  fakeExponential(minBlobGasPrice, excessBlobGas, blobGaspriceUpdateFraction)

// fakeExponential approximates factor * e ** (numerator / denominator) using
// Taylor expansion.
export const fakeExponential = (
  factor: bigint,
  numerator: bigint,
  denominator: bigint
): bigint => {
  let output = 0n
  let accum = factor * denominator

  for (let i = 1n; accum > 0n; i++) {
    output += accum

    accum = (accum * numerator) / denominator / i
  }

  return output / denominator
}
