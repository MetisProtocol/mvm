package chainconfig

import (
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

	// the address of batcher address with height must be sorted in descending order,
	// otherwise the search might be fail.
	// since this data must be static, it's better to sort it before using instead of sorting it in the program.
	TxChainBatcherAddresses []BatcherAddressAtHeight `json:"txChainBatcherAddresses"`
	BlobBatcherAddresses    []BatcherAddressAtHeight `json:"blobBatcherAddresses"`
}
