package client

import (
	"encoding/binary"
	"encoding/json"
	"math"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/params"
	"github.com/MetisProtocol/mvm/l2geth/rollup"

	preimage "github.com/ethereum-optimism/optimism/go/op-preimage"
)

const (
	L2ClaimLocalIndex preimage.LocalIndexKey = iota + 1
	L2ClaimBlockNumberLocalIndex
	L2ChainIDLocalIndex

	// These local keys are only used for custom chains
	L2ChainConfigLocalIndex
	RollupConfigLocalIndex
)

// CustomChainIDIndicator is used to detect when the program should load custom chain configuration
const CustomChainIDIndicator = uint64(math.MaxUint64)

type BootInfo struct {
	L2Claim            common.Hash
	L2ClaimBlockNumber uint64
	L2ChainID          uint64

	L2ChainConfig *params.ChainConfig
	RollupConfig  *rollup.Config
}

type oracleClient interface {
	Get(key preimage.Key) []byte
}

type BootstrapClient struct {
	r oracleClient
}

func NewBootstrapClient(r oracleClient) *BootstrapClient {
	return &BootstrapClient{r: r}
}

func (br *BootstrapClient) BootInfo() *BootInfo {
	l2Claim := common.BytesToHash(br.r.Get(L2ClaimLocalIndex))
	l2ClaimBlockNumber := binary.BigEndian.Uint64(br.r.Get(L2ClaimBlockNumberLocalIndex))
	l2ChainID := binary.BigEndian.Uint64(br.r.Get(L2ChainIDLocalIndex))

	var l2ChainConfig params.ChainConfig
	err := json.Unmarshal(br.r.Get(L2ChainConfigLocalIndex), &l2ChainConfig)
	if err != nil {
		panic("failed to bootstrap l2ChainConfig")
	}
	var rollupConfig rollup.Config
	err = json.Unmarshal(br.r.Get(RollupConfigLocalIndex), &rollupConfig)
	if err != nil {
		panic("failed to bootstrap rollup config")
	}

	return &BootInfo{
		L2Claim:            l2Claim,
		L2ClaimBlockNumber: l2ClaimBlockNumber,
		L2ChainID:          l2ChainID,
		L2ChainConfig:      &l2ChainConfig,
		RollupConfig:       &rollupConfig,
	}
}
