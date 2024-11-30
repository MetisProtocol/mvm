import axios from 'axios'
import { Blob } from './blob'

interface BeaconChainRequestHandler {
  request(url: string, params?: any): Promise<any>
}

class DefaultRequestHandler implements BeaconChainRequestHandler {
  private readonly baseUrl: string

  constructor(baseUrl: string) {
    this.baseUrl = baseUrl
  }

  async request(url: string, params?: any): Promise<any> {
    const response = await axios.get(`${url}`, {
      baseURL: this.baseUrl,
      params,
    })

    if (response.status !== 200) {
      throw new Error(`Failed to fetch ${url} from beacon chain`)
    }

    return response.data
  }
}

class DrpcRequestHandler extends DefaultRequestHandler {
  private readonly token?: string

  constructor(baseUrl: string) {
    // base url format is: https://lb.drpc.org/rest/eth-beacon-chain-sepolia?dkey={token}
    // we need to parse the url first and extract the base url and the token
    const url = new URL(baseUrl)
    const searchParams = new URLSearchParams(url.search)

    super(url.origin + url.pathname)

    this.token = searchParams.get('dkey')
  }

  async request(url: string, params?: any): Promise<any> {
    params = params || {}
    if (this.token) {
      params.dkey = this.token
    }

    return super.request(url, params)
  }
}

class HandlerFactory {
  static createHandler(baseUrl: string): BeaconChainRequestHandler {
    const url = new URL(baseUrl)
    if (url.hostname.includes('drpc.org')) {
      return new DrpcRequestHandler(baseUrl)
    }

    return new DefaultRequestHandler(baseUrl)
  }
}

export class L1BeaconClient {
  private readonly handler: BeaconChainRequestHandler

  public readonly beaconChainGenesisPromise: Promise<any>

  public readonly beaconChainConfigPromise: Promise<any>

  constructor(baseUrl: string) {
    this.handler = HandlerFactory.createHandler(baseUrl)

    this.beaconChainGenesisPromise = this.request(`eth/v1/beacon/genesis`)
    this.beaconChainConfigPromise = this.request(`eth/v1/config/spec`)
  }

  async request(url: string, params?: any): Promise<any> {
    return this.handler.request(url, params)
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
}
