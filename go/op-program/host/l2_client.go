package host

import (
	"context"
	"fmt"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/ethclient"
)

type L2Client struct {
	// l2Head is the L2 block hash that we use to fetch L2 output
	l2Head common.Hash

	*ethclient.Client
}

func NewL2Client(client *ethclient.Client, l2Head common.Hash) *L2Client {
	return &L2Client{
		Client: client,
		l2Head: l2Head,
	}
}

func (s *L2Client) OutputByRoot(ctx context.Context, l2OutputRoot common.Hash) (common.Hash, error) {
	block, err := s.BlockByHash(ctx, s.l2Head)
	if err != nil {
		return common.Hash{}, err
	}

	if block.Root() != l2OutputRoot {
		// For fault proofs, we only reference outputs at the l2 head at boot time
		// The caller shouldn't be requesting outputs at any other block
		// If they are, there is no chance of recovery and we should panic to avoid retrying forever
		panic(fmt.Errorf("output root %v from specified L2 block %v does not match requested output root %v", block.Root(), s.l2Head, l2OutputRoot))
	}

	return l2OutputRoot, nil
}
