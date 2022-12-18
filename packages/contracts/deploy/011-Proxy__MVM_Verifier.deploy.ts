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

  await deployAndRegister({
    hre,
    name: 'Proxy__MVM_Verifier',
    contract: 'L1ChugSplashProxy',
    iface: 'MVM_Verifier',
    args: [deployer],
  })
}

deployFn.tags = ['Proxy__MVM_Verifier', 'andromeda-0331', 'andromeda-predeploy']

export default deployFn
