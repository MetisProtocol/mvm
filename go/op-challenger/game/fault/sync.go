package fault

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethereum-optimism/optimism/op-service/eth"

	"github.com/MetisProtocol/mvm/l2geth/rollup"
)

var ErrNotInSync = errors.New("local node too far behind")

type syncStatusValidator struct {
	statusProvider rollup.RollupClient
}

func newSyncStatusValidator(statusProvider rollup.RollupClient) *syncStatusValidator {
	return &syncStatusValidator{
		statusProvider: statusProvider,
	}
}

func (s *syncStatusValidator) ValidateNodeSynced(ctx context.Context, gameL1Head eth.BlockID) error {
	highestSynced, err := s.statusProvider.GetHighestSynced()
	if err != nil {
		return fmt.Errorf("failed to retrieve local node sync status: %w", err)
	}
	if highestSynced <= gameL1Head.Number {
		return fmt.Errorf("%w require L1 block above %v but at %v", ErrNotInSync, gameL1Head.Number, highestSynced)
	}
	return nil
}
