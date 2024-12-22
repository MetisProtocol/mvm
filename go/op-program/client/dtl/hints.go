package rollup

import (
	"github.com/MetisProtocol/mvm/l2geth/common"
	preimage "github.com/ethereum-optimism/optimism/go/op-preimage"
)

const (
	HintStateBatch = "dtl-state-batch"
)

type StateBatch common.Hash

var _ preimage.Hint = StateBatch(common.Hash{})

func (l StateBatch) Hint() string {
	return HintStateBatch + " " + (common.Hash)(l).String()
}
