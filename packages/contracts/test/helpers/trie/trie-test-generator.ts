/* External Imports */
import { ethers } from 'ethers'
import { BaseTrie, SecureTrie } from 'merkle-patricia-tree'
import { default as seedbytes } from 'random-bytes-seed'
import * as rlp from 'rlp'

export const fromHexString = (inp: Buffer | string): Buffer => {
  if (typeof inp !== 'string') {
    return inp
  }

  if (typeof inp === 'string' && inp.startsWith('0x')) {
    return Buffer.from(inp.slice(2), 'hex')
  }

  return Buffer.from(inp)
}

export const toHexString = (inp: Buffer | string | number | null): string => {
  if (typeof inp === 'number') {
    return '0x' + BigInt(inp).toString(16)
  } else {
    return '0x' + fromHexString(inp).toString('hex')
  }
}

export interface TrieNode {
  key: string
  val: string
}

export interface InclusionProofTest {
  key: string
  val: string
  proof: string
  root: string
}

export interface NodeUpdateTest extends InclusionProofTest {
  newRoot: string
}

export interface EthereumAccount {
  address?: string
  nonce: number
  balance: number
  codeHash: string
  storageRoot?: string
  storage?: TrieNode[]
}

export interface AccountProofTest {
  address: string
  account: EthereumAccount
  accountTrieWitness: string
  accountTrieRoot: string
}

export interface AccountUpdateTest extends AccountProofTest {
  newAccountTrieRoot: string
}

const rlpEncodeAccount = (account: EthereumAccount): string => {
  return (
    '0x' +
    rlp
      .encode([
        account.nonce,
        account.balance,
        account.storageRoot || ethers.constants.HashZero,
        account.codeHash || ethers.constants.HashZero,
      ])
      .toString('hex')
  )
}

const rlpDecodeAccount = (encoded: string): EthereumAccount => {
  const decoded = rlp.decode(fromHexString(encoded)) as any
  return {
    nonce: decoded[0].length ? parseInt(decoded[0], 16) : 0,
    balance: decoded[1].length ? parseInt(decoded[1], 16) : 0,
    storageRoot: decoded[2].length
      ? toHexString(decoded[2])
      : ethers.constants.HashZero,
    codeHash: decoded[3].length
      ? toHexString(decoded[3])
      : ethers.constants.HashZero,
  }
}

const makeTrie = async (
  nodes: TrieNode[],
  secure?: boolean
): Promise<{
  trie: SecureTrie | BaseTrie
  TrieClass: any
}> => {
  const TrieClass = secure ? SecureTrie : BaseTrie
  const trie = new TrieClass()

  for (const node of nodes) {
    await trie.put(fromHexString(node.key), fromHexString(node.val))
  }

  return {
    trie,
    TrieClass,
  }
}

export class TrieTestGenerator {
  constructor(
    public _TrieClass: any,
    public _trie: SecureTrie | BaseTrie,
    public _nodes: TrieNode[],
    public _subGenerators?: TrieTestGenerator[]
  ) {}

  static async fromNodes(opts: {
    nodes: TrieNode[]
    secure?: boolean
  }): Promise<TrieTestGenerator> {
    const { trie, TrieClass } = await makeTrie(opts.nodes, opts.secure)

    return new TrieTestGenerator(TrieClass, trie, opts.nodes)
  }

  static async fromRandom(opts: {
    seed: string
    nodeCount: number
    secure?: boolean
    keySize?: number
    valSize?: number
  }): Promise<TrieTestGenerator> {
    const getRandomBytes = seedbytes(opts.seed)
    const nodes: TrieNode[] = [...Array(opts.nodeCount)].map(() => {
      return {
        key: toHexString(getRandomBytes(opts.keySize || 32)),
        val: toHexString(getRandomBytes(opts.valSize || 32)),
      }
    })

    return TrieTestGenerator.fromNodes({
      nodes,
      secure: opts.secure,
    })
  }

  static async fromAccounts(opts: {
    accounts: EthereumAccount[]
    secure?: boolean
  }): Promise<TrieTestGenerator> {
    const subGenerators: TrieTestGenerator[] = []

    for (const account of opts.accounts) {
      if (account.storage) {
        const subGenerator = await TrieTestGenerator.fromNodes({
          nodes: account.storage,
          secure: opts.secure,
        })

        account.storageRoot = toHexString(subGenerator._trie.root)
        subGenerators.push(subGenerator)
      }
    }

    const nodes = opts.accounts.map((account) => {
      return {
        key: account.address,
        val: rlpEncodeAccount(account),
      }
    })

    const { trie, TrieClass } = await makeTrie(nodes, opts.secure)

    return new TrieTestGenerator(TrieClass, trie, nodes, subGenerators)
  }

  public async makeInclusionProofTest(
    key: string | number
  ): Promise<InclusionProofTest> {
    if (typeof key === 'number') {
      key = this._nodes[key].key
    }

    const trie = this._trie.copy()

    const proof = await this.prove(key)
    const val = await trie.get(fromHexString(key))

    return {
      proof: toHexString(rlp.encode(proof)),
      key: toHexString(key),
      val: toHexString(val),
      root: toHexString(trie.root),
    }
  }

  public async makeAllInclusionProofTests(): Promise<InclusionProofTest[]> {
    return Promise.all(
      this._nodes.map(async (node) => {
        return this.makeInclusionProofTest(node.key)
      })
    )
  }

  public async makeNodeUpdateTest(
    key: string | number,
    val: string
  ): Promise<NodeUpdateTest> {
    if (typeof key === 'number') {
      key = this._nodes[key].key
    }

    const trie = this._trie.copy()

    const proof = await this.prove(key)
    const oldRoot = trie.root

    await trie.put(fromHexString(key), fromHexString(val))
    const newRoot = trie.root

    return {
      proof: toHexString(rlp.encode(proof)),
      key: toHexString(key),
      val: toHexString(val),
      root: toHexString(oldRoot),
      newRoot: toHexString(newRoot),
    }
  }

  public async makeAccountProofTest(
    address: string | number
  ): Promise<AccountProofTest> {
    if (typeof address === 'number') {
      address = this._nodes[address].key
    }

    const trie = this._trie.copy()

    const proof = await this.prove(address)
    const account = await trie.get(fromHexString(address))

    return {
      address,
      account: rlpDecodeAccount(toHexString(account)),
      accountTrieWitness: toHexString(rlp.encode(proof)),
      accountTrieRoot: toHexString(trie.root),
    }
  }

  public async makeAccountUpdateTest(
    address: string | number,
    account: EthereumAccount
  ): Promise<AccountUpdateTest> {
    if (typeof address === 'number') {
      address = this._nodes[address].key
    }

    const trie = this._trie.copy()

    const proof = await this.prove(address)
    const oldRoot = trie.root

    await trie.put(
      fromHexString(address),
      fromHexString(rlpEncodeAccount(account))
    )
    const newRoot = trie.root

    return {
      address,
      account,
      accountTrieWitness: toHexString(rlp.encode(proof)),
      accountTrieRoot: toHexString(oldRoot),
      newAccountTrieRoot: toHexString(newRoot),
    }
  }

  private async prove(key: string): Promise<any> {
    return this._TrieClass.prove(this._trie, fromHexString(key))
  }
}
