package rpc

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	gethrpc "github.com/ethereum/go-ethereum/rpc"

	"github.com/ethereum-optimism/optimism/op-service/metrics"
	"github.com/ethereum-optimism/optimism/op-service/rpc"
)

type BatcherDriver interface {
	StartBatchSubmitting() error
	StopBatchSubmitting(ctx context.Context) error
	SubmitBlocks(ctx context.Context, startBlock, endBlock uint64) ([]common.Hash, error)
}

type adminAPI struct {
	*rpc.CommonAdminAPI
	b BatcherDriver
}

func NewAdminAPI(dr BatcherDriver, m metrics.RPCMetricer, log log.Logger) *adminAPI {
	return &adminAPI{
		CommonAdminAPI: rpc.NewCommonAdminAPI(m, log),
		b:              dr,
	}
}

func GetAdminAPI(api *adminAPI) gethrpc.API {
	return gethrpc.API{
		Namespace: "admin",
		Service:   api,
	}
}

func (a *adminAPI) StartBatcher(_ context.Context) error {
	return a.b.StartBatchSubmitting()
}

func (a *adminAPI) StopBatcher(ctx context.Context) error {
	return a.b.StopBatchSubmitting(ctx)
}

type batcherAPI struct {
	b BatcherDriver
}

func NewBatcherAPI(dr BatcherDriver) *batcherAPI {
	return &batcherAPI{
		b: dr,
	}
}

func GetBatcherAPI(api *batcherAPI) gethrpc.API {
	return gethrpc.API{
		Namespace: "batcher",
		Service:   api,
	}
}

func (a *batcherAPI) SubmitBlocks(ctx context.Context, startBlock, endBlock uint64) ([]common.Hash, error) {
	return a.b.SubmitBlocks(ctx, startBlock, endBlock)
}
