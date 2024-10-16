/* Imports: External */
import { LevelUp } from 'levelup'
import { toBigInt } from 'ethers'

export class SimpleDB {
  constructor(public db: LevelUp) {}

  public async get<TEntry>(key: string, index: number): Promise<TEntry | null> {
    try {
      // TODO: Better checks here.
      return JSON.parse(await this.db.get(this._makeKey(key, index)))
    } catch (err) {
      return null
    }
  }

  public async range<TEntry>(
    key: string,
    startIndex: number,
    endIndex: number,
    reverse?: boolean, // add this so we could retrieve the entries in reverse order
    peek?: boolean // add this so we could just peek the first entry
  ): Promise<TEntry[] | []> {
    try {
      return new Promise<any[]>((resolve, reject) => {
        const entries: any[] = []
        this.db
          .createValueStream({
            gte: this._makeKey(key, startIndex),
            lt: this._makeKey(key, endIndex),
            reverse,
          })
          .on('data', (transaction: string) => {
            entries.push(JSON.parse(transaction))
            if (peek) {
              // If we're just peeking, we can stop the stream early.
              resolve(entries)
            }
          })
          .on('error', (err: any) => {
            resolve(null)
          })
          .on('close', () => {
            // TODO: Close vs end? Need to double check later.
            resolve(entries)
          })
          .on('end', () => {
            resolve(entries)
          })
      })
    } catch (err) {
      return []
    }
  }

  public async rangeKV<TKey, TValue>(
    key: string,
    startIndex: number,
    endIndex?: number,
    reverse?: boolean, // add this so we could retrieve the entries in reverse order
    peek?: boolean // add this so we could just peek the first entry
  ): Promise<{ key: TKey; value: TValue }[] | []> {
    try {
      return new Promise<any[]>((resolve, reject) => {
        const data: { key: TKey; value: TValue }[] = []
        this.db
          .createReadStream({
            gte: this._makeKey(key, startIndex),
            lt: endIndex
              ? this._makeKey(key, endIndex)
              : this._makeUpperBoundKey(key),
            reverse,
          })
          .on('data', (k, v) => {
            data.push({
              key: JSON.parse(k),
              value: JSON.parse(v),
            })
            if (peek) {
              // If we're just peeking, we can stop the stream early.
              resolve(data)
            }
          })
          .on('error', (err: any) => {
            resolve(null)
          })
          .on('close', () => {
            // TODO: Close vs end? Need to double check later.
            resolve(data)
          })
          .on('end', () => {
            resolve(data)
          })
      })
    } catch (err) {
      return []
    }
  }

  public async put<TEntry>(
    entries: {
      key: string
      index: number
      value: TEntry
    }[]
  ): Promise<void> {
    return this.db.batch(
      entries.map((entry) => {
        return {
          type: 'put',
          key: this._makeKey(entry.key, entry.index),
          value: JSON.stringify(entry.value),
        }
      })
    )
  }

  private _makeKey(key: string, index: number): string {
    // prettier-ignore
    return `${key}:${toBigInt(index).toString().padStart(32, '0')}`
  }

  private _makeUpperBoundKey(key: string): string {
    // prettier-ignore
    return `${key}:${toBigInt(2 ** 32 - 1).toString().padStart(32, '0')}`
  }
}
