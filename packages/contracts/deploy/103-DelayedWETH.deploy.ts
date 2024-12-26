import { DeployFunction } from 'hardhat-deploy/dist/types'
import { registerAddress } from '../src/hardhat-deploy-ethers'
import { ethers, upgrades } from 'hardhat'

const deployFn: DeployFunction = async (hre) => {
  const { deployer } = await hre.getNamedAccounts()

  // Deploy DelayedWETH using upgrades plugin
  const DelayedWETHFactory = await ethers.getContractFactory(
    'DelayedWETH',
    await ethers.getSigner(deployer)
  )

  const factory = await upgrades.deployProxy(DelayedWETHFactory, [deployer], {
    initializer: 'initialize',
    constructorArgs: [
      // withdrawal delay
      604800,
    ],
  })
  await factory.deployed()

  await registerAddress({
    hre,
    name: 'DelayedWETH',
    address: factory.address,
  })
}

deployFn.tags = ['DelayedWETH', 'weth', 'faultproof']
export default deployFn
