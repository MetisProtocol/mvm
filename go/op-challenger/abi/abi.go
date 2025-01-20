package abi

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

//go:embed override/DisputeGameFactory.json
var disputeGameFactory []byte

//go:embed override/FaultDisputeGame.json
var faultDisputeGame []byte

//go:embed override/StateCommitmentChain.json
var stateCommitmentChain []byte

func LoadDisputeGameFactoryABI() *abi.ABI {
	return loadABI(disputeGameFactory)
}

func LoadFaultDisputeGameABI() *abi.ABI {
	return loadABI(faultDisputeGame)
}

func LoadSCCABI() *abi.ABI {
	return loadABI(stateCommitmentChain)
}

func EncodePacked(params ...interface{}) ([]byte, error) {
	var buffer bytes.Buffer

	for _, param := range params {
		switch v := param.(type) {
		case common.Address:
			buffer.Write(v.Bytes())
		case uint8, uint16, uint32, uint64, int8, int16, int32, int64:
			err := binary.Write(&buffer, binary.BigEndian, v)
			if err != nil {
				return nil, fmt.Errorf("failed to encode integer: %v", err)
			}
		case string:
			buffer.Write([]byte(v))
		case []byte:
			buffer.Write(v)
		default:
			return nil, fmt.Errorf("unsupported type: %T", v)
		}
	}

	return buffer.Bytes(), nil
}

func loadABI(json []byte) *abi.ABI {
	if parsed, err := abi.JSON(bytes.NewReader(json)); err != nil {
		panic(err)
	} else {
		return &parsed
	}
}
