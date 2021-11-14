/* Imports: External */
import { DeployFunction } from 'hardhat-deploy/dist/types'

/* Imports: Internal */
import { deployAndRegister } from '../src/hardhat-deploy-ethers'

const deployFn: DeployFunction = async (hre) => {
  const { deployer } = await hre.getNamedAccounts()

  await deployAndRegister({
    hre,
    name: 'Proxy__MVM_ChainManager',
    contract: 'L1ChugSplashProxy',
    iface: 'MVM_L2ChainManagerOnL1',
    args: [deployer],
  })
}

deployFn.tags = ['Proxy__MVM_ChainManager']

export default deployFn
