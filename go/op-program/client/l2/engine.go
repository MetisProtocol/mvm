package l2

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum-optimism/optimism/op-service/eth"

	l2common "github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/ethereum-optimism/optimism/go/op-program/client/l2/engineapi"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"

	"github.com/MetisProtocol/mvm/l2geth/core/types"
)

var ErrNotFound = errors.New("not found")

type OracleEngine struct {
	api       *engineapi.L2EngineAPI
	backend   engineapi.EngineBackend
	rollupCfg *rollup.Config
}

func NewOracleEngine(rollupCfg *rollup.Config, logger log.Logger, backend engineapi.EngineBackend) *OracleEngine {
	engineAPI := engineapi.NewL2EngineAPI(logger, backend, nil)
	return &OracleEngine{
		api:       engineAPI,
		backend:   backend,
		rollupCfg: rollupCfg,
	}
}

func (o *OracleEngine) L2OutputRoot(l2ClaimBlockNum uint64) (eth.Bytes32, error) {
	outBlock := o.backend.GetHeaderByNumber(l2ClaimBlockNum)
	if outBlock == nil {
		return eth.Bytes32{}, fmt.Errorf("failed to get L2 block at %d", l2ClaimBlockNum)
	}

	return eth.Bytes32(outBlock.Root), nil
}

func (o *OracleEngine) GetPayload(ctx context.Context, payloadInfo eth.PayloadInfo) (*eth.ExecutionPayloadEnvelope, error) {
	var res *eth.ExecutionPayloadEnvelope
	var err error
	switch method := o.rollupCfg.GetPayloadVersion(payloadInfo.Timestamp); method {
	case eth.GetPayloadV3:
		res, err = o.api.GetPayloadV3(ctx, payloadInfo.ID)
	case eth.GetPayloadV2:
		res, err = o.api.GetPayloadV2(ctx, payloadInfo.ID)
	default:
		return nil, fmt.Errorf("unsupported GetPayload version: %s", method)
	}
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (o *OracleEngine) ForkchoiceUpdate(ctx context.Context, state *eth.ForkchoiceState, attr *eth.PayloadAttributes) (*eth.ForkchoiceUpdatedResult, error) {
	return o.api.ForkchoiceUpdated(ctx, state, attr)
}

func (o *OracleEngine) NewPayload(ctx context.Context, payload *eth.ExecutionPayload, parentBeaconBlockRoot *common.Hash) (*eth.PayloadStatusV1, error) {
	return o.api.NewPayload(ctx, payload, []l2common.Hash{}, (*l2common.Hash)(parentBeaconBlockRoot))
}

func (o *OracleEngine) PayloadByHash(ctx context.Context, hash common.Hash) (*eth.ExecutionPayloadEnvelope, error) {
	block := o.backend.GetBlockByHash(l2common.Hash(hash))
	if block == nil {
		return nil, ErrNotFound
	}

	txs := block.Transactions()
	opaqueTxs := make([]eth.Data, len(txs))
	for i, _ := range txs {
		opaqueTxs = append(opaqueTxs, txs.GetRlp(i))
	}

	header := block.Header()
	return &eth.ExecutionPayloadEnvelope{
		ParentBeaconBlockRoot: nil,
		ExecutionPayload: &eth.ExecutionPayload{
			ParentHash:   common.Hash(block.ParentHash()),
			FeeRecipient: common.Address(block.Coinbase()),
			StateRoot:    eth.Bytes32(header.Root),
			ReceiptsRoot: eth.Bytes32(header.ReceiptHash),
			LogsBloom:    eth.Bytes256(header.Bloom),
			PrevRandao:   eth.Bytes32{},
			BlockNumber:  eth.Uint64Quantity(header.Number.Uint64()),
			GasLimit:     eth.Uint64Quantity(header.GasLimit),
			GasUsed:      eth.Uint64Quantity(header.GasUsed),
			Timestamp:    eth.Uint64Quantity(header.Time),
			ExtraData:    header.Extra,
			BlockHash:    common.Hash(header.Hash()),
			Transactions: opaqueTxs,
		},
	}, nil
}

func (o *OracleEngine) PayloadByNumber(ctx context.Context, n uint64) (*eth.ExecutionPayloadEnvelope, error) {
	hash := o.backend.GetCanonicalHash(n)
	if hash == (l2common.Hash{}) {
		return nil, ErrNotFound
	}
	return o.PayloadByHash(ctx, common.Hash(hash))
}

func (o *OracleEngine) L2BlockRefByLabel(ctx context.Context, label eth.BlockLabel) (eth.L2BlockRef, error) {
	var header *types.Header
	switch label {
	case eth.Unsafe:
		header = o.backend.CurrentHeader()
	case eth.Safe:
		header = o.backend.CurrentSafeBlock()
	case eth.Finalized:
		header = o.backend.CurrentFinalBlock()
	default:
		return eth.L2BlockRef{}, fmt.Errorf("unknown label: %v", label)
	}
	if header == nil {
		return eth.L2BlockRef{}, ErrNotFound
	}
	block := o.backend.GetBlockByHash(header.Hash())
	if block == nil {
		return eth.L2BlockRef{}, ErrNotFound
	}

	return L2BlockToBlockRef(block), nil
}

func (o *OracleEngine) L2BlockRefByHash(ctx context.Context, l2Hash common.Hash) (eth.L2BlockRef, error) {
	block := o.backend.GetBlockByHash(l2common.Hash(l2Hash))
	if block == nil {
		return eth.L2BlockRef{}, ErrNotFound
	}
	return L2BlockToBlockRef(block), nil
}

func (o *OracleEngine) L2BlockRefByNumber(ctx context.Context, n uint64) (eth.L2BlockRef, error) {
	hash := o.backend.GetCanonicalHash(n)
	if hash == (l2common.Hash{}) {
		return eth.L2BlockRef{}, ErrNotFound
	}
	return o.L2BlockRefByHash(ctx, common.Hash(hash))
}

func (o *OracleEngine) SystemConfigByL2Hash(ctx context.Context, hash common.Hash) (eth.SystemConfig, error) {
	payload, err := o.PayloadByHash(ctx, hash)
	if err != nil {
		return eth.SystemConfig{}, err
	}
	return derive.PayloadToSystemConfig(o.rollupCfg, payload.ExecutionPayload)
}

func L2BlockToBlockRef(block *types.Block) eth.L2BlockRef {
	return eth.L2BlockRef{
		Hash:       common.Hash(block.Hash()),
		Number:     block.NumberU64(),
		ParentHash: common.Hash(block.ParentHash()),
		Time:       block.Time(),

		// unlike op using the deposit tx to save l1 block info in every block,
		// for these two fields we need to retrieve them from the DTL,
		// but since we are in the oracle engine, we don't have access to the DTL,
		// need to use hint to get the l1 block info from the host
		L1Origin:       eth.BlockID{},
		SequenceNumber: 0,
	}
}
