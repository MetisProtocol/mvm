import { DeployFunction } from 'hardhat-deploy/dist/types'
import {
  deployWithOZTransparentProxy,
  getDeployedContract,
  registerAddress,
} from '../src/hardhat-deploy-ethers'
import { ethers } from 'hardhat'

const deployFn: DeployFunction = async (hre) => {
  const { deployer } = await hre.getNamedAccounts()

  // Get required contracts
  const metisConfig = await getDeployedContract(hre, 'MetisConfig')
  const addressManager = await getDeployedContract(hre, 'Lib_AddressManager')
  const metisToken = (hre as any).deployConfig.mvmMetisAddress

  const lockingPool = await deployWithOZTransparentProxy({
    hre,
    name: 'LockingPool',
    args: [
      deployer,
      metisToken, // token address (Metis Token)
      7 * 24 * 60 * 60, // lockPeriod (one-hour in seconds)
      1000, // slashRatio (10%)
      addressManager.address, // address manager address
      metisConfig.address,
    ],
    options: {
      unsafeAllow: ['constructor'],
    },
  })

  if (lockingPool.newDeploy) {
    await registerAddress({
      hre,
      name: 'FaultProofLockingPool', // Must match LOCKING_POOL_NAME in FaultDisputeGame
      address: lockingPool.contract.address,
    })
  }
}

deployFn.tags = ['LockingPool', 'faultproof']

export default deployFn
