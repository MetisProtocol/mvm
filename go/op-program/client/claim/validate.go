package claim

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/rollup"
	merkletrie "github.com/ethereum-optimism/optimism/go/op-program/client/merkel"
)

var ErrClaimNotValid = errors.New("invalid claim")

func ValidateClaim(claimedOutputRoot common.Hash, stateRoots []hexutil.Bytes, disputedBatchHeader *rollup.BatchHeader) error {
	root, _ := merkletrie.WriteTrie(stateRoots)

	outputBatchHeader := &rollup.BatchHeader{
		BatchRoot:         common.Hash(root),
		BatchSize:         disputedBatchHeader.BatchSize,
		PrevTotalElements: disputedBatchHeader.PrevTotalElements,
		ExtraData:         disputedBatchHeader.ExtraData,
	}

	if claimedOutputRoot != outputBatchHeader.Hash() {
		return fmt.Errorf("claimed output root %v does not match computed output root %v", claimedOutputRoot.Hex(), outputBatchHeader.Hash().Hex())
	}

	return nil
}
