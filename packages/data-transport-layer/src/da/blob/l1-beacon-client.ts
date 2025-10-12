import axios, { AxiosInstance } from 'axios'
import { Blob } from './blob'

export class L1BeaconClient {
  private readonly http: AxiosInstance

  public readonly beaconChainGenesisPromise: Promise<any>
  public readonly beaconChainConfigPromise: Promise<any>

  constructor(endpoint: string) {
    const parsed = new URL(endpoint)

    // extract baseURL (origin + pathname, trim trailing slash)
    const normalizedPath = parsed.pathname.endsWith('/')
      ? parsed.pathname.slice(0, -1)
      : parsed.pathname
    const baseURL = `${parsed.origin}${normalizedPath}`

    // create axios instance with baseURL and default params from endpoint.searchParams
    const defaultParams = Object.fromEntries(parsed.searchParams)
    this.http = axios.create({
      baseURL,
      params: defaultParams,
    })

    this.beaconChainGenesisPromise = this.request(`eth/v1/beacon/genesis`)
    this.beaconChainConfigPromise = this.request(`eth/v1/config/spec`)
  }

  // checks the beacon chain version, usually just use this as a ping method
  async checkVersion(): Promise<void> {
    await this.request(`eth/v1/node/version`)
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
    const response = await this.request(`eth/v1/beacon/blob_sidecars/${slot}`, {
      indices: indices.join(','),
    })
    return response.data
  }

  // calculate the slot number from a given timestamp
  async getTimeToSlotFn(): Promise<(timestamp: number) => number> {
    const [genesisResponse, configResponse] = await Promise.all([
      this.beaconChainGenesisPromise,
      this.beaconChainConfigPromise,
    ])

    const genesisTime = Number(genesisResponse.data.genesis_time)
    const secondsPerSlot = Number(configResponse.data.SECONDS_PER_SLOT)

    return (timestamp: number) => {
      if (timestamp < genesisTime) {
        throw new Error(
          `Provided timestamp (${timestamp}) precedes genesis time (${genesisTime})`
        )
      }
      return Math.floor((timestamp - genesisTime) / secondsPerSlot)
    }
  }

  async getChainId(): Promise<string> {
    const response = await this.beaconChainConfigPromise
    return response.data.DEPOSIT_NETWORK_ID
  }

  private async request(
    url: string,
    params?: Record<string, unknown>
  ): Promise<any> {
    // Use axios instance so baseURL and default params are handled correctly.
    // Provided `params` will override defaults.
    const response = await this.http.request({
      url,
      method: 'GET',
      params: params ?? undefined,
      validateStatus: () => true, // handle status manually below
    })

    // accept any 2xx as success
    if (!(response.status >= 200 && response.status < 300)) {
      throw new Error(
        `Failed to fetch ${url} from beacon chain with status code ${
          response.status
        }: Data: ${JSON.stringify(response.data)}`
      )
    }

    return response.data
  }
}
