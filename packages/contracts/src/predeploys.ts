/**
 * Predeploys are Solidity contracts that are injected into the initial L2 state and provide
 * various useful functions.
 *
 * Notes:
 * 0x42...04 was the address of the OVM_ProxySequencerEntrypoint. This contract is no longer in
 * use and has therefore been removed. We may place a new predeployed contract at this address
 * in the future. See https://github.com/ethereum-optimism/optimism/pull/549 for more info.
 */
export const predeploys = {
  OVM_L2ToL1MessagePasser: '0x4200000000000000000000000000000000000000',
  OVM_DeployerWhitelist: '0x4200000000000000000000000000000000000002',
  MVM_ChainConfig: '0x4200000000000000000000000000000000000005',
  L2CrossDomainMessenger: '0x4200000000000000000000000000000000000007',
  OVM_GasPriceOracle: '0x420000000000000000000000000000000000000F',
  L2StandardBridge: '0x4200000000000000000000000000000000000010',
  OVM_SequencerFeeVault: '0x4200000000000000000000000000000000000011',
  L2StandardTokenFactory: '0x4200000000000000000000000000000000000012',
  OVM_L1BlockNumber: '0x4200000000000000000000000000000000000013',
  MVM_Coinbase: '0xDeadDeAddeAddEAddeadDEaDDEAdDeaDDeAD0000',
  OVM_ETH: '0x420000000000000000000000000000000000000A',
}
