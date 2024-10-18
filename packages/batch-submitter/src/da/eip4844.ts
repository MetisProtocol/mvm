// Constants
const BlobTxMinBlobGasprice = BigInt(1)
const BlobTxBlobGaspriceUpdateFraction = BigInt(3338477)

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
  let output = BigInt(0)
  let accum = factor * denominator

  for (let i = BigInt(1); accum > BigInt(0); i++) {
    output += accum

    accum = (accum * numerator) / denominator / i
  }

  return output / denominator
}
