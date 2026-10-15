package rawdb

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/ethdb"
	"github.com/ethereum/go-ethereum/crypto"
)

// These namespaces are auxiliary node data, never consensus trie data.
var (
	OVMAllowancePrefix      = []byte("ovm-allowance-v1-")
	OVMTransferMemberPrefix = []byte("ovm-transfer-v1/member/")
	OVMTransferUndoPrefix   = []byte("ovm-transfer-v1/undo/")
	OVMIndexMetadataKey     = []byte("ovm-transfer-v1/meta")
	OVMIndexHeadKey         = []byte("ovm-transfer-v1/head")
	OVMIndexReadyKey        = []byte("ovm-transfer-v1/ready")
	OVMIndexDisabledKey     = []byte("ovm-transfer-v1/disabled")
)

// OVMIndexPosition identifies an exact block, not just a height.
type OVMIndexPosition struct {
	Number uint64
	Hash   common.Hash
}

func ovmIndexKey(prefix, suffix []byte) []byte {
	key := make([]byte, len(prefix)+len(suffix))
	copy(key, prefix)
	copy(key[len(prefix):], suffix)
	return key
}

// ReadOVMIndexRecord distinguishes absence from empty/corrupt data and I/O errors.
// Callers serialize index access with the chain lock.
func ReadOVMIndexRecord(db ethdb.KeyValueReader, key []byte) ([]byte, error) {
	exists, err := db.Has(key)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	value, err := db.Get(key)
	if err != nil {
		return nil, err
	}
	if len(value) == 0 {
		return nil, errors.New("empty OVM index record")
	}
	return value, nil
}

// EncodeOVMIndexPosition uses uint64 big endian followed by the block hash.
func EncodeOVMIndexPosition(p OVMIndexPosition) []byte {
	value := make([]byte, 40)
	binary.BigEndian.PutUint64(value, p.Number)
	copy(value[8:], p.Hash[:])
	return value
}

// DecodeOVMIndexPosition rejects non-canonical encodings.
func DecodeOVMIndexPosition(value []byte) (OVMIndexPosition, error) {
	if len(value) != 40 {
		return OVMIndexPosition{}, errors.New("invalid OVM index position length")
	}
	p := OVMIndexPosition{binary.BigEndian.Uint64(value), common.BytesToHash(value[8:])}
	if p.Hash == (common.Hash{}) {
		return OVMIndexPosition{}, errors.New("zero OVM index block hash")
	}
	return p, nil
}

// ReadOVMIndexPosition returns nil only when the record does not exist.
func ReadOVMIndexPosition(db ethdb.KeyValueReader, key []byte) (*OVMIndexPosition, error) {
	value, err := ReadOVMIndexRecord(db, key)
	if err != nil || value == nil {
		return nil, err
	}
	p, err := DecodeOVMIndexPosition(value)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// OVMAllowanceTrieKey derives the secure storage key for allowances[owner][spender]
// at OVM_ETH mapping base slot 1.
func OVMAllowanceTrieKey(owner, spender common.Address) common.Hash {
	base := make([]byte, 32)
	base[31] = 1
	inner := crypto.Keccak256(common.LeftPadBytes(owner[:], 32), base)
	slot := crypto.Keccak256(common.LeftPadBytes(spender[:], 32), inner)
	return common.BytesToHash(crypto.Keccak256(slot))
}

// ReadOVMAllowance verifies the relationship against its secure storage key.
func ReadOVMAllowance(db ethdb.KeyValueReader, trieKey common.Hash) ([]byte, error) {
	value, err := ReadOVMIndexRecord(db, ovmIndexKey(OVMAllowancePrefix, trieKey[:]))
	if err != nil || value == nil {
		return nil, err
	}
	if len(value) != 40 || OVMAllowanceTrieKey(common.BytesToAddress(value[:20]), common.BytesToAddress(value[20:])) != trieKey {
		return nil, errors.New("invalid OVM allowance relationship")
	}
	return value, nil
}

// WriteOVMAllowance stages an idempotent relationship. The caller must serialize
// writers and commit the batch; conflicting existing records are never replaced.
func WriteOVMAllowance(db ethdb.KeyValueReader, batch ethdb.KeyValueWriter, owner, spender common.Address) error {
	key := OVMAllowanceTrieKey(owner, spender)
	value := append(append([]byte(nil), owner[:]...), spender[:]...)
	old, err := ReadOVMAllowance(db, key)
	if err != nil {
		return err
	}
	if old != nil {
		if !bytes.Equal(old, value) {
			return errors.New("conflicting OVM allowance relationship")
		}
		return nil
	}
	return batch.Put(ovmIndexKey(OVMAllowancePrefix, key[:]), value)
}

// ReadOVMTransferMember returns the first canonical occurrence of an address.
func ReadOVMTransferMember(db ethdb.KeyValueReader, address common.Address) (*OVMIndexPosition, error) {
	return ReadOVMIndexPosition(db, ovmIndexKey(OVMTransferMemberPrefix, address[:]))
}

func WriteOVMTransferMember(db ethdb.KeyValueWriter, address common.Address, p OVMIndexPosition) error {
	return db.Put(ovmIndexKey(OVMTransferMemberPrefix, address[:]), EncodeOVMIndexPosition(p))
}

func DeleteOVMTransferMember(db ethdb.KeyValueWriter, address common.Address) error {
	return db.Delete(ovmIndexKey(OVMTransferMemberPrefix, address[:]))
}

// WriteOVMTransferUndo requires sorted unique addresses. Empty lists are explicit.
func WriteOVMTransferUndo(db ethdb.KeyValueWriter, hash common.Hash, addresses []common.Address) error {
	value := make([]byte, 8+20*len(addresses))
	binary.BigEndian.PutUint64(value, uint64(len(addresses)))
	for i, address := range addresses {
		if i > 0 && bytes.Compare(addresses[i-1][:], address[:]) >= 0 {
			return errors.New("unordered OVM undo addresses")
		}
		copy(value[8+20*i:], address[:])
	}
	return db.Put(ovmIndexKey(OVMTransferUndoPrefix, hash[:]), value)
}

// ReadOVMTransferUndo validates lengths before allocating; a missing list is an error.
func ReadOVMTransferUndo(db ethdb.KeyValueReader, hash common.Hash) ([]common.Address, error) {
	value, err := ReadOVMIndexRecord(db, ovmIndexKey(OVMTransferUndoPrefix, hash[:]))
	if err != nil {
		return nil, err
	}
	if len(value) < 8 || (len(value)-8)%20 != 0 || binary.BigEndian.Uint64(value) != uint64((len(value)-8)/20) {
		return nil, fmt.Errorf("missing or malformed OVM undo for %s", hash)
	}
	addresses := make([]common.Address, (len(value)-8)/20)
	for i := range addresses {
		copy(addresses[i][:], value[8+20*i:8+20*(i+1)])
		if i > 0 && bytes.Compare(addresses[i-1][:], addresses[i][:]) >= 0 {
			return nil, errors.New("unordered OVM undo addresses")
		}
	}
	return addresses, nil
}
