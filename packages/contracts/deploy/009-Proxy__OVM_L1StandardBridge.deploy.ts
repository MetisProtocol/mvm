/* Imports: External */
import { DeployFunction } from 'hardhat-deploy/dist/types'

/* Imports: Internal */
import { deployAndRegister } from '../src/hardhat-deploy-ethers'
import {
  defaultHardhatNetworkHdAccountsConfigParams,
  defaultHardhatNetworkParams,
} from 'hardhat/internal/core/config/default-config'

const deployFn: DeployFunction = async (hre) => {
  const { deployer } = await hre.getNamedAccounts()
  const { chainId } = await hre.ethers.provider.getNetwork()
  var bridge;
  if (chainId === defaultHardhatNetworkParams.chainId) {
    bridge = 'L1StandardBridgeLocal'
  } else
  {
    bridge = 'L1StandardBridge'
  }
  await deployAndRegister({
    hre,
    name: 'Proxy__OVM_L1StandardBridge',
    contract: 'L1ChugSplashProxy',
    iface: bridge,
    args: [deployer],
  })
}

deployFn.tags = ['Proxy__OVM_L1StandardBridge']

export default deployFn
