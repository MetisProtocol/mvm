/* Imports: External */
import { DeployFunction } from 'hardhat-deploy/dist/types'

/* Imports: Internal */
import {
  deployAndRegister,
  getDeployedContract,
} from '../src/hardhat-deploy-ethers'

const deployFn: DeployFunction = async (hre) => {
  const { deployer } = await hre.getNamedAccounts()

  await deployAndRegister({
    hre,
    name: 'MVM_ProposerRegistry',
    args: [deployer],
  })

  // register fault dispute game to factory
  const register = await getDeployedContract(hre, 'MVM_ProposerRegistry', {
    signerOrProvider: deployer,
  })

  console.log('Setting proposer in MVM_ProposerRegistry...')
  await register.setProposer(
    Number((hre as any).deployConfig.l2chainid),
    (hre as any).deployConfig.ovmProposerAddress
  )

  console.log('Transferring ownership of MVM_ProposerRegistry...')
  await register.transferOwnership((hre as any).deployConfig.mvmMetisManager)
}

deployFn.tags = ['MVM_ProposerRegistry']

export default deployFn
