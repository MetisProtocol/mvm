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
)

var (
	MetisSepoliaRollupConfig = &RollupConfig{
		L1ChainId:    big.NewInt(11155111),
		InboxAddress: common.HexToAddress("0xFf00000000000000000000000001115511159902"),
		TxChainBatcherAddresses: []BatcherAddressAtHeight{
			{
				Height:  0,
				Address: common.HexToAddress("0x578c88EeEe23Db03E70aDB2445F0043bEC3C416E"),
			},
		},
		BlobBatcherAddresses: []BatcherAddressAtHeight{
			{
				Height:  0,
				Address: common.HexToAddress("0x578c88EeEe23Db03E70aDB2445F0043bEC3C416E"),
			},
		},
	}

	MetisAndromedaRollupConfig = &RollupConfig{
		L1ChainId:    big.NewInt(11155111),
		InboxAddress: common.HexToAddress("0xFf00000000000000000000000000000000001088"),
		TxChainBatcherAddresses: []BatcherAddressAtHeight{
			{
				Height:  0,
				Address: common.HexToAddress("0x1A9da0aedA630dDf2748a453BF6d92560762D914"),
			},
		},
		BlobBatcherAddresses: []BatcherAddressAtHeight{
			{
				Height:  0,
				Address: common.HexToAddress("0x1A9da0aedA630dDf2748a453BF6d92560762D914"),
			},
		},
	}
)

var l2ChainConfigsByChainID = map[uint64]*params.ChainConfig{
	59902: MetisAndromedaChainConfig,
	1088:  MetisSepoliaChainConfig,
}

var l2RollupConfigsByChainID = map[uint64]*RollupConfig{
	59902: MetisSepoliaRollupConfig,
	1088:  MetisAndromedaRollupConfig,
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
