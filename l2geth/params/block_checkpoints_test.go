package params_test

import (
	"encoding/json"
	"math/big"
	"os"
	"testing"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/params"
)

func TestBlockHashCheckpointSnapshot(t *testing.T) {
	data, err := os.ReadFile("testdata/andromeda-checkpoints.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []json.RawMessage
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	if len(fixtures) != 232 {
		t.Fatalf("have %d fixtures, want 232", len(fixtures))
	}
	for i, fixture := range fixtures {
		var header types.Header
		if err := json.Unmarshal(fixture, &header); err != nil {
			t.Fatal(err)
		}
		var rpc struct {
			Hash common.Hash `json:"hash"`
		}
		if err := json.Unmarshal(fixture, &rpc); err != nil {
			t.Fatal(err)
		}
		number := uint64(i) * 100_000
		if header.Number == nil || !header.Number.IsUint64() || header.Number.Uint64() != number {
			t.Fatalf("fixture %d has wrong height", i)
		}
		hash, ok := params.BlockHashCheckpoint(big.NewInt(1088), number)
		if !ok || hash == (common.Hash{}) || hash != rpc.Hash || hash != header.Hash() {
			t.Fatalf("checkpoint %d: table %s, RPC %s, local %s, found %t", number, hash, rpc.Hash, header.Hash(), ok)
		}
	}
	if params.AndromedaCheckpointInterval != 100_000 || params.AndromedaLastCheckpoint != 23_100_000 {
		t.Fatal("unexpected snapshot bounds")
	}
}

func TestBlockHashCheckpointScope(t *testing.T) {
	for _, id := range []*big.Int{nil, big.NewInt(1), big.NewInt(59902), new(big.Int).Add(new(big.Int).Lsh(big.NewInt(1), 64), big.NewInt(1088))} {
		if hash, ok := params.BlockHashCheckpoint(id, 100_000); ok || hash != (common.Hash{}) {
			t.Fatalf("unexpected checkpoint for chain %v", id)
		}
	}
	for _, number := range []uint64{1, 99_999, 100_001, 23_100_001, 23_200_000, ^uint64(0)} {
		if _, ok := params.BlockHashCheckpoint(big.NewInt(1088), number); ok {
			t.Fatalf("unexpected checkpoint at %d", number)
		}
	}
	hash, _ := params.BlockHashCheckpoint(big.NewInt(1088), 100_000)
	hash[0] ^= 0xff
	again, _ := params.BlockHashCheckpoint(big.NewInt(1088), 100_000)
	if hash == again {
		t.Fatal("caller mutated checkpoint table")
	}
}
