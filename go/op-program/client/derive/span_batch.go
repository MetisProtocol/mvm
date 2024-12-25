package derive

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"

	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/log"

	"github.com/MetisProtocol/mvm/l2geth/core/types"
)

// Batch format
//
// SpanBatchType := 1
// spanBatch := SpanBatchType ++ prefix ++ payload
// prefix := l1_timestamp ++ l1_origin_num ++ parent_check ++ l1_origin_check
// payload := block_count ++ origin_bits ++ block_tx_counts ++ block_extra_data ++ txs
// txs := contract_creation_bits ++ y_parity_bits ++ tx_sigs ++ tx_tos ++ tx_datas ++ tx_nonces ++ tx_gases ++ protected_bits ++ queue_origin_bits ++ seq_y_parity_bits ++ tx_seq_sigs ++ l1_tx_origins

var ErrTooBigSpanBatchSize = errors.New("span batch size limit reached")

var ErrEmptySpanBatch = errors.New("span-batch must not be empty")

type spanBatchPrefix struct {
	l2StartBlock  uint64   // L2 block number of the first block in the span
	parentCheck   [20]byte // First 20 bytes of the first block's parent hash
	l1OriginCheck [20]byte // First 20 bytes of the last block's L1 origin hash
}

type spanBatchPayload struct {
	blockCount        uint64        // Number of L2 block in the span
	originBits        *big.Int      // Standard span-batch bitlist of blockCount bits. Each bit indicates if the L1 origin is changed at the L2 block.
	l1Blocks          []uint64      // List of L1 block numbers for each L2 block
	l1BlockTimestamps []uint64      // List of L1 block timestamps for each L2 block
	blockTxCounts     []uint64      // List of transaction counts for each L2 block
	extraDatas        [][]byte      // List of L2 block extra datas
	txs               *spanBatchTxs // Transactions encoded in SpanBatch specs
}

// RawSpanBatch is another representation of SpanBatch, that encodes inner according to SpanBatch specs.
type RawSpanBatch struct {
	spanBatchPrefix
	spanBatchPayload
}

// GetBatchType returns its batch type (batch_version)
func (b *RawSpanBatch) GetBatchType() int {
	return derive.SpanBatchType
}

// decodeParentCheck parses inner into bp.parentCheck
func (bp *spanBatchPrefix) decodeParentCheck(r *bytes.Reader) error {
	_, err := io.ReadFull(r, bp.parentCheck[:])
	if err != nil {
		return fmt.Errorf("failed to read parent check: %w", err)
	}
	return nil
}

// decodeL1OriginCheck parses inner into bp.decodeL1OriginCheck
func (bp *spanBatchPrefix) decodeL1OriginCheck(r *bytes.Reader) error {
	_, err := io.ReadFull(r, bp.l1OriginCheck[:])
	if err != nil {
		return fmt.Errorf("failed to read l1 origin check: %w", err)
	}
	return nil
}

func (bp *spanBatchPrefix) decodeL2StartBlock(r *bytes.Reader) (err error) {
	bp.l2StartBlock, err = binary.ReadUvarint(r)
	if err != nil {
		return fmt.Errorf("failed to read l2 start block: %w", err)
	}
	return nil
}

// decodePrefix parses inner into bp.spanBatchPrefix
func (bp *spanBatchPrefix) decodePrefix(r *bytes.Reader) error {
	if err := bp.decodeL2StartBlock(r); err != nil {
		return err
	}
	if err := bp.decodeParentCheck(r); err != nil {
		return err
	}
	if err := bp.decodeL1OriginCheck(r); err != nil {
		return err
	}
	return nil
}

// decodeOriginBits parses inner into bp.originBits
func (bp *spanBatchPayload) decodeOriginBits(r *bytes.Reader) error {
	if bp.blockCount > derive.MaxSpanBatchElementCount {
		return ErrTooBigSpanBatchSize
	}
	bits, err := decodeSpanBatchBits(r, bp.blockCount)
	if err != nil {
		return fmt.Errorf("failed to decode origin bits: %w", err)
	}
	bp.originBits = bits
	return nil
}

// decodeBlockCount parses inner into bp.blockCount
func (bp *spanBatchPayload) decodeBlockCount(r *bytes.Reader) error {
	blockCount, err := binary.ReadUvarint(r)
	if err != nil {
		return fmt.Errorf("failed to read block count: %w", err)
	}
	// number of L2 block in span batch cannot be greater than MaxSpanBatchElementCount
	if blockCount > derive.MaxSpanBatchElementCount {
		return ErrTooBigSpanBatchSize
	}
	if blockCount == 0 {
		return ErrEmptySpanBatch
	}
	bp.blockCount = blockCount
	return nil
}

// decodeBlockTxCounts parses inner into bp.blockTxCounts
// and sets bp.txs.totalBlockTxCount as sum(bp.blockTxCounts)
func (bp *spanBatchPayload) decodeBlockTxCounts(r *bytes.Reader) error {
	var blockTxCounts []uint64
	for i := 0; i < int(bp.blockCount); i++ {
		blockTxCount, err := binary.ReadUvarint(r)
		if err != nil {
			return fmt.Errorf("failed to read block tx count: %w", err)
		}
		// number of txs in single L2 block cannot be greater than MaxSpanBatchElementCount
		// every tx will take at least single byte
		if blockTxCount > derive.MaxSpanBatchElementCount {
			return ErrTooBigSpanBatchSize
		}
		blockTxCounts = append(blockTxCounts, blockTxCount)
	}
	bp.blockTxCounts = blockTxCounts
	return nil
}

// decodeL1Blocks parses inner into bp.l1Blocks
func (bp *spanBatchPayload) decodeL1Blocks(r *bytes.Reader) error {
	var l1Blocks []uint64
	for i := 0; i < int(bp.blockCount); i++ {
		l1Block, err := binary.ReadUvarint(r)
		if err != nil {
			return fmt.Errorf("failed to read l1 blocks: %w", err)
		}
		l1Blocks = append(l1Blocks, l1Block)
	}
	bp.l1Blocks = l1Blocks
	return nil
}

// decodeL1BlockTimestamps parses inner into bp.l1BlockTimestamps
func (bp *spanBatchPayload) decodeL1BlockTimestamps(r *bytes.Reader) error {
	var l1BlockTimestamps []uint64
	for i := 0; i < int(bp.blockCount); i++ {
		l1BlockTimestamp, err := binary.ReadUvarint(r)
		if err != nil {
			return fmt.Errorf("failed to read l1 block timestamps: %w", err)
		}
		l1BlockTimestamps = append(l1BlockTimestamps, l1BlockTimestamp)
	}
	bp.l1BlockTimestamps = l1BlockTimestamps
	return nil
}

// decodeL2BlockExtraDatas parses inner into bp.extraDatas
func (bp *spanBatchPayload) decodeL2BlockExtraDatas(r *bytes.Reader) error {
	var extraDatas [][]byte
	for i := 0; i < int(bp.blockCount); i++ {
		extraData := make([]byte, 97)
		if _, err := io.ReadFull(r, extraData); err != nil {
			return fmt.Errorf("failed to read l2 block extra data: %w", err)
		}
		extraDatas = append(extraDatas, extraData)
	}
	bp.extraDatas = extraDatas
	return nil
}

// decodeTxs parses inner into bp.txs
func (bp *spanBatchPayload) decodeTxs(r *bytes.Reader) error {
	if bp.txs == nil {
		bp.txs = &spanBatchTxs{}
	}
	if bp.blockTxCounts == nil {
		return errors.New("failed to read txs: blockTxCounts not set")
	}
	totalBlockTxCount := uint64(0)
	for i := 0; i < len(bp.blockTxCounts); i++ {
		total, overflow := math.SafeAdd(totalBlockTxCount, bp.blockTxCounts[i])
		if overflow {
			return ErrTooBigSpanBatchSize
		}
		totalBlockTxCount = total
	}
	// total number of txs in span batch cannot be greater than MaxSpanBatchElementCount
	if totalBlockTxCount > derive.MaxSpanBatchElementCount {
		return ErrTooBigSpanBatchSize
	}
	bp.txs.totalBlockTxCount = totalBlockTxCount
	bp.txs.l1Timestamps = bp.l1BlockTimestamps
	bp.txs.l1BlockNumbers = bp.l1Blocks
	bp.txs.blockTxCounts = bp.blockTxCounts
	if err := bp.txs.decode(r); err != nil {
		return err
	}
	return nil
}

// decodePayload parses inner into bp.spanBatchPayload
func (bp *spanBatchPayload) decodePayload(r *bytes.Reader) error {
	if err := bp.decodeBlockCount(r); err != nil {
		return err
	}
	if err := bp.decodeOriginBits(r); err != nil {
		return err
	}
	if err := bp.decodeL1Blocks(r); err != nil {
		return err
	}
	if err := bp.decodeL1BlockTimestamps(r); err != nil {
		return err
	}
	if err := bp.decodeBlockTxCounts(r); err != nil {
		return err
	}
	if err := bp.decodeL2BlockExtraDatas(r); err != nil {
		return err
	}
	if err := bp.decodeTxs(r); err != nil {
		return err
	}
	return nil
}

// decode reads the byte encoding of SpanBatch from Reader stream
func (b *RawSpanBatch) decode(r *bytes.Reader) error {
	if err := b.decodePrefix(r); err != nil {
		return fmt.Errorf("failed to decode span batch prefix: %w", err)
	}
	if err := b.decodePayload(r); err != nil {
		return fmt.Errorf("failed to decode span batch payload: %w", err)
	}
	return nil
}

// derive converts RawSpanBatch into SpanBatch, which has a list of SpanBatchElement.
// We need chain config constants to derive values for making payload attributes.
func (b *RawSpanBatch) derive(chainID *big.Int) (*SpanBatch, error) {
	if b.blockCount == 0 {
		return nil, ErrEmptySpanBatch
	}

	if err := b.txs.recoverV(chainID); err != nil {
		return nil, err
	}
	fullTxs, err := b.txs.fullTxs(chainID, b.l2StartBlock)
	if err != nil {
		return nil, err
	}

	spanBatch := SpanBatch{
		ChainID:       chainID,
		ParentCheck:   b.parentCheck,
		L1OriginCheck: b.l1OriginCheck,
		Batches:       make([]*SpanBatchElement, 0, b.blockCount),
	}

	txIdx := 0
	for i := 0; i < int(b.blockCount); i++ {
		batch := SpanBatchElement{
			ExtraData:    b.extraDatas[i],
			Transactions: make(types.Transactions, 0, b.blockTxCounts[i]),
			Timestamp:    fullTxs[txIdx].L1Timestamp(),
			EpochNum:     rollup.Epoch(fullTxs[txIdx].L1BlockNumber().Uint64()),
		}

		for j := 0; j < int(b.blockTxCounts[i]); j++ {
			batch.Transactions = append(batch.Transactions, fullTxs[txIdx])
			txIdx++
		}
		spanBatch.Batches = append(spanBatch.Batches, &batch)
	}
	return &spanBatch, nil
}

// ToSpanBatch converts RawSpanBatch to SpanBatch,
// which implements a wrapper of derive method of RawSpanBatch
func (b *RawSpanBatch) ToSpanBatch(chainID *big.Int) (*SpanBatch, error) {
	spanBatch, err := b.derive(chainID)
	if err != nil {
		return nil, err
	}
	return spanBatch, nil
}

// SpanBatchElement is a derived form of input to build a L2 block.
// similar to SingularBatch, but does not have ParentHash and EpochHash
// because Span batch spec does not contain parent hash and epoch hash of every block in the span.
type SpanBatchElement struct {
	EpochNum     rollup.Epoch // aka l1 num
	Timestamp    uint64
	ExtraData    []byte
	Transactions types.Transactions
}

// SpanBatch is an implementation of Batch interface,
// containing the input to build a span of L2 blocks in derived form (SpanBatchElement)
type SpanBatch struct {
	ParentCheck   [20]byte // First 20 bytes of the first block's parent hash
	L1OriginCheck [20]byte // First 20 bytes of the last block's L1 origin hash
	L2StartBlock  uint64
	ChainID       *big.Int
	Batches       []*SpanBatchElement // List of block input in derived form

	// caching
	originBits    *big.Int
	blockTxCounts []uint64
	sbtxs         *spanBatchTxs
}

func (b *SpanBatch) AsSingularBatch() (*derive.SingularBatch, bool) { return nil, false }
func (b *SpanBatch) AsSpanBatch() (*SpanBatch, bool)                { return b, true }

// GetBatchType returns its batch type (batch_version)
func (b *SpanBatch) GetBatchType() int {
	return derive.SpanBatchType
}

// GetTimestamp returns timestamp of the first block in the span
func (b *SpanBatch) GetTimestamp() uint64 {
	return b.Batches[0].Timestamp
}

// TxCount returns the tx count for the batch
func (b *SpanBatch) TxCount() (count uint64) {
	for _, txCount := range b.blockTxCounts {
		count += txCount
	}
	return
}

// LogContext creates a new log context that contains information of the batch
func (b *SpanBatch) LogContext(log log.Logger) log.Logger {
	if len(b.Batches) == 0 {
		return log.New("block_count", 0)
	}
	return log.New(
		"batch_type", "SpanBatch",
		"batch_timestamp", b.Batches[0].Timestamp,
		"parent_check", hexutil.Encode(b.ParentCheck[:]),
		"origin_check", hexutil.Encode(b.L1OriginCheck[:]),
		"start_epoch_number", b.GetStartEpochNum(),
		"end_epoch_number", b.GetBlockEpochNum(len(b.Batches)-1),
		"block_count", len(b.Batches),
		"txs", b.TxCount(),
	)
}

// GetStartEpochNum returns epoch number(L1 origin block number) of the first block in the span
func (b *SpanBatch) GetStartEpochNum() rollup.Epoch {
	return b.Batches[0].EpochNum
}

// CheckOriginHash checks if the l1OriginCheck matches the first 20 bytes of given hash, probably L1 block hash from the current canonical L1 chain.
func (b *SpanBatch) CheckOriginHash(hash common.Hash) bool {
	return bytes.Equal(b.L1OriginCheck[:], hash.Bytes()[:20])
}

// CheckParentHash checks if the parentCheck matches the first 20 bytes of given hash, probably the current L2 safe head.
func (b *SpanBatch) CheckParentHash(hash common.Hash) bool {
	return bytes.Equal(b.ParentCheck[:], hash.Bytes()[:20])
}

// GetBlockEpochNum returns the epoch number(L1 origin block number) of the block at the given index in the span.
func (b *SpanBatch) GetBlockEpochNum(i int) uint64 {
	return uint64(b.Batches[i].EpochNum)
}

// GetBlockTimestamp returns the timestamp of the block at the given index in the span.
func (b *SpanBatch) GetBlockTimestamp(i int) uint64 {
	return b.Batches[i].Timestamp
}

// GetBlockTransactions returns the encoded transactions of the block at the given index in the span.
func (b *SpanBatch) GetBlockTransactions(i int) types.Transactions {
	return b.Batches[i].Transactions
}

// GetBlockCount returns the number of blocks in the span
func (b *SpanBatch) GetBlockCount() int {
	return len(b.Batches)
}

func (b *SpanBatch) peek(n int) *SpanBatchElement { return b.Batches[len(b.Batches)-1-n] }

func (b *SpanBatch) DeriveL2Blocks() []*types.Block {
	blocks := make([]*types.Block, 0, len(b.Batches))
	for i, batch := range b.Batches {
		header := &types.Header{
			Number: new(big.Int).SetUint64(b.L2StartBlock + uint64(i)),
			Time:   new(big.Int).SetUint64(batch.Timestamp).Uint64(),
			Extra:  batch.ExtraData,
		}

		blocks = append(blocks, types.NewBlock(header, batch.Transactions, nil, nil))
	}
	return blocks
}

// DeriveSpanBatch derives SpanBatch from BatchData.
func DeriveSpanBatch(batchData *BatchData, chainID *big.Int) (*SpanBatch, error) {
	rawSpanBatch, ok := batchData.inner.(*RawSpanBatch)
	if !ok {
		return nil, derive.NewCriticalError(errors.New("failed type assertion to SpanBatch"))
	}
	// If the batch type is Span batch, derive block inputs from RawSpanBatch.
	return rawSpanBatch.ToSpanBatch(chainID)
}
