import { DeployFunction } from 'hardhat-deploy/dist/types'
import {
  deployAndRegister,
  getDeployedContract,
} from '../src/hardhat-deploy-ethers'

const deployFn: DeployFunction = async (hre) => {
  const { deployer } = await hre.getNamedAccounts()

  console.log('Deploying PreimageOracle...')

  await deployAndRegister({
    hre,
    name: 'PreimageOracle',
    contract: 'PreimageOracle',
    args: [126000, 86400],
  })

  const preimageOracle = await getDeployedContract(hre, 'PreimageOracle', {
    signerOrProvider: deployer,
  })

  console.log('Deploying MIPS VM...')

  await deployAndRegister({
    hre,
    name: 'MIPS64',
    contract: 'MIPS64',
    args: [preimageOracle.address, 0],
  })
}

deployFn.tags = ['MIPS64', 'cannon', 'faultproof']
export default deployFn
