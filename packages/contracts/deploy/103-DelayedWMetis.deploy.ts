import { DeployFunction } from 'hardhat-deploy/dist/types'
import {
  deployWithOZTransparentProxy,
  getDeployedContract,
  registerAddress,
} from '../src/hardhat-deploy-ethers'

const deployFn: DeployFunction = async (hre) => {
  const { deployer } = await hre.getNamedAccounts()

  const metisConfig = await getDeployedContract(hre, 'MetisConfig')

  const delayedWMetis = await deployWithOZTransparentProxy({
    hre,
    name: 'DelayedWMetis',
    args: [deployer, metisConfig.address],
    options: {
      constructorArgs: [
        // withdrawal delay
        86400,
        // metis token address
        (hre as any).deployConfig.mvmMetisAddress,
      ],
      unsafeAllow: ['constructor', 'state-variable-immutable'],
    },
  })

  if (delayedWMetis.newDeploy) {
    await registerAddress({
      hre,
      name: 'DelayedWMetis',
      address: delayedWMetis.contract.address,
    })
  }
}

deployFn.tags = ['DelayedWMetis', 'wmetis', 'faultproof']
export default deployFn
