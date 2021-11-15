/* Imports: External */
import { DeployFunction } from 'hardhat-deploy/dist/types'
import { registerAddress } from '../src/hardhat-deploy-ethers'

/* Imports: Internal */
import {
  deployAndRegister,
  getDeployedContract,
} from '../src/hardhat-deploy-ethers'

const deployFn: DeployFunction = async (hre) => {
  
  const { deployer } = await hre.getNamedAccounts()
  const Lib_AddressManager = await getDeployedContract(
    hre,
    'Lib_AddressManager'
  )

  await deployAndRegister({
    hre,
    name: 'MVM_Verifier',
    args: [Lib_AddressManager.address, (hre as any).deployConfig.mvmMetisAddress],
  })
  
  const MVM_Verifier = await getDeployedContract(
    hre,
    'MVM_Verifier'
  )
  
  await registerAddress({
    hre,
    name: (hre as any).deployConfig.l2chainid + '_MVM_FraudVerifier',
    address: MVM_Verifier.address,
  })
}

deployFn.dependencies = ['Lib_AddressManager']
deployFn.tags = ['MVM_Verifier']

export default deployFn
