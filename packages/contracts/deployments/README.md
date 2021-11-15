# Optimism Regenesis Deployments
## LAYER 2

### Chain IDs:
- Mainnet: 10
- Kovan: 69
- Goerli: 420
*The contracts relevant for the majority of developers are `OVM_ETH` and the cross-domain messengers. The L2 addresses don't change.*

### Predeploy contracts:
|Contract|Address|
|--|--|
|OVM_L2ToL1MessagePasser|0x4200000000000000000000000000000000000000|
|OVM_DeployerWhitelist|0x4200000000000000000000000000000000000002|
|MVM_ChainConfig|0x4200000000000000000000000000000000000005|
|L2CrossDomainMessenger|0x4200000000000000000000000000000000000007|
|OVM_GasPriceOracle|0x420000000000000000000000000000000000000F|
|L2StandardBridge|0x4200000000000000000000000000000000000010|
|OVM_SequencerFeeVault|0x4200000000000000000000000000000000000011|
|L2StandardTokenFactory|0x4200000000000000000000000000000000000012|
|OVM_L1BlockNumber|0x4200000000000000000000000000000000000013|
|MVM_Coinbase|0xDeadDeAddeAddEAddeadDEaDDEAdDeaDDeAD0000|
|OVM_ETH|0x420000000000000000000000000000000000000A|

---
---

## LAYER 1

## STARDUST

Network : __rinkeby (chain id: 4)__

|Contract|Address|
|--|--|
|BondManager|[0xec0b054F9524fa803d4555339BC70Be159e0485B](https://rinkeby.etherscan.io/address/0xec0b054F9524fa803d4555339BC70Be159e0485B)|
|CanonicalTransactionChain|[0x2b9c6FE921982284145ec1930989bd482268a3f9](https://rinkeby.etherscan.io/address/0x2b9c6FE921982284145ec1930989bd482268a3f9)|
|ChainStorageContainer-CTC-batches|[0x0561bdA99BD43A15459eC3f826C077A96929F5Fd](https://rinkeby.etherscan.io/address/0x0561bdA99BD43A15459eC3f826C077A96929F5Fd)|
|ChainStorageContainer-CTC-queue|[0x4B4D0f95688697bA221b90147eA16C30a7884DE4](https://rinkeby.etherscan.io/address/0x4B4D0f95688697bA221b90147eA16C30a7884DE4)|
|ChainStorageContainer-SCC-batches|[0x9f19b8391D664c51bDBfEFaa77D5C01764BdCD8E](https://rinkeby.etherscan.io/address/0x9f19b8391D664c51bDBfEFaa77D5C01764BdCD8E)|
|L1StandardBridge_for_verification_only|[0x7AE95D1241d7B27312baA8245dfAC80B08E2e68a](https://rinkeby.etherscan.io/address/0x7AE95D1241d7B27312baA8245dfAC80B08E2e68a)|
|Lib_AddressManager|[0x3870c347e84dE40F8bD8b5FB8CbCC423Ed38CE12](https://rinkeby.etherscan.io/address/0x3870c347e84dE40F8bD8b5FB8CbCC423Ed38CE12)|
|MVM_DiscountOracle|[0xa9BdA5A0881e59587B119685027a601Cae1Cc83d](https://rinkeby.etherscan.io/address/0xa9BdA5A0881e59587B119685027a601Cae1Cc83d)|
|MVM_L2ChainManagerOnL1_for_verification_only|[0x23b1BFb369667cc0bDa7B1da628268d3531d1D38](https://rinkeby.etherscan.io/address/0x23b1BFb369667cc0bDa7B1da628268d3531d1D38)|
|MVM_Verifier|[0xBde12E56D6d029Bed3e61d6E3b0CCb3311c0bCFb](https://rinkeby.etherscan.io/address/0xBde12E56D6d029Bed3e61d6E3b0CCb3311c0bCFb)|
|OVM_L1CrossDomainMessenger|[0xe4d0940df19f04C60006BA3420D13a82E3af7bA2](https://rinkeby.etherscan.io/address/0xe4d0940df19f04C60006BA3420D13a82E3af7bA2)|
|Proxy__MVM_ChainManager|[0x5553c94Cf01e1e631F9F92F26Afb1383F17a8D30](https://rinkeby.etherscan.io/address/0x5553c94Cf01e1e631F9F92F26Afb1383F17a8D30)|
|Proxy__OVM_L1CrossDomainMessenger|[0xBa6f1aAF3F909F208b224F3985eCda745FA9101A](https://rinkeby.etherscan.io/address/0xBa6f1aAF3F909F208b224F3985eCda745FA9101A)|
|Proxy__OVM_L1StandardBridge|[0x325049462E30472E7d27Fc53DF7d5a49210cBC70](https://rinkeby.etherscan.io/address/0x325049462E30472E7d27Fc53DF7d5a49210cBC70)|
|StateCommitmentChain|[0xc4139869E2e988ac71c65f8b68F02fD8A0C50B49](https://rinkeby.etherscan.io/address/0xc4139869E2e988ac71c65f8b68F02fD8A0C50B49)|
<!--
Implementation addresses. DO NOT use these addresses directly.
Use their proxied counterparts seen above.

-->
---
