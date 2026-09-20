package core

import (
	"fmt"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/core/rawdb"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/ethdb"
	"github.com/MetisProtocol/mvm/l2geth/params"
)

// CheckpointMismatchError indicates a block conflicts with a hardcoded checkpoint.
type CheckpointMismatchError struct {
	Number uint64
	Want   common.Hash
	Have   common.Hash
}

func (e *CheckpointMismatchError) Error() string {
	return fmt.Sprintf("block hash checkpoint mismatch at height %d: have %s, want %s", e.Number, e.Have.Hex(), e.Want.Hex())
}

func verifyBlockCheckpoint(config *params.ChainConfig, number uint64, hash common.Hash) error {
	if config != nil {
		if want, ok := params.BlockHashCheckpoint(config.ChainID, number); ok && hash != want {
			return &CheckpointMismatchError{Number: number, Want: want, Have: hash}
		}
	}
	return nil
}

func verifyHeaderCheckpoint(config *params.ChainConfig, header *types.Header) error {
	if config == nil {
		return nil
	}
	if _, ok := params.BlockHashCheckpoint(config.ChainID, 0); !ok {
		return nil
	}
	// Do not let Uint64 truncation turn a malformed height into a valid one.
	if header.Number == nil || !header.Number.IsUint64() {
		return fmt.Errorf("invalid block number for checkpoint validation: %v", header.Number)
	}
	// Only hash headers covered by the table, rather than adding an RLP hash to
	// every header import.
	number := header.Number.Uint64()
	if _, ok := params.BlockHashCheckpoint(config.ChainID, number); !ok {
		return nil
	}
	return verifyBlockCheckpoint(config, number, header.Hash())
}

// verifyStoredCheckpoints runs before startup repairs can rewind or rewrite the
// database. ReadCanonicalHash checks both the freezer and the key-value store.
// Missing entries are allowed: the node may not have synced that far yet.
func verifyStoredCheckpoints(db ethdb.Reader, config *params.ChainConfig) error {
	if config == nil {
		return nil
	}
	if _, ok := params.BlockHashCheckpoint(config.ChainID, 0); !ok {
		return nil
	}
	for number := uint64(0); number <= params.AndromedaLastCheckpoint; number += params.AndromedaCheckpointInterval {
		if hash := rawdb.ReadCanonicalHash(db, number); hash != (common.Hash{}) {
			if err := verifyBlockCheckpoint(config, number, hash); err != nil {
				return err
			}
		}
	}
	return nil
}
