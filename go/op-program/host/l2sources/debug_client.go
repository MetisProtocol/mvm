package l2sources

import (
	"context"
	"fmt"

	"github.com/ethereum-optimism/optimism/op-service/sources/batching"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/MetisProtocol/mvm/l2geth/common"
)

type DebugClient struct {
	callContext batching.CallContextFn
}

func NewDebugClient(callContext batching.CallContextFn) *DebugClient {
	return &DebugClient{callContext}
}

func (o *DebugClient) NodeByHash(ctx context.Context, hash common.Hash) ([]byte, error) {
	// MPT nodes are stored as the hash of the node (with no prefix)
	node, err := o.dbGet(ctx, hash[:])
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve state MPT node: %w", err)
	}
	return node, nil
}

func (o *DebugClient) dbGet(ctx context.Context, key []byte) ([]byte, error) {
	var node hexutil.Bytes
	err := o.callContext(ctx, &node, "debug_dbGet", hexutil.Encode(key))
	if err != nil {
		return nil, fmt.Errorf("fetch error %x: %w", key, err)
	}
	return node, nil
}
