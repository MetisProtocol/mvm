package claim

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/log"

	"github.com/ethereum-optimism/optimism/op-service/eth"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/consensus"
	dtl "github.com/ethereum-optimism/optimism/go/op-program/client/dtl"
)

var ErrClaimNotValid = errors.New("invalid claim")

func ValidateClaim(log log.Logger, l2ClaimBlockNum uint64, claimedOutputRoot eth.Bytes32, dtlOracle dtl.Oracle, src consensus.ChainHeaderReader) error {
	l2Head := src.CurrentHeader()
	if l2Head == nil {
		return fmt.Errorf("cannot retrieve l2 head")
	}
	if l2Head.Number.Uint64() != l2ClaimBlockNum {
		return fmt.Errorf("claim block number %v does not match l2 head block number %v", l2ClaimBlockNum, l2Head.Number)
	}

	disputedBatchHeader := dtlOracle.StateBatchHeaderByHash(common.Hash(claimedOutputRoot))
	disputedStateRoots := dtlOracle.StateBatchesByHash(common.Hash(claimedOutputRoot))
	startBlockIndex := disputedBatchHeader.PrevTotalElements.Uint64()

	for batchOffset, disputedStateRoot := range disputedStateRoots {
		if startBlockIndex+uint64(batchOffset)+1 == l2ClaimBlockNum {
			if disputedStateRoots[batchOffset] != src.CurrentHeader().Root {
				return fmt.Errorf("disputed state root %v does not match l2 head root %v", disputedStateRoot, src.CurrentHeader().Root)
			} else {
				return nil
			}
		}
	}

	return fmt.Errorf("claimed block number %v not found in disputed state roots", l2ClaimBlockNum)
}
