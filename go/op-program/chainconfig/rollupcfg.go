package chainconfig

import (
	"errors"
	"math/big"

	"github.com/MetisProtocol/mvm/l2geth/common"
)

type InboxSenderType uint8

type BatcherAddressAtHeight struct {
	Height  uint64         `json:"height"`
	Address common.Address `json:"address"`
}

type RollupConfig struct {
	L1ChainId    *big.Int       `json:"l1ChainId"`
	InboxAddress common.Address `json:"inboxAddress"`
	SCCAddress   common.Address `json:"sccAddress"`
	CTCAddress   common.Address `json:"ctcAddress"`

	// the address of batcher address with height must be sorted in descending order,
	// otherwise the search might be fail.
	// since this data must be static, it's better to sort it before using instead of sorting it in the program.
	TxChainBatcherAddresses []BatcherAddressAtHeight `json:"txChainBatcherAddresses"`
	BlobBatcherAddresses    []BatcherAddressAtHeight `json:"blobBatcherAddresses"`
}

func (c RollupConfig) Check() error {
	if c.L1ChainId == nil {
		return errors.New("missing L1ChainId")
	}
	if c.InboxAddress == (common.Address{}) {
		return errors.New("missing InboxAddress")
	}
	if c.SCCAddress == (common.Address{}) {
		return errors.New("missing SCCAddress")
	}
	if c.CTCAddress == (common.Address{}) {
		return errors.New("missing CTCAddress")
	}
	if len(c.TxChainBatcherAddresses) == 0 {
		return errors.New("missing TxChainBatcherAddresses")
	}
	if len(c.BlobBatcherAddresses) == 0 {
		return errors.New("missing BlobBatcherAddresses")
	}
	return nil
}

var (
	MetisSepoliaRollupConfig = &RollupConfig{
		L1ChainId:    big.NewInt(11155111),
		InboxAddress: common.HexToAddress("0xFf00000000000000000000000001115511159902"),
		SCCAddress:   common.HexToAddress("0x9DCC53737FcB3E86a17CF435ca3c15390D4FC7Ed"),
		CTCAddress:   common.HexToAddress("0x5435d351e0aCc874579eC67Ba46440ee6AC892b8"),
		TxChainBatcherAddresses: []BatcherAddressAtHeight{
			{
				Height:  5536996,
				Address: common.HexToAddress("0x578c88EeEe23Db03E70aDB2445F0043bEC3C416E"),
			},
		},
		BlobBatcherAddresses: []BatcherAddressAtHeight{
			{
				Height:  7683900,
				Address: common.HexToAddress("0xd664e05a1f64E4a105C5a64dBf6761e90b16832e"),
			},
		},
	}
	MetisAndromedaRollupConfig = &RollupConfig{
		L1ChainId:    big.NewInt(1088),
		InboxAddress: common.HexToAddress("0xFf00000000000000000000000000000000001088"),
		SCCAddress:   common.HexToAddress("0xA2FaAAC9120c1Ff75814F0c6DdB119496a12eEA6"),
		CTCAddress:   common.HexToAddress("0x56a76bcC92361f6DF8D75476feD8843EdC70e1C9"),
		TxChainBatcherAddresses: []BatcherAddressAtHeight{
			{
				Height:  19439547,
				Address: common.HexToAddress("0x1A9da0aedA630dDf2748a453BF6d92560762D914"),
			},
		},
		BlobBatcherAddresses: []BatcherAddressAtHeight{
			// FIXME: update this later, this is just a placeholder, since currently we don't have blob batcher address right now
			{
				Height:  19439547,
				Address: common.HexToAddress("0x1A9da0aedA630dDf2748a453BF6d92560762D914"),
			},
		},
	}
)
