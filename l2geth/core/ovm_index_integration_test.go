package core

import (
	"math/big"
	"path/filepath"
	"testing"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/consensus/ethash"
	"github.com/MetisProtocol/mvm/l2geth/core/rawdb"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/core/vm"
	"github.com/MetisProtocol/mvm/l2geth/ethdb"
	"github.com/MetisProtocol/mvm/l2geth/params"
	"github.com/MetisProtocol/mvm/l2geth/rollup/dump"
	"github.com/MetisProtocol/mvm/l2geth/rollup/rcfg"
	"github.com/ethereum/go-ethereum/crypto"
)

func openIndexChain(t *testing.T, db ethdb.Database, g *Genesis) *BlockChain {
	t.Helper()
	// These fixtures temporarily switch the process-wide OVM setting. Disable
	// speculative prefetch, whose legacy goroutines outlive InsertChain/Stop.
	chain, err := NewBlockChain(db, &CacheConfig{TrieDirtyDisabled: true, TrieCleanNoPrefetch: true}, g.Config, ethash.NewFaker(), vm.Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(chain.Stop)
	return chain
}

func assertIndexReady(t *testing.T, db ethdb.Database, block *types.Block) {
	t.Helper()
	for _, key := range [][]byte{rawdb.OVMIndexHeadKey, rawdb.OVMIndexReadyKey} {
		p, err := rawdb.ReadOVMIndexPosition(db, key)
		if err != nil || p == nil || *p != ovmIndexPosition(block.Header()) {
			t.Fatalf("%s: %v, %v", key, p, err)
		}
	}
}

func TestOVMIndexNodeLifecycle(t *testing.T) {
	genesis, blocks, addresses := ovmPreimageBlocks(t)
	dir := t.TempDir()
	open := func() ethdb.Database {
		db, err := rawdb.NewLevelDBDatabaseWithFreezer(filepath.Join(dir, "chaindata"), 16, 16, filepath.Join(dir, "ancient"), "")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		return db
	}
	db := open()
	genesis.MustCommit(db)
	chain := openIndexChain(t, db, genesis)
	if _, err := chain.InsertChain(blocks); err != nil {
		t.Fatal(err)
	}
	assertIndexReady(t, db, blocks[1])
	assertIndexMembers(t, db, map[common.Address]rawdb.OVMIndexPosition{addresses[2]: ovmIndexPosition(blocks[1].Header())})
	// Rollback keeps historical records, but must rewind eligibility immediately.
	chain.Rollback([]common.Hash{blocks[1].Hash()})
	assertIndexReady(t, db, blocks[0])
	assertIndexMembers(t, db, map[common.Address]rawdb.OVMIndexPosition{})
	// Fast state promotion also aligns from stored receipts.
	if err := chain.FastSyncCommitHead(blocks[1].Hash()); err != nil {
		t.Fatal(err)
	}
	assertIndexReady(t, db, blocks[1])
	// FastSyncCommitHead deliberately does not change persisted LastBlock. Restart
	// must therefore rewind the index to the persisted executed head.
	chain.Stop()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = open()
	chain = openIndexChain(t, db, genesis)
	assertIndexReady(t, db, blocks[0])
	assertIndexMembers(t, db, map[common.Address]rawdb.OVMIndexPosition{})
	// Restore the stored executed head as if canonical publication completed just
	// before a crash, leaving the auxiliary index behind.
	chain.Stop()
	rawdb.WriteHeadBlockHash(db, blocks[1].Hash())
	rawdb.WriteHeadHeaderHash(db, blocks[1].Hash())
	rawdb.WriteHeadFastBlockHash(db, blocks[1].Hash())
	chain = openIndexChain(t, db, genesis)
	assertIndexReady(t, db, blocks[1])
	if err := chain.SetHead(1); err != nil {
		t.Fatal(err)
	}
	assertIndexReady(t, db, blocks[0])
	if rawdb.ReadHeader(db, blocks[1].Hash(), 2) != nil {
		t.Fatal("SetHead did not delete old header")
	}
	assertIndexMembers(t, db, map[common.Address]rawdb.OVMIndexPosition{})
	// Reimport after destructive rewind regenerates undo and membership.
	if _, err := chain.InsertChain(blocks[1:]); err != nil {
		t.Fatal(err)
	}
	assertIndexReady(t, db, blocks[1])
	if err := chain.Reset(); err != nil {
		t.Fatal(err)
	}
	assertIndexReady(t, db, chain.Genesis())
	assertIndexMembers(t, db, map[common.Address]rawdb.OVMIndexPosition{})
}

func TestOVMIndexNodeReorgAndAllowance(t *testing.T) {
	genesis, _, addresses := ovmPreimageBlocks(t)
	// Extend the existing actual-EVM Transfer program with a zero-value Approval.
	account := genesis.Alloc[dump.OvmEthAddress]
	code := append([]byte(nil), account.Code[:len(account.Code)-1]...)
	code = append(code, 0x60, 0, 0x60, 0, 0x52)
	for _, topic := range []common.Hash{common.BytesToHash(addresses[3][:]), common.BytesToHash(addresses[2][:]), ovmIndexApprovalTopic} {
		code = append(code, 0x7f)
		code = append(code, topic[:]...)
	}
	code = append(code, 0x60, 32, 0x60, 0, 0xa3, 0x00)
	account.Code = code
	genesis.Alloc[dump.OvmEthAddress] = account
	generator := rawdb.NewMemoryDatabase()
	defer generator.Close()
	parent := genesis.MustCommit(generator)
	key, err := crypto.HexToECDSA("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	old, _ := GenerateChain(genesis.Config, parent, ethash.NewFaker(), generator, 2, func(n int, b *BlockGen) {
		tx := types.NewTransaction(uint64(n), dump.OvmEthAddress, new(big.Int), 100000, new(big.Int), nil)
		tx.SetL2Tx(1)
		signed, err := types.SignTx(tx, types.MakeSigner(genesis.Config, b.Number()), key)
		if err != nil {
			t.Fatal(err)
		}
		b.AddTx(signed)
	})
	other, _ := GenerateChain(genesis.Config, parent, ethash.NewFaker(), generator, 3, func(_ int, b *BlockGen) { b.SetExtra([]byte("other branch")) })
	extension, _ := GenerateChain(genesis.Config, old[1], ethash.NewFaker(), generator, 3, func(_ int, b *BlockGen) { b.SetExtra([]byte("old branch extended")) })
	db := rawdb.NewMemoryDatabase()
	defer db.Close()
	genesis.MustCommit(db)
	chain := openIndexChain(t, db, genesis)
	if _, err := chain.InsertChain(old); err != nil {
		t.Fatal(err)
	}
	first := ovmIndexPosition(old[0].Header())
	assertIndexMembers(t, db, map[common.Address]rawdb.OVMIndexPosition{addresses[2]: first})
	if pair, err := rawdb.ReadOVMAllowance(db, rawdb.OVMAllowanceTrieKey(addresses[2], addresses[3])); err != nil || pair == nil {
		t.Fatalf("missing Approval relation: %v", err)
	}
	if _, err := chain.InsertChain(other); err != nil {
		t.Fatal(err)
	}
	assertIndexReady(t, db, other[2])
	assertIndexMembers(t, db, map[common.Address]rawdb.OVMIndexPosition{})
	if pair, err := rawdb.ReadOVMAllowance(db, rawdb.OVMAllowanceTrieKey(addresses[2], addresses[3])); err != nil || pair == nil {
		t.Fatal("reorg deleted relation")
	}
	if _, err := chain.InsertChain(extension); err != nil {
		t.Fatal(err)
	}
	assertIndexReady(t, db, extension[2])
	assertIndexMembers(t, db, map[common.Address]rawdb.OVMIndexPosition{addresses[2]: first})
	// Imported roots and receipt commitments remain the generator's consensus data.
	for _, block := range append(old, extension...) {
		stored := chain.GetBlockByHash(block.Hash())
		if stored == nil || stored.Root() != block.Root() || stored.ReceiptHash() != block.ReceiptHash() || stored.GasUsed() != block.GasUsed() {
			t.Fatal("index changed consensus block")
		}
	}
}

func TestOVMIndexUnindexedStartup(t *testing.T) {
	genesis, blocks, _ := ovmPreimageBlocks(t)
	db := rawdb.NewMemoryDatabase()
	defer db.Close()
	genesis.MustCommit(db)
	chain := openIndexChain(t, db, genesis)
	if _, err := chain.InsertChain(blocks); err != nil {
		t.Fatal(err)
	}
	chain.Stop()
	// Simulate a database produced by a pre-index binary.
	for _, prefix := range [][]byte{[]byte("ovm-transfer-v1/"), rawdb.OVMAllowancePrefix} {
		it := db.NewIteratorWithPrefix(prefix)
		batch := db.NewBatch()
		for it.Next() {
			if err := batch.Delete(append([]byte(nil), it.Key()...)); err != nil {
				t.Fatal(err)
			}
		}
		err := it.Error()
		it.Release()
		if err != nil {
			t.Fatal(err)
		}
		if err := batch.Write(); err != nil {
			t.Fatal(err)
		}
	}
	// Historical receipts are deliberately absent: startup must not try to
	// backfill, or reject an otherwise usable legacy node for this reason.
	rawdb.DeleteReceipts(db, blocks[1].Hash(), 2)
	chain = openIndexChain(t, db, genesis)
	if chain.ovmIndex != nil || chain.CurrentBlock().Hash() != blocks[1].Hash() {
		t.Fatal("legacy node did not start without indexing")
	}
	extension, _ := GenerateChain(genesis.Config, blocks[1], ethash.NewFaker(), db, 1, nil)
	if _, err := chain.InsertChain(extension); err != nil {
		t.Fatal(err)
	}
	other, _ := GenerateChain(genesis.Config, chain.Genesis(), ethash.NewFaker(), db, 4, func(_ int, b *BlockGen) { b.SetExtra([]byte("legacy header fork")) })
	headers := make([]*types.Header, len(other))
	for n, block := range other {
		headers[n] = block.Header()
	}
	if _, err := chain.InsertHeaderChain(headers, 1); err != nil {
		t.Fatal(err)
	}
	chain.Stop()
	chain = openIndexChain(t, db, genesis)
	if chain.ovmIndex != nil || chain.CurrentBlock().Hash() != extension[0].Hash() {
		t.Fatal("legacy header reorg/restart enabled indexing or changed executed head")
	}
	for _, prefix := range [][]byte{rawdb.OVMAllowancePrefix, []byte("ovm-transfer-v1/")} {
		it := db.NewIteratorWithPrefix(prefix)
		exists, err := it.Next(), it.Error()
		it.Release()
		if exists || err != nil {
			t.Fatalf("legacy node created index records: %t, %v", exists, err)
		}
	}
}

func TestOVMIndexStartupStateRepair(t *testing.T) {
	genesis, blocks, _ := ovmPreimageBlocks(t)
	db := rawdb.NewMemoryDatabase()
	t.Cleanup(func() { db.Close() })
	genesis.MustCommit(db)
	chain := openIndexChain(t, db, genesis)
	if _, err := chain.InsertChain(blocks); err != nil {
		t.Fatal(err)
	}
	chain.Stop()
	// A missing state triggers the existing startup repair. Index must follow it.
	broken := types.CopyHeader(blocks[1].Header())
	broken.Root = common.HexToHash("0xdead")
	fake := types.NewBlockWithHeader(broken).WithBody(blocks[1].Transactions(), blocks[1].Uncles())
	rawdb.WriteBlock(db, fake)
	rawdb.WriteHeadBlockHash(db, fake.Hash())
	rawdb.WriteCanonicalHash(db, fake.Hash(), 2)
	chain = openIndexChain(t, db, genesis)
	assertIndexReady(t, db, blocks[0])
	assertIndexMembers(t, db, map[common.Address]rawdb.OVMIndexPosition{})
}

func TestOVMIndexDisabledForNonOVM(t *testing.T) {
	old := rcfg.UsingOVM
	rcfg.UsingOVM = false
	t.Cleanup(func() { rcfg.UsingOVM = old })
	db := rawdb.NewMemoryDatabase()
	t.Cleanup(func() { db.Close() })
	genesis := &Genesis{Config: params.AllEthashProtocolChanges}
	parent := genesis.MustCommit(db)
	chain := openIndexChain(t, db, genesis)
	blocks, _ := GenerateChain(genesis.Config, parent, ethash.NewFaker(), db, 1, nil)
	if _, err := chain.InsertChain(blocks); err != nil {
		t.Fatal(err)
	}
	for _, prefix := range [][]byte{rawdb.OVMAllowancePrefix, []byte("ovm-transfer-v1/")} {
		it := db.NewIteratorWithPrefix(prefix)
		exists, err := it.Next(), it.Error()
		it.Release()
		if exists || err != nil {
			t.Fatalf("non-OVM index records: %t, %v", exists, err)
		}
	}
}

func TestOVMIndexFastSyncDoesNotBackfill(t *testing.T) {
	genesis, _, _ := ovmPreimageBlocks(t)
	db := rawdb.NewMemoryDatabase()
	t.Cleanup(func() { db.Close() })
	parent := genesis.MustCommit(db)
	chain := openIndexChain(t, db, genesis)
	blocks, _ := GenerateChain(genesis.Config, parent, ethash.NewFaker(), db, 2, nil)
	for _, block := range blocks {
		rawdb.WriteBlock(db, block)
		rawdb.WriteCanonicalHash(db, block.Hash(), block.NumberU64())
		// No receipts: fast promotion must not read them to build missing indexes.
	}
	rawdb.WriteHeadHeaderHash(db, blocks[1].Hash())
	rawdb.WriteHeadFastBlockHash(db, blocks[1].Hash())
	if err := chain.FastSyncCommitHead(blocks[1].Hash()); err != nil {
		t.Fatal(err)
	}
	if !chain.ovmIndex.disabled {
		t.Fatal("fast pivot did not disable incomplete indexes")
	}
	head, err := rawdb.ReadOVMIndexPosition(db, rawdb.OVMIndexHeadKey)
	if err != nil || head == nil || *head != ovmIndexPosition(parent.Header()) {
		t.Fatal("fast pivot backfilled index head")
	}
	ready, err := rawdb.ReadOVMIndexPosition(db, rawdb.OVMIndexReadyKey)
	if err != nil || ready != nil {
		t.Fatal("fast pivot left a ready index")
	}
	chain.Stop()
	rawdb.WriteHeadBlockHash(db, blocks[1].Hash())
	chain = openIndexChain(t, db, genesis)
	if chain.ovmIndex != nil {
		t.Fatal("restart resumed disabled index")
	}
}

func TestOVMIndexUnindexedHeadRepairDoesNotInitialize(t *testing.T) {
	genesis, blocks, _ := ovmPreimageBlocks(t)
	db := rawdb.NewMemoryDatabase()
	t.Cleanup(func() { db.Close() })
	genesis.MustCommit(db)
	// Old unindexed head has lost its state; ordinary startup repairs to genesis.
	broken := types.CopyHeader(blocks[0].Header())
	broken.Root = common.HexToHash("0xdead")
	block := types.NewBlockWithHeader(broken).WithBody(blocks[0].Transactions(), blocks[0].Uncles())
	rawdb.WriteBlock(db, block)
	rawdb.WriteCanonicalHash(db, block.Hash(), 1)
	rawdb.WriteHeadBlockHash(db, block.Hash())
	rawdb.WriteHeadHeaderHash(db, block.Hash())
	rawdb.WriteHeadFastBlockHash(db, block.Hash())
	chain := openIndexChain(t, db, genesis)
	if chain.CurrentBlock().NumberU64() != 0 || chain.ovmIndex != nil {
		t.Fatal("head repair initialized legacy indexes")
	}
	if meta, err := rawdb.ReadOVMIndexRecord(db, rawdb.OVMIndexMetadataKey); err != nil || meta != nil {
		t.Fatal("head repair created metadata")
	}
}

// Header synchronization can reorg the canonical mappings before the new
// branch's bodies, receipts or state have arrived. The executed head still
// points to the old branch during this interval.
func TestOVMIndexHeaderReorg(t *testing.T) {
	for _, action := range []string{"headers", "restart", "rewind"} {
		t.Run(action, func(t *testing.T) {
			genesis, blocks, addresses := ovmPreimageBlocks(t)
			db := rawdb.NewMemoryDatabase()
			t.Cleanup(func() { db.Close() })
			parent := genesis.MustCommit(db)
			chain := openIndexChain(t, db, genesis)
			if _, err := chain.InsertChain(blocks); err != nil {
				t.Fatal(err)
			}
			generator := rawdb.NewMemoryDatabase()
			defer generator.Close()
			genesis.MustCommit(generator)
			other, _ := GenerateChain(genesis.Config, parent, ethash.NewFaker(), generator, 3, func(_ int, b *BlockGen) { b.SetExtra([]byte("header fork")) })
			headers := make([]*types.Header, len(other))
			for n, block := range other {
				headers[n] = block.Header()
			}
			if action == "restart" {
				// Crash window: canonical header publication has completed, but
				// index reconciliation has not run. Keep the old index prefix.
				chain.chainmu.Lock()
				err := db.Delete(rawdb.OVMIndexReadyKey)
				for _, header := range headers {
					if err == nil {
						_, err = chain.hc.WriteHeader(header)
					}
				}
				chain.chainmu.Unlock()
				if err != nil {
					t.Fatal(err)
				}
			} else if _, err := chain.InsertHeaderChain(headers, 1); err != nil {
				t.Fatal(err)
			}
			if chain.CurrentBlock().Hash() != blocks[1].Hash() || chain.CurrentHeader().Hash() != other[2].Hash() {
				t.Fatal("fixture did not separate executed and header heads")
			}
			switch action {
			case "restart":
				chain.Stop()
				chain = openIndexChain(t, db, genesis)
			case "rewind":
				// There are no bodies or receipts for the new canonical branch.
				// SetHead must only undo existing index entries, never index it.
				if err := chain.SetHead(1); err != nil {
					t.Fatal(err)
				}
			}
			assertIndexMembers(t, db, map[common.Address]rawdb.OVMIndexPosition{})
			if action == "rewind" {
				assertIndexReady(t, db, parent)
				return
			}
			ready, err := rawdb.ReadOVMIndexPosition(db, rawdb.OVMIndexReadyKey)
			if err != nil || ready != nil {
				t.Fatalf("header-only reorg left index ready: %v, %v", ready, err)
			}
			if action == "headers" {
				// Completing execution on the header fork must build its index.
				if _, err := chain.InsertChain(other); err != nil {
					t.Fatal(err)
				}
				assertIndexReady(t, db, other[2])
				assertIndexMembers(t, db, map[common.Address]rawdb.OVMIndexPosition{})
				return
			}
			// Header synchronization can return to the already executed branch.
			// Its stored receipts must restore membership and readiness.
			extension, _ := GenerateChain(genesis.Config, blocks[1], ethash.NewFaker(), db, 2, func(_ int, b *BlockGen) { b.SetExtra([]byte("return to executed fork")) })
			if _, err := chain.InsertHeaderChain([]*types.Header{extension[0].Header(), extension[1].Header()}, 1); err != nil {
				t.Fatal(err)
			}
			assertIndexReady(t, db, blocks[1])
			if _, err := chain.InsertChain(extension); err != nil {
				t.Fatal(err)
			}
			assertIndexReady(t, db, extension[1])
			assertIndexMembers(t, db, map[common.Address]rawdb.OVMIndexPosition{addresses[2]: ovmIndexPosition(blocks[1].Header())})
		})
	}
}

func TestOVMIndexShortHeaderReorgRestart(t *testing.T) {
	genesis, blocks, _ := ovmPreimageBlocks(t)
	db := rawdb.NewMemoryDatabase()
	t.Cleanup(func() { db.Close() })
	genesis.MustCommit(db)
	chain := openIndexChain(t, db, genesis)
	if _, err := chain.InsertChain(blocks); err != nil {
		t.Fatal(err)
	}
	// Publish a synthetic shorter but heavier header branch, leaving no
	// canonical mapping at the executed head's height. Stop before index work
	// to exercise startup recovery from the previous implementation as well.
	header := types.CopyHeader(blocks[0].Header())
	header.Extra = []byte("shorter header fork")
	header.Difficulty = new(big.Int).Mul(blocks[0].Difficulty(), big.NewInt(4))
	chain.chainmu.Lock()
	_, err := chain.hc.WriteHeader(header)
	chain.chainmu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if rawdb.ReadCanonicalHash(db, 2) != (common.Hash{}) {
		t.Fatal("short fork retained canonical mapping above its head")
	}
	chain.Stop()
	chain = openIndexChain(t, db, genesis)
	if chain.CurrentBlock().Hash() != blocks[1].Hash() {
		t.Fatal("index recovery changed executed head")
	}
	assertIndexMembers(t, db, map[common.Address]rawdb.OVMIndexPosition{})
	if ready, err := rawdb.ReadOVMIndexPosition(db, rawdb.OVMIndexReadyKey); err != nil || ready != nil {
		t.Fatalf("noncanonical executed head marked ready: %v, %v", ready, err)
	}
}

func TestOVMIndexRestartRejectsMissingCanonicalMapping(t *testing.T) {
	genesis, blocks, _ := ovmPreimageBlocks(t)
	db := rawdb.NewMemoryDatabase()
	t.Cleanup(func() { db.Close() })
	genesis.MustCommit(db)
	chain := openIndexChain(t, db, genesis)
	if _, err := chain.InsertChain(blocks); err != nil {
		t.Fatal(err)
	}
	chain.Stop()
	// Absence inside the selected header chain is corruption, unlike mappings
	// above the head removed by a legitimate shorter header reorg.
	rawdb.DeleteCanonicalHash(db, 2)
	chain, err := NewBlockChain(db, &CacheConfig{TrieDirtyDisabled: true, TrieCleanNoPrefetch: true}, genesis.Config, ethash.NewFaker(), vm.Config{}, nil)
	if err == nil {
		chain.Stop()
		t.Fatal("accepted missing canonical mapping within header chain")
	}
}
