## Go Ethereum

Official Golang implementation of the Ethereum protocol.

## Optimism

The same codebase is used to run both the Sequencer. Runtime
configuration will determine the mode of operation. The configuration flags
can be configured using either environment variables or passed at runtime as
flags.

A prebuilt Docker image is available at `metisdao/l2geth`.

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
