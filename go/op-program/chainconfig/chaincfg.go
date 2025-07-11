package chainconfig

import (
	"fmt"
	"math/big"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/params"
)

var (
	MetisSepoliaChainConfig = &params.ChainConfig{
		ChainID:             big.NewInt(59902),
		HomesteadBlock:      big.NewInt(0),
		DAOForkBlock:        big.NewInt(0),
		DAOForkSupport:      false,
		EIP150Block:         big.NewInt(0),
		EIP150Hash:          common.Hash{},
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
		ByzantiumBlock:      big.NewInt(0),
		ConstantinopleBlock: big.NewInt(0),
		PetersburgBlock:     big.NewInt(0),
		IstanbulBlock:       big.NewInt(0),
		MuirGlacierBlock:    big.NewInt(0),
		BerlinBlock:         big.NewInt(0),
		ShanghaiBlock:       big.NewInt(1000000),
		EWASMBlock:          nil,
		Clique: &params.CliqueConfig{
			Period: 0,
			Epoch:  30000,
		},
	}
	MetisAndromedaChainConfig = &params.ChainConfig{
		ChainID:             big.NewInt(1088),
		HomesteadBlock:      big.NewInt(0),
		DAOForkBlock:        big.NewInt(0),
		DAOForkSupport:      false,
		EIP150Block:         big.NewInt(0),
		EIP150Hash:          common.Hash{},
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
		ByzantiumBlock:      big.NewInt(0),
		ConstantinopleBlock: big.NewInt(0),
		PetersburgBlock:     big.NewInt(0),
		IstanbulBlock:       big.NewInt(0),
		MuirGlacierBlock:    big.NewInt(0),
		BerlinBlock:         big.NewInt(3380000),
		ShanghaiBlock:       big.NewInt(18118000),
		EWASMBlock:          nil,
		Clique: &params.CliqueConfig{
			Period: 0,
			Epoch:  30000,
		},
	}
	MetisDevnetChainConfig = &params.ChainConfig{
		ChainID:             big.NewInt(57005),
		HomesteadBlock:      big.NewInt(0),
		DAOForkBlock:        big.NewInt(0),
		DAOForkSupport:      false,
		EIP150Block:         big.NewInt(0),
		EIP150Hash:          common.Hash{},
		EIP155Block:         big.NewInt(0),
		EIP158Block:         big.NewInt(0),
		ByzantiumBlock:      big.NewInt(0),
		ConstantinopleBlock: big.NewInt(0),
		PetersburgBlock:     big.NewInt(0),
		IstanbulBlock:       big.NewInt(0),
		MuirGlacierBlock:    big.NewInt(0),
		BerlinBlock:         big.NewInt(0),
		ShanghaiBlock:       big.NewInt(0),
		EWASMBlock:          nil,
		Clique: &params.CliqueConfig{
			Period: 0,
			Epoch:  30000,
		},
	}
)

var l2ChainNamesByChainID = map[uint64]string{
	59902: "metis-sepolia",
	1088:  "metis-andromeda",
	57005: "metis-devnet",
}

var l2ChainConfigsByChainID = map[uint64]*params.ChainConfig{
	59902: MetisSepoliaChainConfig,
	1088:  MetisAndromedaChainConfig,
	57005: MetisDevnetChainConfig,
}

var l2RollupConfigsByChainID = map[uint64]*RollupConfig{
	59902: MetisSepoliaRollupConfig,
	1088:  MetisAndromedaRollupConfig,
}

func handleLegacyName(name string) string {
	switch name {
	case "mainnet":
		return "metis-andromeda"
	case "sepolia":
		return "metis-sepolia"
	case "devnet":
		return "metis-devnet"
	default:
		return name
	}
}

// ChainByName returns a chain, from known available configurations, by name.
// ChainByName returns nil when the chain name is unknown.
func ChainByName(name string) *params.ChainConfig {
	// Handle legacy name aliases
	name = handleLegacyName(name)

	switch name {
	case "metis-sepolia":
		return MetisSepoliaChainConfig
	case "metis-andromeda":
		return MetisAndromedaChainConfig
	case "metis-devnet":
		return MetisDevnetChainConfig
	}

	return nil
}

func RollupConfigByChainID(chainID uint64) (*RollupConfig, error) {
	rollupCfg, ok := l2RollupConfigsByChainID[chainID]
	if !ok {
		return nil, fmt.Errorf("chain ID %d not found", chainID)
	}

	return rollupCfg, nil
}

func ChainConfigByChainID(chainID uint64) (*params.ChainConfig, error) {
	chainCfg, ok := l2ChainConfigsByChainID[chainID]
	if !ok {
		return nil, fmt.Errorf("chain ID %d not found", chainID)
	}

	return chainCfg, nil
}

func AvailableNetworks() []string {
	networks := make([]string, 0, len(l2ChainNamesByChainID))
	for _, name := range l2ChainNamesByChainID {
		networks = append(networks, name)
	}
	return networks
}
