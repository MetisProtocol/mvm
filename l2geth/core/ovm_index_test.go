package core

import (
	"bytes"
	"errors"
	"math/big"
	"testing"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/core/rawdb"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/ethdb"
	"github.com/MetisProtocol/mvm/l2geth/rollup/dump"
)

func indexEvent(topic common.Hash, from, to common.Address, amount byte) *types.Log {
	data := make([]byte, 32)
	data[31] = amount
	return &types.Log{Address: dump.OvmEthAddress, Topics: []common.Hash{topic, common.BytesToHash(from[:]), common.BytesToHash(to[:])}, Data: data}
}

// indexBlock constructs committed receipt history, independently of the index.
func indexBlock(t *testing.T, db ethdb.Database, parent *types.Header, tag byte, events ...*types.Log) *types.Header {
	t.Helper()
	header := &types.Header{Number: new(big.Int), Difficulty: big.NewInt(1), GasLimit: 10000000, Extra: []byte{tag}, TxHash: types.EmptyRootHash, ReceiptHash: types.EmptyRootHash}
	if parent != nil {
		header.Number.Add(parent.Number, big.NewInt(1))
		header.ParentHash = parent.Hash()
	}
	receipts := types.Receipts{}
	if len(events) > 0 {
		receipt := &types.Receipt{Status: 1, CumulativeGasUsed: 50000, Logs: events}
		receipt.Bloom = types.CreateBloom(types.Receipts{receipt})
		receipts = append(receipts, receipt)
		header.TxHash = common.HexToHash("0x01")
		header.ReceiptHash = types.DeriveSha(receipts)
		header.GasUsed = receipt.CumulativeGasUsed
		header.Bloom = receipt.Bloom
	}
	rawdb.WriteHeader(db, header)
	rawdb.WriteReceipts(db, header.Hash(), header.Number.Uint64(), receipts)
	return header
}

func indexCanonical(db ethdb.Database, headers ...*types.Header) {
	for _, header := range headers {
		rawdb.WriteCanonicalHash(db, header.Hash(), header.Number.Uint64())
	}
}

func newTestOVMIndex(t *testing.T, db ethdb.Database) (*ovmMigrationIndex, *types.Header) {
	t.Helper()
	genesis := indexBlock(t, db, nil, 0)
	indexCanonical(db, genesis)
	rawdb.WriteHeadBlockHash(db, genesis.Hash())
	rawdb.WriteHeadHeaderHash(db, genesis.Hash())
	rawdb.WriteHeadFastBlockHash(db, genesis.Hash())
	idx := &ovmMigrationIndex{db: db, genesis: genesis, interrupted: func() bool { return false }}
	if err := idx.align(genesis, nil); err != nil {
		t.Fatal(err)
	}
	return idx, genesis
}

func assertIndexMembers(t *testing.T, db ethdb.Database, want map[common.Address]rawdb.OVMIndexPosition) {
	t.Helper()
	it := db.NewIteratorWithPrefix(rawdb.OVMTransferMemberPrefix)
	defer it.Release()
	count := 0
	for it.Next() {
		key := it.Key()
		if len(key) != len(rawdb.OVMTransferMemberPrefix)+20 {
			t.Fatal("bad member key")
		}
		address := common.BytesToAddress(key[len(rawdb.OVMTransferMemberPrefix):])
		p, err := rawdb.DecodeOVMIndexPosition(it.Value())
		if err != nil {
			t.Fatal(err)
		}
		if expected, ok := want[address]; !ok || expected != p {
			t.Fatalf("unexpected member %s at %+v", address, p)
		}
		count++
	}
	if err := it.Error(); err != nil {
		t.Fatal(err)
	}
	if count != len(want) {
		t.Fatalf("members %d, want %d", count, len(want))
	}
}

func TestOVMIndexReorg(t *testing.T) {
	db := rawdb.NewMemoryDatabase()
	defer db.Close()
	idx, g := newTestOVMIndex(t, db)
	a, b, c, z := common.HexToAddress("0x01"), common.HexToAddress("0x02"), common.HexToAddress("0x03"), common.Address{}
	commonBlock := indexBlock(t, db, g, 1, indexEvent(ovmIndexTransferTopic, a, z, 1))
	old := indexBlock(t, db, commonBlock, 2, indexEvent(ovmIndexTransferTopic, b, z, 1), indexEvent(ovmIndexTransferTopic, b, a, 2), indexEvent(ovmIndexTransferTopic, a, b, 1), indexEvent(ovmIndexTransferTopic, c, z, 0), indexEvent(ovmIndexApprovalTopic, b, c, 0))
	empty := indexBlock(t, db, old, 3)
	indexCanonical(db, commonBlock, old, empty)
	if err := idx.align(empty, nil); err != nil {
		t.Fatal(err)
	}
	assertIndexMembers(t, db, map[common.Address]rawdb.OVMIndexPosition{a: ovmIndexPosition(commonBlock), b: ovmIndexPosition(old)})
	undo, err := rawdb.ReadOVMTransferUndo(db, old.Hash())
	if err != nil || len(undo) != 1 || undo[0] != b {
		t.Fatalf("undo %v %v", undo, err)
	}
	undo, err = rawdb.ReadOVMTransferUndo(db, empty.Hash())
	if err != nil || len(undo) != 0 {
		t.Fatalf("empty undo %v %v", undo, err)
	}
	branch := indexBlock(t, db, commonBlock, 4, indexEvent(ovmIndexTransferTopic, c, z, 1))
	indexCanonical(db, branch)
	if err := idx.align(branch, nil); err != nil {
		t.Fatal(err)
	}
	assertIndexMembers(t, db, map[common.Address]rawdb.OVMIndexPosition{a: ovmIndexPosition(commonBlock), c: ovmIndexPosition(branch)})
	if pair, err := rawdb.ReadOVMAllowance(db, rawdb.OVMAllowanceTrieKey(b, c)); err != nil || pair == nil {
		t.Fatalf("old branch relationship lost: %v", err)
	}
	// Re-entering the original branch must recompute membership from receipts.
	indexCanonical(db, old, empty)
	if err := idx.align(empty, nil); err != nil {
		t.Fatal(err)
	}
	assertIndexMembers(t, db, map[common.Address]rawdb.OVMIndexPosition{a: ovmIndexPosition(commonBlock), b: ovmIndexPosition(old)})
	// Rewind across first occurrence and then replay all history.
	if err := idx.align(g, nil); err != nil {
		t.Fatal(err)
	}
	assertIndexMembers(t, db, map[common.Address]rawdb.OVMIndexPosition{})
	if err := idx.align(empty, nil); err != nil {
		t.Fatal(err)
	}
	ready, err := rawdb.ReadOVMIndexPosition(db, rawdb.OVMIndexReadyKey)
	if err != nil || ready == nil || *ready != ovmIndexPosition(empty) {
		t.Fatalf("not ready: %v %v", ready, err)
	}
}

var errIndexInjected = errors.New("injected index failure")

type indexFaultDB struct {
	ethdb.Database
	failBatch   bool
	failReady   bool
	afterCommit bool
}
type indexFaultBatch struct {
	ethdb.Batch
	db *indexFaultDB
}

func (d *indexFaultDB) NewBatch() ethdb.Batch { return &indexFaultBatch{d.Database.NewBatch(), d} }
func (b *indexFaultBatch) Write() error {
	if b.db.failBatch {
		b.db.failBatch = false
		return errIndexInjected
	}
	if err := b.Batch.Write(); err != nil {
		return err
	}
	if b.db.afterCommit {
		b.db.afterCommit = false
		return errIndexInjected
	}
	return nil
}
func (d *indexFaultDB) Put(key, value []byte) error {
	if d.failReady && bytes.Equal(key, rawdb.OVMIndexReadyKey) {
		return errIndexInjected
	}
	return d.Database.Put(key, value)
}

func TestOVMIndexAtomicRecovery(t *testing.T) {
	for _, mode := range []string{"before", "after", "ready"} {
		t.Run(mode, func(t *testing.T) {
			base := rawdb.NewMemoryDatabase()
			defer base.Close()
			db := &indexFaultDB{Database: base}
			idx, g := newTestOVMIndex(t, db)
			a, b := common.HexToAddress("0x01"), common.HexToAddress("0x02")
			block := indexBlock(t, db, g, 1, indexEvent(ovmIndexTransferTopic, a, b, 1), indexEvent(ovmIndexApprovalTopic, a, b, 1))
			indexCanonical(db, block)
			db.failBatch = mode == "before"
			db.afterCommit = mode == "after"
			db.failReady = mode == "ready"
			if err := idx.align(block, nil); !errors.Is(err, errIndexInjected) {
				t.Fatalf("lost error: %v", err)
			}
			head, err := rawdb.ReadOVMIndexPosition(db, rawdb.OVMIndexHeadKey)
			if err != nil {
				t.Fatal(err)
			}
			member, err := rawdb.ReadOVMTransferMember(db, a)
			if err != nil {
				t.Fatal(err)
			}
			pair, err := rawdb.ReadOVMAllowance(db, rawdb.OVMAllowanceTrieKey(a, b))
			if err != nil {
				t.Fatal(err)
			}
			if mode == "before" {
				if *head != ovmIndexPosition(g) || member != nil || pair != nil {
					t.Fatal("partial apply")
				}
			} else {
				if *head != ovmIndexPosition(block) || member == nil || pair == nil {
					t.Fatal("committed batch incomplete")
				}
			}
			if ready, err := rawdb.ReadOVMIndexPosition(db, rawdb.OVMIndexReadyKey); err != nil || ready != nil {
				t.Fatal("failed build marked ready")
			}
			db.failReady = false
			// A fresh index object resumes the persisted prefix, including uncertain commits.
			idx = &ovmMigrationIndex{db: db, genesis: g, interrupted: func() bool { return false }}
			if err := idx.align(block, nil); err != nil {
				t.Fatal(err)
			}
			db.failBatch = mode == "before"
			db.afterCommit = mode == "after"
			db.failReady = mode == "ready"
			if err := idx.align(g, nil); !errors.Is(err, errIndexInjected) {
				t.Fatalf("undo error: %v", err)
			}
			db.failReady = false
			if err := idx.align(g, nil); err != nil {
				t.Fatal(err)
			}
			assertIndexMembers(t, db, map[common.Address]rawdb.OVMIndexPosition{})
			if pair, err := rawdb.ReadOVMAllowance(db, rawdb.OVMAllowanceTrieKey(a, b)); err != nil || pair == nil {
				t.Fatal("undo removed allowance")
			}
		})
	}
}

func TestOVMIndexRejectsBadEvidence(t *testing.T) {
	for _, mode := range []string{"padding", "topics", "data", "receipt", "missing_receipts", "missing_header", "missing_undo", "wrong_member", "metadata", "ready", "interrupt"} {
		t.Run(mode, func(t *testing.T) {
			db := rawdb.NewMemoryDatabase()
			defer db.Close()
			idx, g := newTestOVMIndex(t, db)
			a := common.HexToAddress("0x01")
			event := indexEvent(ovmIndexTransferTopic, a, common.Address{}, 1)
			switch mode {
			case "padding":
				event.Topics[1][0] = 1
			case "topics":
				event.Topics = event.Topics[:2]
			case "data":
				event.Data = event.Data[:31]
			}
			block := indexBlock(t, db, g, 1, event)
			indexCanonical(db, block)
			switch mode {
			case "receipt":
				rawdb.WriteReceipts(db, block.Hash(), 1, nil)
			case "missing_receipts":
				rawdb.DeleteReceipts(db, block.Hash(), 1)
			case "missing_header":
				rawdb.DeleteHeader(db, block.Hash(), 1)
			case "metadata":
				if err := db.Put(rawdb.OVMIndexMetadataKey, []byte{1}); err != nil {
					t.Fatal(err)
				}
			case "ready":
				if err := db.Put(rawdb.OVMIndexReadyKey, rawdb.EncodeOVMIndexPosition(ovmIndexPosition(block))); err != nil {
					t.Fatal(err)
				}
			case "interrupt":
				idx.interrupted = func() bool { return true }
			case "missing_undo", "wrong_member":
				if err := idx.align(block, nil); err != nil {
					t.Fatal(err)
				}
				if mode == "missing_undo" {
					if err := db.Delete(append(append([]byte(nil), rawdb.OVMTransferUndoPrefix...), block.Hash().Bytes()...)); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := rawdb.WriteOVMTransferMember(db, a, ovmIndexPosition(g)); err != nil {
						t.Fatal(err)
					}
				}
				if err := idx.align(g, nil); err == nil {
					t.Fatal("accepted corrupt undo")
				}
				return
			}
			if err := idx.align(block, nil); err == nil {
				t.Fatal("accepted invalid evidence")
			}
			assertIndexMembers(t, db, map[common.Address]rawdb.OVMIndexPosition{})
		})
	}
}

func TestOVMIndexEventSelection(t *testing.T) {
	db := rawdb.NewMemoryDatabase()
	defer db.Close()
	idx, g := newTestOVMIndex(t, db)
	a, b, c := common.HexToAddress("0x01"), common.HexToAddress("0x02"), common.HexToAddress("0x03")
	unrelated := indexEvent(ovmIndexTransferTopic, b, c, 1)
	unrelated.Address = common.HexToAddress("0xffff")
	unrelated.Data = nil
	unknown := &types.Log{Address: dump.OvmEthAddress, Topics: []common.Hash{common.HexToHash("0xdead")}}
	block := indexBlock(t, db, g, 1, unrelated, unknown, indexEvent(ovmIndexApprovalTopic, b, c, 0), indexEvent(ovmIndexTransferTopic, c, a, 0), indexEvent(ovmIndexTransferTopic, a, common.Address{}, 1))
	indexCanonical(db, block)
	if err := idx.align(block, nil); err != nil {
		t.Fatal(err)
	}
	assertIndexMembers(t, db, map[common.Address]rawdb.OVMIndexPosition{a: ovmIndexPosition(block)})
	if pair, err := rawdb.ReadOVMAllowance(db, rawdb.OVMAllowanceTrieKey(b, c)); err != nil || pair == nil {
		t.Fatal("zero approval missing")
	}
}
