import { BaseProvider, JsonRpcProvider as V5JsonRpcProvider, FallbackProvider as V5FallbackProvider } from '@ethersproject/providers'
import {
  JsonRpcProvider as V6JsonRpcProvider,
  FallbackProvider as V6FallbackProvider,
  AbstractProvider
} from 'ethers'

// This function converts an ethers V5 provider to a V6 provider,
// need this to maintain compatibility with the V5 provider
export const v5ToV6ProviderWrapper = (
  provider: BaseProvider
): AbstractProvider => {
  if (provider instanceof V5JsonRpcProvider) {
    return new V6JsonRpcProvider(provider.connection.url)
  } else if (provider instanceof V5FallbackProvider) {
    const providers = provider.providerConfigs.map((c) =>
      v5ToV6ProviderWrapper(c.provider as BaseProvider)
    )
    return new V6FallbackProvider(providers)
  }

  throw new Error('Unsupported provider')
}
