import { L2Transaction } from '@localtest911/core-utils'

export class SingularBatch {
  constructor(
    public blockNumber: number,
    public parentHash: string,
    public epochNum: number,
    public epochHash: string,
    public timestamp: number,
    public transactions: L2Transaction[]
  ) {}

  static batchType(): number {
    return 0 // SingularBatchType
  }

  epoch(): { hash: string; number: number } {
    return { hash: this.epochHash, number: this.epochNum }
  }
}
