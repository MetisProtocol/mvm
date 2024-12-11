package rollup

import (
	"github.com/ethereum-optimism/optimism/op-service/eth"

	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/rlp"
	dtl "github.com/MetisProtocol/mvm/l2geth/rollup"
	preimage "github.com/ethereum-optimism/optimism/go/op-preimage"
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
	L2BatchOfBlock(block uint64) *dtl.Batch
	L2BlockMeta(block uint64) *BlockMeta
	L2StateCommitment(block uint64) eth.Bytes32
	L2BatchTransactions(block uint64) types.Transactions
}

// PreimageOracle implements Oracle using by interfacing with the pure preimage.Oracle
// to fetch pre-images to decode into the requested data.
type PreimageOracle struct {
	oracle preimage.Oracle
	hint   preimage.Hinter
}

var _ Oracle = (*PreimageOracle)(nil)

func NewPreimageOracle(raw preimage.Oracle, hint preimage.Hinter) *PreimageOracle {
	return &PreimageOracle{
		oracle: raw,
		hint:   hint,
	}
}

func (p *PreimageOracle) L2BatchOfBlock(block uint64) *dtl.Batch {
	p.hint.Hint(RollupBatchOfBlock(block))

	l2BatchBytes := p.oracle.Get(preimage.RollupBlockBatchKey(block))

	var batchInfo dtl.Batch
	if err := rlp.DecodeBytes(l2BatchBytes, &batchInfo); err != nil {
		panic("failed to unmarshal l2BlockBytes, err: " + err.Error())
	}

	return &batchInfo
}

func (p *PreimageOracle) L2BlockMeta(block uint64) *BlockMeta {
	p.hint.Hint(RollupBlockMeta(block))

	blockMetaBytes := p.oracle.Get(preimage.RollupBlockMetaKey(block))

	var blockMeta BlockMeta
	rlp.DecodeBytes(blockMetaBytes, &blockMeta)

	return &blockMeta
}

func (p *PreimageOracle) L2StateCommitment(block uint64) eth.Bytes32 {
	p.hint.Hint(RollupBlockStateCommitment(block))

	l2BlockStateCommitment := p.oracle.Get(preimage.RollupBlockStateCommitmentKey(block))

	return eth.Bytes32(l2BlockStateCommitment)
}

func (p *PreimageOracle) L2BatchTransactions(block uint64) types.Transactions {
	p.hint.Hint(RollupBatchTransactions(block))

	txsBytes := p.oracle.Get(preimage.RollupBatchTransactionsKey(block))

	var txs types.Transactions
	rlp.DecodeBytes(txsBytes, &txs)

	return txs
}
