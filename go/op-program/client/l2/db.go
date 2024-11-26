package l2

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/ethdb/memorydb"
)

//var codePrefixedKeyLength = common.HashLength + len(rawdb.CodePrefix)

var ErrInvalidKeyLength = errors.New("pre-images must be identified by 32-byte hash keys")

type OracleKeyValueStore struct {
	db     ethdb.KeyValueStore
	oracle StateOracle
}

func NewOracleBackedDB(oracle StateOracle) *OracleKeyValueStore {
	return &OracleKeyValueStore{
		db:     memorydb.New(),
		oracle: oracle,
	}
}

func (o *OracleKeyValueStore) Get(key []byte) ([]byte, error) {
	has, err := o.db.Has(key)
	if err != nil {
		return nil, fmt.Errorf("checking in-memory db: %w", err)
	}
	if has {
		return o.db.Get(key)
	}

	// NOTE: old version storage of contract code does not have prefix
	// if len(key) == codePrefixedKeyLength && bytes.HasPrefix(key, rawdb.CodePrefix) {
	// 	 key = key[len(rawdb.CodePrefix):]
	// 	 return o.oracle.CodeByHash(*(*[common.HashLength]byte)(key)), nil
	// }

	if len(key) != common.HashLength {
		return nil, ErrInvalidKeyLength
	}
	return o.oracle.NodeByHash(*(*[common.HashLength]byte)(key)), nil
}

func (o *OracleKeyValueStore) HasAncient(kind string, number uint64) (bool, error) {
	panic("implement me")
}

func (o *OracleKeyValueStore) Ancient(kind string, number uint64) ([]byte, error) {
	panic("implement me")
}

func (o *OracleKeyValueStore) Ancients() (uint64, error) {
	panic("implement me")
}

func (o *OracleKeyValueStore) AncientSize(kind string) (uint64, error) {
	panic("implement me")
}

func (o *OracleKeyValueStore) AppendAncient(number uint64, hash, header, body, receipt, td []byte) error {
	panic("implement me")
}

func (o *OracleKeyValueStore) TruncateAncients(n uint64) error {
	panic("implement me")
}

func (o *OracleKeyValueStore) Sync() error {
	panic("implement me")
}

func (o *OracleKeyValueStore) NewBatch() ethdb.Batch {
	return o.db.NewBatch()
}

func (o *OracleKeyValueStore) Put(key []byte, value []byte) error {
	return o.db.Put(key, value)
}

func (o *OracleKeyValueStore) Close() error {
	return nil
}

// Remaining methods are unused when accessing the state for block processing so leaving unimplemented.

func (o *OracleKeyValueStore) Has(key []byte) (bool, error) {
	panic("not supported")
}

func (o *OracleKeyValueStore) Delete(key []byte) error {
	panic("not supported")
}

func (o *OracleKeyValueStore) Stat(property string) (string, error) {
	panic("not supported")
}

func (o *OracleKeyValueStore) NewIterator() ethdb.Iterator {
	panic("not supported")
}

func (o *OracleKeyValueStore) NewIteratorWithStart(start []byte) ethdb.Iterator {
	panic("not supported")
}

func (o *OracleKeyValueStore) NewIteratorWithPrefix(prefix []byte) ethdb.Iterator {
	panic("not supported")
}

func (o *OracleKeyValueStore) Compact(start []byte, limit []byte) error {
	panic("not supported")
}
