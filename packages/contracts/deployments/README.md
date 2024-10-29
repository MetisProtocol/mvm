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
|BondManager|[0xF1C14733B0D7F2B08d2FeC14F93DA99662DbD217](https://rinkeby.etherscan.io/address/0xF1C14733B0D7F2B08d2FeC14F93DA99662DbD217)|
|CanonicalTransactionChain|[0x5c562387117599241Da6761A3039a11AC192983c](https://rinkeby.etherscan.io/address/0x5c562387117599241Da6761A3039a11AC192983c)|
|ChainStorageContainer-CTC-batches|[0xC579b942D0B9d56cff8089f0AD7c10279C0C538c](https://rinkeby.etherscan.io/address/0xC579b942D0B9d56cff8089f0AD7c10279C0C538c)|
|ChainStorageContainer-CTC-queue|[0xf5EBC3F5465B155D8f25EdCF9BCfDB93a4Fab11c](https://rinkeby.etherscan.io/address/0xf5EBC3F5465B155D8f25EdCF9BCfDB93a4Fab11c)|
|ChainStorageContainer-SCC-batches|[0x02b2a06aBD053ecEAFFde1B8685537fa83dbc7AA](https://rinkeby.etherscan.io/address/0x02b2a06aBD053ecEAFFde1B8685537fa83dbc7AA)|
|L1StandardBridge_for_verification_only|[0xca6D4e6030ceCc3d600a3E9fe6CC1D8370864696](https://rinkeby.etherscan.io/address/0xca6D4e6030ceCc3d600a3E9fe6CC1D8370864696)|
|Lib_AddressManager|[0xD5D7825A091607CBD99531C409b16bE16aeFc7eA](https://rinkeby.etherscan.io/address/0xD5D7825A091607CBD99531C409b16bE16aeFc7eA)|
|MVM_DiscountOracle|[0x96517e9CEae97c0f264ed83f35D4166C7B66a0B5](https://rinkeby.etherscan.io/address/0x96517e9CEae97c0f264ed83f35D4166C7B66a0B5)|
|MVM_L2ChainManagerOnL1_for_verification_only|[0xa90C255aFAd5dF326Ac59130A6A729048C025B5D](https://rinkeby.etherscan.io/address/0xa90C255aFAd5dF326Ac59130A6A729048C025B5D)|
|OVM_L1CrossDomainMessenger|[0x69636924A27AF5A15ca6b10fBae5962e4958b9BF](https://rinkeby.etherscan.io/address/0x69636924A27AF5A15ca6b10fBae5962e4958b9BF)|
|Proxy__MVM_ChainManager|[0x372fcdBc28e6AFa27018Cda8eA698Ed83354EC5A](https://rinkeby.etherscan.io/address/0x372fcdBc28e6AFa27018Cda8eA698Ed83354EC5A)|
|Proxy__OVM_L1CrossDomainMessenger|[0x18F9138FCed74163A9007c357EA91Df140071a36](https://rinkeby.etherscan.io/address/0x18F9138FCed74163A9007c357EA91Df140071a36)|
|Proxy__OVM_L1StandardBridge|[0xc293264DED30f60068eE394A74Ed3c038F650697](https://rinkeby.etherscan.io/address/0xc293264DED30f60068eE394A74Ed3c038F650697)|
|StateCommitmentChain|[0xF8703F8369E41a6734b28ED6Ed7608343176E0b6](https://rinkeby.etherscan.io/address/0xF8703F8369E41a6734b28ED6Ed7608343176E0b6)|
<!--
Implementation addresses. DO NOT use these addresses directly.
Use their proxied counterparts seen above.

-->
---
## STARDUST

Network : __rinkeby (chain id: 4)__

|Contract|Address|
|--|--|
|BondManager|[0x9D9cb79c7741adD5A468FEaA7d8c9F21A9D16873](https://rinkeby.etherscan.io/address/0x9D9cb79c7741adD5A468FEaA7d8c9F21A9D16873)|
|CanonicalTransactionChain|[0x8872d61135E71745Da6Ddda1F98d4b79E599E889](https://rinkeby.etherscan.io/address/0x8872d61135E71745Da6Ddda1F98d4b79E599E889)|
|ChainStorageContainer-CTC-batches|[0xa04060eFAFE3c63De460E53151c0206A886576a0](https://rinkeby.etherscan.io/address/0xa04060eFAFE3c63De460E53151c0206A886576a0)|
|ChainStorageContainer-CTC-queue|[0x3f33339857C795a50E7F741C3df4C2abb9d97383](https://rinkeby.etherscan.io/address/0x3f33339857C795a50E7F741C3df4C2abb9d97383)|
|ChainStorageContainer-SCC-batches|[0x4e9F8D9CDE0f19490b7e6Cc04CE20F9612262C72](https://rinkeby.etherscan.io/address/0x4e9F8D9CDE0f19490b7e6Cc04CE20F9612262C72)|
|L1StandardBridge_for_verification_only|[0x7AE95D1241d7B27312baA8245dfAC80B08E2e68a](https://rinkeby.etherscan.io/address/0x7AE95D1241d7B27312baA8245dfAC80B08E2e68a)|
|Lib_AddressManager|[0xC9EB2B0bD7dbA69bb72886E9cF5da34d1Ca88C38](https://rinkeby.etherscan.io/address/0xC9EB2B0bD7dbA69bb72886E9cF5da34d1Ca88C38)|
|MVM_CanonicalTransaction|[0xCCB4a3279310Ed85A3ff1Ef84DE1a9d91fAF56e0](https://rinkeby.etherscan.io/address/0xCCB4a3279310Ed85A3ff1Ef84DE1a9d91fAF56e0)|
|MVM_CanonicalTransaction_for_verification_only|[0x76d4Fc1CB6D554ff9A065914A22C46df0ffB8A6D](https://rinkeby.etherscan.io/address/0x76d4Fc1CB6D554ff9A065914A22C46df0ffB8A6D)|
|MVM_DiscountOracle|[0x9db3BedF13fa81a887DA2010470E4A5E49523239](https://rinkeby.etherscan.io/address/0x9db3BedF13fa81a887DA2010470E4A5E49523239)|
|MVM_L2ChainManagerOnL1_for_verification_only|[0x23b1BFb369667cc0bDa7B1da628268d3531d1D38](https://rinkeby.etherscan.io/address/0x23b1BFb369667cc0bDa7B1da628268d3531d1D38)|
|MVM_Verifier|[0xA9b8E3a95e0E22352747Ab5395Ec535Cd113016a](https://rinkeby.etherscan.io/address/0xA9b8E3a95e0E22352747Ab5395Ec535Cd113016a)|
|MVM_Verifier_for_verification_only|[0xe47bc1F78BFF44b144b4830f0651908012d1E99d](https://rinkeby.etherscan.io/address/0xe47bc1F78BFF44b144b4830f0651908012d1E99d)|
|OVM_L1CrossDomainMessenger|[0xFbB32A0b32FE568B5e11829C83c4f20397c6f740](https://rinkeby.etherscan.io/address/0xFbB32A0b32FE568B5e11829C83c4f20397c6f740)|
|Proxy__MVM_CanonicalTransaction|[0x4fB8A54377d5c2D24a61Fb51D78cceC0B3221412](https://rinkeby.etherscan.io/address/0x4fB8A54377d5c2D24a61Fb51D78cceC0B3221412)|
|Proxy__MVM_ChainManager|[0x5553c94Cf01e1e631F9F92F26Afb1383F17a8D30](https://rinkeby.etherscan.io/address/0x5553c94Cf01e1e631F9F92F26Afb1383F17a8D30)|
|Proxy__MVM_Verifier|[0x33f81D2E1E1203A3186BE79022CC36C5b929E9f9](https://rinkeby.etherscan.io/address/0x33f81D2E1E1203A3186BE79022CC36C5b929E9f9)|
|Proxy__OVM_L1CrossDomainMessenger|[0xfD1b91066D27345023eBE2FE0D4C59d78c46129f](https://rinkeby.etherscan.io/address/0xfD1b91066D27345023eBE2FE0D4C59d78c46129f)|
|Proxy__OVM_L1StandardBridge|[0x056999aea33e5A6e51b5cF24a0684d565dF741EF](https://rinkeby.etherscan.io/address/0x056999aea33e5A6e51b5cF24a0684d565dF741EF)|
|StateCommitmentChain|[0xA9917d31D30048Dcf257639FE777F6606A100F89](https://rinkeby.etherscan.io/address/0xA9917d31D30048Dcf257639FE777F6606A100F89)|
<!--
Implementation addresses. DO NOT use these addresses directly.
Use their proxied counterparts seen above.

-->
---
## GOERLI

Network : __goerli (chain id: 5)__

|Contract|Address|
|--|--|
|BondManager|[0x68c39fc25cd754009C87B3160D5Fc9c155A6dFb9](https://goerli.etherscan.io/address/0x68c39fc25cd754009C87B3160D5Fc9c155A6dFb9)|
|CanonicalTransactionChain|[0x6Aec60fc997B4e2931b892398517b56F7b3C48Dd](https://goerli.etherscan.io/address/0x6Aec60fc997B4e2931b892398517b56F7b3C48Dd)|
|ChainStorageContainer-CTC-batches|[0x6782E5641Ba2D8036Db827B42744f0a93Dd45f07](https://goerli.etherscan.io/address/0x6782E5641Ba2D8036Db827B42744f0a93Dd45f07)|
|ChainStorageContainer-CTC-queue|[0x65d84E31C6958D6b46686DDfe894Dd35B8876F11](https://goerli.etherscan.io/address/0x65d84E31C6958D6b46686DDfe894Dd35B8876F11)|
|ChainStorageContainer-SCC-batches|[0x553417fEB2B366c7a4f9aAa2C8B9330CAFfd95E4](https://goerli.etherscan.io/address/0x553417fEB2B366c7a4f9aAa2C8B9330CAFfd95E4)|
|L1StandardBridge_for_verification_only|[0x55d3fE19b97D5f948aE277056c1F96fe18E8B0c0](https://goerli.etherscan.io/address/0x55d3fE19b97D5f948aE277056c1F96fe18E8B0c0)|
|Lib_AddressManager|[0x0C40f1f7A3B348F8e223F25e9d5808eA5FB43349](https://goerli.etherscan.io/address/0x0C40f1f7A3B348F8e223F25e9d5808eA5FB43349)|
|MVM_CanonicalTransaction_for_verification_only|[0x174eADC85E3B39F48bD378bc63301247854E9385](https://goerli.etherscan.io/address/0x174eADC85E3B39F48bD378bc63301247854E9385)|
|MVM_DiscountOracle|[0xd6274CEA1fD5a84b30bbC851DF6c73A5D1A78A32](https://goerli.etherscan.io/address/0xd6274CEA1fD5a84b30bbC851DF6c73A5D1A78A32)|
|MVM_L2ChainManagerOnL1_for_verification_only|[0x828c94303251ee2A66d4aEC2714500384453307F](https://goerli.etherscan.io/address/0x828c94303251ee2A66d4aEC2714500384453307F)|
|MVM_Verifier_for_verification_only|[0x538c6E5C076f8d752F5fBad308a381A370092579](https://goerli.etherscan.io/address/0x538c6E5C076f8d752F5fBad308a381A370092579)|
|OVM_L1CrossDomainMessenger|[0x2562Fbe25611C70e7674fb6bc1268F26D484C8F8](https://goerli.etherscan.io/address/0x2562Fbe25611C70e7674fb6bc1268F26D484C8F8)|
|Proxy__MVM_CanonicalTransaction|[0x9257d9d478fb71B98Cc2d1866B1A8C504a8B64C7](https://goerli.etherscan.io/address/0x9257d9d478fb71B98Cc2d1866B1A8C504a8B64C7)|
|Proxy__MVM_ChainManager|[0xE11986E6dDE546294b43f1eCc7181749322E89bf](https://goerli.etherscan.io/address/0xE11986E6dDE546294b43f1eCc7181749322E89bf)|
|Proxy__MVM_Verifier|[0x7ae231F97dd9EFFBE1F84474c050F124074Fb45e](https://goerli.etherscan.io/address/0x7ae231F97dd9EFFBE1F84474c050F124074Fb45e)|
|Proxy__OVM_L1CrossDomainMessenger|[0x914Aed79Cd083B5043C75A90616CC2A0477bf86c](https://goerli.etherscan.io/address/0x914Aed79Cd083B5043C75A90616CC2A0477bf86c)|
|Proxy__OVM_L1StandardBridge|[0xCF7257A86A5dBba34bAbcd2680f209eb9a05b2d2](https://goerli.etherscan.io/address/0xCF7257A86A5dBba34bAbcd2680f209eb9a05b2d2)|
|StateCommitmentChain|[0xd7344Cd0cC2C1A4c208B87fF227aDd1A576ac397](https://goerli.etherscan.io/address/0xd7344Cd0cC2C1A4c208B87fF227aDd1A576ac397)|
<!--
Implementation addresses. DO NOT use these addresses directly.
Use their proxied counterparts seen above.

-->
---
## ANDROMEDA

Network : __mainnet (chain id: 1)__

|Contract|Address|
|--|--|
|BondManager|[0xf51B9C9a1c12e7E48BEC15DC358D0C1f0d7Eb3be](https://etherscan.io/address/0xf51B9C9a1c12e7E48BEC15DC358D0C1f0d7Eb3be)|
|CanonicalTransactionChain|[0x56a76bcC92361f6DF8D75476feD8843EdC70e1C9](https://etherscan.io/address/0x56a76bcC92361f6DF8D75476feD8843EdC70e1C9)|
|ChainStorageContainer-CTC-batches|[0x38473Feb3A6366757A249dB2cA4fBB2C663416B7](https://etherscan.io/address/0x38473Feb3A6366757A249dB2cA4fBB2C663416B7)|
|ChainStorageContainer-CTC-queue|[0xA91Ea6F5d1EDA8e6686639d6C88b309cF35D2E57](https://etherscan.io/address/0xA91Ea6F5d1EDA8e6686639d6C88b309cF35D2E57)|
|ChainStorageContainer-SCC-batches|[0x10739F09f6e62689c0aA8A1878816de9e166d6f9](https://etherscan.io/address/0x10739F09f6e62689c0aA8A1878816de9e166d6f9)|
|L1StandardBridge_for_verification_only|[0x101500214981e7A5Ad2334D8404eaF365C2c3113](https://etherscan.io/address/0x101500214981e7A5Ad2334D8404eaF365C2c3113)|
|Lib_AddressManager|[0x918778e825747a892b17C66fe7D24C618262867d](https://etherscan.io/address/0x918778e825747a892b17C66fe7D24C618262867d)|
|MVM_CanonicalTransaction_for_verification_only|[0x431e877E216714647a4DCcEFFC03d7B4Fd4B825E](https://etherscan.io/address/0x431e877E216714647a4DCcEFFC03d7B4Fd4B825E)|
|MVM_DiscountOracle|[0xC8953ca384b4AdC8B1b11B030Afe2F05471664b0](https://etherscan.io/address/0xC8953ca384b4AdC8B1b11B030Afe2F05471664b0)|
|MVM_L2ChainManagerOnL1_for_verification_only|[0x9E2E3be85df5Ca63DE7674BA64ffD564075f3B48](https://etherscan.io/address/0x9E2E3be85df5Ca63DE7674BA64ffD564075f3B48)|
|MVM_Verifier|[0x9Ed4739afd706122591E75F215208ecF522C0Fd3](https://etherscan.io/address/0x9Ed4739afd706122591E75F215208ecF522C0Fd3)|
|MVM_Verifier_for_verification_only|[0xB2e2060A179e67cA4299Cc79fA337B98791DE069](https://etherscan.io/address/0xB2e2060A179e67cA4299Cc79fA337B98791DE069)|
|OVM_L1CrossDomainMessenger|[0x8bF439ef7167023F009E24b21719Ca5f768Ecb36](https://etherscan.io/address/0x8bF439ef7167023F009E24b21719Ca5f768Ecb36)|
|Proxy__MVM_CanonicalTransaction|[0x6A1DB7d799FBA381F2a518cA859ED30cB8E1d41a](https://etherscan.io/address/0x6A1DB7d799FBA381F2a518cA859ED30cB8E1d41a)|
|Proxy__MVM_ChainManager|[0xf3d58D1794f2634d6649a978f2dc093898FEEBc0](https://etherscan.io/address/0xf3d58D1794f2634d6649a978f2dc093898FEEBc0)|
|Proxy__MVM_Verifier|[0xe70DD4dE81D282B3fa92A6700FEE8339d2d9b5cb](https://etherscan.io/address/0xe70DD4dE81D282B3fa92A6700FEE8339d2d9b5cb)|
|Proxy__OVM_L1CrossDomainMessenger|[0x081D1101855bD523bA69A9794e0217F0DB6323ff](https://etherscan.io/address/0x081D1101855bD523bA69A9794e0217F0DB6323ff)|
|Proxy__OVM_L1StandardBridge|[0x3980c9ed79d2c191A89E02Fa3529C60eD6e9c04b](https://etherscan.io/address/0x3980c9ed79d2c191A89E02Fa3529C60eD6e9c04b)|
|StateCommitmentChain|[0xf209815E595Cdf3ed0aAF9665b1772e608AB9380](https://etherscan.io/address/0xf209815E595Cdf3ed0aAF9665b1772e608AB9380)|
<!--
Implementation addresses. DO NOT use these addresses directly.
Use their proxied counterparts seen above.

-->
---
