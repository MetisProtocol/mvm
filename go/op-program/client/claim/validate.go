package claim

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/log"

	"github.com/ethereum-optimism/optimism/op-service/eth"

	"github.com/MetisProtocol/mvm/l2geth/consensus"
)

var ErrClaimNotValid = errors.New("invalid claim")

type L2Source interface {
	L2BlockRefByLabel(ctx context.Context, label eth.BlockLabel) (eth.L2BlockRef, error)
	L2OutputRoot(uint64) (eth.Bytes32, error)
}

func ValidateClaim(log log.Logger, l2ClaimBlockNum uint64, claimedOutputRoot eth.Bytes32, src consensus.ChainHeaderReader) error {
	l2Head := src.CurrentHeader()
	if l2Head == nil {
		return fmt.Errorf("cannot retrieve l2 head")
	}
	if l2Head.Number.Uint64() != l2ClaimBlockNum {
		return fmt.Errorf("claim block number %v does not match l2 head block number %v", l2ClaimBlockNum, l2Head.Number)
	}

	outputRootHex := l2Head.Root.Hex()
	log.Info("Validating claim", "output", outputRootHex, "claim", claimedOutputRoot.String())

	if claimedOutputRoot != eth.Bytes32(l2Head.Root) {
		return fmt.Errorf("%w: claim: %v actual: %v", ErrClaimNotValid, claimedOutputRoot, outputRootHex)
	}

	return nil
}
