import { DeployFunction } from 'hardhat-deploy/dist/types'
import { registerAddress } from '../src/hardhat-deploy-ethers'
import { ethers, upgrades } from 'hardhat'

const deployFn: DeployFunction = async (hre) => {
  const { deployer } = await hre.getNamedAccounts()

  // Deploy MetisConfig using upgrades plugin
  const MetisConfigFactory = await ethers.getContractFactory(
    'MetisConfig',
    await ethers.getSigner(deployer)
  )
  const metisConfig = await upgrades.deployProxy(
    MetisConfigFactory,
    [deployer, false],
    {
      initializer: 'initialize',
    }
  )
  await metisConfig.deployed()

  await registerAddress({
    hre,
    name: 'MetisConfig',
    address: metisConfig.address,
  })
}

deployFn.tags = ['MetisConfig', 'config', 'faultproof']
export default deployFn
