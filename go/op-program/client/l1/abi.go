package l1

import (
	_ "embed"
	"strings"

	"github.com/MetisProtocol/mvm/l2geth/accounts/abi"
)

var (
	//go:embed abis/scc.json
	sccABIJson string
	//go:embed abis/ctc.json
	ctcABIJson string
)

var (
	SCCABI, _ = abi.JSON(strings.NewReader(sccABIJson))
	CTCABI, _ = abi.JSON(strings.NewReader(ctcABIJson))
)
