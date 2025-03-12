import { DeployFunction } from 'hardhat-deploy/dist/types'
import {
  deployAndRegister,
  getDeployedContract,
} from '../src/hardhat-deploy-ethers'
import { ethers } from 'ethers'

const deployFn: DeployFunction = async (hre) => {
  const { deployer } = await hre.getNamedAccounts()

  const absolutePrestate = (hre as any).deployConfig.absolutePrestate
  if (!absolutePrestate) {
    throw new Error('absolutePrestate is required to deploy fault dispute game')
  }

  const delayedWMetis = await getDeployedContract(hre, 'Proxy__DelayedWMetis')
  const addressManager = await getDeployedContract(hre, 'Lib_AddressManager')
  const mips = await getDeployedContract(hre, 'MIPS')
  const disputeGameFactory = await getDeployedContract(
    hre,
    'Proxy__DisputeGameFactory',
    {
      iface: 'DisputeGameFactory',
      signerOrProvider: deployer,
    }
  )

  await deployAndRegister({
    hre,
    name: 'FaultDisputeGame',
    contract: 'FaultDisputeGame',
    args: [
      0, // gameType 0 for permissionless game
      (hre as any).deployConfig.absolutePrestate, // absolutePrestate of mips program
      73, // maxGameDepth
      30, // splitDepth
      0, // clockExtension
      86400, // maxClockDuration
      mips.address, // address of MIPS VM contract
      delayedWMetis.address, // address of DelayedWMetis contract
      addressManager.address, // address of AddressManager contract
      (hre as any).deployConfig.l2chainid, // L2 chain ID
    ],
  })

  // register fault dispute game to factory
  const faultDisputeGame = await getDeployedContract(hre, 'FaultDisputeGame')

  console.log('Registering FaultDisputeGame to DisputeGameFactory...')
  await disputeGameFactory.setImplementation(0, faultDisputeGame.address)

  console.log('Setting init bond for DisputeGame...')
  await disputeGameFactory.setInitBond(0, ethers.utils.parseEther('4'))
}

deployFn.tags = ['FaultDisputeGame', 'game', 'faultproof']
export default deployFn
