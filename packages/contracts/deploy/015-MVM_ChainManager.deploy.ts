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

  const { chainId } = await hre.ethers.provider.getNetwork()

  const chainmanager = await getDeployedContract(
    hre,
    'Proxy__MVM_ChainManager',
    {
      iface: 'MVM_L2ChainManagerOnL1',
    }
  )
  // Set up a reference to the proxy as if it were the L1StandardBridge contract.
  const contract = await getDeployedContract(hre, 'Proxy__MVM_ChainManager', {
    iface: 'MVM_L2ChainManagerOnL1',
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
  const managerArtifact = getContractDefinition('MVM_L2ChainManagerOnL1')
  const managerCode = managerArtifact.deployedBytecode

  console.log(`Setting chain manager code...`)
  await proxy.setCode(managerCode)

  console.log(`Confirming that manager code is correct...`)
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

  // Next we need to set the `messenger` address by executing a setStorage operation. We'll
  // check that this operation was correctly executed by calling `messenger()` and checking
  // that the result matches the value we initialized.
  const l1CrossDomainMessengerAddress = await Lib_AddressManager.getAddress(
    'Proxy__OVM_L1CrossDomainMessenger'
  )

  // Critical error, should never happen.
  if (
    hexStringEquals(l1CrossDomainMessengerAddress, ethers.constants.AddressZero)
  ) {
    throw new Error(`L1CrossDomainMessenger address is set to address(0)`)
  }

  console.log(
    `Setting messenger address to ${l1CrossDomainMessengerAddress}...`
  )
  await proxy.setStorage(
    ethers.utils.hexZeroPad('0x00', 32),
    ethers.utils.hexZeroPad(l1CrossDomainMessengerAddress, 32)
  )

  console.log(`Confirming that messenger address was correctly set...`)
  await waitUntilTrue(async () => {
    return hexStringEquals(
      await contract.messenger(),
      l1CrossDomainMessengerAddress
    )
  })

  console.log(`Setting addressmgr address to ${Lib_AddressManager.address}...`)
  // Set Slot 1 to the Address Manager Address
  await proxy.setStorage(
    hre.ethers.utils.hexZeroPad('0x01', 32),
    hre.ethers.utils.hexZeroPad(Lib_AddressManager.address, 32)
  )

  console.log(`Confirming that addressmgr address was correctly set...`)
  await waitUntilTrue(async () => {
    return hexStringEquals(
      await contract.addressmgr(),
      Lib_AddressManager.address
    )
  })

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

  // Deploy a copy of the implementation so it can be successfully verified on Etherscan.
  console.log(`Deploying a copy of the bridge for Etherscan verification...`)
  await deployAndRegister({
    hre,
    name: 'MVM_L2ChainManagerOnL1_for_verification_only',
    contract: 'MVM_L2ChainManagerOnL1',
    args: [],
  })
}

deployFn.tags = ['MVM_ChainManager', 'upgrade']

export default deployFn
