import { DeployFunction } from 'hardhat-deploy/dist/types'
import { registerAddress } from '../src/hardhat-deploy-ethers'
import { ethers, upgrades } from 'hardhat'

const deployFn: DeployFunction = async (hre) => {
  const { deployer } = await hre.getNamedAccounts()

  // Deploy DisputeGameFactory using upgrades plugin
  const DisputeGameFactory = await ethers.getContractFactory(
    'DisputeGameFactory',
    await ethers.getSigner(deployer)
  )
  const factory = await upgrades.deployProxy(DisputeGameFactory, [deployer], {
    initializer: 'initialize',
  })
  await factory.deployed()

  await registerAddress({
    hre,
    name: 'DisputeGameFactory',
    address: factory.address,
  })
}

deployFn.tags = ['DisputeGameFactory', 'factory', 'faultproof']
export default deployFn
