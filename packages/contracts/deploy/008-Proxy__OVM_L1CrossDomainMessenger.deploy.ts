/* Imports: External */
import { DeployFunction } from 'hardhat-deploy/dist/types'

/* Imports: Internal */
import {
  hexStringEquals,
  getDeployedContract,
  deployAndRegister,
  waitUntilTrue,
} from '../src/hardhat-deploy-ethers'

const deployFn: DeployFunction = async (hre) => {
  const Lib_AddressManager = await getDeployedContract(
    hre,
    'Lib_AddressManager'
  )

  await deployAndRegister({
    hre,
    name: 'Proxy__OVM_L1CrossDomainMessenger',
    contract: 'Lib_ResolvedDelegateProxy',
    iface: 'L1CrossDomainMessenger',
    args: [Lib_AddressManager.address, 'OVM_L1CrossDomainMessenger'],
    postDeployAction: async (contract) => {
      console.log(`Initializing Proxy__OVM_L1CrossDomainMessenger...`)
      await contract.initialize(Lib_AddressManager.address)

      console.log(`Checking that contract was correctly initialized...`)
      await waitUntilTrue(async () => {
        return hexStringEquals(
          await contract.libAddressManager(),
          Lib_AddressManager.address
        )
      })
    },
  })
}

deployFn.tags = ['Proxy__OVM_L1CrossDomainMessenger']

export default deployFn
