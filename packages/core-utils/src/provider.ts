/**
 * Provider Utilities
 */

import { ethers } from 'ethersv6'

// Copied from @ethersproject/providers since it is not
// currently exported
export interface FallbackProviderConfig {
  // The Provider
  provider: ethers.AbstractProvider
  // Timeout before also triggering the next provider; this does not stop
  // this provider and if its result comes back before a quorum is reached
  // it will be incorporated into the vote
  // - lower values will cause more network traffic but may result in a
  //   faster retult.
  stallTimeout?: number
  // The priority to favour this Provider; higher values are used first
  priority?: number
  // How much this provider contributes to the quorum; sometimes a specific
  // provider may be more reliable or trustworthy than others, but usually
  // this should be left as the default
  weight?: number
}

export const FallbackProvider = (config: string | FallbackProviderConfig[]) => {
  const configs = []
  if (typeof config === 'string') {
    const urls = config.split(',')
    for (const [i, url] of urls.entries()) {
      configs.push({
        priority: i,
        provider: new ethers.JsonRpcProvider(url),
      })
    }
    return new ethers.FallbackProvider(configs)
  }

  return new ethers.FallbackProvider(config)
}
