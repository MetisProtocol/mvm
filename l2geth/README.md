## Go Ethereum

Official Golang implementation of the Ethereum protocol.

## Optimism

The same codebase is used to run both the Sequencer. Runtime
configuration will determine the mode of operation. The configuration flags
can be configured using either environment variables or passed at runtime as
flags.

A prebuilt Docker image is available at `metisdao/l2geth`.

### P2P block source whitelist

Use `--p2p.whitelist /path/to/block-peers.json` to restrict which peers can supply
blocks for import. Rules are selected using the final **network ID** (including
`--networkid` and `[Eth].NetworkId`), not the genesis chain ID. For example:

```json
{
  "networks": [
    {
      "networkId": 1088,
      "peers": [
        "enode://<128-character-hex-public-key>@203.0.113.10:30303"
      ]
    },
    {"networkId": 59902, "peers": []}
  ]
}
```

Each entry is a complete `enode://PUBLIC_KEY@IP:PORT` URL, parsed with the
existing enode parser. Replace the example public key with the peer's actual
128-character hexadecimal public key. Key-only enodes, account addresses,
private keys, hashed node IDs, hostnames and CIDR ranges are not accepted.
For IPv6 use brackets, for example `enode://PUBLIC_KEY@[2001:db8::1]:30303`.
TCP ports and optional `?discport=...` values must be valid enode syntax but do
not participate in whitelist matching.

Each enode binds one exact IP address to one authenticated node identity.
Repeat a key with different IPs when necessary. IP matching uses the actual TCP
remote address, so configure the address visible to this node after NAT, not a
peer's advertised address. IPv4-mapped IPv6 addresses match their IPv4 equivalents.
The former `{ "ip": "...", "publicKey": "..." }` entry format is not accepted.

```sh
geth --networkid 1088 --p2p.whitelist /path/to/block-peers.json
```

The path can also be set as `BlockPeerWhitelistFile` in the `[Eth]` TOML section;
an explicitly supplied CLI flag overrides it. Relative paths use the process
working directory. The entire JSON file is validated at startup and loaded once;
restart to apply changes. Invalid files, unknown fields, invalid entries and
duplicate network IDs stop startup, even if the invalid entry belongs to another
network. An omitted file or network leaves block sources unrestricted. An
explicit empty `peers` array rejects all P2P block sources for that network.
The startup log reports the selected network, whether the supplied policy is
active, and its rule count.

This controls full/fast **block import sources**, including broadcast blocks,
announcements, headers, bodies, receipts and state downloaded during sync. It
does not disconnect non-whitelisted peers or prevent serving their requests,
broadcasting blocks/transactions, or UDP discovery. Static/trusted peers have no
exemption. With no eligible source, P2P sync waits without falling back to other
peers. Existing connection limits, protocol checks and `--netrestrict` still
apply. Light-client mode rejects an active policy rather than ignoring it.

The whitelist authenticates the immediate transmitting peer, not the original
block producer. Blocks forwarded by an allowed peer still undergo normal
consensus and checkpoint validation. Transaction handling, local block creation,
file imports and rollup data sources keep their existing behavior. The whitelist
does not add peers automatically or support runtime/RPC updates.

### Andromeda block hash checkpoints

Chain ID `1088` enforces 232 hardcoded block hashes, from block `0` through
`23,100,000` inclusive at intervals of `100,000` blocks. The hashes were fetched
from `https://andromeda.metis.io` on 2026-09-20 and recomputed using this client's
header RLP encoding. The table is in `params/block_checkpoints.go`; its offline
header fixtures are in `params/testdata/andromeda-checkpoints.json`.

Block and header imports, fast-sync receipt imports, and local block writes reject
conflicting checkpoints before committing them. Known-block shortcuts and
promotion of stored side chains also enforce the checkpoints. Startup checks all
existing canonical checkpoint hashes in both the hot database and ancient store
before header/block chain recovery or rewinding. A conflict stops startup with
the height, expected hash and actual hash; it does not automatically roll back
the database.
Check the chain configuration and database source before replacing inconsistent
data or resyncing from a known-good source.

Genesis setup and commit (`genesis.go`) do not perform checkpoint validation;
their existing configuration and database compatibility checks still apply.
Block `0` remains in the checkpoint table and is checked when opening the
header/block chain, alongside the other checkpoints. These checks are mandatory
for every chain using ID `1088`, including custom genesis configurations that
reuse it. Other chain IDs and heights without a checkpoint keep their existing
validation rules. Missing checkpoints are allowed
while syncing. Checkpoints supplement execution and consensus validation; they do
not skip those checks or extend coverage beyond the last recorded height.
Neither runtime validation nor automated tests contact the RPC endpoint.

To extend coverage, fetch `eth_getBlockByNumber` with full transactions disabled
for each additional multiple of `100,000`. Verify the returned number and 32-byte
hash, decode each response as `types.Header`, and require `Header.Hash()` to match
the RPC hash. Append the verified hashes and header fixtures, update the last
checkpoint constant, snapshot count assertions and acquisition date, and run
`go test ./params ./core -run Checkpoint` from this directory. Updating checkpoints
requires a code release; there is no runtime override or disable flag.

### OVM_ETH migration address records

While OVM mode is active (`rcfg.UsingOVM`), balance setters and standard
OVM_ETH `Transfer` logs automatically record the involved addresses as SHA3
preimages. This includes zero amounts, mint/burn addresses and holders with no
account leaf. No additional flag is needed; VM SHA3 preimage recording may remain
disabled. Other contracts' events and `Approval` events do not add these records.

Normal block insertion persists each record as
`secure-key- || keccak256(address[20]) -> address[20]`. These auxiliary records
do not affect state roots, gas or receipts. They use the existing state journal
and roll back with reverted execution. Records deduplicate within a block;
repeated addresses may be written again in later blocks, and unique addresses
increase database storage. Entries already persisted for a non-canonical block
may remain as harmless address candidates.

The migration tool can read these records to locate OVM_ETH balance slots. They
do not establish ERC20 retention eligibility or guarantee that a state witness
is unnecessary. Upgrading only records addresses touched by subsequent execution;
it does not backfill old balances, untouched holders or missing initial-state
addresses. Historical replay or separate evidence is still needed for those
gaps. This feature adds no history backfill command or allowance-pair index.

## Contribution

Thank you for considering to help out with the source code! We welcome contributions
from anyone on the internet, and are grateful for even the smallest of fixes!

If you'd like to contribute to go-ethereum, please fork, fix, commit and send a pull request
for the maintainers to review and merge into the main code base. If you wish to submit
more complex changes though, please check up with the core devs first on [our gitter channel](https://gitter.im/ethereum/go-ethereum)
to ensure those changes are in line with the general philosophy of the project and/or get
some early feedback which can make both your efforts much lighter as well as our review
and merge procedures quick and simple.

Please make sure your contributions adhere to our coding guidelines:

- Code must adhere to the official Go [formatting](https://golang.org/doc/effective_go.html#formatting)
  guidelines (i.e. uses [gofmt](https://golang.org/cmd/gofmt/)).
- Code must be documented adhering to the official Go [commentary](https://golang.org/doc/effective_go.html#commentary)
  guidelines.
- Pull requests need to be based on and opened against the `master` branch.
- Commit messages should be prefixed with the package(s) they modify.
  - E.g. "eth, rpc: make trace configs optional"

Please see the [Developers' Guide](https://github.com/MetisProtocol/l2geth/wiki/Developers'-Guide)
for more details on configuring your environment, managing project dependencies, and
testing procedures.

## License

The go-ethereum library (i.e. all code outside of the `cmd` directory) is licensed under the
[GNU Lesser General Public License v3.0](https://www.gnu.org/licenses/lgpl-3.0.en.html),
also included in our repository in the `COPYING.LESSER` file.

The go-ethereum binaries (i.e. all code inside of the `cmd` directory) is licensed under the
[GNU General Public License v3.0](https://www.gnu.org/licenses/gpl-3.0.en.html), also
included in our repository in the `COPYING` file.
