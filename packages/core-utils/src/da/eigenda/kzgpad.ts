export const BYTES_PER_SYMBOL: number = 32

// ConvertByPaddingEmptyByte takes bytes and inserts an empty byte at the front of every 31 bytes.
// The empty byte is padded at the low address, because we use big endian to interpret a field element.
// This ensures every 32 bytes are within the valid range of a field element for the bn254 curve.
// If the input data is not a multiple of 31, the remainder is added to the output by
// inserting a 0 and the remainder. The output does not necessarily need to be a multiple of 32.
export const convertByPaddingEmptyByte: (data: Uint8Array) => Uint8Array = (
  data: Uint8Array
): Uint8Array => {
  const data_size: number = data.length
  const parse_size: number = BYTES_PER_SYMBOL - 1
  const put_size: number = BYTES_PER_SYMBOL
  const data_len: number = Math.ceil(data_size / parse_size)
  const valid_data: Uint8Array = new Uint8Array(data_len * put_size)
  let valid_end: number = valid_data.length

  for (let i = 0; i < data_len; i++) {
    const start: number = i * parse_size
    let end: number = start + parse_size

    if (end > data.length) {
      end = data.length
      valid_end = end - start + 1 + i * put_size
    }

    // With big endian, set the first byte to always be 0 to ensure data is within the valid range
    valid_data[i * BYTES_PER_SYMBOL] = 0x00
    valid_data.set(data.slice(start, end), i * BYTES_PER_SYMBOL + 1)
  }

  return valid_data.slice(0, valid_end)
}

// RemoveEmptyByteFromPaddedBytes takes bytes and removes the first byte from every 32 bytes.
// This reverses the change made by the function ConvertByPaddingEmptyByte.
// The function does not assume the input is a multiple of BYTES_PER_SYMBOL (32 bytes).
// For the remainder of the input, the first byte is taken out, and the rest is appended to
// the output.
export const removeEmptyByteFromPaddedBytes: (data: Uint8Array) => Uint8Array =
  (data: Uint8Array): Uint8Array => {
    const data_size: number = data.length
    const parse_size: number = BYTES_PER_SYMBOL
    const data_len: number = Math.ceil(data_size / parse_size)
    const put_size: number = BYTES_PER_SYMBOL - 1
    const valid_data: Uint8Array = new Uint8Array(data_len * put_size)
    let valid_len: number = valid_data.length

    for (let i = 0; i < data_len; i++) {
      // Add 1 to leave the first empty byte untouched
      const start: number = i * parse_size + 1
      let end: number = start + put_size

      if (end > data.length) {
        end = data.length
        valid_len = end - start + i * put_size
      }

      valid_data.set(data.slice(start, end), i * put_size)
    }

    return valid_data.slice(0, valid_len)
  }
