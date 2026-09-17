package core

import (
	"bytes"
	"math/big"
	"path/filepath"
	"testing"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/consensus/ethash"
	"github.com/MetisProtocol/mvm/l2geth/core/rawdb"
	"github.com/MetisProtocol/mvm/l2geth/core/state"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/core/vm"
	"github.com/MetisProtocol/mvm/l2geth/params"
	"github.com/MetisProtocol/mvm/l2geth/rollup/dump"
	"github.com/MetisProtocol/mvm/l2geth/rollup/rcfg"
	"github.com/ethereum/go-ethereum/crypto"
)

// The fixture executes a native transfer and a small ERC20-style program which
// writes balance slots directly and emits Transfer. The ERC20 holders never get
// account leaves, so trie preimages cannot accidentally satisfy the assertion.
func ovmPreimageBlocks(t *testing.T) (*Genesis, []*types.Block, []common.Address) {
	t.Helper()
	old := rcfg.UsingOVM
	rcfg.UsingOVM = true
	t.Cleanup(func() { rcfg.UsingOVM = old })
	key, err := crypto.HexToECDSA("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	sender := crypto.PubkeyToAddress(key.PublicKey)
	recipient, from, to := common.HexToAddress("0x1111"), common.HexToAddress("0x2222"), common.HexToAddress("0x3333")
	var code []byte
	pushHash := func(h common.Hash) { code = append(code, 0x7f); code = append(code, h[:]...) }
	code = append(code, 0x60, 4)
	pushHash(state.GetOVMBalanceKey(from))
	code = append(code, 0x55)
	code = append(code, 0x60, 7)
	pushHash(state.GetOVMBalanceKey(to))
	code = append(code, 0x55)
	code = append(code, 0x60, 7, 0x60, 0, 0x52) // MSTORE(0, 7)
	pushHash(common.BytesToHash(to[:]))
	pushHash(common.BytesToHash(from[:]))
	pushHash(common.BytesToHash(crypto.Keccak256([]byte("Transfer(address,address,uint256)"))))
	code = append(code, 0x60, 32, 0x60, 0, 0xa3, 0x00) // LOG3; STOP
	cfg := *params.AllEthashProtocolChanges
	cfg.ChainID = big.NewInt(108)
	cfg.BerlinBlock = new(big.Int)
	cfg.ShanghaiBlock = nil
	genesis := &Genesis{Config: &cfg, GasLimit: params.GenesisGasLimit, Alloc: GenesisAlloc{
		sender: {Balance: big.NewInt(100)},
		rcfg.L2GasPriceOracleAddress: {Balance: new(big.Int), Storage: map[common.Hash]common.Hash{
			rcfg.L2GasPriceSlot: common.BigToHash(big.NewInt(1)),
		}},
		dump.OvmEthAddress: {Balance: new(big.Int), Code: code, Storage: map[common.Hash]common.Hash{
			state.GetOVMBalanceKey(from): common.BigToHash(big.NewInt(11)),
		}},
	}}
	db := rawdb.NewMemoryDatabase()
	defer db.Close()
	parent := genesis.MustCommit(db)
	blocks, _ := GenerateChain(&cfg, parent, ethash.NewFaker(), db, 2, func(i int, b *BlockGen) {
		dest, value := recipient, big.NewInt(5)
		if i == 1 {
			dest, value = dump.OvmEthAddress, new(big.Int)
		}
		tx := types.NewTransaction(uint64(i), dest, value, 100000, new(big.Int), nil)
		tx.SetL2Tx(1)
		tx, err = types.SignTx(tx, types.MakeSigner(&cfg, b.Number()), key)
		if err != nil {
			t.Fatal(err)
		}
		b.AddTx(tx)
	})
	return genesis, blocks, []common.Address{sender, recipient, from, to}
}

func TestOVMPreimageConsensus(t *testing.T) {
	_, blocks, _ := ovmPreimageBlocks(t)
	// Captured with the pre-change StateDB via a Go source overlay. Address
	// recording must preserve the state, gas and consensus receipt bytes.
	want := []struct {
		root     string
		gas      uint64
		receipts string
	}{
		{"0x72ee9cf0e1a003e4325a79c51ce510594e26815e58955c8fc2d20e1f586d6798", 21000, "0x056b23fbba480696b65fe5a59b8f2148a1299103c4f57df839233af2cf4ca2d2"},
		{"0x249a35bade0a4d8fb8f832c0c2c1b94508b8d4eb59e2278ec77615a8b09a1b59", 49895, "0x3ad1122e3006cf55092634f7f86461006cc6f93b99ecd0794c75aa65962d83e0"},
	}
	for i, b := range blocks {
		if b.Root() != common.HexToHash(want[i].root) || b.GasUsed() != want[i].gas || b.ReceiptHash() != common.HexToHash(want[i].receipts) {
			t.Fatalf("block %d root=%s gas=%d receipts=%s", i+1, b.Root(), b.GasUsed(), b.ReceiptHash())
		}
	}
}

func TestOVMPreimageRevertedExecution(t *testing.T) {
	genesis, _, addresses := ovmPreimageBlocks(t)
	account := genesis.Alloc[dump.OvmEthAddress]
	// Run the same storage writes and LOG3, then revert instead of stopping.
	account.Code = append(account.Code[:len(account.Code)-1], 0x60, 0, 0x60, 0, 0xfd)
	genesis.Alloc[dump.OvmEthAddress] = account
	db := rawdb.NewMemoryDatabase()
	defer db.Close()
	parent := genesis.MustCommit(db)
	s, err := state.New(parent.Root(), state.NewDatabase(db))
	if err != nil {
		t.Fatal(err)
	}
	key, err := crypto.HexToECDSA("0123456789012345678901234567890123456789012345678901234567890123")
	if err != nil {
		t.Fatal(err)
	}
	header := &types.Header{Number: big.NewInt(1), GasLimit: params.GenesisGasLimit}
	tx := types.NewTransaction(0, dump.OvmEthAddress, big.NewInt(5), 100000, new(big.Int), nil)
	tx.SetL2Tx(1)
	tx, err = types.SignTx(tx, types.MakeSigner(genesis.Config, header.Number), key)
	if err != nil {
		t.Fatal(err)
	}
	s.Prepare(tx.Hash(), common.Hash{}, 0)
	receipt, err := ApplyTransaction(genesis.Config, nil, &header.Coinbase, new(GasPool).AddGas(header.GasLimit), s, header, tx, &header.GasUsed, vm.Config{EnablePreimageRecording: false})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Status != types.ReceiptStatusFailed || len(receipt.Logs) != 0 {
		t.Fatal("expected reverted receipt without logs")
	}
	for _, a := range []common.Address{addresses[2], addresses[3], dump.OvmEthAddress} {
		if _, exists := s.Preimages()[common.BytesToHash(crypto.Keccak256(a[:]))]; exists {
			t.Fatalf("reverted execution retained preimage for %s", a)
		}
	}
	if !bytes.Equal(s.Preimages()[common.BytesToHash(crypto.Keccak256(addresses[0][:]))], addresses[0][:]) {
		t.Fatal("revert removed sender preimage recorded before the call")
	}
	for i, want := range []int64{100, 0, 11, 0} {
		if s.GetBalance(addresses[i]).Int64() != want {
			t.Fatalf("reverted balance %s: %s", addresses[i], s.GetBalance(addresses[i]))
		}
	}
}

func TestOVMPreimageBlockPersistence(t *testing.T) {
	genesis, blocks, addresses := ovmPreimageBlocks(t)
	path := filepath.Join(t.TempDir(), "chaindata")
	db, err := rawdb.NewLevelDBDatabase(path, 16, 16, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	genesis.MustCommit(db)
	chain, err := NewBlockChain(db, &CacheConfig{TrieDirtyDisabled: true}, genesis.Config, ethash.NewFaker(), vm.Config{EnablePreimageRecording: false}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(chain.Stop)
	// Generator and importer use independent databases, forcing real execution.
	if _, err := chain.InsertChain(blocks); err != nil {
		t.Fatal(err)
	}
	s, err := chain.State()
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []int64{95, 5, 4, 7} {
		if s.GetBalance(addresses[i]).Int64() != want {
			t.Fatalf("balance %s: got %s want %d", addresses[i], s.GetBalance(addresses[i]), want)
		}
		if s.GetState(dump.OvmEthAddress, state.GetOVMBalanceKey(addresses[i])) != common.BigToHash(big.NewInt(want)) {
			t.Fatal("wrong balance storage")
		}
	}
	if s.Exist(addresses[2]) || s.Exist(addresses[3]) {
		t.Fatal("ERC20 holder acquired an account leaf")
	}
	receipts := chain.GetReceiptsByHash(blocks[1].Hash())
	if len(receipts) != 1 || receipts[0].Status != 1 || len(receipts[0].Logs) != 1 {
		t.Fatal("ERC20 program did not execute")
	}
	event := receipts[0].Logs[0]
	if event.Address != dump.OvmEthAddress || len(event.Topics) != 3 || event.Topics[1] != common.BytesToHash(addresses[2][:]) || event.Topics[2] != common.BytesToHash(addresses[3][:]) || !bytes.Equal(event.Data, common.LeftPadBytes([]byte{7}, 32)) {
		t.Fatal("wrong Transfer receipt")
	}
	chain.Stop()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = rawdb.NewLevelDBDatabase(path, 16, 16, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range addresses {
		hash := common.BytesToHash(crypto.Keccak256(a[:]))
		if got := rawdb.ReadPreimage(db, hash); !bytes.Equal(got, a[:]) {
			t.Fatalf("persisted preimage %s: %x", a, got)
		}
		key := append([]byte("secure-key-"), hash[:]...)
		if got, err := db.Get(key); err != nil || !bytes.Equal(got, a[:]) {
			t.Fatalf("migration preimage %s: %x, %v", a, got, err)
		}
	}
}
