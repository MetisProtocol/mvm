import { DeployFunction } from 'hardhat-deploy/dist/types'
import {
  deployWithOZTransparentProxy,
  getDeployedContract,
  registerAddress,
} from '../src/hardhat-deploy-ethers'

const deployFn: DeployFunction = async (hre) => {
  const { deployer } = await hre.getNamedAccounts()

  const addressManager = await getDeployedContract(hre, 'Lib_AddressManager')

  const factory = await deployWithOZTransparentProxy({
    hre,
    name: 'DisputeGameFactory',
    args: [deployer],
    options: {
      constructorArgs: [
        // metis token address
        (hre as any).deployConfig.mvmMetisAddress,
        addressManager,
        (hre as any).deployConfig.faultDisputeTimeout,
        (hre as any).deployConfig.l2chainid, // L2 chain ID
      ],
      unsafeAllow: ['constructor', 'state-variable-immutable'],
    },
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
