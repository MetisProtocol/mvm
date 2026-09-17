package state

import (
	"os"
	"testing"

	"github.com/MetisProtocol/mvm/l2geth/rollup/rcfg"
)

func TestMain(m *testing.M) {
	// The inherited state and trie-sync fixtures store native account balances.
	// OVM-specific tests explicitly select their mode and restore it with Cleanup.
	rcfg.UsingOVM = false
	os.Exit(m.Run())
}
