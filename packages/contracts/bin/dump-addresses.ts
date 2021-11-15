import { Wallet } from 'ethers'
import path from 'path'
import dirtree from 'directory-tree'
import fs from 'fs'

import hre from 'hardhat'

const network = process.env.CONTRACTS_TARGET_NETWORK


const main = async () => {

  // Stuff below this line is currently required for CI to work properly. We probably want to
  // update our CI so this is no longer necessary. But I'm adding it for backwards compat so we can
  // get the hardhat-deploy stuff merged. Woot.
  const nicknames = {
    Lib_AddressManager: 'AddressManager',

  }

  const contracts: any = dirtree(
    path.resolve(__dirname, `../deployments/` + network)
  )
    .children.filter((child) => {
      return child.extension === '.json'
    })
    .reduce((contractsAccumulator, child) => {
      const contractName = child.name.replace('.json', '')
      // eslint-disable-next-line @typescript-eslint/no-var-requires
      const artifact = require(path.resolve(
        __dirname,
        `../deployments/${network}/${child.name}`
      ))
      contractsAccumulator[nicknames[contractName] || contractName] =
        artifact.address
      return contractsAccumulator
    }, {})

  const addresses = JSON.stringify(contracts, null, 2)
  const dumpsPath = path.resolve(__dirname, '../dist/dumps')
  if (!fs.existsSync(dumpsPath)) {
    fs.mkdirSync(dumpsPath)
  }
  const addrsPath = path.resolve(dumpsPath, 'addresses.json')
  fs.writeFileSync(addrsPath, addresses)
}

main()
  .then(() => process.exit(0))
  .catch((error) => {
    console.log(
      JSON.stringify({ error: error.message, stack: error.stack }, null, 2)
    )
    process.exit(1)
  })
