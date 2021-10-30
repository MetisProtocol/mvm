/* Imports: External */
import { DeployFunction } from 'hardhat-deploy/dist/types'

/* Imports: Internal */
import {
  deployAndRegister,
  getDeployedContract,
} from '../src/hardhat-deploy-ethers'

const deployFn: DeployFunction = async (hre) => {
  const Lib_AddressManager = await getDeployedContract(
    hre,
    'Lib_AddressManager'
  )
  const MVM_AddressManager = await getDeployedContract(
    hre,
    'MVM_AddressManager'
  )
  console.log(MVM_AddressManager.address,Lib_AddressManager.address)
  await deployAndRegister({
    hre,
    name: 'StateCommitmentChain',
    args: [
      MVM_AddressManager.address,
      Lib_AddressManager.address,
      (hre as any).deployConfig.sccFraudProofWindow,
      (hre as any).deployConfig.sccSequencerPublishWindow,
    ],
  })
}

deployFn.tags = ['StateCommitmentChain', 'upgrade']

export default deployFn
