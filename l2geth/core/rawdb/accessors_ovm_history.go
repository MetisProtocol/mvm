package rawdb

import (
	"errors"
	"fmt"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/ethdb"
	"github.com/MetisProtocol/mvm/l2geth/rlp"
	"github.com/syndtr/goleveldb/leveldb"
)

// ovmAncientRecord reads only existing canonical ancient records. Unlike legacy
// convenience readers, index construction must not discard storage errors.
func ovmAncientRecord(db ethdb.Reader, table string, hash common.Hash, number uint64) ([]byte, error) {
	count, err := db.Ancients()
	if errors.Is(err, errNotSupported) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if number >= count {
		return nil, nil
	}
	canonical, err := db.Ancient(freezerHashTable, number)
	if err != nil {
		return nil, err
	}
	if len(canonical) != 32 {
		return nil, errors.New("invalid ancient canonical hash")
	}
	if common.BytesToHash(canonical) != hash {
		return nil, nil
	}
	value, err := db.Ancient(table, number)
	if err != nil {
		return nil, err
	}
	if len(value) == 0 {
		return nil, errors.New("empty ancient OVM history record")
	}
	return value, nil
}

func readOVMHistory(db ethdb.Reader, table string, key []byte, hash common.Hash, number uint64) ([]byte, error) {
	value, err := ovmAncientRecord(db, table, hash, number)
	if err != nil || value != nil {
		return value, err
	}
	exists, err := db.Has(key)
	if err != nil {
		return nil, err
	}
	if exists {
		value, err = db.Get(key)
		if err == nil {
			if len(value) == 0 {
				return nil, errors.New("empty hot OVM history record")
			}
			return value, nil
		}
		if !errors.Is(err, leveldb.ErrNotFound) {
			return nil, err
		}
	}
	// The freezer can move the record between the first cold lookup and hot Get.
	value, err = ovmAncientRecord(db, table, hash, number)
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, fmt.Errorf("missing OVM history %s at %d (%s)", table, number, hash)
	}
	return value, nil
}

// ReadOVMIndexHeader authenticates both the hash and height of an index ancestor.
func ReadOVMIndexHeader(db ethdb.Reader, p OVMIndexPosition) (*types.Header, error) {
	value, err := readOVMHistory(db, freezerHeaderTable, headerKey(p.Number, p.Hash), p.Hash, p.Number)
	if err != nil {
		return nil, err
	}
	var header types.Header
	if err := rlp.DecodeBytes(value, &header); err != nil {
		return nil, err
	}
	if header.Number == nil || !header.Number.IsUint64() || header.Number.Uint64() != p.Number || header.Hash() != p.Hash {
		return nil, errors.New("OVM index header identity mismatch")
	}
	return &header, nil
}

// ReadOVMIndexReceipts reads storage encodings without requiring block bodies.
// Callers must authenticate the resulting consensus receipts against the header.
func ReadOVMIndexReceipts(db ethdb.Reader, p OVMIndexPosition) (types.Receipts, error) {
	value, err := readOVMHistory(db, freezerReceiptTable, blockReceiptsKey(p.Number, p.Hash), p.Hash, p.Number)
	if err != nil {
		return nil, err
	}
	var stored []*types.ReceiptForStorage
	if err := rlp.DecodeBytes(value, &stored); err != nil {
		return nil, err
	}
	receipts := make(types.Receipts, len(stored))
	for i, receipt := range stored {
		receipts[i] = (*types.Receipt)(receipt)
	}
	return receipts, nil
}

// ReadOVMCanonicalHash is an error-preserving canonical lookup for index replay.
func ReadOVMCanonicalHash(db ethdb.Reader, number uint64) (common.Hash, error) {
	hash, err := ReadOVMCanonicalHashIfPresent(db, number)
	if err == nil && hash == (common.Hash{}) {
		err = errors.New("missing OVM canonical hash")
	}
	return hash, err
}

// ReadOVMCanonicalHashIfPresent permits an absent mapping after a shorter,
// heavier header-only reorg. Malformed records and I/O errors still fail closed.
func ReadOVMCanonicalHashIfPresent(db ethdb.Reader, number uint64) (common.Hash, error) {
	cold := func() ([]byte, error) {
		count, err := db.Ancients()
		if errors.Is(err, errNotSupported) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if number >= count {
			return nil, nil
		}
		value, err := db.Ancient(freezerHashTable, number)
		if err != nil {
			return nil, err
		}
		if len(value) != 32 {
			return nil, errors.New("invalid ancient canonical hash")
		}
		return value, nil
	}
	value, err := cold()
	if err != nil {
		return common.Hash{}, err
	}
	if value == nil {
		value, err = ReadOVMIndexRecord(db, headerHashKey(number))
		if err != nil && !errors.Is(err, leveldb.ErrNotFound) {
			return common.Hash{}, err
		}
		if value == nil {
			value, err = cold()
		}
	}
	if err != nil {
		return common.Hash{}, err
	}
	if value == nil {
		return common.Hash{}, nil
	}
	if len(value) != 32 || common.BytesToHash(value) == (common.Hash{}) {
		return common.Hash{}, errors.New("malformed OVM canonical hash")
	}
	return common.BytesToHash(value), nil
}

// OVMIndexCanInitialize permits new indexes only on a genesis-only database.
// Head repair must not make an old or partially fast-synced database look fresh.
func OVMIndexCanInitialize(db ethdb.Reader, genesis common.Hash) (bool, error) {
	for _, key := range [][]byte{headBlockKey, headHeaderKey, headFastBlockKey} {
		value, err := ReadOVMIndexRecord(db, key)
		if err != nil {
			return false, err
		}
		if len(value) != common.HashLength || common.BytesToHash(value) != genesis {
			return false, nil
		}
	}
	exists, err := db.Has(headerHashKey(1))
	if err != nil || exists {
		return false, err
	}
	count, err := db.Ancients()
	if errors.Is(err, errNotSupported) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return count <= 1, nil
}
