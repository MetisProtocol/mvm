/* Imports: External */
import { DeployFunction } from 'hardhat-deploy/dist/types'

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
  const contract = await getDeployedContract(
    hre,
    'Proxy__MVM_StateCommitmentChain',
    {
      iface: 'MVM_StateCommitmentChain',
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
  const managerArtifact = getContractDefinition('MVM_StateCommitmentChain')
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

  // Set Slot 2 to the sccFraudProofWindow
  console.log(
    `Setting sccFraudProofWindow to ${
      (hre as any).deployConfig.sccFraudProofWindow
    }...`
  )
  await proxy.setStorage(
    hre.ethers.utils.hexZeroPad('0x01', 32),
    hre.ethers.utils.hexZeroPad(
      hre.ethers.utils.hexValue((hre as any).deployConfig.sccFraudProofWindow),
      32
    )
  )

  const fraudProofWindow = await contract.FRAUD_PROOF_WINDOW()
  console.log(`Confirming that sccFraudProofWindow  ${fraudProofWindow}`)
  await waitUntilTrue(async () => {
    return hexStringEquals(
      hre.ethers.utils.hexValue(fraudProofWindow),
      hre.ethers.utils.hexValue((hre as any).deployConfig.sccFraudProofWindow)
    )
  })

  // Set Slot 3 to the sccSequencerPublishWindow
  console.log(
    `Setting sccSequencerPublishWindow to ${
      (hre as any).deployConfig.sccSequencerPublishWindow
    }...`
  )
  await proxy.setStorage(
    hre.ethers.utils.hexZeroPad('0x02', 32),
    hre.ethers.utils.hexZeroPad(
      hre.ethers.utils.hexValue(
        (hre as any).deployConfig.sccSequencerPublishWindow
      ),
      32
    )
  )

  const sequencerPublishWindow = await contract.SEQUENCER_PUBLISH_WINDOW()
  console.log(
    `Confirming that sccSequencerPublishWindow  ${sequencerPublishWindow}`
  )
  await waitUntilTrue(async () => {
    return hexStringEquals(
      hre.ethers.utils.hexValue(sequencerPublishWindow),
      hre.ethers.utils.hexValue(
        (hre as any).deployConfig.sccSequencerPublishWindow
      )
    )
  })

  // Set Slot 4 to the default chain id
  console.log(`Setting default chain id`)
  await proxy.setStorage(
    hre.ethers.utils.hexZeroPad('0x03', 32),
    hre.ethers.utils.hexZeroPad('0x0440', 32)
  )

  const defaultChainId = await contract.DEFAULT_CHAINID()
  console.log(`Confirming that defaultChainId is 1088`)
  await waitUntilTrue(async () => {
    return hexStringEquals(
      hre.ethers.utils.hexValue(defaultChainId),
      hre.ethers.utils.hexValue(1088)
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

  // await registerAddress({
  //   hre,
  //   name: (hre as any).deployConfig.l2chainid + '_MVM_StateCommitmentChain',
  //   address: contract.address,
  // })

  // Replace StateCommitmentChain to contract.address
  await registerAddress({
    hre,
    name: 'StateCommitmentChain',
    address: contract.address,
  })

  console.log(`Deploying MVM_StateCommitmentChain...`)
  await deployAndRegister({
    hre,
    name: 'MVM_StateCommitmentChain_for_verification_only',
    contract: 'MVM_StateCommitmentChain',
    args: [
      Lib_AddressManager.address,
      (hre as any).deployConfig.sccFraudProofWindow,
      (hre as any).deployConfig.sccSequencerPublishWindow,
    ],
  })
}

deployFn.tags = [
  'MVM_StateCommitmentChain',
  'upgrade',
  'storage',
  'andromeda-predeploy',
]

export default deployFn
