import { DeployFunction } from 'hardhat-deploy/dist/types'
import {
  deployAndRegister,
  getDeployedContract,
} from '../src/hardhat-deploy-ethers'

const deployFn: DeployFunction = async (hre) => {
  const { deployer } = await hre.getNamedAccounts()

  const absolutePrestate = (hre as any).deployConfig.absolutePrestate
  if (!absolutePrestate) {
    throw new Error('absolutePrestate is required to deploy fault dispute game')
  }
  const proposer = (hre as any).deployConfig.faultDisputeProposer
  if (!proposer) {
    throw new Error(
      'proposer is required to deploy permissioned fault dispute game'
    )
  }
  const challenger = (hre as any).deployConfig.faultDisputeChallenger
  if (!challenger) {
    throw new Error(
      'challenger is required to deploy permissioned fault dispute game'
    )
  }

  const delayedWETH = await getDeployedContract(hre, 'DelayedWETH')
  const addressManager = await getDeployedContract(hre, 'Lib_AddressManager')
  const mips = await getDeployedContract(hre, 'MIPS')
  const disputeGameFactory = await getDeployedContract(
    hre,
    'DisputeGameFactory',
    {
      iface: 'DisputeGameFactory',
      signerOrProvider: deployer,
    }
  )

  await deployAndRegister({
    hre,
    name: 'PermissionedDisputeGame',
    contract: 'PermissionedDisputeGame',
    args: [
      1, // gameType 0 for permissionless game
      (hre as any).deployConfig.absolutePrestate, // absolutePrestate of mips program
      73, // maxGameDepth
      30, // splitDepth
      10800, // clockExtension
      302400, // maxClockDuration
      mips.address, // address of MIPS VM contract
      delayedWETH.address, // address of DelayedWETH contract
      addressManager.address, // address of AddressManager contract
      (hre as any).deployConfig.l2chainid, // L2 chain ID
      proposer, // proposer
      challenger, // challenger
    ],
  })

  // register fault dispute game to factory
  const faultDisputeGame = await getDeployedContract(
    hre,
    'PermissionedDisputeGame'
  )

  console.log('Registering PermissionedDisputeGame to DisputeGameFactory...')
  await disputeGameFactory.setImplementation(1, faultDisputeGame.address)
}

deployFn.tags = ['PermissionedDisputeGame', 'game', 'faultproof']
export default deployFn
