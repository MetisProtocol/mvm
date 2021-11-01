/* Imports: External */
import { DeployFunction } from 'hardhat-deploy/dist/types'
import { registerAddress, registerAddressToMvm } from '../src/hardhat-deploy-ethers'

const deployFn: DeployFunction = async (hre) => {
  const { deploy } = hre.deployments
  const { deployer } = await hre.getNamedAccounts()

  await deploy('Lib_AddressManager', {
    from: deployer,
    args: [],
    log: true,
    waitConfirmations: (hre as any).deployConfig.numDeployConfirmations,
  })
  
  const result =  await deploy('MVM_AddressManager', {
    from: deployer,
    args: [],
    log: true,
  })

  await registerAddress({
    hre,
    name: 'MVM_AddressManager',
    address: result.address,
  })
  
  await registerAddressToMvm({
    hre,
    name: '1088_MVM_Sequencer',
    address: (hre as any).deployConfig.ovmSequencerAddress,
  })
  await registerAddressToMvm({
    hre,
    name: '1088_MVM_Proposer',
    address: (hre as any).deployConfig.ovmProposerAddress,
  })
  await registerAddressToMvm({
    hre,
    name: '666_MVM_Proposer',
    address: (hre as any).deployConfig.ovmProposerAddress,
  })
  await registerAddressToMvm({
    hre,
    name: '1088_MVM_RELAYER',
    address: (hre as any).deployConfig.ovmRelayerAddress,
  })
  
  
}

deployFn.tags = ['Lib_AddressManager']

export default deployFn
