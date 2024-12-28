import { DeployFunction } from 'hardhat-deploy/dist/types'
import {
  deployWithOZTransparentProxy,
  getDeployedContract,
  registerAddress,
  waitUntilTrue,
} from '../src/hardhat-deploy-ethers'
import { hexStringEquals } from '@metis.io/core-utils'

const deployFn: DeployFunction = async (hre) => {
  const { deployer } = await hre.getNamedAccounts()

  const factory = await deployWithOZTransparentProxy({
    hre,
    name: 'DisputeGameFactory',
    args: [deployer],
    options: {
      constructorArgs: [],
      unsafeAllow: ['constructor'],
    },
  })

  console.log(`DisputeGameFactory deployed to: ${factory.contract.address}`)

  // link the scc with the dispute game factory
  let sccProxy = await getDeployedContract(
    hre,
    'Proxy__MVM_StateCommitmentChain',
    {
      iface: 'L1ChugSplashProxy',
      signerOrProvider: deployer,
    }
  )

  // Set Slot 4 to the DISPUTE_GAME_FACTORY
  console.log(`Setting DISPUTE_GAME_FACTORY to ${factory.contract.address}...`)
  await sccProxy.setStorage(
    hre.ethers.utils.hexZeroPad('0x04', 32),
    hre.ethers.utils.hexZeroPad(
      hre.ethers.utils.hexValue(factory.contract.address),
      32
    )
  )

  sccProxy = await getDeployedContract(hre, 'Proxy__MVM_StateCommitmentChain', {
    iface: 'MVM_StateCommitmentChain',
    signerOrProvider: deployer,
  })

  const disputeGameFactoryAddress = await sccProxy.DISPUTE_GAME_FACTORY()
  console.log(
    `Confirming that disputeGameFactoryAddress ${disputeGameFactoryAddress}`
  )
  await waitUntilTrue(async () => {
    return hexStringEquals(
      hre.ethers.utils.hexValue(disputeGameFactoryAddress),
      hre.ethers.utils.hexValue(
        hre.ethers.utils.hexZeroPad(factory.contract.address, 32)
      )
    )
  })

  if (factory.newDeploy) {
    await registerAddress({
      hre,
      name: 'DisputeGameFactory',
      address: factory.contract.address,
    })
  }
}

deployFn.tags = ['DisputeGameFactory', 'factory', 'faultproof']
export default deployFn
