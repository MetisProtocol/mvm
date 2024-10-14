import { L2Transaction } from '@metis.io/core-utils'

export class SingularBatch {
  constructor(
    public parentHash: string,
    public epochNum: number,
    public epochHash: string,
    public timestamp: number,
    public transactions: L2Transaction[]
  ) {}

  get batchType(): number {
    return 0 // SingularBatchType
  }

  epoch(): { hash: string; number: number } {
    return { hash: this.epochHash, number: this.epochNum }
  }
}
