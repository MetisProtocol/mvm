package testutil

import (
	"os"

	"github.com/MetisProtocol/mvm/l2geth/log"
)

func CreateLogger() log.Logger {
	logger := log.New()
	logger.SetHandler(log.LvlFilterHandler(log.LvlInfo, log.StreamHandler(os.Stdout, log.TerminalFormat(true))))
	return logger
}
