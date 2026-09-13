package core

import (
	"math/big"
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

func TestStatePrefetcherRFD(t *testing.T) {
	// UsingOVM is global; these cases must run serially.
	for _, tc := range []struct {
		name    string
		chainID *big.Int
		height  int64
		ovm     bool
		balance int64
		skip    bool
	}{
		{"mainnet_before_RFD", params.MetisMainnetChainID, params.MetisMainnetRFDUpdateForkNum.Int64() - 1, true, 50000, true},
		{"mainnet_exact_gas_balance", params.MetisMainnetChainID, params.MetisMainnetRFDUpdateForkNum.Int64() - 1, true, 100000, true},
		{"mainnet_at_RFD", params.MetisMainnetChainID, params.MetisMainnetRFDUpdateForkNum.Int64(), true, 50000, false},
		{"mainnet_after_RFD", params.MetisMainnetChainID, params.MetisMainnetRFDUpdateForkNum.Int64() + 1, true, 50000, false},
		{"sepolia_before_RFD", params.MetisSepoliaChainID, params.MetisSepoliaRFDUpdateForkNum.Int64() - 1, true, 50000, true},
		{"sepolia_at_RFD", params.MetisSepoliaChainID, params.MetisSepoliaRFDUpdateForkNum.Int64(), true, 50000, false},
		{"sepolia_after_RFD", params.MetisSepoliaChainID, params.MetisSepoliaRFDUpdateForkNum.Int64() + 1, true, 50000, false},
		{"other_OVM_chain", big.NewInt(108), 1, true, 50000, false},
		{"non_OVM_before_RFD", params.MetisMainnetChainID, 1, false, 200000, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			old := rcfg.UsingOVM
			rcfg.UsingOVM = tc.ovm
			t.Cleanup(func() { rcfg.UsingOVM = old })
			cfg := *params.AllEthashProtocolChanges
			cfg.ChainID = new(big.Int).Set(tc.chainID)
			cfg.BerlinBlock = new(big.Int)
			cfg.ShanghaiBlock = nil
			db, err := state.New(common.Hash{}, state.NewDatabase(rawdb.NewMemoryDatabase()))
			if err != nil {
				t.Fatal(err)
			}
			key, err := crypto.HexToECDSA("0123456789012345678901234567890123456789012345678901234567890123")
			if err != nil {
				t.Fatal(err)
			}
			sender := crypto.PubkeyToAddress(key.PublicKey)
			slot := state.GetOVMBalanceKey(sender)
			// Gas purchase drains this slot without adding a clearing refund.
			// Before RFD, the following SSTORE would then SubRefund(4800)
			// from zero. Exercise the actual prefetch entry point.
			// Write 1 to the sender's balance slot, then to slot 0 as an
			// execution marker so the enabled cases cannot pass on a revert.
			code := append([]byte{0x60, 0x01, 0x7f}, slot.Bytes()...)
			code = append(code, 0x55, 0x60, 0x01, 0x60, 0x00, 0x55, 0x00)
			db.SetCode(dump.OvmEthAddress, code)
			db.SetBalance(sender, big.NewInt(tc.balance))
			db.Finalise(true)
			header := &types.Header{Number: big.NewInt(tc.height), GasLimit: 1000000, Difficulty: big.NewInt(1)}
			tx, err := types.SignTx(types.NewTransaction(0, dump.OvmEthAddress, new(big.Int), 100000, big.NewInt(1), nil), types.MakeSigner(&cfg, header.Number), key)
			if err != nil {
				t.Fatal(err)
			}
			block := types.NewBlockWithHeader(header).WithBody([]*types.Transaction{tx}, nil)
			before := db.IntermediateRoot(true)
			newStatePrefetcher(&cfg, &BlockChain{engine: ethash.NewFaker()}, nil).Prefetch(block, db, vm.Config{}, nil)
			if tc.skip {
				if after := db.IntermediateRoot(true); after != before {
					t.Fatalf("skipped prefetch changed state: %s -> %s", before, after)
				}
			} else {
				if db.GetNonce(sender) != 1 {
					t.Fatal("transaction was not executed")
				}
				if db.GetState(dump.OvmEthAddress, common.Hash{}) != common.BigToHash(big.NewInt(1)) {
					t.Fatal("contract did not complete execution")
				}
			}
		})
	}
}
