package state

import (
	"math/big"
	"testing"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/core/rawdb"
	"github.com/MetisProtocol/mvm/l2geth/rollup/dump"
	"github.com/ethereum/go-ethereum/crypto"
)

func auditState(t *testing.T) (*StateDB, *OVMAuditObserver) {
	s := newOVMPreimageState(t, true)
	s.SetCode(dump.OvmEthAddress, []byte{0})
	o := &OVMAuditObserver{Index: NewOVMAuditIndex(rawdb.NewMemoryDatabase()), Total: ZeroOVMAuditDelta()}
	s.SetOVMAudit(o)
	return s, o
}

func TestOVMAuditAccounting(t *testing.T) {
	a, b := common.Address{1}, common.Address{2}
	supply := common.BigToHash(big.NewInt(2))
	for _, mode := range []string{"transfer", "mint", "burn", "shortfall", "reverted", "partial-revert", "negative", "unknown", "unknown-reverted"} {
		t.Run(mode, func(t *testing.T) {
			s, o := auditState(t)
			s.SetBalance(a, big.NewInt(100))
			s.SetState(dump.OvmEthAddress, supply, common.BigToHash(big.NewInt(100)))
			s.BeginOVMAuditScope("transaction", common.Hash{1}, 0)
			want := "0"
			switch mode {
			case "transfer":
				s.SubBalance(a, big.NewInt(20))
				s.AddBalance(b, big.NewInt(20))
			case "mint":
				s.AddBalance(b, big.NewInt(20))
				s.SetState(dump.OvmEthAddress, supply, common.BigToHash(big.NewInt(120)))
			case "burn":
				s.SubBalance(a, big.NewInt(20))
				s.SetState(dump.OvmEthAddress, supply, common.BigToHash(big.NewInt(80)))
			case "shortfall":
				s.AuditOVMGas(a, big.NewInt(120), big.NewInt(100), 120, big.NewInt(1), false)
				s.SubBalance(a, big.NewInt(100))
				s.AddBalance(a, big.NewInt(90))
				s.AddBalance(b, big.NewInt(30))
				want = "20"
			case "reverted":
				snap := s.Snapshot()
				s.AuditOVMSelfdestruct(a, b, big.NewInt(100), 0, 1, false)
				s.AddBalance(b, big.NewInt(100))
				s.RevertToSnapshot(snap)
			case "partial-revert":
				s.SubBalance(a, big.NewInt(10))
				snap := s.Snapshot()
				s.AddBalance(a, big.NewInt(80))
				s.RevertToSnapshot(snap)
				s.AddBalance(b, big.NewInt(10))
			case "negative":
				s.SubBalance(a, big.NewInt(3))
				want = "-3"
			case "unknown":
				s.SetState(dump.OvmEthAddress, common.Hash{99}, common.Hash{1})
			case "unknown-reverted":
				snap := s.Snapshot()
				s.SetState(dump.OvmEthAddress, common.Hash{99}, common.Hash{1})
				s.RevertToSnapshot(snap)
			}
			s.Finalise(true)
			s.EndOVMAuditScope(mode == "shortfall")
			if mode == "unknown" {
				if o.Err == nil {
					t.Fatal("unknown write accepted")
				}
				return
			}
			if o.Err != nil {
				t.Fatal(o.Err)
			}
			if o.Total.Difference != want {
				t.Fatalf("got %+v want %s", o.Total, want)
			}
			if mode == "shortfall" && (o.Total.Gas != "20" || o.Total.Unexplained != "0" || !o.Events[0].Failed) {
				t.Fatalf("lost gas attribution: %+v", o)
			}
			if mode == "reverted" && (!o.Events[0].Causes[0].Reverted || o.Total.Suicide != "0") {
				t.Fatal("counted reverted suicide")
			}
		})
	}
}

func TestOVMAuditStorageAndCopies(t *testing.T) {
	s, o := auditState(t)
	a := common.Address{1}
	b := common.Address{2}
	s.BeginOVMAuditScope("transaction", common.Hash{}, 0)
	preimage := append(common.LeftPadBytes(a[:], 32), make([]byte, 32)...)
	s.AuditOVMPreimage(crypto.Keccak256Hash(preimage), preimage)
	s.SetState(dump.OvmEthAddress, GetOVMBalanceKey(a), common.BigToHash(big.NewInt(7)))
	inner := crypto.Keccak256Hash(common.LeftPadBytes(a[:], 32), common.LeftPadBytes([]byte{1}, 32))
	outerPreimage := append(common.LeftPadBytes(b[:], 32), inner[:]...)
	s.AuditOVMPreimage(inner, append(common.LeftPadBytes(a[:], 32), common.LeftPadBytes([]byte{1}, 32)...))
	s.AuditOVMPreimage(crypto.Keccak256Hash(outerPreimage), outerPreimage)
	s.SetState(dump.OvmEthAddress, crypto.Keccak256Hash(outerPreimage), common.BigToHash(big.NewInt(500)))
	s.EndOVMAuditScope(false)
	if o.Err != nil || o.Total.Difference != "7" {
		t.Fatalf("%+v %v", o.Total, o.Err)
	}
	root := s.IntermediateRoot(true)
	scan, err := s.ScanOVMAudit(o.Index, root)
	if err != nil {
		t.Fatal(err)
	}
	if scan.Balances != "7" || scan.BalanceSlots != 1 || s.Exist(a) {
		t.Fatalf("wrong scan %+v", scan)
	}
	copy := s.Copy()
	if copy.OVMAudit() != nil {
		t.Fatal("copy shares observer")
	}
	copy.AddBalance(a, big.NewInt(2))
	if o.Total.Difference != "7" {
		t.Fatal("copy changed observer")
	}
	if s.IntermediateRoot(true) != root {
		t.Fatal("scan changed state")
	}
	if _, err = s.Commit(true); err != nil {
		t.Fatal(err)
	}
	if err = s.Reset(root); err != nil {
		t.Fatal(err)
	}
	if s.OVMAudit() != nil {
		t.Fatal("reset retained observer")
	}
}

func TestOVMAuditNestedRollback(t *testing.T) {
	s, o := auditState(t)
	a, b := common.Address{1}, common.Address{2}
	s.SetBalance(a, big.NewInt(9))
	s.BeginOVMAuditScope("transaction", common.Hash{}, 0)
	outer := s.Snapshot()
	inner := s.Snapshot()
	s.AuditOVMSelfdestruct(a, b, big.NewInt(9), 0, 2, false)
	s.AddBalance(b, big.NewInt(9))
	s.RevertToSnapshot(inner)
	s.AuditOVMSelfdestruct(a, b, big.NewInt(9), 1, 1, false)
	s.AddBalance(b, big.NewInt(9))
	s.RevertToSnapshot(outer)
	s.EndOVMAuditScope(false)
	if o.Err != nil || o.Total.Difference != "0" || o.Total.Suicide != "0" {
		t.Fatalf("%+v %v", o.Total, o.Err)
	}
	for _, c := range o.Events[0].Causes {
		if !c.Reverted {
			t.Fatal("outer revert did not revert child event")
		}
	}
}
