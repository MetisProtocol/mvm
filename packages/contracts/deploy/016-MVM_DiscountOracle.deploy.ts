/* Imports: External */
import { DeployFunction } from 'hardhat-deploy/dist/types'

/* Imports: Internal */
import {
  registerAddress,
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
    name: 'MVM_DiscountOracle',
    args: [Lib_AddressManager.address, '0'],
  })

  const MVM_DiscountOracle = await getDeployedContract(
    hre,
    'MVM_DiscountOracle',
    {
      signerOrProvider: deployer,
      iface: 'MVM_DiscountOracle',
    }
  )

  const l1bridge = await getDeployedContract(hre, 'Proxy__OVM_L1StandardBridge')

  const chainmgr = await getDeployedContract(hre, 'Proxy__MVM_ChainManager')
  //temporarily set to deployer for now
  await registerAddress({
    hre,
    name: 'METIS_MANAGER',
    address: deployer,
  })

  await MVM_DiscountOracle.setWhitelistedXDomainSender(l1bridge.address, true)
  const accessStored =
    await MVM_DiscountOracle.callStatic.isXDomainSenderAllowed(l1bridge.address)
  console.log('l1bridge.address access:', accessStored)

  await MVM_DiscountOracle.setWhitelistedXDomainSender(chainmgr.address, true)
  const accessStored2 =
    await MVM_DiscountOracle.callStatic.isXDomainSenderAllowed(chainmgr.address)
  console.log('chainmgr.address access:', accessStored2)
}

deployFn.tags = ['MVM_DiscountOracle', 'upgrade3']

export default deployFn
