package derive

import (
	"fmt"
	"math/big"

	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/rlp"
)

type headerInfo struct {
	hash common.Hash
	*types.Header
}

var _ eth.BlockInfo = (*headerInfo)(nil)

func (h headerInfo) Hash() common.Hash {
	return h.hash
}

func (h headerInfo) ParentHash() common.Hash {
	return common.Hash(h.Header.ParentHash)
}

func (h headerInfo) Coinbase() common.Address {
	return common.Address(h.Header.Coinbase)
}

func (h headerInfo) Root() common.Hash {
	return common.Hash(h.Header.Root)
}

func (h headerInfo) NumberU64() uint64 {
	return h.Header.Number.Uint64()
}

func (h headerInfo) Time() uint64 {
	return h.Header.Time
}

func (h headerInfo) MixDigest() common.Hash {
	return common.Hash(h.Header.MixDigest)
}

func (h headerInfo) BaseFee() *big.Int {
	return nil
}

func (h headerInfo) BlobBaseFee() *big.Int {
	return nil
}

func (h headerInfo) ReceiptHash() common.Hash {
	return common.Hash(h.Header.ReceiptHash)
}

func (h headerInfo) GasUsed() uint64 {
	return h.Header.GasUsed
}

func (h headerInfo) GasLimit() uint64 {
	return h.Header.GasLimit
}

func (h headerInfo) ParentBeaconRoot() *common.Hash {
	return nil
}

func (h headerInfo) HeaderRLP() ([]byte, error) {
	h.Header.Hash()
	return rlp.EncodeToBytes(h.Header)
}

func ToHeaderInfo(header *types.Header) headerInfo {
	return headerInfo{
		hash:   common.Hash(header.Hash()),
		Header: header,
	}
}

func ToExecutionPayloadEnvelope(block *types.Block) (*eth.ExecutionPayloadEnvelope, error) {
	// Unfortunately eth_getBlockByNumber either returns full transactions, or only tx-hashes.
	// There is no option for encoded transactions.
	opaqueTxs := make([]hexutil.Bytes, len(block.Transactions()))
	for i, tx := range block.Transactions() {
		data, err := rlp.EncodeToBytes(tx)
		if err != nil {
			return nil, fmt.Errorf("failed to encode tx %d from RPC: %w", i, err)
		}
		opaqueTxs[i] = data
	}

	payload := &eth.ExecutionPayload{
		ParentHash:   common.Hash(block.ParentHash()),
		FeeRecipient: common.Address(block.Coinbase()),
		StateRoot:    eth.Bytes32(block.Root()),
		ReceiptsRoot: eth.Bytes32(block.ReceiptHash()),
		LogsBloom:    eth.Bytes256(block.Bloom()),
		PrevRandao:   eth.Bytes32(block.MixDigest()), // mix-digest field is used for prevRandao post-merge
		BlockNumber:  eth.Uint64Quantity(block.Number().Uint64()),
		GasLimit:     eth.Uint64Quantity(block.GasLimit()),
		GasUsed:      eth.Uint64Quantity(block.GasUsed()),
		Timestamp:    eth.Uint64Quantity(block.Time()),
		ExtraData:    eth.BytesMax32(block.Extra()),
		BlockHash:    common.Hash(block.Hash()),
		Transactions: opaqueTxs,
	}

	return &eth.ExecutionPayloadEnvelope{
		ExecutionPayload: payload,
	}, nil
}
