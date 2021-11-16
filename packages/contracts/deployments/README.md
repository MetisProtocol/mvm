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

## TRIAL

Network : __rinkeby (chain id: 4)__

|Contract|Address|
|--|--|
|BondManager|[0xB83ABFFFf2CF471D7800d83949fB4c25AAF417c8](https://rinkeby.etherscan.io/address/0xB83ABFFFf2CF471D7800d83949fB4c25AAF417c8)|
|CanonicalTransactionChain|[0x91AD1A3B89a2Ba71d5A0CbCa95cA69212B3D0D26](https://rinkeby.etherscan.io/address/0x91AD1A3B89a2Ba71d5A0CbCa95cA69212B3D0D26)|
|ChainStorageContainer-CTC-batches|[0xf29874E7963e98D15Cf1de1884a8Cd2fb38F80A0](https://rinkeby.etherscan.io/address/0xf29874E7963e98D15Cf1de1884a8Cd2fb38F80A0)|
|ChainStorageContainer-CTC-queue|[0x2E5D661bee7Db3B70D12652786D20d1e416C8233](https://rinkeby.etherscan.io/address/0x2E5D661bee7Db3B70D12652786D20d1e416C8233)|
|ChainStorageContainer-SCC-batches|[0xD74DB63e76c6f7B8267321d41AfbD2901A3868C2](https://rinkeby.etherscan.io/address/0xD74DB63e76c6f7B8267321d41AfbD2901A3868C2)|
|L1StandardBridge_for_verification_only|[0x1B37aD392a53Efe90DD838A8B60D61bD78089dDe](https://rinkeby.etherscan.io/address/0x1B37aD392a53Efe90DD838A8B60D61bD78089dDe)|
|Lib_AddressManager|[0x493C031A103A91801B1D1eb2e1D809840ce918ba](https://rinkeby.etherscan.io/address/0x493C031A103A91801B1D1eb2e1D809840ce918ba)|
|MVM_DiscountOracle|[0xB548ad7E98046c92bCd5f3a9fF6a40C3047de3de](https://rinkeby.etherscan.io/address/0xB548ad7E98046c92bCd5f3a9fF6a40C3047de3de)|
|MVM_L2ChainManagerOnL1_for_verification_only|[0xdF8D8B78d9736A7BdcdFd289aB3430c27e83be6E](https://rinkeby.etherscan.io/address/0xdF8D8B78d9736A7BdcdFd289aB3430c27e83be6E)|
|MVM_Verifier|[0xb5B9f9A26fb2A9Ca72B1A54Cd7Aa416B85a09Dc3](https://rinkeby.etherscan.io/address/0xb5B9f9A26fb2A9Ca72B1A54Cd7Aa416B85a09Dc3)|
|OVM_L1CrossDomainMessenger|[0x2A990a65f810c6DeE782DabDC6AB7F4Cfa1B2f4E](https://rinkeby.etherscan.io/address/0x2A990a65f810c6DeE782DabDC6AB7F4Cfa1B2f4E)|
|Proxy__MVM_ChainManager|[0x52B47D2819f66d7e8B81287c060d9f76c7576eD5](https://rinkeby.etherscan.io/address/0x52B47D2819f66d7e8B81287c060d9f76c7576eD5)|
|Proxy__OVM_L1CrossDomainMessenger|[0xaAf4465a0FDd7aDB98F9d9E08bcB8e0aeD666245](https://rinkeby.etherscan.io/address/0xaAf4465a0FDd7aDB98F9d9E08bcB8e0aeD666245)|
|Proxy__OVM_L1StandardBridge|[0x3616D89770ed103Cd9995ff2d25Ece8A95633DB9](https://rinkeby.etherscan.io/address/0x3616D89770ed103Cd9995ff2d25Ece8A95633DB9)|
|StateCommitmentChain|[0x7cF8F3C1C18587311BB8D6D4863B919a7c77225B](https://rinkeby.etherscan.io/address/0x7cF8F3C1C18587311BB8D6D4863B919a7c77225B)|
<!--
Implementation addresses. DO NOT use these addresses directly.
Use their proxied counterparts seen above.

-->
---
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
