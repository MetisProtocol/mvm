package rollup

import (
	"fmt"

	ethcommon "github.com/ethereum/go-ethereum/common"

	"github.com/MetisProtocol/mvm/l2geth/common"
	dtl "github.com/MetisProtocol/mvm/l2geth/rollup"
	preimage "github.com/ethereum-optimism/optimism/go/op-preimage"
	merkletrie "github.com/ethereum-optimism/optimism/go/op-program/client/merkel"
)

type BlockMeta struct {
	Index            uint64
	BatchIndex       uint64
	Timestamp        uint64
	TransactionCount uint64
	Confirmed        bool
}

// Oracle defines the high-level API used to retrieve rollup data.
// The returned data is always the preimage of the requested hash.
type Oracle interface {
	StateBatchHeaderByHash(batchHash common.Hash) *dtl.BatchHeader
	StateBatchesByHash(batchHash common.Hash) []common.Hash
}

// PreimageOracle implements Oracle using by interfacing with the pure preimage.Oracle
// to fetch pre-images to decode into the requested data.
type PreimageOracle struct {
	oracle preimage.Oracle
	hint   preimage.Hinter
}

var _ Oracle = (*PreimageOracle)(nil)

func NewPreimageOracle(raw preimage.Oracle, hint preimage.Hinter) *PreimageOracle {
	oracle := &PreimageOracle{
		oracle: raw,
		hint:   hint,
	}

	return oracle
}

func (p *PreimageOracle) StateBatchHeaderByHash(batchHash common.Hash) *dtl.BatchHeader {
	p.hint.Hint(StateBatch(batchHash))
	encodedHeader := p.oracle.Get(preimage.Keccak256Key(batchHash))

	batchHeader := new(dtl.BatchHeader)
	if err := batchHeader.Unpack(encodedHeader); err != nil {
		panic(fmt.Errorf("invalid batch header %s: %w", batchHash, err))
	}

	return batchHeader
}

func (p *PreimageOracle) StateBatchesByHash(batchHash common.Hash) []common.Hash {
	batchHeader := p.StateBatchHeaderByHash(batchHash)

	stateRoots := merkletrie.ReadTrie(ethcommon.Hash(batchHeader.BatchRoot), func(key ethcommon.Hash) []byte {
		return p.oracle.Get(preimage.Keccak256Key(key))
	})

	stateRootHashes := make([]common.Hash, 0, len(stateRoots))
	for _, root := range stateRoots {
		stateRootHashes = append(stateRootHashes, common.BytesToHash(root))
	}

	return stateRootHashes
}
