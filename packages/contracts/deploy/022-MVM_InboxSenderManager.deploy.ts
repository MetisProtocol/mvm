/* Imports: External */
import { DeployFunction } from 'hardhat-deploy/dist/types'

/* Imports: Internal */
import { getContractDefinition, getContractInterface } from '../src'
import {
  deployAndRegister,
  getAdvancedContract,
  getDeployedContract,
  hexStringEquals,
  registerAddress,
  waitUntilTrue,
} from '../src/hardhat-deploy-ethers'

const deployFn: DeployFunction = async (hre) => {
  const { deployer } = await hre.getNamedAccounts()

  const Lib_AddressManager = await getDeployedContract(
    hre,
    'Lib_AddressManager'
  )

  // Set up a reference to the proxy as if it were the L1StandardBridge contract.
  const contract = await getDeployedContract(
    hre,
    'Proxy__MVM_InboxSenderManager',
    {
      iface: 'MVM_InboxSenderManager',
      signerOrProvider: deployer,
    }
  )

  // Because of the `iface` parameter supplied to the deployment function above, the `contract`
  // variable that we here will have the interface of the L1StandardBridge contract. However,
  // we also need to interact with the contract as if it were a L1ChugSplashProxy contract so
  // we instantiate a new ethers.Contract object with the same address and signer but with the
  // L1ChugSplashProxy interface.
  const proxy = getAdvancedContract({
    hre,
    contract: new hre.ethers.Contract(
      contract.address,
      getContractInterface('L1ChugSplashProxy'),
      contract.signer
    ),
  })

  // First we need to set the correct implementation code. We'll set the code and then check
  // that the code was indeed correctly set.
  const managerArtifact = getContractDefinition('MVM_InboxSenderManager')
  const managerCode = managerArtifact.deployedBytecode

  console.log(`Setting verifier code...`)
  await proxy.setCode(managerCode)

  console.log(`Confirming that verifier code is correct...`)
  await waitUntilTrue(async () => {
    const implementation = await proxy.callStatic.getImplementation()
    return (
      !hexStringEquals(implementation, hre.ethers.constants.AddressZero) &&
      hexStringEquals(
        await contract.provider.getCode(implementation),
        managerCode
      )
    )
  })

  // Set Slot 1 to the Address Manager Address
  console.log(`Setting addressmgr address to ${Lib_AddressManager.address}...`)
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

  // Set Slot 3 => 0x0 to the defaultInboxSender
  // Set Slot 3 => 0x1 to the defaultInboxBlobSender
  console.log(
    `Setting defaultInboxSender & defaultInboxBlobSender to ${
      (hre as any).deployConfig.inboxSenderAddress
    } & ${(hre as any).deployConfig.inboxBlobSenderAddress} ...`
  )

  await proxy.setStorage(
    hre.ethers.utils.keccak256(
      hre.ethers.utils.defaultAbiCoder.encode(
        ['uint256', 'uint256'],
        ['0x03', '0x00']
      )
    ),
    hre.ethers.utils.hexZeroPad(
      (hre as any).deployConfig.inboxSenderAddress,
      32
    )
  )

  await proxy.setStorage(
    hre.ethers.utils.keccak256(
      hre.ethers.utils.defaultAbiCoder.encode(
        ['uint256', 'uint256'],
        ['0x03', '0x01']
      )
    ),
    hre.ethers.utils.hexZeroPad(
      (hre as any).deployConfig.inboxBlobSenderAddress,
      32
    )
  )

  const defaultInboxSender = await contract.defaultInboxSender(0)
  console.log(`Confirming that defaultInboxSender ${defaultInboxSender}`)
  await waitUntilTrue(async () => {
    return hexStringEquals(
      hre.ethers.utils.hexValue(defaultInboxSender),
      hre.ethers.utils.hexValue((hre as any).deployConfig.inboxSenderAddress)
    )
  })

  const defaultInboxBlobSender = await contract.defaultInboxSender(1)
  console.log(
    `Confirming that defaultInboxBlobSender ${defaultInboxBlobSender}`
  )
  await waitUntilTrue(async () => {
    return hexStringEquals(
      hre.ethers.utils.hexValue(defaultInboxBlobSender),
      hre.ethers.utils.hexValue(
        (hre as any).deployConfig.inboxBlobSenderAddress
      )
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
        from: hre.ethers.constants.AddressZero,
      }),
      owner
    )
  })

  // Replace Proxy__MVM_InboxSenderManager to contract.address
  await registerAddress({
    hre,
    name: 'Proxy__MVM_InboxSenderManager',
    address: contract.address,
  })

  console.log(`Deploying MVM_InboxSenderManager...`)
  await deployAndRegister({
    hre,
    name: 'MVM_InboxSenderManager',
    contract: 'MVM_InboxSenderManager',
    args: [
      Lib_AddressManager.address,
      (hre as any).deployConfig.inboxSenderAddress,
    ],
  })
}

deployFn.tags = [
  'MVM_InboxSenderManager',
  'upgrade',
  'storage',
  'andromeda-predeploy',
]

export default deployFn
