package core

import (
	"os"
	"testing"

	"github.com/MetisProtocol/mvm/l2geth/rollup/rcfg"
)

func TestMain(m *testing.M) {
	// The inherited Ethereum chain/txpool fixtures use native account balances
	// and Ethereum fees. OVM-specific tests explicitly enable OVM and restore it
	// with Cleanup before the parallel txpool tests run.
	rcfg.UsingOVM = false
	os.Exit(m.Run())
}
