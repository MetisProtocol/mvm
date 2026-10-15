package rawdb

import (
	"errors"
	"math/big"
	"testing"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/ethdb"
	"github.com/MetisProtocol/mvm/l2geth/ethdb/memorydb"
	"github.com/syndtr/goleveldb/leveldb"
)

type ovmHistoryFault struct {
	ethdb.Database
	ancientErr error
	hasErr     error
	getErr     error
	getHook    func()
}

func (d *ovmHistoryFault) Ancient(kind string, number uint64) ([]byte, error) {
	if d.ancientErr != nil {
		return nil, d.ancientErr
	}
	return d.Database.Ancient(kind, number)
}
func (d *ovmHistoryFault) Has(key []byte) (bool, error) {
	if d.hasErr != nil {
		return false, d.hasErr
	}
	return d.Database.Has(key)
}
func (d *ovmHistoryFault) Get(key []byte) ([]byte, error) {
	if d.getHook != nil {
		hook := d.getHook
		d.getHook = nil
		hook()
	}
	if d.getErr != nil {
		return nil, d.getErr
	}
	return d.Database.Get(key)
}

func TestOVMIndexHistoryFreezer(t *testing.T) {
	db, err := NewDatabaseWithFreezer(memorydb.New(), t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	genesis := types.NewBlock(&types.Header{Number: new(big.Int), Difficulty: big.NewInt(1)}, nil, nil, nil)
	WriteAncientBlock(db, genesis, nil, big.NewInt(1))
	receipt := &types.Receipt{Status: 1, CumulativeGasUsed: 21000, Logs: []*types.Log{{Address: common.HexToAddress("0x1234"), Data: []byte{1}}}}
	receipt.Bloom = types.CreateBloom(types.Receipts{receipt})
	receipts := types.Receipts{receipt}
	tx := types.NewTransaction(0, common.Address{}, new(big.Int), 21000, new(big.Int), nil)
	block := types.NewBlock(&types.Header{Number: big.NewInt(1), ParentHash: genesis.Hash(), Difficulty: big.NewInt(1), GasUsed: 21000}, []*types.Transaction{tx}, nil, receipts)
	WriteHeader(db, block.Header())
	WriteReceipts(db, block.Hash(), 1, receipts)
	WriteCanonicalHash(db, block.Hash(), 1)
	p := OVMIndexPosition{1, block.Hash()}
	// Simulate freezing between the hot Has and Get. A genuine not-found is the
	// only hot error which allows retrying the now-populated ancient record.
	moving := &ovmHistoryFault{Database: db, getErr: leveldb.ErrNotFound, getHook: func() { WriteAncientBlock(db, block, receipts, big.NewInt(2)) }}
	got, err := ReadOVMIndexReceipts(moving, p)
	if err != nil || types.DeriveSha(got) != block.ReceiptHash() {
		t.Fatalf("freezer rotation: %v", err)
	}
	DeleteHeader(db, block.Hash(), 1)
	DeleteReceipts(db, block.Hash(), 1)
	DeleteCanonicalHash(db, 1)
	header, err := ReadOVMIndexHeader(db, p)
	if err != nil || header.Hash() != block.Hash() {
		t.Fatalf("cold header: %v", err)
	}
	got, err = ReadOVMIndexReceipts(db, p)
	if err != nil || types.DeriveSha(got) != block.ReceiptHash() {
		t.Fatalf("cold receipts: %v", err)
	}
	hash, err := ReadOVMCanonicalHash(db, 1)
	if err != nil || hash != block.Hash() {
		t.Fatalf("cold canonical: %v", err)
	}
	failure := errors.New("ancient I/O")
	broken := &ovmHistoryFault{Database: db, ancientErr: failure}
	if _, err := ReadOVMIndexReceipts(broken, p); !errors.Is(err, failure) {
		t.Fatalf("lost ancient error: %v", err)
	}
	if _, err := ReadOVMCanonicalHash(broken, 1); !errors.Is(err, failure) {
		t.Fatalf("lost canonical error: %v", err)
	}
	if _, err := ReadOVMIndexHeader(db, OVMIndexPosition{1, common.HexToHash("0xdead")}); err == nil {
		t.Fatal("used canonical ancient for wrong fork")
	}
}

func TestOVMIndexHistoryErrors(t *testing.T) {
	db := NewMemoryDatabase()
	defer db.Close()
	header := &types.Header{Number: big.NewInt(1), Difficulty: big.NewInt(1)}
	WriteHeader(db, header)
	WriteReceipts(db, header.Hash(), 1, nil)
	WriteCanonicalHash(db, header.Hash(), 1)
	p := OVMIndexPosition{1, header.Hash()}
	failure := errors.New("hot I/O")
	for _, broken := range []*ovmHistoryFault{{Database: db, hasErr: failure}, {Database: db, getErr: failure}} {
		if _, err := ReadOVMIndexReceipts(broken, p); !errors.Is(err, failure) {
			t.Fatalf("lost hot error: %v", err)
		}
		if _, err := ReadOVMIndexHeader(broken, p); !errors.Is(err, failure) {
			t.Fatalf("lost header error: %v", err)
		}
		if _, err := ReadOVMCanonicalHash(broken, 1); !errors.Is(err, failure) {
			t.Fatalf("lost hash error: %v", err)
		}
	}
	for _, bad := range [][]byte{nil, {0xff}, {0xc1, 0xff}} {
		if err := db.Put(blockReceiptsKey(1, header.Hash()), bad); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadOVMIndexReceipts(db, p); err == nil {
			t.Fatalf("accepted receipt %x", bad)
		}
	}
	if err := db.Put(headerKey(1, header.Hash()), []byte{0xc0}); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadOVMIndexHeader(db, p); err == nil {
		t.Fatal("accepted corrupt header")
	}
	if err := db.Put(headerHashKey(1), make([]byte, 32)); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadOVMCanonicalHash(db, 1); err == nil {
		t.Fatal("accepted zero canonical hash")
	}
}

type ovmInitializationAncients struct {
	ethdb.Database
	count uint64
}

func (d ovmInitializationAncients) Ancients() (uint64, error) { return d.count, nil }

func TestOVMIndexGenesisOnlyInitialization(t *testing.T) {
	for _, mode := range []string{"genesis", "executed", "header", "fast", "canonical", "ancient", "missing_head"} {
		t.Run(mode, func(t *testing.T) {
			db := NewMemoryDatabase()
			defer db.Close()
			genesis, later := common.HexToHash("0x01"), common.HexToHash("0x02")
			WriteHeadBlockHash(db, genesis)
			WriteHeadHeaderHash(db, genesis)
			WriteHeadFastBlockHash(db, genesis)
			var source ethdb.Database = db
			switch mode {
			case "executed":
				WriteHeadBlockHash(db, later)
			case "header":
				WriteHeadHeaderHash(db, later)
			case "fast":
				WriteHeadFastBlockHash(db, later)
			case "canonical":
				WriteCanonicalHash(db, later, 1)
			case "ancient":
				source = ovmInitializationAncients{db, 2}
			case "missing_head":
				if err := db.Delete(headBlockKey); err != nil {
					t.Fatal(err)
				}
			}
			fresh, err := OVMIndexCanInitialize(source, genesis)
			if err != nil || fresh != (mode == "genesis") {
				t.Fatalf("fresh=%t, err=%v", fresh, err)
			}
		})
	}
}

func TestOVMIndexOptionalCanonicalHash(t *testing.T) {
	db := NewMemoryDatabase()
	defer db.Close()
	if hash, err := ReadOVMCanonicalHashIfPresent(db, 3); err != nil || hash != (common.Hash{}) {
		t.Fatalf("absent canonical hash: %v, %v", hash, err)
	}
	if _, err := ReadOVMCanonicalHash(db, 3); err == nil {
		t.Fatal("required canonical hash accepted absence")
	}
	for _, value := range [][]byte{{}, {1}, make([]byte, 32)} {
		if err := db.Put(headerHashKey(3), value); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadOVMCanonicalHashIfPresent(db, 3); err == nil {
			t.Fatalf("corrupt mapping treated as absent: %x", value)
		}
	}
	failure := errors.New("canonical I/O")
	if _, err := ReadOVMCanonicalHashIfPresent(&ovmHistoryFault{Database: db, hasErr: failure}, 3); !errors.Is(err, failure) {
		t.Fatalf("lost optional canonical read error: %v", err)
	}
}
