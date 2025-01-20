import { DeployFunction } from 'hardhat-deploy/dist/types'
import {
  deployWithOZTransparentProxy,
  registerAddress,
} from '../src/hardhat-deploy-ethers'
import { ethers, upgrades } from 'hardhat'

const deployFn: DeployFunction = async (hre) => {
  const { deployer } = await hre.getNamedAccounts()

  const metisConfig = await deployWithOZTransparentProxy({
    hre,
    name: 'MetisConfig',
    args: [deployer, false],
    options: {
      constructorArgs: [],
      unsafeAllow: ['constructor'],
    },
  })

  if (metisConfig.newDeploy) {
    await registerAddress({
      hre,
      name: 'MetisConfig',
      address: metisConfig.contract.address,
    })
  }
}

deployFn.tags = ['MetisConfig', 'config', 'faultproof']
export default deployFn
