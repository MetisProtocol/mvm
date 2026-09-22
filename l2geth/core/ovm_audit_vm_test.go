package core

import (
	"math/big"
	"testing"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/core/rawdb"
	"github.com/MetisProtocol/mvm/l2geth/core/state"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/core/vm"
	"github.com/MetisProtocol/mvm/l2geth/params"
	"github.com/MetisProtocol/mvm/l2geth/rollup/dump"
	"github.com/MetisProtocol/mvm/l2geth/rollup/rcfg"
)

func TestOVMAuditEVMDestruction(t *testing.T) {
	old := rcfg.UsingOVM
	rcfg.UsingOVM = true
	defer func() { rcfg.UsingOVM = old }()
	for _, updated := range []bool{false, true} {
		for _, self := range []bool{false, true} {
			for _, revert := range []bool{false, true} {
				cfg := *params.TestChainConfig
				cfg.ChainID = big.NewInt(1088)
				cfg.ShanghaiBlock = nil
				height := int64(749999)
				if updated {
					height++
				}
				from, a, b, outer := common.Address{1}, common.Address{2}, common.Address{3}, common.Address{4}
				if self {
					b = a
				}
				var roots [2]common.Hash
				for enabled := 0; enabled < 2; enabled++ {
					db := rawdb.NewMemoryDatabase()
					s, err := state.New(common.Hash{}, state.NewDatabase(db))
					if err != nil {
						t.Fatal(err)
					}
					s.SetCode(dump.OvmEthAddress, []byte{0})
					s.SetBalance(from, big.NewInt(100))
					s.SetBalance(a, big.NewInt(7))
					code := append([]byte{byte(vm.PUSH20)}, b[:]...)
					code = append(code, byte(vm.SELFDESTRUCT))
					s.SetCode(a, code)
					target := a
					if revert {
						// CALL A with no value, then revert the enclosing call.
						call := []byte{0x60, 0, 0x60, 0, 0x60, 0, 0x60, 0, 0x60, 0, 0x73}
						call = append(call, a[:]...)
						call = append(call, 0x62, 0x0f, 0xff, 0xff, 0xf1, 0x60, 0, 0x60, 0, 0xfd)
						s.SetCode(outer, call)
						target = outer
					}
					o := &state.OVMAuditObserver{Index: state.NewOVMAuditIndex(db), Total: state.ZeroOVMAuditDelta()}
					if enabled == 1 {
						s.SetOVMAudit(o)
						s.BeginOVMAuditScope("transaction", common.Hash{1}, 0)
					}
					evm := vm.NewEVM(vm.Context{CanTransfer: CanTransfer, Transfer: Transfer, Origin: from, BlockNumber: big.NewInt(height), Time: big.NewInt(0), Difficulty: big.NewInt(0), GasPrice: big.NewInt(1), GasLimit: 2000000}, s, &cfg, vm.Config{})
					_, _, err = evm.Call(vm.AccountRef(from), target, nil, 1500000, big.NewInt(0))
					if (err != nil) != revert {
						t.Fatalf("updated=%v self=%v revert=%v: %v", updated, self, revert, err)
					}
					s.Finalise(true)
					s.EndOVMAuditScope(err != nil)
					roots[enabled] = s.IntermediateRoot(true)
					if enabled == 1 {
						want := "0"
						if !updated && !revert {
							want = "7"
						}
						if o.Err != nil || o.Total.Difference != want || o.Total.Suicide != want || o.Total.Unexplained != "0" {
							t.Fatalf("updated=%v self=%v revert=%v: %+v %v", updated, self, revert, o.Total, o.Err)
						}
						if len(o.Events) != 1 || len(o.Events[0].Causes) != 1 || o.Events[0].Causes[0].Reverted != revert {
							t.Fatalf("lost event: %+v", o.Events)
						}
					}
					db.Close()
				}
				if roots[0] != roots[1] {
					t.Fatal("audit altered execution root")
				}
			}
		}
	}
}

func TestOVMAuditHistoricalGasAndFailedExecution(t *testing.T) {
	old := rcfg.UsingOVM
	rcfg.UsingOVM = true
	defer func() { rcfg.UsingOVM = old }()
	for _, height := range []int64{3247674, 3247675, 3247681} {
		for _, revert := range []bool{false, true} {
			cfg := *params.TestChainConfig
			cfg.ChainID = big.NewInt(1088)
			cfg.ShanghaiBlock = nil
			db := rawdb.NewMemoryDatabase()
			s, _ := state.New(common.Hash{}, state.NewDatabase(db))
			defer db.Close()
			from, to := common.Address{1}, common.Address{2}
			s.SetCode(dump.OvmEthAddress, []byte{0})
			if revert {
				s.SetCode(to, []byte{0x60, 0, 0x60, 0, 0xfd})
			}
			o := &state.OVMAuditObserver{Index: state.NewOVMAuditIndex(db), Total: state.ZeroOVMAuditDelta()}
			s.SetOVMAudit(o)
			s.BeginOVMAuditScope("transaction", common.Hash{1}, 0)
			evm := vm.NewEVM(vm.Context{CanTransfer: CanTransfer, Transfer: Transfer, Origin: from, Coinbase: dump.OvmFeeWallet, BlockNumber: big.NewInt(height), Time: big.NewInt(0), Difficulty: big.NewInt(0), GasPrice: big.NewInt(1), GasLimit: 1000000}, s, &cfg, vm.Config{})
			msg := types.NewMessage(from, &to, 0, big.NewInt(0), 200000, big.NewInt(1), nil, true, big.NewInt(0), 0, types.QueueOriginSequencer)
			_, gas, failed, err := ApplyMessageWithBlockNumber(evm, msg, new(GasPool).AddGas(1000000), uint64(height))
			if err != nil {
				t.Fatal(err)
			}
			s.Finalise(true)
			s.EndOVMAuditScope(failed)
			want := uint64(21000)
			if height == 3247675 || height == 3247681 {
				want += 100000
			}
			if revert {
				want += 6
			}
			if gas != want || failed != revert {
				t.Fatalf("height=%d revert=%v gas=%d want=%d failed=%v", height, revert, gas, want, failed)
			}
			if o.Err != nil || o.Total.Difference != "200000" || o.Total.Gas != "200000" || o.Total.Unexplained != "0" {
				t.Fatalf("%+v %v", o.Total, o.Err)
			}
		}
	}
}
