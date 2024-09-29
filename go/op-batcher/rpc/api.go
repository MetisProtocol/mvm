package rpc

import (
	"context"
	"errors"

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
	// return a.b.StartBatchSubmitting()
	// TODO: currently we still need the old Metis ts batcher to submit commitment to the chain to maintain compatibility,
	//       so we can only allow this new batcher to be invoked manually by the old batcher for now.
	return errors.New("we do not allow the batcher to auto execute")
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

// SubmitBlocks allows other components to invoke op-batcher to submit a range of blocks to L1.
// Please make sure this is only called by the old Metis ts batcher. DO NOT expose this to the public.
func (a *batcherAPI) SubmitBlocks(ctx context.Context, startBlock, endBlock uint64) ([]common.Hash, error) {
	return a.b.SubmitBlocks(ctx, startBlock, endBlock)
}
