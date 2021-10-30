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
|L2CrossDomainMessenger|0x4200000000000000000000000000000000000007|
|OVM_GasPriceOracle|0x420000000000000000000000000000000000000F|
|L2StandardBridge|0x4200000000000000000000000000000000000010|
|OVM_SequencerFeeVault|0x4200000000000000000000000000000000000011|
|L2StandardTokenFactory|0x4200000000000000000000000000000000000012|
|OVM_L1BlockNumber|0x4200000000000000000000000000000000000013|
|OVM_ETH|0xDeadDeAddeAddEAddeadDEaDDEAdDeaDDeAD0000|
|WETH9|0x4200000000000000000000000000000000000006|

---
---

## LAYER 1

## OPTIMISTIC-KOVAN

Network : __undefined (chain id: 69)__

|Contract|Address|
|--|--|
|OVM_GasPriceOracle|[0x038a8825A3C3B0c08d52Cc76E5E361953Cf6Dc76](https://undefined.etherscan.io/address/0x038a8825A3C3B0c08d52Cc76E5E361953Cf6Dc76)|
|OVM_L2StandardTokenFactory|[undefined](https://undefined.etherscan.io/address/undefined)|
<!--
Implementation addresses. DO NOT use these addresses directly.
Use their proxied counterparts seen above.

-->
---
## MAINNET

Network : __mainnet (chain id: 1)__

|Contract|Address|
|--|--|
|Lib_AddressManager|[0xdE1FCfB0851916CA5101820A69b13a4E276bd81F](https://etherscan.io/address/0xdE1FCfB0851916CA5101820A69b13a4E276bd81F)|
|OVM_CanonicalTransactionChain|[0x4BF681894abEc828B212C906082B444Ceb2f6cf6](https://etherscan.io/address/0x4BF681894abEc828B212C906082B444Ceb2f6cf6)|
|OVM_ChainStorageContainer-CTC-batches|[0x3EA1a3839D8ca9a7ff3c567a9F36f4C4DbECc3eE](https://etherscan.io/address/0x3EA1a3839D8ca9a7ff3c567a9F36f4C4DbECc3eE)|
|OVM_ChainStorageContainer-CTC-queue|[0xA0b912b3Ea71A04065Ff82d3936D518ED6E38039](https://etherscan.io/address/0xA0b912b3Ea71A04065Ff82d3936D518ED6E38039)|
|OVM_ChainStorageContainer-SCC-batches|[0x77eBfdFcC906DDcDa0C42B866f26A8D5A2bb0572](https://etherscan.io/address/0x77eBfdFcC906DDcDa0C42B866f26A8D5A2bb0572)|
|OVM_ExecutionManager|[0x2745C24822f542BbfFB41c6cB20EdF766b5619f5](https://etherscan.io/address/0x2745C24822f542BbfFB41c6cB20EdF766b5619f5)|
|OVM_FraudVerifier|[0x042065416C5c665dc196076745326Af3Cd840D15](https://etherscan.io/address/0x042065416C5c665dc196076745326Af3Cd840D15)|
|OVM_L1CrossDomainMessenger|[0xbfba066b5cA610Fe70AdCE45FcB622F945891bb0](https://etherscan.io/address/0xbfba066b5cA610Fe70AdCE45FcB622F945891bb0)|
|OVM_L1MultiMessageRelayer|[0xF26391FBB1f77481f80a7d646AC08ba3817eA891](https://etherscan.io/address/0xF26391FBB1f77481f80a7d646AC08ba3817eA891)|
|OVM_SafetyChecker|[0xfe1F9Cf28ecDb12110aa8086e6FD343EA06035cC](https://etherscan.io/address/0xfe1F9Cf28ecDb12110aa8086e6FD343EA06035cC)|
|OVM_StateCommitmentChain|[0xE969C2724d2448F1d1A6189d3e2aA1F37d5998c1](https://etherscan.io/address/0xE969C2724d2448F1d1A6189d3e2aA1F37d5998c1)|
|OVM_StateManagerFactory|[0xd0e3e318154716BD9d007E1E6B021Eab246ff98d](https://etherscan.io/address/0xd0e3e318154716BD9d007E1E6B021Eab246ff98d)|
|OVM_StateTransitionerFactory|[0x38A6ed6fd76035684caDef38cF49a2FffA782B67](https://etherscan.io/address/0x38A6ed6fd76035684caDef38cF49a2FffA782B67)|
|Proxy__OVM_L1CrossDomainMessenger|[0x25ace71c97B33Cc4729CF772ae268934F7ab5fA1](https://etherscan.io/address/0x25ace71c97B33Cc4729CF772ae268934F7ab5fA1)|
|Proxy__OVM_L1StandardBridge|[0x99C9fc46f92E8a1c0deC1b1747d010903E884bE1](https://etherscan.io/address/0x99C9fc46f92E8a1c0deC1b1747d010903E884bE1)|
|mockOVM_BondManager|[0xCd76de5C57004d47d0216ec7dAbd3c72D8c49057](https://etherscan.io/address/0xCd76de5C57004d47d0216ec7dAbd3c72D8c49057)|
<!--
Implementation addresses. DO NOT use these addresses directly.
Use their proxied counterparts seen above.


-->
---
## KOVAN

Network : __kovan (chain id: 42)__

|Contract|Address|
|--|--|
|BondManager|[0x298f56D28f0aD2aAa181C4Cdf55a523F4ad5DBcF](https://kovan.etherscan.io/address/0x298f56D28f0aD2aAa181C4Cdf55a523F4ad5DBcF)|
|CanonicalTransactionChain|[0x90193cb12F7CA9629973eF690a46Aae109D4d66F](https://kovan.etherscan.io/address/0x90193cb12F7CA9629973eF690a46Aae109D4d66F)|
|ChainStorageContainer-CTC-batches|[0x4D2032761aFB3dF79C4E31f8882aa814c8c8f145](https://kovan.etherscan.io/address/0x4D2032761aFB3dF79C4E31f8882aa814c8c8f145)|
|ChainStorageContainer-CTC-queue|[0x9769887064F17a76eB69a6da1380b693669B9129](https://kovan.etherscan.io/address/0x9769887064F17a76eB69a6da1380b693669B9129)|
|ChainStorageContainer-SCC-batches|[0x8E4820AfaA1306A1B2c7067aC0e50ba7DAe24F3f](https://kovan.etherscan.io/address/0x8E4820AfaA1306A1B2c7067aC0e50ba7DAe24F3f)|
|Lib_AddressManager|[0x100Dd3b414Df5BbA2B542864fF94aF8024aFdf3a](https://kovan.etherscan.io/address/0x100Dd3b414Df5BbA2B542864fF94aF8024aFdf3a)|
|OVM_L1CrossDomainMessenger|[0x86EBb8c797cC4768004182D0B2f43B42b9a72e2c](https://kovan.etherscan.io/address/0x86EBb8c797cC4768004182D0B2f43B42b9a72e2c)|
|Proxy__L1CrossDomainMessenger|[0x4361d0F75A0186C05f971c566dC6bEa5957483fD](https://kovan.etherscan.io/address/0x4361d0F75A0186C05f971c566dC6bEa5957483fD)|
|Proxy__L1StandardBridge|[0x22F24361D548e5FaAfb36d1437839f080363982B](https://kovan.etherscan.io/address/0x22F24361D548e5FaAfb36d1437839f080363982B)|
|StateCommitmentChain|[0x2C1561bA6b4e6EDF8Ca40F12955C36477747Ff11](https://kovan.etherscan.io/address/0x2C1561bA6b4e6EDF8Ca40F12955C36477747Ff11)|
<!--
Implementation addresses. DO NOT use these addresses directly.
Use their proxied counterparts seen above.


-->
---
## GOERLI

Network : __goerli (chain id: 5)__

|Contract|Address|
|--|--|
|BondManager|[0xE5AE60bD6F8DEe4D0c2BC9268e23B92F1cacC58F](https://goerli.etherscan.io/address/0xE5AE60bD6F8DEe4D0c2BC9268e23B92F1cacC58F)|
|CanonicalTransactionChain|[0x2ebA8c4EfDB39A8Cd8f9eD65c50ec079f7CEBD81](https://goerli.etherscan.io/address/0x2ebA8c4EfDB39A8Cd8f9eD65c50ec079f7CEBD81)|
|ChainStorageContainer-CTC-batches|[0x0821Ff73FD88bb73E90F2Ea459B57430dff731Dd](https://goerli.etherscan.io/address/0x0821Ff73FD88bb73E90F2Ea459B57430dff731Dd)|
|ChainStorageContainer-CTC-queue|[0xf96dc01589969B85e27017F1bC449CB981eED9C8](https://goerli.etherscan.io/address/0xf96dc01589969B85e27017F1bC449CB981eED9C8)|
|ChainStorageContainer-SCC-batches|[0x829863Ce01B475B7d030539d2181d49E7A4b8aD9](https://goerli.etherscan.io/address/0x829863Ce01B475B7d030539d2181d49E7A4b8aD9)|
|Lib_AddressManager|[0x2F7E3cAC91b5148d336BbffB224B4dC79F09f01D](https://goerli.etherscan.io/address/0x2F7E3cAC91b5148d336BbffB224B4dC79F09f01D)|
|Proxy__L1CrossDomainMessenger|[0xEcC89b9EDD804850C4F343A278Be902be11AaF42](https://goerli.etherscan.io/address/0xEcC89b9EDD804850C4F343A278Be902be11AaF42)|
|Proxy__L1StandardBridge|[0x73298186A143a54c20ae98EEE5a025bD5979De02](https://goerli.etherscan.io/address/0x73298186A143a54c20ae98EEE5a025bD5979De02)|
|StateCommitmentChain|[0x1afcA918eff169eE20fF8AB6Be75f3E872eE1C1A](https://goerli.etherscan.io/address/0x1afcA918eff169eE20fF8AB6Be75f3E872eE1C1A)|
<!--
Implementation addresses. DO NOT use these addresses directly.
Use their proxied counterparts seen above.

L1CrossDomainMessenger: 
 - 0xd32718Fdb54e482C5Aa8eb7007cC898d798B3185
 - https://goerli.etherscan.io/address/0xd32718Fdb54e482C5Aa8eb7007cC898d798B3185)
-->
---
## CUSTOM

Network : __ropsten (chain id: 3)__

|Contract|Address|
|--|--|
|Lib_AddressManager|[0x0659A97068958Ebaba97121A6D7a2a95924824Ea](https://ropsten.etherscan.io/address/0x0659A97068958Ebaba97121A6D7a2a95924824Ea)|
|MVM_AddressManager|[0xbdB493827007eE26c16F10F6EABad6E97D9ead7D](https://ropsten.etherscan.io/address/0xbdB493827007eE26c16F10F6EABad6E97D9ead7D)|
|OVM_CanonicalTransactionChain|[0xA75d83A22BaDdaFB38B78675Ea1535525E302502](https://ropsten.etherscan.io/address/0xA75d83A22BaDdaFB38B78675Ea1535525E302502)|
|OVM_ChainStorageContainer-CTC-batches|[0xB167Cb0b51983858EEc1E1716dF18a59A1fe35B4](https://ropsten.etherscan.io/address/0xB167Cb0b51983858EEc1E1716dF18a59A1fe35B4)|
|OVM_ChainStorageContainer-CTC-queue|[0x407154421a86338306c1e5abFFA6fF42d2cFeEdC](https://ropsten.etherscan.io/address/0x407154421a86338306c1e5abFFA6fF42d2cFeEdC)|
|OVM_ChainStorageContainer-SCC-batches|[0x408888871A2Ffa9C7b73a07Aec0de0542b9ed43b](https://ropsten.etherscan.io/address/0x408888871A2Ffa9C7b73a07Aec0de0542b9ed43b)|
|OVM_ExecutionManager|[0x14835B093D320AA5c9806BBC64C17F0F2546D9EE](https://ropsten.etherscan.io/address/0x14835B093D320AA5c9806BBC64C17F0F2546D9EE)|
|OVM_FraudVerifier|[0x6D31CEaaa0588A62fFb99eCa3Bde0F22Bd7DBb7B](https://ropsten.etherscan.io/address/0x6D31CEaaa0588A62fFb99eCa3Bde0F22Bd7DBb7B)|
|OVM_L1MultiMessageRelayer|[0x4Df2b6836e11c63b7D762868979e14Df481F09a7](https://ropsten.etherscan.io/address/0x4Df2b6836e11c63b7D762868979e14Df481F09a7)|
|OVM_SafetyChecker|[0x65933e6885FeBC647659766A7837dd410cCDcb65](https://ropsten.etherscan.io/address/0x65933e6885FeBC647659766A7837dd410cCDcb65)|
|OVM_StateCommitmentChain|[0x45dC37Fe52a97A38B52F3f29633776b4A04324a5](https://ropsten.etherscan.io/address/0x45dC37Fe52a97A38B52F3f29633776b4A04324a5)|
|OVM_StateManagerFactory|[0x76f18Cc5F9DB41905a285866B9277Ac451F3f75B](https://ropsten.etherscan.io/address/0x76f18Cc5F9DB41905a285866B9277Ac451F3f75B)|
|OVM_StateTransitionerFactory|[0xF256665EdDf4cf2Eb456A53F9899e597c30384D5](https://ropsten.etherscan.io/address/0xF256665EdDf4cf2Eb456A53F9899e597c30384D5)|
|Proxy__MVM_AddressManager|[0xE3e7A4B35574Ce4b9Bc661cD93e8804Da548932a](https://ropsten.etherscan.io/address/0xE3e7A4B35574Ce4b9Bc661cD93e8804Da548932a)|
|Proxy__OVM_L1CrossDomainMessenger|[0xAce6d10F1E1942d2743DDe10a1388F31e3aA5e85](https://ropsten.etherscan.io/address/0xAce6d10F1E1942d2743DDe10a1388F31e3aA5e85)|
|Proxy__OVM_L1StandardBridge|[0xFEE2d383Ee292283eC43bdf0fa360296BE1e1149](https://ropsten.etherscan.io/address/0xFEE2d383Ee292283eC43bdf0fa360296BE1e1149)|
|mockOVM_BondManager|[0x3860B063928436F548c3A58A40c4d1d171E78838](https://ropsten.etherscan.io/address/0x3860B063928436F548c3A58A40c4d1d171E78838)|
<!--
Implementation addresses. DO NOT use these addresses directly.
Use their proxied counterparts seen above.

OVM_L1CrossDomainMessenger: 
 - 0xD2457203be19A6F6B0298e5a02b97B8811405487
 - https://ropsten.etherscan.io/address/0xD2457203be19A6F6B0298e5a02b97B8811405487)
-->
---
