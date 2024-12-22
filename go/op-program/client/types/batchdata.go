package types

import (
	"errors"
	"math/big"

	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

type DAType uint8

const (
	DAInvalid DAType = iota
	DAMemoLabel
	DABlob = 3
)

type CompressionType uint8

const (
	CompressionNone CompressionType = iota
	CompressionZlib                 = 11
)

type BatchSubmissionData struct {
	DAType            DAType
	CompressionType   CompressionType
	PrevTotalElements uint64
	BatchIndex        uint64
	BatchSize         uint64
	BlobTxHashes      []common.Hash
}

func (b *BatchSubmissionData) Decode(data []byte) error {
	if len(data) <= 70 {
		return errors.New("invalid data length")
	}
	b.DAType = DAType(data[0])
	if b.DAType != DABlob {
		return errors.New("unsupported DA type")
	}
	b.CompressionType = CompressionType(data[1])
	if b.CompressionType != CompressionNone {
		return errors.New("unsupported compression type")
	}
	b.BatchIndex = big.NewInt(0).SetBytes(data[2:34]).Uint64()
	b.PrevTotalElements = big.NewInt(0).SetBytes(data[34:66]).Uint64()
	b.BatchSize = big.NewInt(0).SetBytes(data[66:70]).Uint64()
	for i := 70; i < len(data); i += 32 {
		b.BlobTxHashes = append(b.BlobTxHashes, common.BytesToHash(data[i:i+32]))
	}
	return nil
}

type TransactionBatchEntry struct {
	Index             uint64 `json:"index"`
	BlockNumber       uint64 `json:"blockNumber"`
	Timestamp         uint64 `json:"timestamp"`
	Submitter         string `json:"submitter"`
	Size              uint64 `json:"size"`
	Root              string `json:"root"`
	PrevTotalElements uint64 `json:"prevTotalElements"`
	ExtraData         string `json:"extraData"`
	L1TransactionHash string `json:"l1TransactionHash"`
}

type BlockEntry struct {
	Index        uint64             `json:"index"`
	BatchIndex   uint64             `json:"batchIndex"`
	Timestamp    uint64             `json:"timestamp"`
	Transactions []TransactionEntry `json:"transactions"`
	Confirmed    bool               `json:"confirmed"`
}

type TransactionEntry struct {
	Index       uint64                            `json:"index"`
	BatchIndex  uint64                            `json:"batchIndex"`
	Data        string                            `json:"data"`
	BlockNumber uint64                            `json:"blockNumber"`
	Timestamp   uint64                            `json:"timestamp"`
	GasLimit    string                            `json:"gasLimit"`
	Target      string                            `json:"target"`
	Origin      string                            `json:"origin"`
	Value       string                            `json:"value"`
	QueueOrigin string                            `json:"queueOrigin"`
	QueueIndex  *uint64                           `json:"queueIndex"`
	Decoded     *DecodedSequencerBatchTransaction `json:"decoded"`
	Confirmed   bool                              `json:"confirmed"`
	SeqSign     *string                           `json:"seqSign"`
}

type DecodedSequencerBatchTransaction struct {
	Sig struct {
		R string `json:"r"`
		S string `json:"s"`
		V uint8  `json:"v"`
	} `json:"sig"`
	Value    string `json:"value"`
	GasLimit string `json:"gasLimit"`
	GasPrice string `json:"gasPrice"`
	Nonce    string `json:"nonce"`
	Target   string `json:"target"`
	Data     string `json:"data"`
}

type BlobTxInfo struct {
	BlockRef   eth.L1BlockRef
	Tx         *ethtypes.Transaction
	BlobHashes []eth.IndexedBlobHash
}

type RawBatchInfo struct {
	BatchIndex       uint64
	TotalBlobTxCount uint64
	BlobTransactions []*BlobTxInfo
}
