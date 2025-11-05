package rollup

import (
	preimage "github.com/ethereum-optimism/optimism/op-preimage"
	"github.com/ethereum/go-ethereum/common"
)

const (
	HintStateBatch = "dtl-state-batch"
)

type StateBatch common.Hash

var _ preimage.Hint = StateBatch(common.Hash{})

func (l StateBatch) Hint() string {
	return HintStateBatch + " " + (common.Hash)(l).String()
}
