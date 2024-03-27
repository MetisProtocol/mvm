/* Imports: External */
import { DeployFunction } from 'hardhat-deploy/dist/types'

/* Imports: Internal */
import { deployAndRegister } from '../src/hardhat-deploy-ethers'

const deployFn: DeployFunction = async (hre) => {
  const { deployer } = await hre.getNamedAccounts()

  await deployAndRegister({
    hre,
    name: 'Proxy__MVM_StateCommitmentChain',
    contract: 'L1ChugSplashProxy',
    iface: 'MVM_StateCommitmentChain',
    args: [deployer],
  })
}

deployFn.tags = [
  'Proxy__MVM_StateCommitmentChain',
  'storage',
  'andromeda-predeploy',
]

export default deployFn
