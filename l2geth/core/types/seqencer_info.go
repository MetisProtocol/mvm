package types

import (
	"github.com/MetisProtocol/mvm/l2geth/common"
)

type SequencerInfo struct {
	SequencerAddress common.Address `json:"sequencerAddress"`
	SequencerUrl     string         `json:"sequencerUrl"`
	SequencerHeight  uint64         `json:"sequencerHeight"`
}

type SequencerInfoList struct {
	SeqList []SequencerInfo `json:"seqList"`
}
