package chainconfig

import (
	"fmt"

	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum/go-ethereum/params"
)

var MetisAndromedaChainConfig, MetisSepoliaChainConfig, MetisLocalDevChainConfig *params.ChainConfig

func init() {
	mustLoadConfig := func(chainID uint64) *params.ChainConfig {
		cfg, err := params.LoadOPStackChainConfig(chainID)
		if err != nil {
			panic(err)
		}
		return cfg
	}
	MetisAndromedaChainConfig = mustLoadConfig(1088)
	MetisSepoliaChainConfig = mustLoadConfig(666)
	MetisLocalDevChainConfig = mustLoadConfig(108800)
}

func RollupConfigByChainID(chainID uint64) (*rollup.Config, error) {
	config, err := rollup.LoadOPStackRollupConfig(chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to get rollup config for chain ID %d: %w", chainID, err)
	}
	return config, nil
}

func ChainConfigByChainID(chainID uint64) (*params.ChainConfig, error) {
	return params.LoadOPStackChainConfig(chainID)
}

func newUint64(val uint64) *uint64 { return &val }
