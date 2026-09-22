# OVM accounting audit during full sync

The audit follows normal block execution from genesis and measures changes in
`sum(OVM_ETH balances) - OVM_ETH.totalSupply`. It attributes changes to the
historical gas shortfall and pre-SDUpdate SELFDESTRUCT paths, and reports any
remaining unexplained changes. It does not alter execution or relax migration
checks.

## Run a dedicated node

Use a **new datadir**, the correct Metis genesis, and your normal mainnet network
and peer configuration. Preserve the existing node and its data. These examples
assume `geth` is the newly built binary and `genesis.json` is the verified mainnet
genesis used by your nodes:

```sh
geth --datadir /data/ovm-audit-node init /config/genesis.json

geth --datadir /data/ovm-audit-node \
  --networkid 1088 --syncmode full --gcmode full \
  --ovm.audit \
  --ovm.audit.to 23182122 \
  --ovm.audit.to-hash 0x31ccdca9cb607cdde996f7a8a82519bc873ffff68b898a3d8472069fc5724226 \
  --ovm.audit.dir /data/ovm-audit-reports
```

Include your existing P2P peer configuration in the second command. Peers must
serve the historical blocks and transaction metadata needed for full execution.
This mode does not use the rollup sequencer/miner path. Do not pass `--mine`,
`--dev`, or `--exitwhensynced`.

`--syncmode full` is required explicitly: this client otherwise defaults to fast
sync, which skips historical execution. Automatic fallback to fast sync is
disabled during auditing. **Archive GC is not required**; `--gcmode full` retains
the node's normal state caching, persistence and recovery behavior. Historical
audit records and classification evidence consume additional disk space, but
the audit does not retain every historical state. It persists the single
requested target's state when that block executes, including on a sidechain,
so a later reorg or restart can still scan that exact root after full GC.

The first audit run requires the full block, fast block and header heads all to
be at genesis. Enabling the audit on an already synchronized node cannot recover
the missing history. Audit configuration can also be saved under `[Eth.OVMAudit]`
in TOML (`Enabled`, `To`, `ToHash`, `Dir`, `Witness`); the explicit CLI full-sync
requirement still applies. CLI audit flags override corresponding TOML values.

## State and evidence

Startup independently scans genesis OVM_ETH storage. It uses authenticated
address/SHA3 preimages and supplemental witness records to classify balances,
allowances and metadata. During sync it learns from native balance operations,
EVM SHA3 operations and OVM Transfer/Approval events. An address can hold an OVM
balance even without an account leaf. Startup scans stored preimages twice to
resolve nested allowances independently of hash ordering, using bounded batches
on both passes.

The supported layout has balances at mapping slot 0, allowances at mapping slot
1, and totalSupply at slot 2. Metadata uses slots 3–6; long metadata strings are
bounded to 32 MiB. An unknown nonzero state slot or unclassified net storage
change stops auditing instead of silently excluding it. A completely reverted
unknown write does not affect accounting.

Optional `--ovm.audit.witness /config/witness.jsonl` supplies these records, one
per line, using 20-byte hexadecimal addresses:

```json
{"type":"address","address":"0x0000000000000000000000000000000000001234"}
{"type":"allowance","owner":"0x0000000000000000000000000000000000001234","spender":"0x0000000000000000000000000000000000005678"}
```

The witness provides mapping preimages, not balances or permission to ignore
unknown state. Slot keys are recomputed. Its exact bytes are hashed into the run
identity. A change to the witness, genesis, execution configuration, target or
audit format requires a new audit run. The run identity also records the
effective sequencer-related environment configuration.

## Recovery and errors

Restart with the same arguments and datadir. There is no separate resume flag.
Audit records are stored in a versioned chaindata namespace, atomically with each
block's body and receipts. A block contributes only after it belongs to the
executed canonical chain. Restart rewinds, repeated execution and chain reorgs
are reconciled by block hash. Sidechain records never contribute to the final
canonical report.

The cached progress is written every 1,000 canonical blocks; logs report progress
at approximately 30-second intervals while work is advancing. The cache may lag
or lead the node after a crash and is reconciled from block records. JSON reports
are derived outputs and can be regenerated; do not edit chaindata audit keys.

Storage failures, evidence gaps, incompatible restarts and target mismatches
stop the audit node with a nonzero exit status. They are local audit failures,
not invalid-block reports or reasons to penalize a peer. `failure.json` captures
the failure and last reconciled position when the report filesystem is writable.
After fixing disk availability, restart using the same options. If classification
requires a different witness, start a fresh audit with that corrected input.

Normal signals use the node's existing shutdown path. A restart may reexecute
blocks whose state was still in the normal full-node cache; retained audit
records must match exactly when those blocks execute again. A signal during
final report export leaves no completed replacement file; rerun to regenerate.

## Completion and interpretation

At the requested canonical height, import pauses, the block hash is checked, and
the target OVM storage is scanned independently. The audit checks balance and
supply components separately, as well as:

```text
D_target - D_genesis
  = sum(canonical transaction discrepancy deltas)
  + sum(canonical block-level discrepancy deltas)
```

A heavier branch can become canonical only after its tip passes the requested
height. In that case the node audits the target ancestor and its retained state;
post-target blocks are excluded from totals and reports. The same arguments can
resume this run even when the executed head is above the target. The requested
hash must still match the target on the executed head's ancestry.

The node then writes the following files and exits successfully:

- `run.json`: genesis, execution configuration and immutable audit inputs.
- `events.jsonl`: canonical anomalous transactions/block scopes and candidate
  cause events, including reverted SELFDESTRUCT events. Amounts are signed
  decimal wei strings. Receipts that failed execution can still contribute gas
  accounting changes.
- `summary.json`: independently scanned initial/target state totals, coverage,
  gas/SELFDESTRUCT attribution and unexplained amounts. `progress.json`, when
  present, is a periodic cache and is not the final report.

`executionVerified`, `accountingReconciled`, and
`intervalCausesFullyExplained` are separate results. A successful audit can still
contain unexplained changes. Opposite unexplained changes do not cancel the
per-event completeness indicator. A nonzero genesis difference is reported
separately and cannot be explained by later transactions.

For the explicitly bound block 23182122/hash above, the report also compares the
observed difference with the prior diagnosis of **39864711177577535 wei**. This
comparison is informational; the amount is not a general allowance or a
consensus rule.

Final scanning and report export can take time and require free disk space.
Reports are published by temporary-file rename after syncing their contents;
`summary.json` is published last. Header synchronization may have downloaded
headers above the target. Normal import stops execution at the target; a reorg
that first makes the target canonical at a later tip stops after that promotion.

## Validation

From the `l2geth` directory:

```sh
go test ./core/state ./core ./cmd/geth ./eth -run TestOVMAudit -count=1
go test -race ./core/state ./core ./cmd/geth ./eth -run TestOVMAudit -count=1
go build ./cmd/geth
git diff --check
```

Local fixtures validate accounting, actual EVM behavior, import, reorg, restart
and injected storage failures. They do not establish the cause of the mainnet
difference. First validate a short real synchronization interval and restart;
then run the full target audit. Record elapsed synchronization time, peak memory
and datadir/report sizes when comparing audited and unaudited runs.
