package core

import (
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/consensus/ethash"
	"github.com/MetisProtocol/mvm/l2geth/core/rawdb"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/core/vm"
	"github.com/MetisProtocol/mvm/l2geth/ethdb"
	"github.com/MetisProtocol/mvm/l2geth/ethdb/memorydb"
	"github.com/MetisProtocol/mvm/l2geth/params"
	lru "github.com/hashicorp/golang-lru"
)

func checkpointConfig() *params.ChainConfig {
	config := *params.TestChainConfig
	config.ChainID = big.NewInt(1088)
	return &config
}

func checkpointHeaders(t *testing.T) []*types.Header {
	t.Helper()
	data, err := os.ReadFile("../params/testdata/andromeda-checkpoints.json")
	if err != nil {
		t.Fatal(err)
	}
	var headers []*types.Header
	if err := json.Unmarshal(data, &headers); err != nil {
		t.Fatal(err)
	}
	return headers
}

func requireCheckpointError(t *testing.T, err error, number uint64, have common.Hash) {
	t.Helper()
	var mismatch *CheckpointMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("want checkpoint error, got %v", err)
	}
	want, _ := params.BlockHashCheckpoint(big.NewInt(1088), number)
	if mismatch.Number != number || mismatch.Have != have || mismatch.Want != want {
		t.Fatalf("wrong checkpoint error: %+v", mismatch)
	}
	if !strings.Contains(err.Error(), have.Hex()) || !strings.Contains(err.Error(), want.Hex()) {
		t.Fatalf("error omits hashes: %v", err)
	}
}

func checkpointDB(t *testing.T) ethdb.Database {
	t.Helper()
	db := rawdb.NewMemoryDatabase()
	t.Cleanup(func() { db.Close() })
	genesis := types.NewBlockWithHeader(checkpointHeaders(t)[0])
	rawdb.WriteBlock(db, genesis)
	rawdb.WriteTd(db, genesis.Hash(), 0, big.NewInt(1))
	rawdb.WriteCanonicalHash(db, genesis.Hash(), 0)
	rawdb.WriteHeadHeaderHash(db, genesis.Hash())
	rawdb.WriteHeadBlockHash(db, genesis.Hash())
	rawdb.WriteHeadFastBlockHash(db, genesis.Hash())
	rawdb.WriteChainConfig(db, genesis.Hash(), checkpointConfig())
	return db
}

func checkpointDBSnapshot(db ethdb.Database) map[string]string {
	entries := make(map[string]string)
	it := db.NewIterator()
	defer it.Release()
	for it.Next() {
		entries[string(it.Key())] = string(it.Value())
	}
	return entries
}

func TestCheckpointVerification(t *testing.T) {
	config := checkpointConfig()
	for _, header := range checkpointHeaders(t) {
		if err := verifyBlockCheckpoint(config, header.Number.Uint64(), header.Hash()); err != nil {
			t.Fatal(err)
		}
		bad := types.CopyHeader(header)
		bad.Extra = append(bad.Extra, 1)
		requireCheckpointError(t, verifyBlockCheckpoint(config, bad.Number.Uint64(), bad.Hash()), bad.Number.Uint64(), bad.Hash())
	}
	for _, number := range []uint64{1, 99_999, 100_001, 23_200_000} {
		if err := verifyBlockCheckpoint(config, number, common.Hash{1}); err != nil {
			t.Fatal(err)
		}
	}
	for _, config := range []*params.ChainConfig{nil, {}, {ChainID: big.NewInt(59902)}} {
		if err := verifyBlockCheckpoint(config, 100_000, common.Hash{1}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCheckpointImportEntrypoints(t *testing.T) {
	for _, name := range []string{"blocks", "blocks_callback", "receipts_hot", "receipts_ancient", "headers_validate", "headers_insert_known", "header_write", "block_write", "block_without_state", "known_block", "body_known", "fast_head"} {
		t.Run(name, func(t *testing.T) {
			db := checkpointDB(t)
			config := checkpointConfig()
			hc, err := NewHeaderChain(db, config, ethash.NewFullFaker(), func() bool { return false })
			if err != nil {
				t.Fatal(err)
			}
			bc := &BlockChain{chainConfig: config, db: db, hc: hc}
			first := &types.Header{Number: big.NewInt(99_999), Difficulty: big.NewInt(1)}
			bad := &types.Header{Number: big.NewInt(100_000), Difficulty: big.NewInt(1), ParentHash: first.Hash()}
			block := types.NewBlockWithHeader(bad)
			blocks := types.Blocks{types.NewBlockWithHeader(first), block}
			headers := []*types.Header{first, bad}
			// Simulate a checkpoint already stored by a pre-checkpoint client.
			rawdb.WriteBlock(db, block)
			rawdb.WriteTd(db, block.Hash(), block.NumberU64(), big.NewInt(2))
			before := checkpointDBSnapshot(db)
			index := -1
			switch name {
			case "blocks":
				index, err = bc.InsertChain(blocks)
			case "blocks_callback":
				index, err = bc.InsertChainWithFunc(blocks, nil)
			case "receipts_hot":
				index, err = bc.InsertReceiptChain(blocks, make([]types.Receipts, 2), 0)
			case "receipts_ancient":
				index, err = bc.InsertReceiptChain(blocks, make([]types.Receipts, 2), 100_000)
			case "headers_validate":
				index, err = hc.ValidateHeaderChain(headers, 0)
			case "headers_insert_known":
				index, err = hc.InsertHeaderChain(headers, func(*types.Header) error { t.Fatal("write callback called"); return nil }, time.Now())
			case "header_write":
				_, err = hc.WriteHeader(bad)
			case "block_write":
				_, err = bc.WriteBlockWithState(block, nil, nil, nil, true)
			case "block_without_state":
				err = bc.writeBlockWithoutState(block, big.NewInt(2))
			case "known_block":
				err = bc.writeKnownBlock(block)
			case "body_known":
				err = NewBlockValidator(config, bc, ethash.NewFullFaker()).ValidateBody(block)
			case "fast_head":
				bc.blockCache, _ = lru.New(16)
				err = bc.FastSyncCommitHead(block.Hash())
			}
			requireCheckpointError(t, err, 100_000, block.Hash())
			if index != -1 && index != 1 {
				t.Fatalf("failure index %d, want 1", index)
			}
			if after := checkpointDBSnapshot(db); !reflect.DeepEqual(before, after) {
				t.Fatal("rejected checkpoint changed database")
			}
		})
	}
}

func TestCheckpointHeaderImportAccepts(t *testing.T) {
	db := checkpointDB(t)
	hc, err := NewHeaderChain(db, checkpointConfig(), ethash.NewFullFaker(), func() bool { return false })
	if err != nil {
		t.Fatal(err)
	}
	header := checkpointHeaders(t)[1]
	// Seed the parent metadata required by WriteHeader without replaying 100k blocks.
	rawdb.WriteTd(db, header.ParentHash, 99_999, big.NewInt(100_000))
	rawdb.WriteCanonicalHash(db, header.ParentHash, 99_999)
	if _, err := hc.ValidateHeaderChain([]*types.Header{header}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := hc.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if got := rawdb.ReadCanonicalHash(db, 100_000); got != header.Hash() {
		t.Fatalf("wrong canonical hash: %s", got)
	}
	if _, err := hc.InsertHeaderChain([]*types.Header{header}, func(*types.Header) error { t.Fatal("known header rewritten"); return nil }, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestCheckpointStartup(t *testing.T) {
	for _, number := range []uint64{0, 100_000, 23_100_000} {
		t.Run(new(big.Int).SetUint64(number).String(), func(t *testing.T) {
			db := checkpointDB(t)
			bad := common.Hash{0xff}
			rawdb.WriteCanonicalHash(db, bad, number)
			before := checkpointDBSnapshot(db)
			_, err := NewHeaderChain(db, checkpointConfig(), ethash.NewFullFaker(), func() bool { return false })
			requireCheckpointError(t, err, number, bad)
			_, err = NewBlockChain(db, nil, checkpointConfig(), ethash.NewFullFaker(), vm.Config{}, nil)
			requireCheckpointError(t, err, number, bad)
			if !reflect.DeepEqual(before, checkpointDBSnapshot(db)) {
				t.Fatal("startup modified conflicting database")
			}
		})
	}
	db := checkpointDB(t)
	if _, err := NewHeaderChain(db, checkpointConfig(), ethash.NewFullFaker(), func() bool { return false }); err != nil {
		t.Fatal(err)
	}
	for _, header := range checkpointHeaders(t) {
		rawdb.WriteCanonicalHash(db, header.Hash(), header.Number.Uint64())
	}
	if _, err := NewHeaderChain(db, checkpointConfig(), ethash.NewFullFaker(), func() bool { return false }); err != nil {
		t.Fatal(err)
	}
	foreign := *checkpointConfig()
	foreign.ChainID = big.NewInt(59902)
	rawdb.WriteCanonicalHash(db, common.Hash{0xff}, 100_000)
	if _, err := NewHeaderChain(db, &foreign, ethash.NewFullFaker(), func() bool { return false }); err != nil {
		t.Fatal(err)
	}
}

// Genesis setup retains its original semantics; checkpoint enforcement begins
// when opening the header/block chain, not in genesis.go.
func TestCheckpointGenesisSetupDoesNotValidate(t *testing.T) {
	db := rawdb.NewMemoryDatabase()
	defer db.Close()
	genesis := &Genesis{Config: checkpointConfig(), GasLimit: params.GenesisGasLimit, Difficulty: big.NewInt(1)}
	if _, _, err := SetupGenesisBlock(db, genesis); err != nil {
		t.Fatal(err)
	}
	// Existing historical conflicts must not trigger checkpoint validation during setup either.
	rawdb.WriteCanonicalHash(db, common.Hash{0xff}, 100_000)
	if _, _, err := SetupGenesisBlock(db, genesis); err != nil {
		t.Fatal(err)
	}
	if _, _, err := SetupGenesisBlock(db, nil); err != nil {
		t.Fatal(err)
	}
	block := genesis.ToBlock(nil)
	_, err := NewBlockChain(db, nil, genesis.Config, ethash.NewFullFaker(), vm.Config{}, nil)
	requireCheckpointError(t, err, 0, block.Hash())
	bc := &BlockChain{chainConfig: genesis.Config, db: db}
	requireCheckpointError(t, bc.ResetWithGenesisBlock(block), 0, block.Hash())
	// The original database/genesis compatibility check still applies.
	other := *genesis
	other.ExtraData = []byte{1}
	_, _, err = SetupGenesisBlock(db, &other)
	var mismatch *GenesisMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("want genesis compatibility error, got %v", err)
	}
}

func TestCheckpointStartupAncient(t *testing.T) {
	// A real freezer tests canonical reads with no hot canonical mapping at all.
	for _, custom := range []bool{false, true} {
		t.Run(map[bool]string{false: "genesis", true: "custom_genesis"}[custom], func(t *testing.T) {
			db, err := rawdb.NewDatabaseWithFreezer(memorydb.New(), t.TempDir(), "")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			header := checkpointHeaders(t)[0]
			if custom {
				header.Extra = append(header.Extra, 1)
			}
			block := types.NewBlockWithHeader(header)
			rawdb.WriteAncientBlock(db, block, nil, big.NewInt(1))
			before, _ := db.Ancients()
			_, err = NewHeaderChain(db, checkpointConfig(), ethash.NewFullFaker(), func() bool { return false })
			if custom {
				requireCheckpointError(t, err, 0, block.Hash())
			} else if err != nil {
				t.Fatal(err)
			}
			if after, _ := db.Ancients(); after != before {
				t.Fatal("startup truncated freezer")
			}
		})
	}
}

func TestCheckpointRejectsStoredSideChainPromotion(t *testing.T) {
	for _, mode := range []string{"headers", "blocks"} {
		t.Run(mode, func(t *testing.T) {
			db := checkpointDB(t)
			hc, err := NewHeaderChain(db, checkpointConfig(), ethash.NewFullFaker(), func() bool { return false })
			if err != nil {
				t.Fatal(err)
			}
			parent := types.NewBlockWithHeader(&types.Header{Number: big.NewInt(99_999), Difficulty: big.NewInt(1)})
			bad := types.NewBlockWithHeader(&types.Header{Number: big.NewInt(100_000), Difficulty: big.NewInt(1), ParentHash: parent.Hash()})
			tip := types.NewBlockWithHeader(&types.Header{Number: big.NewInt(100_001), Difficulty: big.NewInt(1), ParentHash: bad.Hash()})
			for _, block := range []*types.Block{parent, bad, tip} {
				rawdb.WriteBlock(db, block)
				rawdb.WriteTd(db, block.Hash(), block.NumberU64(), new(big.Int).SetUint64(block.NumberU64()))
			}
			rawdb.WriteCanonicalHash(db, parent.Hash(), parent.NumberU64())
			rawdb.WriteHeadHeaderHash(db, parent.Hash())
			hc.SetCurrentHeader(parent.Header())
			before := checkpointDBSnapshot(db)
			if mode == "headers" {
				_, err = hc.WriteHeader(tip.Header())
			} else {
				bc := &BlockChain{chainConfig: checkpointConfig(), db: db, hc: hc}
				bc.blockCache, _ = lru.New(16)
				err = bc.reorg(parent, tip)
			}
			requireCheckpointError(t, err, 100_000, bad.Hash())
			if !reflect.DeepEqual(before, checkpointDBSnapshot(db)) {
				t.Fatal("rejected side-chain promotion changed database")
			}
			if hc.CurrentHeader().Hash() != parent.Hash() {
				t.Fatal("rejected side-chain promotion changed head")
			}
		})
	}
}

func TestCheckpointInvalidHeaderNumbers(t *testing.T) {
	for _, number := range []*big.Int{nil, big.NewInt(-1), new(big.Int).Lsh(big.NewInt(1), 64)} {
		header := &types.Header{Number: number}
		if err := verifyHeaderCheckpoint(checkpointConfig(), header); err == nil {
			t.Fatalf("accepted invalid block number %v", number)
		}
		if err := verifyHeaderCheckpoint(params.TestChainConfig, header); err != nil {
			t.Fatalf("changed other chain's validation: %v", err)
		}
	}
}

// checkpointAncientView exposes a sparse ancient checkpoint without constructing
// 100,000 preceding blocks; the underlying hot database contains the Andromeda genesis.
type checkpointAncientView struct {
	ethdb.Database
	hash common.Hash
}

func (db checkpointAncientView) Ancient(kind string, number uint64) ([]byte, error) {
	if kind == "hashes" && number == 100_000 {
		return db.hash.Bytes(), nil
	}
	return db.Database.Ancient(kind, number)
}

func TestCheckpointStartupAncientHistory(t *testing.T) {
	want, _ := params.BlockHashCheckpoint(big.NewInt(1088), 100_000)
	for _, hash := range []common.Hash{want, {0xff}} {
		db := checkpointAncientView{Database: checkpointDB(t), hash: hash}
		before := checkpointDBSnapshot(db)
		_, err := NewHeaderChain(db, checkpointConfig(), ethash.NewFullFaker(), func() bool { return false })
		if hash == want {
			if err != nil {
				t.Fatal(err)
			}
		} else {
			requireCheckpointError(t, err, 100_000, hash)
		}
		if !reflect.DeepEqual(before, checkpointDBSnapshot(db)) {
			t.Fatal("startup changed hot database")
		}
	}
}
