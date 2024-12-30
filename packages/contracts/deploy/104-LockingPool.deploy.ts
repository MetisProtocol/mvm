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
  const disputeGameFactory = await getDeployedContract(
    hre,
    'DisputeGameFactory'
  )
  const metisToken = (hre as any).deployConfig.mvmMetisAddress

  const lockingPool = await deployWithOZTransparentProxy({
    hre,
    name: 'LockingPool',
    args: [deployer, metisConfig.address],
    options: {
      constructorArgs: [
        metisToken, // token address (Metis Token)
        7 * 24 * 60 * 60, // lockPeriod (7 days in seconds)
        1000, // slashRatio (10%)
        disputeGameFactory.address, // disputeGameFactory address
      ],
      unsafeAllow: ['constructor', 'state-variable-immutable'],
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
