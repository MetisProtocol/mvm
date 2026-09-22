package state

import (
	"errors"
	"testing"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/core/rawdb"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/syndtr/goleveldb/leveldb"
)

type codeSizeFailureDB struct {
	Database
	err error
}

func (db codeSizeFailureDB) ContractCodeSize(common.Hash, common.Hash) (int, error) {
	return 0, db.err
}

func TestGetCodeSizeEmptyAndMissingCode(t *testing.T) {
	db, err := rawdb.NewLevelDBDatabase(t.TempDir(), 16, 16, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s, err := New(common.Hash{}, NewDatabase(db))
	if err != nil {
		t.Fatal(err)
	}
	eoa, contract, absent := common.Address{1}, common.Address{2}, common.Address{3}
	code := []byte{0x60, 0, 0x00}
	s.SetNonce(eoa, 1)
	s.SetCode(contract, code)
	root, err := s.Commit(true)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Database().TrieDB().Commit(root, false); err != nil {
		t.Fatal(err)
	}
	if has, err := db.Has(emptyCode[:]); err != nil || has {
		t.Fatalf("fixture must not store an empty-code blob: has=%v err=%v", has, err)
	}
	for _, tc := range []struct {
		name string
		addr common.Address
		want int
	}{
		{"absent", absent, 0},
		{"persisted-eoa", eoa, 0},
		{"persisted-contract", contract, len(code)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Fresh caches force a database lookup for nonempty code.
			s, err := New(root, NewDatabase(db))
			if err != nil {
				t.Fatal(err)
			}
			if got := s.GetCodeSize(tc.addr); got != tc.want || s.Error() != nil {
				t.Fatalf("size=%d want=%d state error=%v", got, tc.want, s.Error())
			}
			if after := s.IntermediateRoot(true); after != root {
				t.Fatal("code-size lookup changed the state root")
			}
		})
	}
	t.Run("fresh-eoa", func(t *testing.T) {
		s, err := New(root, NewDatabase(db))
		if err != nil {
			t.Fatal(err)
		}
		s.SetNonce(absent, 1)
		if size := s.GetCodeSize(absent); size != 0 || s.Error() != nil {
			t.Fatalf("size=%d state error=%v", size, s.Error())
		}
	})
	t.Run("read-error", func(t *testing.T) {
		failure := errors.New("injected code read failure")
		s, err := New(root, codeSizeFailureDB{NewDatabase(db), failure})
		if err != nil {
			t.Fatal(err)
		}
		s.GetCodeSize(contract)
		if !errors.Is(s.Error(), failure) {
			t.Fatalf("lost code read failure: %v", s.Error())
		}
	})
	t.Run("missing-nonempty-code", func(t *testing.T) {
		codeHash := crypto.Keccak256Hash(code)
		if err := db.Delete(codeHash[:]); err != nil {
			t.Fatal(err)
		}
		s, err := New(root, NewDatabase(db))
		if err != nil {
			t.Fatal(err)
		}
		s.GetCodeSize(contract)
		if !errors.Is(s.Error(), leveldb.ErrNotFound) {
			t.Fatalf("missing nonempty code must remain an error: %v", s.Error())
		}
	})
}
