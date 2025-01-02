package abi

import (
	"bytes"
	_ "embed"

	"github.com/ethereum/go-ethereum/accounts/abi"
)

//go:embed override/DisputeGameFactory.json
var disputeGameFactory []byte

//go:embed override/FaultDisputeGame.json
var faultDisputeGame []byte

func LoadDisputeGameFactoryABI() *abi.ABI {
	return loadABI(disputeGameFactory)
}

func LoadFaultDisputeGameABI() *abi.ABI {
	return loadABI(faultDisputeGame)
}

func loadABI(json []byte) *abi.ABI {
	if parsed, err := abi.JSON(bytes.NewReader(json)); err != nil {
		panic(err)
	} else {
		return &parsed
	}
}
