package rawdb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/ethdb"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestOVMAllowanceIndex(t *testing.T) {
	db := NewMemoryDatabase()
	defer db.Close()
	owner, spender := common.HexToAddress("0x1234"), common.HexToAddress("0x5678")
	// Independent ABI-word construction of the nested mapping storage key.
	var inner, outer [64]byte
	copy(inner[12:32], owner[:])
	inner[63] = 1
	copy(outer[12:32], spender[:])
	copy(outer[32:], crypto.Keccak256(inner[:]))
	want := common.BytesToHash(crypto.Keccak256(crypto.Keccak256(outer[:])))
	if got := OVMAllowanceTrieKey(owner, spender); got != want {
		t.Fatalf("slot %s != %s", got, want)
	}
	batch := db.NewBatch()
	if err := WriteOVMAllowance(db, batch, owner, spender); err != nil {
		t.Fatal(err)
	}
	if value, err := ReadOVMAllowance(db, want); err != nil || value != nil {
		t.Fatalf("uncommitted write: %x, %v", value, err)
	}
	if err := batch.Write(); err != nil {
		t.Fatal(err)
	}
	value, err := ReadOVMAllowance(db, want)
	if err != nil || !bytes.Equal(value, append(owner[:], spender[:]...)) {
		t.Fatalf("relationship %x: %v", value, err)
	}
	batch.Reset()
	if err := WriteOVMAllowance(db, batch, owner, spender); err != nil || batch.ValueSize() != 0 {
		t.Fatalf("idempotent write: %v", err)
	}
	if got := ReadPreimage(db, want); got != nil {
		t.Fatal("allowance overwrote preimage namespace")
	}
	for _, bad := range [][]byte{{}, {1}, make([]byte, 40), make([]byte, 41)} {
		if err := db.Put(ovmIndexKey(OVMAllowancePrefix, want[:]), bad); err != nil {
			t.Fatal(err)
		}
		if err := WriteOVMAllowance(db, batch, owner, spender); err == nil {
			t.Fatal("accepted corrupt/conflicting allowance")
		}
	}
}

func TestOVMUndoEncoding(t *testing.T) {
	db := NewMemoryDatabase()
	defer db.Close()
	hash := common.HexToHash("0x1234")
	if _, err := ReadOVMTransferUndo(db, hash); err == nil {
		t.Fatal("missing undo accepted")
	}
	a, b := common.HexToAddress("0x01"), common.HexToAddress("0x02")
	for _, addresses := range [][]common.Address{nil, {a}, {a, b}} {
		if err := WriteOVMTransferUndo(db, hash, addresses); err != nil {
			t.Fatal(err)
		}
		got, err := ReadOVMTransferUndo(db, hash)
		if err != nil || len(got) != len(addresses) {
			t.Fatalf("undo %v: %v", got, err)
		}
		for n := range got {
			if got[n] != addresses[n] {
				t.Fatal("wrong address")
			}
		}
	}
	for _, addresses := range [][]common.Address{{a, a}, {b, a}} {
		if err := WriteOVMTransferUndo(db, hash, addresses); err == nil {
			t.Fatal("accepted noncanonical undo")
		}
	}
	duplicate := make([]byte, 48)
	binary.BigEndian.PutUint64(duplicate, 2)
	copy(duplicate[8:28], a[:])
	copy(duplicate[28:], a[:])
	overflow := bytes.Repeat([]byte{255}, 8)
	for _, bad := range [][]byte{nil, {1}, make([]byte, 9), overflow, duplicate} {
		if err := db.Put(ovmIndexKey(OVMTransferUndoPrefix, hash[:]), bad); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadOVMTransferUndo(db, hash); err == nil {
			t.Fatalf("accepted undo %x", bad)
		}
	}
}

type ovmReadFault struct {
	ethdb.KeyValueReader
	hasErr, getErr error
}

func (d ovmReadFault) Has(key []byte) (bool, error) {
	if d.hasErr != nil {
		return false, d.hasErr
	}
	return d.KeyValueReader.Has(key)
}
func (d ovmReadFault) Get(key []byte) ([]byte, error) {
	if d.getErr != nil {
		return nil, d.getErr
	}
	return d.KeyValueReader.Get(key)
}

func TestOVMIndexReadErrors(t *testing.T) {
	db := NewMemoryDatabase()
	defer db.Close()
	key := OVMIndexHeadKey
	if err := db.Put(key, EncodeOVMIndexPosition(OVMIndexPosition{1, common.HexToHash("0x01")})); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("injected I/O failure")
	for _, reader := range []ovmReadFault{{db, failure, nil}, {db, nil, failure}} {
		if _, err := ReadOVMIndexPosition(reader, key); !errors.Is(err, failure) {
			t.Fatalf("lost read error: %v", err)
		}
	}
	for _, value := range [][]byte{nil, make([]byte, 39), make([]byte, 40), make([]byte, 41)} {
		if _, err := DecodeOVMIndexPosition(value); err == nil {
			t.Fatal("accepted invalid position")
		}
	}
}
