import { DeployFunction } from 'hardhat-deploy/dist/types'
import {
  deployWithOZTransparentProxy,
  registerAddress,
} from '../src/hardhat-deploy-ethers'

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
