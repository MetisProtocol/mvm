// Define ChannelID type
type ChannelID = Uint8Array; // 16-byte array

// Frame interface
export interface Frame {
  id: ChannelID;
  frameNumber: number; // uint16
  data: Uint8Array;
  isLast: boolean;
}

export interface Blob {
  data: Uint8Array; // Stores the blob data
}

export interface BlobWithMetadata {
  txHash: string; // Transaction hash (hex string)
  inclusionBlock: number; // Block number
  timestamp: number;
  blockHash: string; // Block hash (hex string)
  blob: Blob;
}

// FrameWithMetadata interface
export interface FrameWithMetadata {
  txHash: string; // Transaction hash (hex string)
  inclusionBlock: number; // Block number
  timestamp: number;
  blockHash: string; // Block hash (hex string)
  frame: Frame;
}

// Batch interfaces
export interface SingularBatch {
  batchType: BatchType.SingularBatchType;
  parentHash: string;
  epochNumber: string;
  epochHash: string;
  timestamp: string;
  transactions: Uint8Array[]; // List of transactions
}

export interface SpanBatchElement {
  epochNumber: string;
  timestamp: string;
  transactions: Uint8Array[];
}

export interface SpanBatch {
  batchType: BatchType.SpanBatchType;
  parentCheck: string;       // First 20 bytes of the first block's parent hash
  l1OriginCheck: string;     // First 20 bytes of the last block's L1 origin hash
  batches: SpanBatchElement[];
}


// ChannelWithMetadata interface
export interface ChannelWithMetadata {
  id: ChannelID;
  isReady: boolean;
  invalidFrames: boolean;
  invalidBatches: boolean;
  frames: FrameWithMetadata[];
  batches: (SingularBatch | SpanBatch) [];
  batchTypes: number[];
  comprAlgos: CompressionAlgo[];
}

// Compression algorithm enum
export enum CompressionAlgo {
  None = 0,
  Brotli = 1,
  ZlibCM8 = 8,
  ZlibCM15 = 15,
}

// BatchType enum
export enum BatchType {
  SingularBatchType = 0,
  SpanBatchType = 1,
}
