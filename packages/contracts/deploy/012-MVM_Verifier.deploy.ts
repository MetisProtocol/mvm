/* Imports: External */
import { DeployFunction } from 'hardhat-deploy/dist/types'
import { ethers } from 'ethers'
/* Imports: Internal */
import {
  getContractInterface,
  getContractDefinition,
} from '../src/contract-defs'
import {
  hexStringEquals,
  registerAddress,
  getDeployedContract,
  waitUntilTrue,
  getAdvancedContract,
  deployAndRegister,
} from '../src/hardhat-deploy-ethers'

const deployFn: DeployFunction = async (hre) => {
  const { deployer } = await hre.getNamedAccounts()

  const Lib_AddressManager = await getDeployedContract(
    hre,
    'Lib_AddressManager'
  )

  // Set up a reference to the proxy as if it were the L1StandardBridge contract.
  const contract = await getDeployedContract(hre, 'Proxy__MVM_Verifier', {
    iface: 'MVM_Verifier',
    signerOrProvider: deployer,
  })

  // Because of the `iface` parameter supplied to the deployment function above, the `contract`
  // variable that we here will have the interface of the L1StandardBridge contract. However,
  // we also need to interact with the contract as if it were a L1ChugSplashProxy contract so
  // we instantiate a new ethers.Contract object with the same address and signer but with the
  // L1ChugSplashProxy interface.
  const proxy = getAdvancedContract({
    hre,
    contract: new ethers.Contract(
      contract.address,
      getContractInterface('L1ChugSplashProxy'),
      contract.signer
    ),
  })

  // First we need to set the correct implementation code. We'll set the code and then check
  // that the code was indeed correctly set.
  const managerArtifact = getContractDefinition('MVM_Verifier')
  const managerCode = managerArtifact.deployedBytecode

  console.log(`Setting verifier code...`)

  await proxy.setCode(managerCode)

  console.log(`Confirming that verifier code is correct...`)
  await waitUntilTrue(async () => {
    const implementation = await proxy.callStatic.getImplementation()
    return (
      !hexStringEquals(implementation, ethers.constants.AddressZero) &&
      hexStringEquals(
        await contract.provider.getCode(implementation),
        managerCode
      )
    )
  })

  console.log(`Setting addressmgr address to ${Lib_AddressManager.address}...`)
  // Set Slot 1 to the Address Manager Address
  await proxy.setStorage(
    hre.ethers.utils.hexZeroPad('0x00', 32),
    hre.ethers.utils.hexZeroPad(Lib_AddressManager.address, 32)
  )

  console.log(`Confirming that addressmgr address was correctly set...`)
  console.log(await contract.libAddressManager())
  await waitUntilTrue(async () => {
    return hexStringEquals(
      await contract.libAddressManager(),
      Lib_AddressManager.address
    )
  })

  console.log(
    `Setting metis address to ${(hre as any).deployConfig.mvmMetisAddress}...`
  )
  // Set Slot 2 to the Metis Token Address
  await proxy.setStorage(
    hre.ethers.utils.hexZeroPad('0x01', 32),
    hre.ethers.utils.hexZeroPad((hre as any).deployConfig.mvmMetisAddress, 32)
  )

  console.log(`Confirming that metis address was correctly set...`)
  await waitUntilTrue(async () => {
    return hexStringEquals(
      await contract.metis(),
      (hre as any).deployConfig.mvmMetisAddress
    )
  })

  //temporarily setting the metis manager to deployer
  await registerAddress({
    hre,
    name: 'METIS_MANAGER',
    address: deployer,
  })

  //setting the whitelist
  console.log(
    `Setting WhiteList ${(hre as any).deployConfig.mvmMetisManager}...`
  )
  await contract.setWhiteList((hre as any).deployConfig.mvmMetisManager, true)

  console.log(`Confirming that whitelist setting...`)
  await waitUntilTrue(async () => {
    return (
      (await contract.isWhiteListed(
        (hre as any).deployConfig.mvmMetisManager
      )) === true
    )
  })

  //setting the whitelist
  console.log(`Setting minStake ...`)
  await contract.setMinStake('200000000000000000000')

  // Finally we transfer ownership of the proxy to the ovmAddressManagerOwner address.
  const owner = (hre as any).deployConfig.mvmMetisManager
  console.log(`Setting owner address to ${owner}...`)
  await proxy.setOwner(owner)

  console.log(`Confirming that owner address was correctly set...`)
  await waitUntilTrue(async () => {
    return hexStringEquals(
      await proxy.connect(proxy.signer.provider).callStatic.getOwner({
        from: ethers.constants.AddressZero,
      }),
      owner
    )
  })

  await registerAddress({
    hre,
    name: (hre as any).deployConfig.l2chainid + '_MVM_FraudVerifier',
    address: contract.address,
  })

  // Deploy a copy of the implementation so it can be successfully verified on Etherscan.
  console.log(`Deploying a copy of the bridge for Etherscan verification...`)
  await deployAndRegister({
    hre,
    name: 'MVM_Verifier_for_verification_only',
    contract: 'MVM_Verifier',
    args: [],
  })
}

deployFn.tags = ['MVM_Verifier', 'upgrade2', 'andromeda-predeploy']

export default deployFn
