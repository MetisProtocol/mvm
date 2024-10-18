import { expect } from 'chai'
import { toBigInt } from 'ethersv6'

interface deviationRanges {
  percentUpperDeviation?: number
  percentLowerDeviation?: number
  absoluteUpperDeviation?: number
  absoluteLowerDeviation?: number
}

const big100 = BigInt(100)

/**
 * Assert that a number lies within a custom defined range of the target.
 */
export const expectApprox = (
  actual: bigint | number,
  target: bigint | number,
  {
    percentUpperDeviation,
    percentLowerDeviation,
    absoluteUpperDeviation,
    absoluteLowerDeviation,
  }: deviationRanges
): void => {
  actual = toBigInt(actual)
  target = toBigInt(target)

  // Ensure at least one deviation parameter is defined
  const nonNullDeviations =
    percentUpperDeviation ||
    percentLowerDeviation ||
    absoluteUpperDeviation ||
    absoluteLowerDeviation
  if (!nonNullDeviations) {
    throw new Error(
      'Must define at least one parameter to limit the deviation of the actual value.'
    )
  }

  // Upper bound calculation.
  let upper: bigint
  // Set the two possible upper bounds if and only if they are defined.
  const upperPcnt: bigint = !percentUpperDeviation
    ? null
    : (target * toBigInt(100 + percentUpperDeviation)) / big100
  const upperAbs: bigint = !absoluteUpperDeviation
    ? null
    : target + toBigInt(absoluteUpperDeviation)

  if (upperPcnt && upperAbs) {
    // If both are set, take the lesser of the two upper bounds.
    upper = upperPcnt <= upperAbs ? upperPcnt : upperAbs
  } else {
    // Else take whichever is not undefined or set to null.
    upper = upperPcnt || upperAbs
  }

  // Lower bound calculation.
  let lower: bigint
  // Set the two possible lower bounds if and only if they are defined.
  const lowerPcnt: bigint = !percentLowerDeviation
    ? null
    : (target * toBigInt(100 - percentLowerDeviation)) / big100
  const lowerAbs: bigint = !absoluteLowerDeviation
    ? null
    : target - toBigInt(absoluteLowerDeviation)
  if (lowerPcnt && lowerAbs) {
    // If both are set, take the greater of the two lower bounds.
    lower = lowerPcnt >= lowerAbs ? lowerPcnt : lowerAbs
  } else {
    // Else take whichever is not undefined or set to null.
    lower = lowerPcnt || lowerAbs
  }

  // Apply the assertions if they are non-null.
  if (upper) {
    expect(
      actual <= upper,
      `Actual value (${actual}) is greater than the calculated upper bound of (${upper})`
    ).to.be.true
  }
  if (lower) {
    expect(
      actual >= lower,
      `Actual value (${actual}) is less than the calculated lower bound of (${lower})`
    ).to.be.true
  }
}
