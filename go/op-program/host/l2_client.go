package host

import (
	"context"

	"github.com/ethereum-optimism/optimism/op-service/client"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum-optimism/optimism/op-service/sources"
	"github.com/ethereum-optimism/optimism/op-service/sources/caching"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
)

type L2Client struct {
	*sources.L2Client

	// l2Head is the L2 block hash that we use to fetch L2 output
	l2Head common.Hash

	rpcClient client.RPC
}

type L2ClientConfig struct {
	*sources.L2ClientConfig
	L2Head common.Hash
}

func NewL2Client(client client.RPC, log log.Logger, metrics caching.Metrics, config *L2ClientConfig) (*L2Client, error) {
	l2Client, err := sources.NewL2Client(client, log, metrics, config.L2ClientConfig)
	if err != nil {
		return nil, err
	}
	return &L2Client{
		L2Client:  l2Client,
		l2Head:    config.L2Head,
		rpcClient: client,
	}, nil
}

func (s *L2Client) OutputByRoot(ctx context.Context, l2OutputRoot common.Hash) (eth.Output, error) {
	var outputV0 eth.OutputV0
	err := s.rpcClient.CallContext(ctx, &outputV0, "optimism_outputByRoot", s.l2Head, l2OutputRoot)
	if err != nil {
		return nil, err
	}

	return &outputV0, nil
}
