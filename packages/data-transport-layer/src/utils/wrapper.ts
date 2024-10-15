import {
  BaseProvider,
  StaticJsonRpcProvider as V5JsonRpcProvider,
  FallbackProvider as V5FallbackProvider,
  Provider as V5Provider,
} from '@ethersproject/providers'
import {
  JsonRpcProvider as V6JsonRpcProvider,
  FallbackProvider as V6FallbackProvider,
  Provider as V6Provider,
  AbstractProvider as V6AbstractProvider,
} from 'ethers'

// This function converts an ethers V5 provider to a V6 provider,
// need this to maintain compatibility with the V5 provider
export const v5ToV6ProviderWrapper = (provider: BaseProvider): V6Provider => {
  if (provider instanceof V5JsonRpcProvider) {
    return new V6JsonRpcProvider(provider.connection.url)
  } else if (provider instanceof V5FallbackProvider) {
    const providers = provider.providerConfigs.map(
      (c) =>
        v5ToV6ProviderWrapper(c.provider as BaseProvider) as V6AbstractProvider
    )
    return new V6FallbackProvider(providers)
  }

  throw new Error('Unsupported provider')
}

// This function converts an ethers V6 provider to a V5 provider,
// need this to maintain compatibility with the V5 provider
export const v6ToV5ProviderWrapper = (provider: V6Provider): V5Provider => {
  if (provider instanceof V6JsonRpcProvider) {
    return new V5JsonRpcProvider(provider._getConnection().url)
  } else if (provider instanceof V6FallbackProvider) {
    const providers = provider.providerConfigs.map((c) =>
      v6ToV5ProviderWrapper(c.provider)
    )
    return new V5FallbackProvider(providers)
  }

  throw new Error('Unsupported provider')
}
