/* Imports: External */
import { DeployFunction } from 'hardhat-deploy/dist/types'

/* Imports: Internal */
import {
  deployAndRegister,
  getDeployedContract,
} from '../src/hardhat-deploy-ethers'

const deployFn: DeployFunction = async (hre) => {
  await deployAndRegister({
    hre,
    name: 'MVM_ProposerRegistry',
    args: [(hre as any).deployConfig.mvmMetisManager],
  })

  // register fault dispute game to factory
  const register = await getDeployedContract(hre, 'MVM_ProposerRegistry')

  console.log('Setting proposer in MVM_ProposerRegistry...')
  await register.setProposer(
    Number((hre as any).deployConfig.l2chainid),
    (hre as any).deployConfig.ovmProposerAddress
  )
}

deployFn.tags = ['MVM_ProposerRegistry']

export default deployFn
