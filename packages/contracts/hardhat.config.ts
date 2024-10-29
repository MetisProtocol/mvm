import 'solidity-coverage'
import * as dotenv from 'dotenv'

import {
  DEFAULT_ACCOUNTS_HARDHAT,
  RUN_OVM_TEST_GAS,
} from './test/helpers/constants'

// Hardhat plugins
import '@nomiclabs/hardhat-ethers'
import '@nomiclabs/hardhat-waffle'
import '@nomiclabs/hardhat-etherscan'
import 'hardhat-deploy'
import '@typechain/hardhat'
import './tasks/deploy'
import './tasks/l2-gasprice'
import './tasks/set-owner'
import './tasks/whitelist'
import './tasks/withdraw-fees'
import 'hardhat-gas-reporter'

// Load environment variables from .env
dotenv.config()

const enableGasReport = !!process.env.ENABLE_GAS_REPORT
const privateKey =
  process.env.PRIVATE_KEY ||
  '0x0000000000000000000000000000000000000000000000000000000000000000' // this is to avoid hardhat error

const config = {
  networks: {
    hardhat: {
      accounts: DEFAULT_ACCOUNTS_HARDHAT,
      blockGasLimit: RUN_OVM_TEST_GAS * 2,
      live: false,
      saveDeployments: false,
      tags: ['local'],
    },
    andromeda: {
      url: 'http://andromeda.metis.io/?owner=1088',
      saveDeployments: false,
    },
    stardust: {
      chainId: 588,
      url: 'https://stardust.metis.io/?owner=588',
      accounts: [privateKey],
    },
    goerli: {
      chainId: 599,
      url: 'https://goerli.metis.io/?owner=589',
      accounts: [privateKey],
    },
    trial: {
      chainId: 666,
      url: 'https://dev.metis.io/?owner=666',
      accounts: [privateKey],
    },
    custom: {
      chainId: 3151908,
      url: 'http://127.0.0.1:32845',
      accounts: [
        '0xbcdf20249abf0ed6d944c0288fad489e33f66b3960d9e6229c1cd214ed3bbe31',
        '0x39725efee3fb28614de3bacaffe4cc4bd8c436257e2c8bb887c4b5c4be45e76d',
        '0x53321db7c1e331d93a11a41d16f004d7ff63972ec8ec7c25db329728ceeb1710',
        '0xab63b23eb7941c1251757e24b3d2350d2bc05c3c388d06f8fe6feafefb1e8c70',
      ],
    },
  },
  mocha: {
    timeout: 50000,
  },
  solidity: {
    compilers: [
      {
        version: '0.8.9',
        settings: {
          optimizer: { enabled: true, runs: 10_000 },
        },
      },
      {
        version: '0.5.17', // Required for WETH9
        settings: {
          optimizer: { enabled: true, runs: 10_000 },
        },
      },
    ],
    settings: {
      metadata: {
        bytecodeHash: 'none',
      },
      outputSelection: {
        '*': {
          '*': ['metadata', 'storageLayout'],
        },
      },
    },
  },

  typechain: {
    outDir: 'dist/types',
    target: 'ethers-v5',
  },
  paths: {
    deploy: './deploy',
    deployments: './deployments',
  },
  namedAccounts: {
    deployer: {
      default: 0,
    },
  },
  gasReporter: {
    enabled: enableGasReport,
    currency: 'USD',
    gasPrice: 100,
    outputFile: process.env.CI ? 'gas-report.txt' : undefined,
  },
  etherscan: {
    apiKey: process.env.ETHERSCAN_API_KEY,
  },
}

if (
  process.env.CONTRACTS_TARGET_NETWORK &&
  process.env.CONTRACTS_DEPLOYER_KEY &&
  process.env.CONTRACTS_RPC_URL
) {
  config.networks[process.env.CONTRACTS_TARGET_NETWORK] = {
    accounts: [process.env.CONTRACTS_DEPLOYER_KEY],
    url: process.env.CONTRACTS_RPC_URL,
    live: true,
    saveDeployments: true,
    tags: [process.env.CONTRACTS_TARGET_NETWORK],
  }
}

export default config
