import axios from 'axios'
import { Blob } from './blob'

export class L1BeaconClient {
  private readonly baseUrl: string

  private readonly beaconChainGenesisPromise: Promise<any>

  private readonly beaconChainConfigPromise: Promise<any>

  constructor(baseUrl: string) {
    this.baseUrl = baseUrl

    this.beaconChainGenesisPromise = axios.get(
      `${this.baseUrl}/eth/v1/beacon/genesis`
    )

    this.beaconChainConfigPromise = axios.get(
      `${this.baseUrl}/eth/v1/config/spec`
    )
  }

  // checks the beacon chain version, usually just use this as a ping method
  async checkVersion(): Promise<void> {
    const response = await axios.get(`${this.baseUrl}/eth/v1/node/version`)
  }

  // retrieve blobs from the beacon chain
  async getBlobs(timestamp: number, indices: number[]): Promise<any[]> {
    // calculate the beacon chain slot from the given timestamp
    const slot = (await this.getTimeToSlotFn())(timestamp)
    const sidecars = await this.getBlobSidecars(slot, indices)
    const blobs = sidecars.map((sidecar: any) => {
      const blob = new Blob(sidecar.blob)
      return {
        data: blob.toData(),
        kzgCommitment: sidecar.kzg_commitment,
        kzgProof: sidecar.kzg_proof,
      }
    })
    return blobs
  }

  // retrieve blob sidecars from the beacon chain
  async getBlobSidecars(slot: number, indices: number[]): Promise<any[]> {
    const response = await axios.get(
      `${this.baseUrl}/eth/v1/beacon/blob_sidecars/${slot}`,
      {
        params: { indices: indices.join(',') },
      }
    )
    return response.data.data
  }

  // calculate the slot number from a given timestamp
  async getTimeToSlotFn(): Promise<(timestamp: number) => number> {
    const [genesisResponse, configResponse] = await Promise.all([
      this.beaconChainGenesisPromise,
      this.beaconChainConfigPromise,
    ])

    const genesisTime = Number(genesisResponse.data.data.genesis_time)
    const secondsPerSlot = Number(configResponse.data.data.SECONDS_PER_SLOT)

    return (timestamp: number) => {
      if (timestamp < genesisTime) {
        throw new Error(
          `Provided timestamp (${timestamp}) precedes genesis time (${genesisTime})`
        )
      }
      return Math.floor((timestamp - genesisTime) / secondsPerSlot)
    }
  }
}