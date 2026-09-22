package eth

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/consensus/ethash"
	"github.com/MetisProtocol/mvm/l2geth/core"
	"github.com/MetisProtocol/mvm/l2geth/core/rawdb"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/core/vm"
	"github.com/MetisProtocol/mvm/l2geth/eth/downloader"
	"github.com/MetisProtocol/mvm/l2geth/event"
	"github.com/MetisProtocol/mvm/l2geth/params"
	"github.com/MetisProtocol/mvm/l2geth/rollup/dump"
	"github.com/naoina/toml"
)

func TestOVMAuditConfigRoundTrip(t *testing.T) {
	want := DefaultConfig
	want.OVMAudit = core.OVMAuditConfig{Enabled: true, To: 12, ToHash: common.Hash{1}, Dir: "reports", Witness: "witness.jsonl"}
	want.SyncMode = downloader.FullSync
	enc, err := want.MarshalTOML()
	if err != nil {
		t.Fatal(err)
	}
	settings := toml.Config{NormFieldName: func(_ reflect.Type, k string) string { return k }, FieldToKey: func(_ reflect.Type, k string) string { return k }}
	blob, err := settings.Marshal(enc)
	if err != nil {
		t.Fatal(err)
	}
	var got Config
	if err = got.UnmarshalTOML(func(v interface{}) error { return settings.Unmarshal(blob, v) }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want.OVMAudit, got.OVMAudit) || got.SyncMode != want.SyncMode {
		t.Fatalf("%+v", got.OVMAudit)
	}
}

func TestOVMAuditNeverFallsBackToFast(t *testing.T) {
	db := rawdb.NewMemoryDatabase()
	defer db.Close()
	cfg := *params.TestChainConfig
	cfg.Clique = &params.CliqueConfig{Period: 1, Epoch: 30000}
	gen := (&core.Genesis{Config: &cfg, Difficulty: big.NewInt(1), Alloc: core.GenesisAlloc{dump.OvmEthAddress: {Balance: big.NewInt(0), Code: []byte{0}}}}).MustCommit(db)
	engine := ethash.NewFaker()
	bc, err := core.NewBlockChain(db, nil, &cfg, engine, vm.Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	audit := core.OVMAuditConfig{Enabled: true, To: 10, ToHash: common.Hash{1}, Dir: t.TempDir()}
	if err = bc.EnableOVMAudit(audit); err != nil {
		t.Fatal(err)
	}
	bc.Stop()
	// Reproduce the recovery shape which normally enables automatic fast sync.
	fast := types.NewBlockWithHeader(&types.Header{Number: big.NewInt(1), ParentHash: gen.Hash(), Root: gen.Root(), Difficulty: big.NewInt(1), GasLimit: gen.GasLimit()})
	rawdb.WriteBlock(db, fast)
	rawdb.WriteTd(db, fast.Hash(), 1, big.NewInt(2))
	rawdb.WriteHeadFastBlockHash(db, fast.Hash())
	bc, err = core.NewBlockChain(db, nil, &cfg, engine, vm.Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer bc.Stop()
	if err = bc.EnableOVMAudit(audit); err != nil {
		t.Fatal(err)
	}
	pm, err := NewProtocolManager(&cfg, nil, downloader.FullSync, 1, new(event.TypeMux), &testTxPool{}, engine, bc, db, 1, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.downloader.Terminate()
	if pm.fastSync != 0 {
		t.Fatal("audit switched to fast sync")
	}
	s := &Ethereum{blockchain: bc}
	if err = s.StartMining(1); err == nil {
		t.Fatal("runtime mining accepted")
	}
	if err = bc.FastSyncCommitHead(fast.Hash()); !core.IsOVMAuditControl(err) {
		t.Fatalf("fast pivot accepted: %v", err)
	}
}
