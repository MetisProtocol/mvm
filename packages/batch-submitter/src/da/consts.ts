export const FRAME_OVERHEAD_SIZE = 200
export const MAX_RLP_BYTES_PER_CHANNEL = 100_000_000
export const MAX_BLOB_SIZE = (4 * 31 + 3) * 1024 - 4
export const MAX_BLOB_NUM_PER_TX = 6
export const TX_GAS = 21_000
export const CHANNEL_FULL_ERR = new Error('Channel is full')
