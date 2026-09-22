package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/consensus"
	"github.com/MetisProtocol/mvm/l2geth/consensus/ethash"
	"github.com/MetisProtocol/mvm/l2geth/core/rawdb"
	"github.com/MetisProtocol/mvm/l2geth/core/state"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/core/vm"
	"github.com/MetisProtocol/mvm/l2geth/ethdb"
	"github.com/MetisProtocol/mvm/l2geth/params"
	"github.com/MetisProtocol/mvm/l2geth/rollup/dump"
	"github.com/MetisProtocol/mvm/l2geth/rollup/rcfg"
	"github.com/ethereum/go-ethereum/crypto"
)

// No rewards, like Clique, but deterministic headers for import tests.
type auditTestEngine struct{ consensus.Engine }

func (e auditTestEngine) Finalize(c consensus.ChainReader, h *types.Header, s *state.StateDB, txs []*types.Transaction, u []*types.Header) {
	h.Root = s.IntermediateRoot(c.Config().IsEIP158(h.Number))
	h.UncleHash = types.CalcUncleHash(u)
}
func (e auditTestEngine) FinalizeAndAssemble(c consensus.ChainReader, h *types.Header, s *state.StateDB, txs []*types.Transaction, u []*types.Header, r []*types.Receipt) (*types.Block, error) {
	e.Finalize(c, h, s, txs, u)
	return types.NewBlock(h, txs, u, r), nil
}

func auditFixture(t *testing.T, n int) (*Genesis, types.Blocks) {
	t.Helper()
	old := rcfg.UsingOVM
	rcfg.UsingOVM = true
	t.Cleanup(func() { rcfg.UsingOVM = old })
	cfg := *params.TestChainConfig
	cfg.ChainID = big.NewInt(1337)
	cfg.Clique = &params.CliqueConfig{Period: 1, Epoch: 30000}
	cfg.ShanghaiBlock = nil
	key, _ := crypto.HexToECDSA("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	from := common.BytesToAddress(crypto.PubkeyToAddress(key.PublicKey).Bytes())
	gen := &Genesis{Config: &cfg, Difficulty: big.NewInt(1), GasLimit: 8000000, Alloc: GenesisAlloc{
		dump.OvmEthAddress:           {Balance: big.NewInt(0), Code: []byte{0}},
		rcfg.L2GasPriceOracleAddress: {Balance: big.NewInt(0), Code: []byte{0}, Storage: map[common.Hash]common.Hash{rcfg.L2GasPriceSlot: common.BigToHash(big.NewInt(1))}},
		from:                         {Balance: big.NewInt(0)},
	}}
	db := rawdb.NewMemoryDatabase()
	defer db.Close()
	genesis := gen.MustCommit(db)
	blocks, _ := GenerateChain(&cfg, genesis, auditTestEngine{ethash.NewFaker()}, db, n, func(i int, b *BlockGen) {
		tx, err := types.SignTx(types.NewTransaction(uint64(i), common.Address{9}, big.NewInt(0), 21000, big.NewInt(1), nil), types.NewEIP155Signer(cfg.ChainID), key)
		if err != nil {
			t.Fatal(err)
		}
		b.AddTx(tx)
	})
	return gen, blocks
}

func auditChain(t *testing.T, gen *Genesis, db ethdb.Database) *BlockChain {
	t.Helper()
	if rawdb.ReadHeadBlockHash(db) == (common.Hash{}) {
		gen.MustCommit(db)
	}
	bc, err := NewBlockChain(db, &CacheConfig{TrieCleanLimit: 16, TrieDirtyLimit: 16, TrieTimeLimit: time.Second, TrieCleanNoPrefetch: true}, gen.Config, auditTestEngine{ethash.NewFaker()}, vm.Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(bc.Stop)
	return bc
}

func TestOVMAuditFullImport(t *testing.T) {
	for _, channel := range []bool{false, true} {
		t.Run(fmt.Sprint(channel), func(t *testing.T) {
			gen, blocks := auditFixture(t, 4)
			db := rawdb.NewMemoryDatabase()
			t.Cleanup(func() { db.Close() })
			bc := auditChain(t, gen, db)
			cfg := OVMAuditConfig{Enabled: true, To: 3, ToHash: blocks[2].Hash(), Dir: t.TempDir()}
			if err := bc.EnableOVMAudit(cfg); err != nil {
				t.Fatal(err)
			}
			var err error
			if channel {
				_, err = bc.InsertChain(blocks)
			} else {
				bc.chainmu.Lock()
				_, err = bc.insertChain(blocks, true)
				bc.chainmu.Unlock()
			}
			if !IsOVMAuditControl(err) || bc.OVMAuditError() != nil {
				t.Fatalf("%v / %v", err, bc.OVMAuditError())
			}
			if bc.CurrentBlock().Hash() != blocks[2].Hash() {
				t.Fatal("passed target")
			}
			if bc.HasBlock(blocks[3].Hash(), 4) {
				t.Fatal("imported block after target")
			}
			select {
			case <-bc.OVMAuditDone():
			default:
				t.Fatal("no completion")
			}
			blob, err := os.ReadFile(filepath.Join(cfg.Dir, "summary.json"))
			if err != nil {
				t.Fatal(err)
			}
			var summary struct {
				Accounting bool                `json:"accountingReconciled"`
				Total      state.OVMAuditDelta `json:"totals"`
				End        state.OVMAuditScan  `json:"targetState"`
			}
			if err = json.Unmarshal(blob, &summary); err != nil {
				t.Fatal(err)
			}
			// Each zero-funded sender receives 21000 gas, all credited to fee wallet.
			if !summary.Accounting || summary.Total.Gas != "63000" || summary.Total.Difference != "63000" || summary.Total.Unexplained != "0" || summary.End.Difference != "63000" {
				t.Fatalf("%+v", summary)
			}
		})
	}
}

func TestOVMAuditEmptyCodeSize(t *testing.T) {
	gen, _ := auditFixture(t, 0)
	contract := common.Address{9}
	// Return EXTCODESIZE(CALLER). The transaction sender is an existing EOA
	// whose empty-code hash has no corresponding blob in the database.
	gen.Alloc[contract] = GenesisAccount{Balance: new(big.Int), Code: []byte{
		byte(vm.CALLER), byte(vm.EXTCODESIZE), 0x60, 0, byte(vm.MSTORE),
		0x60, 32, 0x60, 0, byte(vm.RETURN),
	}}
	key, err := crypto.HexToECDSA("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	chainDB := rawdb.NewMemoryDatabase()
	t.Cleanup(func() { chainDB.Close() })
	blocks, _ := GenerateChain(gen.Config, gen.MustCommit(chainDB), auditTestEngine{ethash.NewFaker()}, chainDB, 1, func(_ int, b *BlockGen) {
		tx, err := types.SignTx(types.NewTransaction(0, contract, new(big.Int), 60000, big.NewInt(1), nil), types.NewEIP155Signer(gen.Config.ChainID), key)
		if err != nil {
			t.Fatal(err)
		}
		b.AddTx(tx)
	})
	db, err := rawdb.NewLevelDBDatabase(t.TempDir(), 16, 16, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	bc := auditChain(t, gen, db)
	if err = bc.EnableOVMAudit(OVMAuditConfig{Enabled: true, To: 1, ToHash: blocks[0].Hash(), Dir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	if _, err = bc.InsertChain(blocks); !IsOVMAuditControl(err) || bc.OVMAuditError() != nil {
		t.Fatalf("EXTCODESIZE of an EOA stopped the audit: %v / %v", err, bc.OVMAuditError())
	}
	if bc.CurrentBlock().Hash() != blocks[0].Hash() || bc.ovmAudit.cursor.Total.Difference != "60000" {
		t.Fatal("audit failed to import and reconcile the transaction")
	}
}

func TestOVMAuditResumeAndGap(t *testing.T) {
	gen, blocks := auditFixture(t, 4)
	db := rawdb.NewMemoryDatabase()
	t.Cleanup(func() { db.Close() })
	bc := auditChain(t, gen, db)
	cfg := OVMAuditConfig{Enabled: true, To: 4, ToHash: blocks[3].Hash(), Dir: t.TempDir()}
	if err := bc.EnableOVMAudit(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := bc.InsertChain(blocks[:2]); err != nil {
		t.Fatal(err)
	}
	// Crash-safe cursor may lag the chain. Rebuild solely from durable records.
	bc.Stop()
	bc = auditChain(t, gen, db)
	if err := bc.EnableOVMAudit(cfg); err != nil {
		t.Fatal(err)
	}
	if bc.ovmAudit.cursor.Number != 2 || bc.ovmAudit.cursor.Total.Difference != "42000" {
		t.Fatalf("%+v", bc.ovmAudit.cursor)
	}
	changed := cfg
	changed.ToHash = common.Hash{1}
	if err := bc.EnableOVMAudit(changed); err == nil {
		t.Fatal("accepted changed target")
	}
	if err := db.Delete(auditBlockKey(blocks[0].Hash())); err != nil {
		t.Fatal(err)
	}
	bc.ovmAudit.cursor = ovmAuditCursor{Hash: bc.Genesis().Hash(), Total: state.ZeroOVMAuditDelta()}
	if err := bc.ovmAudit.reconcile(bc.CurrentBlock()); err == nil {
		t.Fatal("missing coverage accepted")
	}
}

func TestOVMAuditTargetAndReportFailure(t *testing.T) {
	for _, mode := range []string{"target", "report"} {
		t.Run(mode, func(t *testing.T) {
			gen, blocks := auditFixture(t, 2)
			db := rawdb.NewMemoryDatabase()
			t.Cleanup(func() { db.Close() })
			bc := auditChain(t, gen, db)
			cfg := OVMAuditConfig{Enabled: true, To: 1, ToHash: blocks[0].Hash(), Dir: t.TempDir()}
			if mode == "target" {
				cfg.ToHash = common.Hash{1}
			}
			if err := bc.EnableOVMAudit(cfg); err != nil {
				t.Fatal(err)
			}
			if mode == "report" {
				if err := os.Mkdir(filepath.Join(cfg.Dir, "events.jsonl"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			_, err := bc.InsertChain(blocks)
			if !IsOVMAuditControl(err) || bc.OVMAuditError() == nil {
				t.Fatalf("expected local audit failure, got %v", err)
			}
			if len(bc.BadBlocks()) != 0 {
				t.Fatal("audit failure marked a block invalid")
			}
			if bc.CurrentBlock().NumberU64() != 1 {
				t.Fatal("advanced after audit failure")
			}
		})
	}
}

func TestOVMAuditReorg(t *testing.T) {
	gen, oldChain := auditFixture(t, 2)
	key, _ := crypto.HexToECDSA("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	forkDB := rawdb.NewMemoryDatabase()
	defer forkDB.Close()
	fork, _ := GenerateChain(gen.Config, gen.MustCommit(forkDB), auditTestEngine{ethash.NewFaker()}, forkDB, 4, func(i int, b *BlockGen) {
		tx, err := types.SignTx(types.NewTransaction(uint64(i), common.Address{8}, big.NewInt(0), 21000, big.NewInt(2), nil), types.NewEIP155Signer(gen.Config.ChainID), key)
		if err != nil {
			t.Fatal(err)
		}
		b.AddTx(tx)
	})
	db := rawdb.NewMemoryDatabase()
	t.Cleanup(func() { db.Close() })
	bc := auditChain(t, gen, db)
	cfg := OVMAuditConfig{Enabled: true, To: 4, ToHash: fork[3].Hash(), Dir: t.TempDir()}
	if err := bc.EnableOVMAudit(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := bc.InsertChain(oldChain); err != nil {
		t.Fatal(err)
	}
	if _, err := bc.InsertChain(fork); !IsOVMAuditControl(err) || bc.OVMAuditError() != nil {
		t.Fatalf("%v %v", err, bc.OVMAuditError())
	}
	if bc.ovmAudit.cursor.Total.Difference != "168000" {
		t.Fatalf("fork totals: %+v", bc.ovmAudit.cursor)
	}
	blob, err := os.ReadFile(filepath.Join(cfg.Dir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(blob, []byte(oldChain[0].Hash().Hex())) {
		t.Fatal("orphan included in report")
	}
}

func TestOVMAuditRewindAndReplay(t *testing.T) {
	gen, blocks := auditFixture(t, 4)
	db := rawdb.NewMemoryDatabase()
	t.Cleanup(func() { db.Close() })
	bc := auditChain(t, gen, db)
	cfg := OVMAuditConfig{Enabled: true, To: 4, ToHash: blocks[3].Hash(), Dir: t.TempDir()}
	if err := bc.EnableOVMAudit(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := bc.InsertChain(blocks[:2]); err != nil {
		t.Fatal(err)
	}
	blob, _ := json.Marshal(bc.ovmAudit.cursor)
	if err := db.Put(auditKey("cursor"), blob); err != nil {
		t.Fatal(err)
	}
	if err := bc.SetHead(1); err != nil {
		t.Fatal(err)
	}
	bc.Stop()
	bc = auditChain(t, gen, db)
	if err := bc.EnableOVMAudit(cfg); err != nil {
		t.Fatal(err)
	}
	if bc.ovmAudit.cursor.Number != 1 || bc.ovmAudit.cursor.Total.Difference != "21000" {
		t.Fatalf("rewind: %+v", bc.ovmAudit.cursor)
	}
	if _, err := bc.InsertChain(blocks[1:]); !IsOVMAuditControl(err) || bc.OVMAuditError() != nil {
		t.Fatalf("%v %v", err, bc.OVMAuditError())
	}
	if bc.ovmAudit.cursor.Total.Difference != "84000" {
		t.Fatal("replay double counted")
	}
}

type auditFaultDB struct {
	ethdb.Database
	fail string
}
type auditFaultBatch struct {
	ethdb.Batch
	db                 *auditFaultDB
	record, head, trie bool
}

func (d *auditFaultDB) NewBatch() ethdb.Batch {
	return &auditFaultBatch{Batch: d.Database.NewBatch(), db: d}
}
func (b *auditFaultBatch) Put(k, v []byte) error {
	b.record = b.record || bytes.HasPrefix(k, auditKey("block/"))
	b.head = b.head || string(k) == "LastBlock"
	b.trie = b.trie || len(k) == 32
	return b.Batch.Put(k, v)
}
func (b *auditFaultBatch) Write() error {
	if b.db.fail == "block" && b.record || b.db.fail == "head" && b.head || b.db.fail == "state" && b.trie && !b.record {
		b.db.fail = ""
		return fmt.Errorf("injected audit storage failure")
	}
	return b.Batch.Write()
}

func TestOVMAuditStorageFailures(t *testing.T) {
	for _, phase := range []string{"block", "state", "head"} {
		t.Run(phase, func(t *testing.T) {
			gen, blocks := auditFixture(t, 2)
			db := &auditFaultDB{Database: rawdb.NewMemoryDatabase()}
			t.Cleanup(func() { db.Close() })
			bc := auditChain(t, gen, db)
			cfg := OVMAuditConfig{Enabled: true, To: 2, ToHash: blocks[1].Hash(), Dir: t.TempDir()}
			if err := bc.EnableOVMAudit(cfg); err != nil {
				t.Fatal(err)
			}
			if phase == "state" {
				bc.cacheConfig.TrieDirtyDisabled = true
			}
			db.fail = phase
			_, err := bc.InsertChain(blocks)
			if !IsOVMAuditControl(err) || bc.OVMAuditError() == nil {
				t.Fatalf("%s: %v", phase, err)
			}
			if bc.CurrentBlock().NumberU64() != 0 || bc.ovmAudit.cursor.Number != 0 {
				t.Fatal("failed block counted")
			}
			if len(bc.BadBlocks()) != 0 {
				t.Fatal("local fault became bad block")
			}
			bc.Stop()
			bc = auditChain(t, gen, db)
			if err := bc.EnableOVMAudit(cfg); err != nil {
				t.Fatal(err)
			}
			if _, err := bc.InsertChain(blocks); !IsOVMAuditControl(err) || bc.OVMAuditError() != nil {
				t.Fatalf("resume after %s: %v / %v", phase, err, bc.OVMAuditError())
			}
			if bc.CurrentBlock().NumberU64() != 2 || bc.ovmAudit.cursor.Total.Difference != "42000" {
				t.Fatal("failed storage recovery")
			}
		})
	}
}

func TestOVMAuditRejectsUnobservedHistory(t *testing.T) {
	gen, blocks := auditFixture(t, 2)
	db := rawdb.NewMemoryDatabase()
	t.Cleanup(func() { db.Close() })
	bc := auditChain(t, gen, db)
	if _, err := bc.InsertChain(blocks[:1]); err != nil {
		t.Fatal(err)
	}
	if err := bc.EnableOVMAudit(OVMAuditConfig{Enabled: true, To: 2, ToHash: blocks[1].Hash(), Dir: t.TempDir()}); err == nil {
		t.Fatal("accepted unaudited prefix")
	}
}

func TestOVMAuditCancellation(t *testing.T) {
	gen, blocks := auditFixture(t, 2)
	db := rawdb.NewMemoryDatabase()
	t.Cleanup(func() { db.Close() })
	bc := auditChain(t, gen, db)
	if err := bc.EnableOVMAudit(OVMAuditConfig{Enabled: true, To: 2, ToHash: blocks[1].Hash(), Dir: t.TempDir()}); err != nil {
		t.Fatal(err)
	}
	bc.CancelOVMAudit()
	if _, err := bc.InsertChain(blocks); !IsOVMAuditControl(err) {
		t.Fatalf("%v", err)
	}
	if bc.CurrentBlock().NumberU64() != 0 {
		t.Fatal("imported after cancellation")
	}
}

func TestOVMAuditPersistentRestart(t *testing.T) {
	gen, blocks := auditFixture(t, 3)
	path := filepath.Join(t.TempDir(), "db")
	db, err := rawdb.NewLevelDBDatabase(path, 16, 16, "")
	if err != nil {
		t.Fatal(err)
	}
	bc := auditChain(t, gen, db)
	cfg := OVMAuditConfig{Enabled: true, To: 3, ToHash: blocks[2].Hash(), Dir: t.TempDir()}
	if err = bc.EnableOVMAudit(cfg); err != nil {
		t.Fatal(err)
	}
	if _, err = bc.InsertChain(blocks[:2]); err != nil {
		t.Fatal(err)
	}
	bc.Stop()
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = rawdb.NewLevelDBDatabase(path, 16, 16, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	bc = auditChain(t, gen, db)
	if err = bc.EnableOVMAudit(cfg); err != nil {
		t.Fatal(err)
	}
	if bc.ovmAudit.cursor.Total.Difference != "42000" {
		t.Fatalf("restart: %+v", bc.ovmAudit.cursor)
	}
	if _, err = bc.InsertChain(blocks[2:]); !IsOVMAuditControl(err) || bc.OVMAuditError() != nil {
		t.Fatalf("%v %v", err, bc.OVMAuditError())
	}
}

func TestOVMAuditWitnessAndUnknownGenesis(t *testing.T) {
	gen, blocks := auditFixture(t, 1)
	holder := common.Address{0xff}
	account := gen.Alloc[dump.OvmEthAddress]
	account.Storage = map[common.Hash]common.Hash{state.GetOVMBalanceKey(holder): common.BigToHash(big.NewInt(7))}
	gen.Alloc[dump.OvmEthAddress] = account
	db := rawdb.NewMemoryDatabase()
	t.Cleanup(func() { db.Close() })
	bc := auditChain(t, gen, db)
	cfg := OVMAuditConfig{Enabled: true, To: 1, ToHash: blocks[0].Hash(), Dir: t.TempDir()}
	if err := bc.EnableOVMAudit(cfg); err == nil {
		t.Fatal("unknown genesis accepted")
	}
	cfg.Witness = filepath.Join(t.TempDir(), "witness.jsonl")
	if err := os.WriteFile(cfg.Witness, []byte(fmt.Sprintf("{\"type\":\"address\",\"address\":%q}\n", holder.Hex())), 0600); err != nil {
		t.Fatal(err)
	}
	if err := bc.EnableOVMAudit(cfg); err != nil {
		t.Fatal(err)
	}
	if bc.ovmAudit.manifest.Genesis.Balances != "7" {
		t.Fatal("witness balance omitted")
	}
	if err := os.WriteFile(cfg.Witness, []byte(fmt.Sprintf("{\"type\":\"address\",\"address\":%q}\n\n", holder.Hex())), 0600); err != nil {
		t.Fatal(err)
	}
	if err := bc.EnableOVMAudit(cfg); err == nil {
		t.Fatal("modified witness accepted")
	}
}

// The fixture uses Clique's 1/2 weights while bypassing the embedded fake
// Ethash engine's difficulty formula. Body, receipt and state validation remain
// enabled on both normal import paths.
type auditReorgWeightEngine struct{ auditTestEngine }

func (e auditReorgWeightEngine) VerifyHeaders(_ consensus.ChainReader, headers []*types.Header, _ []bool) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))
	for range headers {
		results <- nil
	}
	return abort, results
}

func TestOVMAuditReorgPastTarget(t *testing.T) {
	for _, target := range []int{3, 140} {
		for _, resume := range []bool{false, true} {
			t.Run(fmt.Sprintf("target=%d/resume=%v", target, resume), func(t *testing.T) {
				gen, _ := auditFixture(t, 0)
				key, _ := crypto.HexToECDSA("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
				makeChain := func(n int, fork bool) types.Blocks {
					db := rawdb.NewMemoryDatabase()
					defer db.Close()
					blocks, _ := GenerateChain(gen.Config, gen.MustCommit(db), auditTestEngine{ethash.NewFaker()}, db, n, func(i int, b *BlockGen) {
						weight, price := int64(2), int64(1)
						if fork {
							price = 2
							if i < n-1 {
								weight = 1
							}
						}
						b.SetDifficulty(big.NewInt(weight))
						tx, err := types.SignTx(types.NewTransaction(uint64(i), common.Address{8}, big.NewInt(0), 21000, big.NewInt(price), nil), types.NewEIP155Signer(gen.Config.ChainID), key)
						if err != nil {
							t.Fatal(err)
						}
						b.AddTx(tx)
					})
					return blocks
				}
				main, fork := makeChain(target-1, false), makeChain(2*(target-1), true)
				db := rawdb.NewMemoryDatabase()
				t.Cleanup(func() { db.Close() })
				bc := auditChain(t, gen, db)
				bc.engine = auditReorgWeightEngine{auditTestEngine{ethash.NewFaker()}}
				cfg := OVMAuditConfig{Enabled: true, To: uint64(target), ToHash: fork[target-1].Hash(), Dir: t.TempDir()}
				if err := bc.EnableOVMAudit(cfg); err != nil {
					t.Fatal(err)
				}
				if resume {
					if err := os.Mkdir(filepath.Join(cfg.Dir, "events.jsonl"), 0700); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := bc.InsertChain(main); err != nil {
					t.Fatal(err)
				}
				_, err := bc.InsertChain(fork)
				if !IsOVMAuditControl(err) {
					t.Fatalf("expected audit stop, got %v", err)
				}
				if (bc.OVMAuditError() != nil) != resume {
					t.Fatalf("unexpected completion: %v", bc.OVMAuditError())
				}
				if bc.CurrentBlock().NumberU64() <= cfg.To || bc.GetCanonicalHash(cfg.To) != cfg.ToHash {
					t.Fatal("fixture did not cross the canonical target")
				}
				want := big.NewInt(int64(target) * 42000).String()
				if bc.ovmAudit.cursor.Number != cfg.To || bc.ovmAudit.cursor.Total.Difference != want {
					t.Fatalf("counted past target: %+v", bc.ovmAudit.cursor)
				}
				// Read through a fresh trie database before graceful shutdown can
				// flush recent state. This also covers target roots older than GC's
				// 128-block retention window on the longer fork.
				s, err := state.New(fork[target-1].Root(), state.NewDatabase(db))
				if err != nil {
					t.Fatal(err)
				}
				scan, err := s.ScanOVMAudit(state.NewOVMAuditIndex(db), fork[target-1].Root())
				if err != nil || scan.Difference != want {
					t.Fatalf("target state was not retained: %+v %v", scan, err)
				}
				if resume {
					// Simulate the previous implementation's cached sum extending
					// beyond the target, then resume without changing its identity.
					if err = bc.ovmAudit.reconcile(bc.CurrentBlock()); err != nil {
						t.Fatal(err)
					}
					blob, _ := json.Marshal(bc.ovmAudit.cursor)
					if err = db.Put(auditKey("cursor"), blob); err != nil {
						t.Fatal(err)
					}
					bc.Stop()
					if err = os.Remove(filepath.Join(cfg.Dir, "events.jsonl")); err != nil {
						t.Fatal(err)
					}
					bc = auditChain(t, gen, db)
					if err = bc.EnableOVMAudit(cfg); err != nil {
						t.Fatal(err)
					}
					if bc.OVMAuditError() != nil {
						t.Fatal(bc.OVMAuditError())
					}
					select {
					case <-bc.OVMAuditDone():
					default:
						t.Fatal("resumed audit did not complete")
					}
				}
				blob, err := os.ReadFile(filepath.Join(cfg.Dir, "summary.json"))
				if err != nil {
					t.Fatal(err)
				}
				var summary struct {
					Blocks uint64              `json:"verifiedBlocks"`
					Total  state.OVMAuditDelta `json:"totals"`
					End    state.OVMAuditScan  `json:"targetState"`
				}
				if err = json.Unmarshal(blob, &summary); err != nil {
					t.Fatal(err)
				}
				if summary.Blocks != cfg.To || summary.Total.Difference != want || summary.End.Root != fork[target-1].Root() || summary.End.Difference != want {
					t.Fatalf("wrong target report: %+v", summary)
				}
				blob, err = os.ReadFile(filepath.Join(cfg.Dir, "events.jsonl"))
				if err != nil {
					t.Fatal(err)
				}
				if bytes.Contains(blob, []byte(fork[target].Hash().Hex())) || bytes.Contains(blob, []byte(main[0].Hash().Hex())) {
					t.Fatal("report includes post-target or orphan events")
				}
			})
		}
	}
}

func TestOVMAuditAllowancePreimageOrder(t *testing.T) {
	for _, outerFirst := range []bool{false, true} {
		t.Run(fmt.Sprint(outerFirst), func(t *testing.T) {
			gen, _ := auditFixture(t, 0)
			owner := common.Address{1}
			innerBytes := append(common.LeftPadBytes(owner[:], 32), common.LeftPadBytes([]byte{1}, 32)...)
			inner := crypto.Keccak256Hash(innerBytes)
			var outer common.Hash
			var outerBytes []byte
			for n := int64(1); ; n++ {
				spender := common.BigToAddress(big.NewInt(n))
				outerBytes = append(common.LeftPadBytes(spender[:], 32), inner[:]...)
				outer = crypto.Keccak256Hash(outerBytes)
				if (bytes.Compare(outer[:], inner[:]) < 0) == outerFirst {
					break
				}
			}
			account := gen.Alloc[dump.OvmEthAddress]
			account.Storage = map[common.Hash]common.Hash{outer: common.BigToHash(big.NewInt(7))}
			gen.Alloc[dump.OvmEthAddress] = account
			db := rawdb.NewMemoryDatabase()
			t.Cleanup(func() { db.Close() })
			bc := auditChain(t, gen, db)
			rawdb.WritePreimages(db, map[common.Hash][]byte{inner: innerBytes, outer: outerBytes})
			// Exceed the startup flush threshold so the second pass must also
			// resolve bases which have already moved from memory to disk.
			preimages := make(map[common.Hash][]byte)
			for n := int64(1); n <= 5000; n++ {
				addr := common.BigToAddress(big.NewInt(n))
				preimages[crypto.Keccak256Hash(addr[:])] = addr.Bytes()
			}
			rawdb.WritePreimages(db, preimages)
			if err := bc.EnableOVMAudit(OVMAuditConfig{Enabled: true, To: 1, ToHash: common.Hash{1}, Dir: t.TempDir()}); err != nil {
				t.Fatalf("complete preimages rejected: %v", err)
			}
			slot, ok := state.NewOVMAuditIndex(db).Lookup(crypto.Keccak256Hash(outer[:]))
			if !ok || slot.Kind != "allowance" {
				t.Fatalf("allowance not persisted: %+v", slot)
			}
			if bc.ovmAudit.manifest.Genesis.Balances != "0" {
				t.Fatal("allowance counted as a balance")
			}
		})
	}
}
