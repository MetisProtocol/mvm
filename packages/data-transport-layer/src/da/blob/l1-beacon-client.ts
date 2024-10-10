import axios from 'axios'
import { Blob } from './blob'

export class L1BeaconClient {
  private readonly baseUrl: string

  constructor(baseUrl: string) {
    this.baseUrl = baseUrl
  }

  // checks the beacon chain version, usually just use this as a ping method
  async checkVersion(): Promise<void> {
    try {
      console.log('Checking L1 Beacon API version...')
      const response = await axios.get(`${this.baseUrl}/eth/v1/node/version`)
      console.log('Beacon Node Version:', response.data.data.version)
    } catch (error) {
      console.error('Failed to check L1 Beacon API version:', error)
      throw error
    }
  }

  // retrieve blobs from the beacon chain
  async getBlobs(timestamp: number, indices: number[]): Promise<any[]> {
    try {
      // calculate the beacon chain slot from the given timestamp
      const slot = (await this.getTimeToSlotFn())(timestamp)
      console.log(`Fetching blobs for slot ${slot} with blob indices:`, indices)
      const sidecars = await this.getBlobSidecars(slot, indices)
      const blobs = sidecars.map((sidecar: any) => {
        const blob = new Blob(sidecar.blob)
        return {
          data: blob.toData(),
          kzgCommitment: sidecar.kzg_commitment,
          kzgProof: sidecar.kzg_proof,
        }
      })
      console.log(`Fetched blobs for slot ${slot}`)
      return blobs
    } catch (error) {
      console.error('Failed to fetch blobs:', error)
      throw error
    }
  }

  // retrieve blob sidecars from the beacon chain
  async getBlobSidecars(slot: number, indices: number[]): Promise<any[]> {
    try {
      console.log(
        `Fetching blob sidecars for slot ${slot} with indices:`,
        indices
      )
      const response = await axios.get(
        `${this.baseUrl}/eth/v1/beacon/blob_sidecars/${slot}`,
        {
          params: { indices: indices.join(',') },
        }
      )
      console.log(`Fetched blob sidecars for slot ${slot}`)
      return response.data.data
    } catch (error) {
      console.error('Failed to fetch blob sidecars:', error)
      throw error
    }
  }

  // calculate the slot number from a given timestamp
  async getTimeToSlotFn(): Promise<(timestamp: number) => number> {
    try {
      console.log('Getting time to slot function...')

      // TODO: We might need to cache these, no need to fetch them every time.
      //       But we need to be careful that these value might change when the beacon chain upgrades.
      const genesisResponsePromise = axios.get(
        `${this.baseUrl}/eth/v1/beacon/genesis`
      )
      const configResponsePromise = axios.get(
        `${this.baseUrl}/eth/v1/config/spec`
      )

      const [genesisResponse, configResponse] = await Promise.all([
        genesisResponsePromise,
        configResponsePromise,
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
    } catch (error) {
      console.error('Failed to get time to slot function:', error)
      throw error
    }
  }
}
