/* Imports: External */
import { Contract, ethers } from 'ethers'
import { Provider } from '@ethersproject/abstract-provider'
import { Signer } from '@ethersproject/abstract-signer'
import { sleep } from '@metis.io/core-utils'
import * as fs from 'fs'
import * as path from 'path'

import '@openzeppelin/hardhat-upgrades'
import { upgrades } from 'hardhat'

export const hexStringEquals = (stringA: string, stringB: string): boolean => {
  if (!ethers.utils.isHexString(stringA)) {
    throw new Error(`input is not a hex string: ${stringA}`)
  }

  if (!ethers.utils.isHexString(stringB)) {
    throw new Error(`input is not a hex string: ${stringB}`)
  }

  return stringA.toLowerCase() === stringB.toLowerCase()
}
export const waitUntilTrue = async (
  check: () => Promise<boolean>,
  opts: {
    retries?: number
    delay?: number
  } = {}
) => {
  opts.retries = opts.retries || 100
  opts.delay = opts.delay || 5000

  let retries = 0
  while (!(await check())) {
    if (retries > opts.retries) {
      throw new Error(`check failed after ${opts.retries} attempts`)
    }
    retries++
    await sleep(opts.delay)
  }
}

export const registerAddress = async ({
  hre,
  name,
  address,
}): Promise<void> => {
  // TODO: Cache these 2 across calls?
  const { deployer } = await hre.getNamedAccounts()
  const Lib_AddressManager = await getDeployedContract(
    hre,
    'Lib_AddressManager',
    {
      signerOrProvider: deployer,
    }
  )

  const currentAddress = await Lib_AddressManager.getAddress(name)
  if (address === currentAddress) {
    console.log(
      `✓ Not registering address for ${name} because it's already been correctly registered`
    )
    return
  }
  console.log(`Registering address for ${name} to ${address}...`)
  await Lib_AddressManager.setAddress(name, address)

  console.log(`Waiting for registration to reflect on-chain...`)
  await waitUntilTrue(async () => {
    return hexStringEquals(await Lib_AddressManager.getAddress(name), address)
  })

  console.log(`✓ Registered address for ${name}`)
}

export const deployAndRegister = async ({
  hre,
  name,
  args,
  contract,
  iface,
  postDeployAction,
}: {
  hre: any
  name: string
  args: any[]
  contract?: string
  iface?: string
  postDeployAction?: (contract: Contract) => Promise<void>
}) => {
  const { deploy } = hre.deployments
  const { deployer } = await hre.getNamedAccounts()

  const result = await deploy(name, {
    contract,
    from: deployer,
    args,
    log: true,
    waitConfirmations: hre.deployConfig.numDeployConfirmations,
  })

  await hre.ethers.provider.waitForTransaction(result.transactionHash)

  if (result.newlyDeployed) {
    if (postDeployAction) {
      const signer = hre.ethers.provider.getSigner(deployer)
      let abi = result.abi
      if (iface !== undefined) {
        const factory = await hre.ethers.getContractFactory(iface)
        abi = factory.interface
      }
      await postDeployAction(
        getAdvancedContract({
          hre,
          contract: new Contract(result.address, abi, signer),
        })
      )
    }

    await registerAddress({
      hre,
      name,
      address: result.address,
    })
  }
}

// Returns a version of the contract object which modifies all of the input contract's methods to:
// 1. Waits for a confirmed receipt with more than deployConfig.numDeployConfirmations confirmations.
// 2. Include simple resubmission logic, ONLY for Kovan, which appears to drop transactions.
export const getAdvancedContract = (opts: {
  hre: any
  contract: Contract
}): Contract => {
  // Temporarily override Object.defineProperty to bypass ether's object protection.
  const def = Object.defineProperty
  Object.defineProperty = (obj, propName, prop) => {
    prop.writable = true
    return def(obj, propName, prop)
  }

  const contract = new Contract(
    opts.contract.address,
    opts.contract.interface,
    opts.contract.signer || opts.contract.provider
  )

  // Now reset Object.defineProperty
  Object.defineProperty = def

  // Override each function call to also `.wait()` so as to simplify the deploy scripts' syntax.
  for (const fnName of Object.keys(contract.functions)) {
    const fn = contract[fnName].bind(contract)
    ;(contract as any)[fnName] = async (...args: any) => {
      const tx = await fn(...args, {
        gasPrice: opts.hre.deployConfig.gasprice || undefined,
      })

      if (typeof tx !== 'object' || typeof tx.wait !== 'function') {
        return tx
      }

      // Special logic for:
      // (1) handling confirmations
      // (2) handling an issue on Kovan specifically where transactions get dropped for no
      //     apparent reason.
      const maxTimeout = 120
      let timeout = 0
      while (true) {
        await sleep(1000)
        const receipt = await contract.provider.getTransactionReceipt(tx.hash)
        if (receipt === null) {
          timeout++
          if (timeout > maxTimeout && opts.hre.network.name === 'kovan') {
            // Special resubmission logic ONLY required on Kovan.
            console.log(
              `WARNING: Exceeded max timeout on transaction. Attempting to submit transaction again...`
            )
            return contract[fnName](...args)
          }
        } else if (
          receipt.confirmations >= opts.hre.deployConfig.numDeployConfirmations
        ) {
          return tx
        }
      }
    }
  }

  return contract
}

export const getDeployedContract = async (
  hre: any,
  name: string,
  options: {
    iface?: string
    signerOrProvider?: Signer | Provider | string
  } = {}
): Promise<Contract> => {
  const deployed = await hre.deployments.get(name)

  await hre.ethers.provider.waitForTransaction(deployed.receipt.transactionHash)

  // Get the correct interface.
  let iface = new hre.ethers.utils.Interface(deployed.abi)
  if (options.iface) {
    const factory = await hre.ethers.getContractFactory(options.iface)
    iface = factory.interface
  }

  let signerOrProvider: Signer | Provider = hre.ethers.provider
  if (options.signerOrProvider) {
    if (typeof options.signerOrProvider === 'string') {
      signerOrProvider = hre.ethers.provider.getSigner(options.signerOrProvider)
    } else {
      signerOrProvider = options.signerOrProvider
    }
  }

  return getAdvancedContract({
    hre,
    contract: new Contract(deployed.address, iface, signerOrProvider),
  })
}

interface OZUpgradesDeployment {
  manifestVersion: string
  admin: {
    address: string
    txHash: string
  }
  proxies: Array<{
    address: string
    txHash: string
    kind: string
  }>
  impls: {
    [key: string]: {
      address: string
      txHash: string
      layout: any
    }
  }
}

const TRANSPARENT_PROXY_BYTECODE = '0x' // minimal proxy bytecode, we don't need the actual bytecode for deployment records

const getImplDeploymentInfo = async (
  hre: any,
  proxyAddress: string
): Promise<{ address: string; txHash: string } | null> => {
  // Get current chain id and network name
  const network = await hre.ethers.provider.getNetwork()
  const chainId = network.chainId

  // Find the .openzeppelin directory
  const ozDirPath = path.join(hre.config.paths.root, '.openzeppelin')
  if (!fs.existsSync(ozDirPath)) {
    return null
  }

  // Look for network specific files
  const files = fs.readdirSync(ozDirPath)
  const networkFile = files.find((f) => f.endsWith(`-${chainId}.json`))
  if (!networkFile) {
    return null
  }

  const filePath = path.join(ozDirPath, networkFile)
  const content = fs.readFileSync(filePath, 'utf8')
  const deployment = JSON.parse(content) as OZUpgradesDeployment

  // Find the proxy entry
  const proxyEntry = deployment.proxies.find(
    (p) => p.address.toLowerCase() === proxyAddress.toLowerCase()
  )
  if (!proxyEntry) {
    return null
  }

  // Get the implementation address
  const implAddress = await upgrades.erc1967.getImplementationAddress(
    proxyAddress
  )

  // Find the implementation info
  for (const [, implInfo] of Object.entries(deployment.impls)) {
    if (implInfo.address.toLowerCase() === implAddress.toLowerCase()) {
      return {
        address: implInfo.address,
        txHash: implInfo.txHash,
      }
    }
  }

  return null
}

const getProxyArtifact = async (hre: any, contractName: string) => {
  try {
    // Try to get the actual TransparentUpgradeableProxy artifact
    const proxyArtifact = await hre.artifacts.readArtifact(
      '@openzeppelin/contracts/proxy/transparent/TransparentUpgradeableProxy.sol:TransparentUpgradeableProxy'
    )
    return {
      ...proxyArtifact,
      contractName: `Proxy__${contractName}`,
    }
  } catch (e) {
    // Fallback to minimal artifact if we can't get the actual one
    console.warn(
      'Warning: Could not load TransparentUpgradeableProxy artifact, using minimal version'
    )
    const implArtifact = await hre.deployments.getExtendedArtifact(contractName)
    return {
      _format: implArtifact._format,
      contractName: `Proxy__${contractName}`,
      sourceName:
        '@openzeppelin/contracts/proxy/transparent/TransparentUpgradeableProxy.sol',
      abi: [
        {
          inputs: [
            {
              internalType: 'address',
              name: '_logic',
              type: 'address',
            },
            {
              internalType: 'address',
              name: 'admin_',
              type: 'address',
            },
            {
              internalType: 'bytes',
              name: '_data',
              type: 'bytes',
            },
          ],
          stateMutability: 'nonpayable',
          type: 'constructor',
        },
        {
          anonymous: false,
          inputs: [
            {
              indexed: false,
              internalType: 'address',
              name: 'implementation',
              type: 'address',
            },
          ],
          name: 'Upgraded',
          type: 'event',
        },
      ],
      bytecode: TRANSPARENT_PROXY_BYTECODE,
      deployedBytecode: TRANSPARENT_PROXY_BYTECODE,
    }
  }
}

export const deployWithOZTransparentProxy = async ({
  hre,
  name,
  args,
  options,
}: {
  hre: any
  name: string
  args: any[]
  options?: any
}): Promise<{
  contract: Contract
  newDeploy: boolean
}> => {
  const { deployer } = await hre.getNamedAccounts()
  const proxyName = `Proxy__${name}`
  const deployment = await hre.deployments.getOrNull(proxyName)
  let implDeployment = await hre.deployments.getOrNull(name)

  const contractFactory = await hre.ethers.getContractFactory(
    name,
    await hre.ethers.getSigner(deployer)
  )

  let contract: Contract
  let newDeploy = false

  if (!deployment) {
    // Deploy new implementation and proxy
    console.log(`Deploying ${name} with proxy...`)
    contract = await upgrades.deployProxy(contractFactory, args, {
      kind: 'transparent',
      initializer: 'initialize',
      ...options,
    })
    await contract.deployed()

    // Get implementation deployment info from OpenZeppelin cache
    const implInfo = await getImplDeploymentInfo(hre, contract.address)
    if (!implInfo) {
      throw new Error(
        'Failed to get implementation deployment info from OpenZeppelin cache'
      )
    }

    console.log(`Implementation of ${name} deployed at ${implInfo.address}`)
    console.log(`Proxy of ${name} deployed at ${contract.address}`)

    // Save implementation deployment
    const implArtifact = await hre.deployments.getExtendedArtifact(name)
    implDeployment = {
      ...implArtifact,
      address: implInfo.address,
      transactionHash: implInfo.txHash,
      receipt: await hre.ethers.provider.getTransactionReceipt(implInfo.txHash),
      args: options && options.constructorArgs ? options.constructorArgs : [],
      implementation: undefined,
    }
    await hre.deployments.save(name, implDeployment)

    // Save proxy deployment
    const proxyArtifact = await getProxyArtifact(hre, name)
    const proxyDeployment = {
      ...proxyArtifact,
      address: contract.address,
      transactionHash: contract.deployTransaction.hash,
      receipt: await contract.deployTransaction.wait(
        hre.deployConfig.numDeployConfirmations
      ),
      args,
      implementation: implInfo.address,
    }
    await hre.deployments.save(proxyName, proxyDeployment)

    newDeploy = true
  } else {
    // Check if implementation needs upgrade by comparing bytecode
    const artifact = await hre.deployments.getArtifact(name)
    const currentImplAddress = await upgrades.erc1967.getImplementationAddress(
      deployment.address
    )
    const currentDeployment = await hre.deployments.get(name)

    if (artifact.bytecode !== currentDeployment.bytecode) {
      console.log(`Implementation bytecode changed for ${name}`)
      console.log(`Current implementation at ${currentImplAddress}`)
      console.log(`Upgrading ${name} implementation...`)

      contract = await upgrades.upgradeProxy(
        deployment.address,
        contractFactory,
        {
          kind: 'transparent',
          ...options,
        }
      )
      await contract.deployed()

      // Get implementation deployment info from OpenZeppelin cache
      const implInfo = await getImplDeploymentInfo(hre, contract.address)
      if (!implInfo) {
        throw new Error(
          'Failed to get implementation deployment info from OpenZeppelin cache'
        )
      }

      console.log(`Implementation of ${name} upgraded to ${implInfo.address}`)
      console.log(`Reusing proxy of ${name} at ${deployment.address}`)

      // Save new implementation deployment
      const implArtifact = await hre.deployments.getExtendedArtifact(name)
      implDeployment = {
        ...implArtifact,
        address: implInfo.address,
        transactionHash: implInfo.txHash,
        receipt: await hre.ethers.provider.getTransactionReceipt(
          implInfo.txHash
        ),
        args: options && options.constructorArgs ? options.constructorArgs : [],
        implementation: undefined,
      }
      await hre.deployments.save(name, implDeployment)
    } else {
      console.log(`Reusing proxy ${name} at ${deployment.address}`)
      console.log(`Reusing implementation ${name} at ${currentImplAddress}`)
      contract = await contractFactory.attach(deployment.address)
    }
  }

  return { contract, newDeploy }
}
