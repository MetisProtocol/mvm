package derive

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rlp"
)

const (
	// SingularBatchType is the first version of Batch format, representing a single L2 block.
	SingularBatchType = 0
	// SpanBatchType is the Batch version used after Delta hard fork, representing a span of L2 blocks.
	SpanBatchType = 1
)

// Batch contains information to build one or multiple L2 blocks.
// Batcher converts L2 blocks into Batch and writes encoded bytes to Channel.
// Derivation pipeline decodes Batch from Channel, and converts to one or multiple payload attributes.
type Batch interface {
	GetBatchType() int
	GetTimestamp() uint64
	LogContext(log.Logger) log.Logger
	AsSpanBatch() (*SpanBatch, bool)
}

type batchWithMetadata struct {
	Batch
	comprAlgo derive.CompressionAlgo
}

func (b batchWithMetadata) LogContext(l log.Logger) log.Logger {
	lgr := b.Batch.LogContext(l)
	if b.comprAlgo == "" {
		return lgr
	}
	return lgr.With("compression_algo", b.comprAlgo)
}

// BatchData is used to represent the typed encoding & decoding.
// and wraps around a single interface InnerBatchData.
// Further fields such as cache can be added in the future, without embedding each type of InnerBatchData.
// Similar design with op-geth's types.Transaction struct.
type BatchData struct {
	inner     InnerBatchData
	ComprAlgo derive.CompressionAlgo
}

// InnerBatchData is the underlying inner of a BatchData.
// This is implemented by SingularBatch and RawSpanBatch.
type InnerBatchData interface {
	GetBatchType() int
	decode(r *bytes.Reader) error
}

func (bd *BatchData) GetBatchType() uint8 {
	return uint8(bd.inner.GetBatchType())
}

// DecodeRLP implements rlp.Decoder
func (b *BatchData) DecodeRLP(s *rlp.Stream) error {
	if b == nil {
		return errors.New("cannot decode into nil BatchData")
	}
	v, err := s.Bytes()
	if err != nil {
		return err
	}
	return b.decodeTyped(v)
}

// UnmarshalBinary decodes the canonical encoding of batch.
func (b *BatchData) UnmarshalBinary(data []byte) error {
	if b == nil {
		return errors.New("cannot decode into nil BatchData")
	}
	return b.decodeTyped(data)
}

// decodeTyped decodes a typed batchData
func (b *BatchData) decodeTyped(data []byte) error {
	if len(data) == 0 {
		return errors.New("batch too short")
	}
	var inner InnerBatchData
	switch data[0] {
	case SpanBatchType:
		inner = new(RawSpanBatch)
	default:
		return fmt.Errorf("unrecognized batch type: %d", data[0])
	}
	if err := inner.decode(bytes.NewReader(data[1:])); err != nil {
		return err
	}
	b.inner = inner
	return nil
}
