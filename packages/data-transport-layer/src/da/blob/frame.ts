export interface Frame {
  id: Uint8Array // channel id
  frameNumber: number // frame index, starts from 0
  data: Uint8Array // frame data
  isLast: boolean // true if this is the last frame in the blob
  inclusionBlock: number // l1 block number that included the frame
}

const readUint16BE = (data: Uint8Array, offset: number): number => {
  // use multiplication instead of bit shifting to avoid different result with go
  return data[offset] * 256 + data[offset + 1]
}

const readUint32BE = (data: Uint8Array, offset: number): number => {
  // use multiplication instead of bit shifting to avoid different result with go
  return (
    data[offset] * 16777216 +
    data[offset + 1] * 65536 +
    data[offset + 2] * 256 +
    data[offset + 3]
  )
}

export const parseFrames = (
  data: Uint8Array,
  inclusionBlock: number
): Frame[] => {
  if (data.length === 0) {
    throw new Error('Data array must not be empty')
  }
  // DerivationVersion0
  if (data[0] !== 0) {
    throw new Error(`Invalid derivation format byte: got ${data[0]}`)
  }
  const frames: Frame[] = []
  let offset = 1

  while (offset < data.length) {
    if (offset + 16 > data.length) {
      throw new Error('Unexpected end of data while reading channel_id')
    }
    const id = data.slice(offset, offset + 16)
    offset += 16

    if (offset + 2 > data.length) {
      throw new Error('Unexpected end of data while reading frame_number')
    }
    const frameNumber = readUint16BE(data, offset)
    offset += 2

    if (offset + 4 > data.length) {
      throw new Error('Unexpected end of data while reading frame_data_length')
    }
    const frameDataLength = readUint32BE(data, offset)
    offset += 4

    if (frameDataLength > 1_000_000) {
      throw new Error(`frame_data_length is too large: ${frameDataLength}`)
    }

    if (offset + frameDataLength > data.length) {
      throw new Error('Unexpected end of data while reading frame_data')
    }
    const frameData = data.slice(offset, offset + frameDataLength)
    offset += frameDataLength

    if (offset >= data.length) {
      throw new Error('Unexpected end of data while reading is_last byte')
    }
    const isLastByte = data[offset]
    let isLast: boolean
    if (isLastByte === 0) {
      isLast = false
    } else if (isLastByte === 1) {
      isLast = true
    } else {
      throw new Error('Invalid byte as is_last')
    }
    offset += 1

    frames.push({ id, frameNumber, data: frameData, isLast, inclusionBlock })
  }

  if (frames.length === 0) {
    throw new Error('Was not able to find any frames')
  }

  return frames
}
