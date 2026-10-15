package eth

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/MetisProtocol/mvm/l2geth/common/hexutil"
	"github.com/MetisProtocol/mvm/l2geth/consensus/ethash"
	"github.com/MetisProtocol/mvm/l2geth/core"
	"github.com/MetisProtocol/mvm/l2geth/core/rawdb"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/core/vm"
	"github.com/MetisProtocol/mvm/l2geth/ethdb"
	"github.com/MetisProtocol/mvm/l2geth/internal/ethapi"
	"github.com/MetisProtocol/mvm/l2geth/node"
	"github.com/MetisProtocol/mvm/l2geth/params"
	"github.com/MetisProtocol/mvm/l2geth/rollup/rcfg"
	"github.com/MetisProtocol/mvm/l2geth/rpc"
)

// The tip's undo is intact, so startup succeeds. Rewinding across block two
// fails, which must prevent config publication and propagate through debug RPC.
func indexRewindFixture(t *testing.T, db ethdb.Database) (*core.Genesis, *core.BlockChain, types.Blocks) {
	t.Helper()
	old := rcfg.UsingOVM
	rcfg.UsingOVM = true
	t.Cleanup(func() { rcfg.UsingOVM = old })
	cfg := *params.AllEthashProtocolChanges
	cfg.IstanbulBlock = big.NewInt(2)
	cfg.MuirGlacierBlock = nil
	cfg.BerlinBlock = nil
	cfg.ShanghaiBlock = nil
	genesis := &core.Genesis{Config: &cfg, GasLimit: params.GenesisGasLimit}
	parent := genesis.MustCommit(db)
	chain, err := core.NewBlockChain(db, &core.CacheConfig{TrieDirtyDisabled: true, TrieCleanNoPrefetch: true}, &cfg, ethash.NewFaker(), vm.Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(chain.Stop)
	blocks, _ := core.GenerateChain(&cfg, parent, ethash.NewFaker(), db, 3, nil)
	if _, err := chain.InsertChain(blocks); err != nil {
		t.Fatal(err)
	}
	key := append(append([]byte(nil), rawdb.OVMTransferUndoPrefix...), blocks[1].Hash().Bytes()...)
	if err := db.Delete(key); err != nil {
		t.Fatal(err)
	}
	return genesis, chain, blocks
}

func TestOVMIndexConfigRewindFailure(t *testing.T) {
	stack, err := node.New(&node.Config{Name: "index-rewind", DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stack.Close() })
	db, err := stack.OpenDatabaseWithFreezer("chaindata", 16, 16, "", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	genesis, chain, blocks := indexRewindFixture(t, db)
	chain.Stop()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	updated := *genesis.Config
	updated.IstanbulBlock = big.NewInt(3)
	genesis.Config = &updated
	config := DefaultConfig
	config.Genesis = genesis
	config.Ethash.PowMode = ethash.ModeFake
	config.NoPruning = true
	config.NoPrefetch = true
	if err := stack.Register(func(ctx *node.ServiceContext) (node.Service, error) {
		return New(ctx, &config)
	}); err != nil {
		t.Fatal(err)
	}
	err = stack.Start()
	if err == nil || !strings.Contains(err.Error(), "rewind chain for configuration upgrade") || !strings.Contains(err.Error(), "missing or malformed OVM undo") {
		t.Fatalf("expected startup rewind failure, got %v", err)
	}
	// The failed constructor must release the DB so that an operator can
	// reopen it, and must retain the old config and executed chain head.
	db, err = stack.OpenDatabaseWithFreezer("chaindata", 16, 16, "", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	stored := rawdb.ReadChainConfig(db, genesis.ToBlock(nil).Hash())
	if stored == nil || stored.IstanbulBlock.Cmp(big.NewInt(2)) != 0 {
		t.Fatalf("failed rewind published new config: %v", stored)
	}
	if rawdb.ReadHeadBlockHash(db) != blocks[2].Hash() {
		t.Fatal("failed rewind changed executed head")
	}
}

func TestOVMIndexDebugRewindFailure(t *testing.T) {
	db := rawdb.NewMemoryDatabase()
	t.Cleanup(func() { db.Close() })
	_, chain, blocks := indexRewindFixture(t, db)
	backend := &EthAPIBackend{eth: &Ethereum{blockchain: chain}, UsingOVM: true}
	server := rpc.NewServer()
	defer server.Stop()
	if err := server.RegisterName("debug", ethapi.NewPrivateDebugAPI(backend)); err != nil {
		t.Fatal(err)
	}
	client := rpc.DialInProc(server)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := client.CallContext(ctx, nil, "debug_setHead", hexutil.Uint64(1))
	if err == nil || !strings.Contains(err.Error(), "missing or malformed OVM undo") {
		t.Fatalf("debug_setHead swallowed rewind failure: %v", err)
	}
	if chain.CurrentBlock().Hash() != blocks[2].Hash() {
		t.Fatal("failed debug rewind changed head")
	}
	// Restore the fixture's empty undo, then retry through the same RPC.
	// Success must still return null and perform the requested rewind.
	if err := rawdb.WriteOVMTransferUndo(db, blocks[1].Hash(), nil); err != nil {
		t.Fatal(err)
	}
	var result interface{}
	if err := client.CallContext(ctx, &result, "debug_setHead", hexutil.Uint64(1)); err != nil {
		t.Fatal(err)
	}
	if result != nil || chain.CurrentBlock().Hash() != blocks[0].Hash() {
		t.Fatalf("successful debug rewind: result=%v, head=%s", result, chain.CurrentBlock().Hash())
	}
	ready, err := rawdb.ReadOVMIndexPosition(db, rawdb.OVMIndexReadyKey)
	if err != nil || ready == nil || ready.Number != 1 || ready.Hash != blocks[0].Hash() {
		t.Fatalf("successful debug rewind left index unready: %v, %v", ready, err)
	}
}
