package state

import (
	"bytes"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/rollup/dump"
	"github.com/ethereum/go-ethereum/crypto"
)

var ovmTransferTopic = common.BytesToHash(crypto.Keccak256([]byte("Transfer(address,address,uint256)")))

// recordOVMAddressPreimage supplies migration with the original address even
// when its only balance lives in OVM_ETH storage and it has no account leaf.
// AddPreimage journals the auxiliary data; normal block insertion persists it.
func (s *StateDB) recordOVMAddressPreimage(addr common.Address) {
	s.AddPreimage(common.BytesToHash(crypto.Keccak256(addr[:])), addr[:])
}

// recordOVMTransferPreimages covers ERC20 balance changes which bypass the
// native balance setters. Malformed events must not affect log processing.
func (s *StateDB) recordOVMTransferPreimages(log *types.Log) {
	if log.Address != dump.OvmEthAddress || len(log.Topics) != 3 ||
		log.Topics[0] != ovmTransferTopic || len(log.Data) != common.HashLength {
		return
	}
	var padding [common.HashLength - common.AddressLength]byte
	if !bytes.Equal(log.Topics[1][:len(padding)], padding[:]) ||
		!bytes.Equal(log.Topics[2][:len(padding)], padding[:]) {
		return
	}
	s.recordOVMAddressPreimage(common.BytesToAddress(log.Topics[1][:]))
	s.recordOVMAddressPreimage(common.BytesToAddress(log.Topics[2][:]))
}
