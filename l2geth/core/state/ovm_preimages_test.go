package state

import (
	"bytes"
	"math/big"
	"reflect"
	"testing"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/core/rawdb"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/rollup/dump"
	"github.com/MetisProtocol/mvm/l2geth/rollup/rcfg"
	"github.com/ethereum/go-ethereum/crypto"
)

func newOVMPreimageState(t *testing.T, ovm bool) *StateDB {
	t.Helper()
	old := rcfg.UsingOVM
	rcfg.UsingOVM = ovm
	t.Cleanup(func() { rcfg.UsingOVM = old })
	s, err := New(common.Hash{}, NewDatabase(rawdb.NewMemoryDatabase()))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func checkOVMPreimages(t *testing.T, s *StateDB, addresses ...common.Address) {
	t.Helper()
	want := make(map[common.Hash][]byte)
	for _, address := range addresses {
		want[common.BytesToHash(crypto.Keccak256(address[:]))] = address.Bytes()
	}
	if !reflect.DeepEqual(s.Preimages(), want) {
		t.Fatalf("preimages: got %x, want %x", s.Preimages(), want)
	}
}

func ovmPreimageLog(from, to common.Address) *types.Log {
	return &types.Log{
		Address: dump.OvmEthAddress,
		Topics:  []common.Hash{ovmTransferTopic, common.BytesToHash(from[:]), common.BytesToHash(to[:])},
		Data:    common.LeftPadBytes([]byte{7}, 32),
	}
}

func TestOVMBalancePreimages(t *testing.T) {
	address := common.HexToAddress("0x123456")
	for _, method := range []string{"add", "sub", "set"} {
		for _, amount := range []int64{0, 7} {
			t.Run(method+big.NewInt(amount).String(), func(t *testing.T) {
				s := newOVMPreimageState(t, true)
				// Populate only contract storage: the holder has no account leaf.
				s.SetState(dump.OvmEthAddress, GetOVMBalanceKey(address), common.BigToHash(big.NewInt(7)))
				expected := int64(7)
				switch method {
				case "add":
					s.AddBalance(address, big.NewInt(amount))
					expected += amount
				case "sub":
					s.SubBalance(address, big.NewInt(amount))
					expected -= amount
				case "set":
					s.SetBalance(address, big.NewInt(amount))
					expected = amount
				}
				s.AddBalance(address, new(big.Int)) // Repeated records deduplicate.
				checkOVMPreimages(t, s, address)
				if s.GetBalance(address).Cmp(big.NewInt(expected)) != 0 {
					t.Fatal("wrong balance")
				}
				if s.Exist(address) {
					t.Fatal("recording created a holder account")
				}
				reference, err := New(common.Hash{}, NewDatabase(rawdb.NewMemoryDatabase()))
				if err != nil {
					t.Fatal(err)
				}
				reference.SetState(dump.OvmEthAddress, GetOVMBalanceKey(address), common.BigToHash(big.NewInt(expected)))
				if s.IntermediateRoot(true) != reference.IntermediateRoot(true) {
					t.Fatal("preimages changed state root")
				}
			})
		}
	}
	t.Run("non_OVM", func(t *testing.T) {
		s := newOVMPreimageState(t, false)
		s.SetBalance(address, big.NewInt(9))
		s.SubBalance(address, big.NewInt(2))
		s.AddBalance(address, big.NewInt(3))
		checkOVMPreimages(t, s)
		if s.GetBalance(address).Int64() != 10 {
			t.Fatal("wrong native balance")
		}
	})
}

func TestOVMTransferPreimages(t *testing.T) {
	from, to := common.HexToAddress("0x1234"), common.HexToAddress("0x5678")
	cases := []struct {
		name string
		ovm  bool
		edit func(*types.Log)
		want []common.Address
	}{
		{"transfer", true, func(*types.Log) {}, []common.Address{from, to}},
		{"zero_amount", true, func(l *types.Log) { l.Data = make([]byte, 32) }, []common.Address{from, to}},
		{"mint", true, func(l *types.Log) { l.Topics[1] = common.Hash{} }, []common.Address{{}, to}},
		{"burn", true, func(l *types.Log) { l.Topics[2] = common.Hash{} }, []common.Address{from, {}}},
		{"self", true, func(l *types.Log) { l.Topics[2] = l.Topics[1] }, []common.Address{from}},
		{"non_OVM", false, func(*types.Log) {}, nil},
		{"other_contract", true, func(l *types.Log) { l.Address = to }, nil},
		{"approval", true, func(l *types.Log) {
			l.Topics[0] = common.BytesToHash(crypto.Keccak256([]byte("Approval(address,address,uint256)")))
		}, nil},
		{"no_topics", true, func(l *types.Log) { l.Topics = nil }, nil},
		{"few_topics", true, func(l *types.Log) { l.Topics = l.Topics[:2] }, nil},
		{"extra_topic", true, func(l *types.Log) { l.Topics = append(l.Topics, common.Hash{}) }, nil},
		{"short_data", true, func(l *types.Log) { l.Data = l.Data[:31] }, nil},
		{"long_data", true, func(l *types.Log) { l.Data = append(l.Data, 0) }, nil},
		{"from_padding", true, func(l *types.Log) { l.Topics[1][0] = 1 }, nil},
		{"to_padding", true, func(l *types.Log) { l.Topics[2][11] = 1 }, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newOVMPreimageState(t, tc.ovm)
			l := ovmPreimageLog(from, to)
			tc.edit(l)
			topics := append([]common.Hash(nil), l.Topics...)
			data := append([]byte(nil), l.Data...)
			address := l.Address
			root := s.IntermediateRoot(true)
			s.AddLog(l)
			checkOVMPreimages(t, s, tc.want...)
			if len(s.Logs()) != 1 || s.Logs()[0] != l || l.Address != address || !reflect.DeepEqual(topics, l.Topics) || !bytes.Equal(data, l.Data) {
				t.Fatal("log changed")
			}
			if s.IntermediateRoot(true) != root {
				t.Fatal("log preimages changed state root")
			}
		})
	}
}

func TestOVMPreimageLifecycle(t *testing.T) {
	s := newOVMPreimageState(t, true)
	a, b, c := common.HexToAddress("0x11"), common.HexToAddress("0x22"), common.HexToAddress("0x33")
	s.SetBalance(a, big.NewInt(9))
	snapshot := s.Snapshot()
	s.SubBalance(a, big.NewInt(1))
	s.AddLog(ovmPreimageLog(a, b))
	checkOVMPreimages(t, s, a, b)
	s.RevertToSnapshot(snapshot)
	checkOVMPreimages(t, s, a)
	if len(s.Logs()) != 0 || s.GetBalance(a).Int64() != 9 {
		t.Fatal("rollback changed original state")
	}
	s.Finalise(true)
	s.Prepare(common.Hash{1}, common.Hash{2}, 1)
	snapshot = s.Snapshot()
	s.AddLog(ovmPreimageLog(a, b))
	s.RevertToSnapshot(snapshot)
	checkOVMPreimages(t, s, a)
	s.AddBalance(b, big.NewInt(2))
	s.Finalise(true)
	checkOVMPreimages(t, s, a, b)
	copyState := s.Copy()
	copyState.AddLog(ovmPreimageLog(b, c))
	checkOVMPreimages(t, copyState, a, b, c)
	checkOVMPreimages(t, s, a, b)
	if err := copyState.Reset(common.Hash{}); err != nil {
		t.Fatal(err)
	}
	checkOVMPreimages(t, copyState)
	checkOVMPreimages(t, s, a, b)
	if err := s.Reset(common.Hash{}); err != nil {
		t.Fatal(err)
	}
	checkOVMPreimages(t, s)
}
