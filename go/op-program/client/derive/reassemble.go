package derive

import (
	"fmt"
	"io"
	"math/big"

	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
)

const (
	maxRLPBytesPerChannel = 10_000_000
)

type ChannelWithMetadata struct {
	ID         derive.ChannelID         `json:"id"`
	IsReady    bool                     `json:"is_ready"`
	Frames     []derive.Frame           `json:"frames"`
	Batches    []Batch                  `json:"batches"`
	BatchTypes []int                    `json:"batch_types"`
	ComprAlgos []derive.CompressionAlgo `json:"compr_algos"`
}

func ProcessFrames(l2ChainID *big.Int, id derive.ChannelID, frames []derive.Frame) (*ChannelWithMetadata, error) {
	ch := NewChannel(id)

	for _, frame := range frames {
		if ch.IsReady() {
			return nil, fmt.Errorf("Channel %v is ready despite having more frames\n", id.String())
			break
		}
		if err := ch.AddFrame(frame); err != nil {
			return nil, fmt.Errorf("Error adding to channel %v. Err: %v\n", id.String(), err)
		}
	}

	var (
		batches    = make([]Batch, 0)
		batchTypes = make([]int, 0)
		comprAlgos = make([]derive.CompressionAlgo, 0)
	)

	if ch.IsReady() {
		br, err := BatchReader(ch.Reader(), maxRLPBytesPerChannel)
		if err == nil {
			for batchData, err := br(); err != io.EOF; batchData, err = br() {
				if err != nil {
					return nil, err
				} else {
					comprAlgos = append(comprAlgos, batchData.ComprAlgo)
					batchType := batchData.GetBatchType()
					batchTypes = append(batchTypes, int(batchType))
					switch batchType {
					case derive.SpanBatchType:
						spanBatch, err := DeriveSpanBatch(batchData, l2ChainID)
						if err != nil {
							return nil, err
						}
						// spanBatch will be nil when errored
						batches = append(batches, spanBatch)
					default:
						return nil, fmt.Errorf("unrecognized batch type: %d for channel %v", batchData.GetBatchType(), id.String())
					}
				}
			}
		} else {
			return nil, fmt.Errorf("Error creating batch reader for channel %v. Err: %v\n", id.String(), err)
		}
	} else {
		return nil, fmt.Errorf("Channel %v is not ready\n", id.String())
	}

	return &ChannelWithMetadata{
		ID:         id,
		Frames:     frames,
		IsReady:    ch.IsReady(),
		Batches:    batches,
		BatchTypes: batchTypes,
		ComprAlgos: comprAlgos,
	}, nil
}
