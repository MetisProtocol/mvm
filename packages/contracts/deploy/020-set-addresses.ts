/* Imports: External */
import { DeployFunction } from 'hardhat-deploy/dist/types'

/* Imports: Internal */
import {
  registerAddress,
  getDeployedContract,
} from '../src/hardhat-deploy-ethers'
import { predeploys } from '../src/predeploys'

const deployFn: DeployFunction = async (hre) => {
  const { deployer } = await hre.getNamedAccounts()
  const contract = await getDeployedContract(
    hre,
    'Proxy__MVM_CanonicalTransaction',
    {
      iface: 'MVM_CanonicalTransaction',
      signerOrProvider: deployer,
    }
  )

  await registerAddress({
    hre,
    name: (hre as any).deployConfig.l2chainid + '_MVM_Sequencer',
    address: contract.address,
  })

  // L2CrossDomainMessenger is the address of the predeploy on L2. We can refactor off-chain
  // services such that we can remove the need to set this address, but for now it's easier
  // to simply keep setting the address.
  await registerAddress({
    hre,
    name: 'L2CrossDomainMessenger',
    address: predeploys.L2CrossDomainMessenger,
  })

  // OVM_Sequencer is the address allowed to submit "Sequencer" blocks to the
  // CanonicalTransactionChain.
  //await registerAddress({
  //  hre,
  //  name: 'OVM_Sequencer',
  //  address: (hre as any).deployConfig.ovmSequencerAddress,
  //})

  // OVM_Proposer is the address allowed to submit state roots (transaction results) to the
  // StateCommitmentChain.
  //await registerAddress({
  //  hre,
  //  name: 'OVM_Proposer',
  //  address: (hre as any).deployConfig.ovmProposerAddress,
  //})

  await registerAddress({
    hre,
    name: 'METIS_MANAGER',
    address: (hre as any).deployConfig.mvmMetisManager,
  })

  // register the {l2chainId}_MVM_Sequencer_Wrapper, it will call MVM contract
  await registerAddress({
    hre,
    name: (hre as any).deployConfig.l2chainid + '_MVM_Sequencer_Wrapper',
    address: (hre as any).deployConfig.ovmSequencerAddress,
  })

  await registerAddress({
    hre,
    name: (hre as any).deployConfig.l2chainid + '_MVM_Proposer',
    address: (hre as any).deployConfig.ovmProposerAddress,
  })
}

deployFn.tags = ['set-addresses', 'upgrade', 'storage']

export default deployFn
