import { DeployFunction } from 'hardhat-deploy/dist/types'
import {
  deployWithOZTransparentProxy,
  getDeployedContract,
  registerAddress,
} from '../src/hardhat-deploy-ethers'

const deployFn: DeployFunction = async (hre) => {
  const { deployer } = await hre.getNamedAccounts()

  const metisConfig = await getDeployedContract(hre, 'MetisConfig')

  const delayedWETH = await deployWithOZTransparentProxy({
    hre,
    name: 'DelayedWETH',
    args: [deployer, metisConfig.address],
    options: {
      constructorArgs: [
        // withdrawal delay
        604800,
      ],
      unsafeAllow: ['constructor', 'state-variable-immutable'],
    },
  })

  if (delayedWETH.newDeploy) {
    await registerAddress({
      hre,
      name: 'DelayedWETH',
      address: delayedWETH.contract.address,
    })
  }
}

deployFn.tags = ['DelayedWETH', 'weth', 'faultproof']
export default deployFn
