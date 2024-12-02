package engineapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/ethereum-optimism/optimism/op-service/eth"
	l2common "github.com/ethereum/go-ethereum/common"

	"github.com/ethereum/go-ethereum/log"

	"github.com/MetisProtocol/mvm/l2geth/beacon/engine"
	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/consensus"
	"github.com/MetisProtocol/mvm/l2geth/core/state"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/core/vm"
	"github.com/MetisProtocol/mvm/l2geth/eth/downloader"
	"github.com/MetisProtocol/mvm/l2geth/params"
	"github.com/MetisProtocol/mvm/l2geth/rlp"
)

type EngineBackend interface {
	CurrentSafeBlock() *types.Header
	CurrentFinalBlock() *types.Header
	GetBlockByHash(hash common.Hash) *types.Block
	GetBlock(hash common.Hash, number uint64) *types.Block
	HasBlockAndState(hash common.Hash, number uint64) bool
	GetCanonicalHash(n uint64) common.Hash

	GetVMConfig() *vm.Config
	Config() *params.ChainConfig
	// Engine retrieves the chain's consensus engine.
	Engine() consensus.Engine

	StateAt(root common.Hash) (*state.StateDB, error)

	InsertBlockWithoutSetHead(block *types.Block) error
	SetCanonical(head *types.Block) (common.Hash, error)
	SetFinalized(header *types.Header)
	SetSafe(header *types.Header)

	consensus.ChainHeaderReader
}

// L2EngineAPI wraps an engine actor, and implements the RPC backend required to serve the engine API.
// This re-implements some of the Geth API work, but changes the API backend so we can deterministically
// build and control the L2 block contents to reach very specific edge cases as desired for testing.
type L2EngineAPI struct {
	log     log.Logger
	backend EngineBackend

	// Functionality for snap sync
	remotes    map[common.Hash]*types.Block
	downloader *downloader.Downloader

	// L2 block building data
	blockProcessor *BlockProcessor
	pendingIndices map[common.Address]uint64 // per account, how many txs from the pool were already included in the block, since the pool is lagging behind block mining.
	l2ForceEmpty   bool                      // when no additional txs may be processed (i.e. when sequencer drift runs out)
	l2TxFailed     []*types.Transaction      // log of failed transactions which could not be included

	payloadID engine.PayloadID // ID of payload that is currently being built
}

func NewL2EngineAPI(log log.Logger, backend EngineBackend, downloader *downloader.Downloader) *L2EngineAPI {
	return &L2EngineAPI{
		log:        log,
		backend:    backend,
		remotes:    make(map[common.Hash]*types.Block),
		downloader: downloader,
	}
}

var (
	STATUS_INVALID = &eth.ForkchoiceUpdatedResult{PayloadStatus: eth.PayloadStatusV1{Status: eth.ExecutionInvalid}, PayloadID: nil}
	STATUS_SYNCING = &eth.ForkchoiceUpdatedResult{PayloadStatus: eth.PayloadStatusV1{Status: eth.ExecutionSyncing}, PayloadID: nil}
)

// computePayloadId computes a pseudo-random payloadid, based on the parameters.
func computePayloadId(headBlockHash common.Hash, attrs *eth.PayloadAttributes) engine.PayloadID {
	// Hash
	hasher := sha256.New()
	hasher.Write(headBlockHash[:])
	_ = binary.Write(hasher, binary.BigEndian, attrs.Timestamp)
	hasher.Write(attrs.PrevRandao[:])
	hasher.Write(attrs.SuggestedFeeRecipient[:])
	_ = binary.Write(hasher, binary.BigEndian, attrs.NoTxPool)
	_ = binary.Write(hasher, binary.BigEndian, uint64(len(attrs.Transactions)))
	for _, tx := range attrs.Transactions {
		_ = binary.Write(hasher, binary.BigEndian, uint64(len(tx))) // length-prefix to avoid collisions
		hasher.Write(tx)
	}
	_ = binary.Write(hasher, binary.BigEndian, *attrs.GasLimit)
	var out engine.PayloadID
	copy(out[:], hasher.Sum(nil)[:8])
	return out
}

func (ea *L2EngineAPI) RemainingBlockGas() uint64 {
	if ea.blockProcessor == nil {
		return 0
	}
	return ea.blockProcessor.gasPool.Gas()
}

func (ea *L2EngineAPI) ForcedEmpty() bool {
	return ea.l2ForceEmpty
}

func (ea *L2EngineAPI) PendingIndices(from common.Address) uint64 {
	return ea.pendingIndices[from]
}

var ErrNotBuildingBlock = errors.New("not currently building a block, cannot include tx from queue")

func (ea *L2EngineAPI) IncludeTx(tx *types.Transaction, from common.Address) error {
	if ea.blockProcessor == nil {
		return ErrNotBuildingBlock
	}
	if ea.l2ForceEmpty {
		ea.log.Info("Skipping including a transaction because e.L2ForceEmpty is true")
		// t.InvalidAction("cannot include any sequencer txs")
		return nil
	}

	err := ea.blockProcessor.CheckTxWithinGasLimit(tx)
	if err != nil {
		return err
	}

	ea.pendingIndices[from] = ea.pendingIndices[from] + 1 // won't retry the tx
	err = ea.blockProcessor.AddTx(tx)
	if err != nil {
		ea.l2TxFailed = append(ea.l2TxFailed, tx)
		return fmt.Errorf("invalid L2 block (tx %d): %w", len(ea.blockProcessor.transactions), err)
	}
	return nil
}

func (ea *L2EngineAPI) startBlock(parent common.Hash, attrs *eth.PayloadAttributes) error {
	if ea.blockProcessor != nil {
		ea.log.Warn("started building new block without ending previous block", "previous", ea.blockProcessor.header, "prev_payload_id", ea.payloadID)
	}

	processor, err := NewBlockProcessorFromPayloadAttributes(ea.backend, parent, attrs)
	if err != nil {
		return err
	}
	ea.blockProcessor = processor
	ea.pendingIndices = make(map[common.Address]uint64)
	ea.l2ForceEmpty = attrs.NoTxPool
	ea.payloadID = computePayloadId(parent, attrs)

	for i, otx := range attrs.Transactions {
		var tx types.Transaction
		if err := rlp.DecodeBytes(otx, &tx); err != nil {
			return fmt.Errorf("transaction %d is not valid: %w", i, err)
		}
		err := ea.blockProcessor.AddTx(&tx)
		if err != nil {
			ea.l2TxFailed = append(ea.l2TxFailed, &tx)
			return fmt.Errorf("failed to apply deposit transaction to L2 block (tx %d): %w", i, err)
		}
	}
	return nil
}

func (ea *L2EngineAPI) endBlock() (*types.Block, error) {
	if ea.blockProcessor == nil {
		return nil, fmt.Errorf("no block is being built currently (id %s)", ea.payloadID)
	}
	processor := ea.blockProcessor
	ea.blockProcessor = nil

	block, err := processor.Assemble()
	if err != nil {
		return nil, fmt.Errorf("assemble block: %w", err)
	}
	return block, nil
}

func (ea *L2EngineAPI) GetPayloadV1(ctx context.Context, payloadId eth.PayloadID) (*eth.ExecutionPayload, error) {
	res, err := ea.getPayload(ctx, payloadId)
	if err != nil {
		return nil, err
	}
	return res.ExecutionPayload, nil
}

func (ea *L2EngineAPI) GetPayloadV2(ctx context.Context, payloadId eth.PayloadID) (*eth.ExecutionPayloadEnvelope, error) {
	return ea.getPayload(ctx, payloadId)
}

func (ea *L2EngineAPI) GetPayloadV3(ctx context.Context, payloadId eth.PayloadID) (*eth.ExecutionPayloadEnvelope, error) {
	return ea.getPayload(ctx, payloadId)
}

func (ea *L2EngineAPI) config() *params.ChainConfig {
	return ea.backend.Config()
}

func (ea *L2EngineAPI) ForkchoiceUpdated(ctx context.Context, state *eth.ForkchoiceState, attr *eth.PayloadAttributes) (*eth.ForkchoiceUpdatedResult, error) {
	return ea.forkchoiceUpdated(ctx, state, attr)
}

// Ported from: https://github.com/ethereum-optimism/op-geth/blob/c50337a60a1309a0f1dca3bf33ed1bb38c46cdd7/eth/catalyst/api.go#L486C1-L507
func (ea *L2EngineAPI) NewPayload(ctx context.Context, params *eth.ExecutionPayload, versionedHashes []common.Hash, beaconRoot *common.Hash) (*eth.PayloadStatusV1, error) {
	// sigh, we don't have any of these
	if params.ExcessBlobGas != nil || params.BlobGasUsed != nil || versionedHashes != nil || beaconRoot != nil {
		return &eth.PayloadStatusV1{Status: eth.ExecutionInvalid}, engine.InvalidParams.With(errors.New("unexpected non-nil params in l2geth"))
	}

	return ea.newPayload(ctx, params, versionedHashes, beaconRoot)
}

func (ea *L2EngineAPI) getPayload(_ context.Context, payloadId eth.PayloadID) (*eth.ExecutionPayloadEnvelope, error) {
	ea.log.Trace("L2Engine API request received", "method", "GetPayload", "id", payloadId)
	if eth.PayloadID(ea.payloadID) != payloadId {
		ea.log.Warn("unexpected payload ID requested for block building", "expected", ea.payloadID, "got", payloadId)
		return nil, engine.UnknownPayload
	}
	bl, err := ea.endBlock()
	if err != nil {
		ea.log.Error("failed to finish block building", "err", err)
		return nil, engine.UnknownPayload
	}

	payload, err := BlockAsPayload(bl)
	if err != nil {
		return nil, err
	}
	return &eth.ExecutionPayloadEnvelope{
		ExecutionPayload: payload,
	}, nil
}

func (ea *L2EngineAPI) forkchoiceUpdated(_ context.Context, state *eth.ForkchoiceState, attr *eth.PayloadAttributes) (*eth.ForkchoiceUpdatedResult, error) {
	ea.log.Trace("L2Engine API request received", "method", "ForkchoiceUpdated", "head", state.HeadBlockHash, "finalized", state.FinalizedBlockHash, "safe", state.SafeBlockHash)
	if state.HeadBlockHash == (l2common.Hash{}) {
		ea.log.Warn("Forkchoice requested update to zero hash")
		return STATUS_INVALID, nil
	}
	// Check whether we have the block yet in our database or not. If not, we'll
	// need to either trigger a sync, or to reject this forkchoice update for a
	// reason.
	block := ea.backend.GetBlockByHash(common.Hash(state.HeadBlockHash))
	if block == nil {
		// unlike op, we don't have a syncer to handle this case, something is wrong
		panic("missing block from preimage oracle")
	}

	valid := func(id *engine.PayloadID) *eth.ForkchoiceUpdatedResult {
		return &eth.ForkchoiceUpdatedResult{
			PayloadStatus: eth.PayloadStatusV1{Status: eth.ExecutionValid, LatestValidHash: &state.HeadBlockHash},
			PayloadID:     (*eth.PayloadID)(id),
		}
	}
	if ea.backend.GetCanonicalHash(block.NumberU64()) != common.Hash(state.HeadBlockHash) {
		// Block is not canonical, set head.
		if latestValid, err := ea.backend.SetCanonical(block); err != nil {
			return &eth.ForkchoiceUpdatedResult{PayloadStatus: eth.PayloadStatusV1{Status: eth.ExecutionInvalid, LatestValidHash: (*l2common.Hash)(&latestValid)}}, err
		}
	} else if ea.backend.CurrentHeader().Hash() == common.Hash(state.HeadBlockHash) {
		// If the specified head matches with our local head, do nothing and keep
		// generating the payload. It's a special corner case that a few slots are
		// missing and we are requested to generate the payload in slot.
	}

	// If the beacon client also advertised a finalized block, mark the local
	// chain final and completely in PoS mode.
	if state.FinalizedBlockHash != (l2common.Hash{}) {
		// If the finalized block is not in our canonical tree, somethings wrong
		finalHeader := ea.backend.GetHeaderByHash(common.Hash(state.FinalizedBlockHash))
		if finalHeader == nil {
			ea.log.Warn("Final block not available in database", "hash", state.FinalizedBlockHash)
			return STATUS_INVALID, engine.InvalidForkChoiceState.With(errors.New("final block not available in database"))
		} else if ea.backend.GetCanonicalHash(finalHeader.Number.Uint64()) != common.Hash(state.FinalizedBlockHash) {
			ea.log.Warn("Final block not in canonical chain", "number", block.NumberU64(), "hash", state.HeadBlockHash)
			return STATUS_INVALID, engine.InvalidForkChoiceState.With(errors.New("final block not in canonical chain"))
		}
		// Set the finalized block
		ea.backend.SetFinalized(finalHeader)
	}
	// Check if the safe block hash is in our canonical tree, if not somethings wrong
	if state.SafeBlockHash != (l2common.Hash{}) {
		safeHeader := ea.backend.GetHeaderByHash(common.Hash(state.SafeBlockHash))
		if safeHeader == nil {
			ea.log.Warn("Safe block not available in database")
			return STATUS_INVALID, engine.InvalidForkChoiceState.With(errors.New("safe block not available in database"))
		}
		if ea.backend.GetCanonicalHash(safeHeader.Number.Uint64()) != common.Hash(state.SafeBlockHash) {
			ea.log.Warn("Safe block not in canonical chain")
			return STATUS_INVALID, engine.InvalidForkChoiceState.With(errors.New("safe block not in canonical chain"))
		}
		// Set the safe block
		ea.backend.SetSafe(safeHeader)
	}
	// If payload generation was requested, create a new block to be potentially
	// sealed by the beacon client. The payload will be requested later, and we
	// might replace it arbitrarily many times in between.
	if attr != nil {
		err := ea.startBlock(common.Hash(state.HeadBlockHash), attr)
		if err != nil {
			ea.log.Error("Failed to start block building", "err", err, "noTxPool", attr.NoTxPool, "txs", len(attr.Transactions), "timestamp", attr.Timestamp)
			return STATUS_INVALID, engine.InvalidPayloadAttributes.With(err)
		}

		return valid(&ea.payloadID), nil
	}
	return valid(nil), nil
}

func toGethWithdrawals(payload *eth.ExecutionPayload) []*types.Withdrawal {
	if payload.Withdrawals == nil {
		return nil
	}

	result := make([]*types.Withdrawal, 0, len(*payload.Withdrawals))

	for _, w := range *payload.Withdrawals {
		result = append(result, &types.Withdrawal{
			Index:     w.Index,
			Validator: w.Validator,
			Address:   common.Address(w.Address),
			Amount:    w.Amount,
		})
	}

	return result
}

func (ea *L2EngineAPI) newPayload(_ context.Context, payload *eth.ExecutionPayload, hashes []common.Hash, root *common.Hash) (*eth.PayloadStatusV1, error) {
	ea.log.Trace("L2Engine API request received", "method", "ExecutePayload", "number", payload.BlockNumber, "hash", payload.BlockHash)
	txs := make([][]byte, len(payload.Transactions))
	for i, tx := range payload.Transactions {
		txs[i] = tx
	}
	block, err := engine.ExecutableDataToBlock(engine.ExecutableData{
		ParentHash:   common.Hash(payload.ParentHash),
		StateRoot:    common.Hash(payload.StateRoot),
		ReceiptsRoot: common.Hash(payload.ReceiptsRoot),
		LogsBloom:    payload.LogsBloom[:],
		Number:       uint64(payload.BlockNumber),
		GasLimit:     uint64(payload.GasLimit),
		GasUsed:      uint64(payload.GasUsed),
		Timestamp:    uint64(payload.Timestamp),
		ExtraData:    payload.ExtraData,
		BlockHash:    common.Hash(payload.BlockHash),
		Transactions: txs,
	}, hashes, root)
	if err != nil {
		log.Debug("Invalid NewPayload params", "params", payload, "error", err)
		return &eth.PayloadStatusV1{Status: eth.ExecutionInvalidBlockHash}, nil
	}
	// If we already have the block locally, ignore the entire execution and just
	// return a fake success.
	if block := ea.backend.GetBlock(common.Hash(payload.BlockHash), uint64(payload.BlockNumber)); block != nil {
		ea.log.Warn("Ignoring already known beacon payload", "number", payload.BlockNumber, "hash", payload.BlockHash, "age", common.PrettyAge(time.Unix(int64(block.Time()), 0)))
		hash := l2common.Hash(block.Hash())
		return &eth.PayloadStatusV1{Status: eth.ExecutionValid, LatestValidHash: &hash}, nil
	}

	// TODO: skipping invalid ancestor check (i.e. not remembering previously failed blocks)

	parent := ea.backend.GetBlock(block.ParentHash(), block.NumberU64()-1)
	if parent == nil {
		ea.remotes[block.Hash()] = block
		// TODO: hack, saying we accepted if we don't know the parent block. Might want to return critical error if we can't actually sync.
		return &eth.PayloadStatusV1{Status: eth.ExecutionAccepted, LatestValidHash: nil}, nil
	}

	if block.Time() <= parent.Time() {
		log.Warn("Invalid timestamp", "parent", block.Time(), "block", block.Time())
		return ea.invalid(errors.New("invalid timestamp"), parent.Header()), nil
	}

	if !ea.backend.HasBlockAndState(block.ParentHash(), block.NumberU64()-1) {
		ea.log.Warn("State not available, ignoring new payload")
		return &eth.PayloadStatusV1{Status: eth.ExecutionAccepted}, nil
	}
	log.Trace("Inserting block without sethead", "hash", block.Hash(), "number", block.Number)
	if err := ea.backend.InsertBlockWithoutSetHead(block); err != nil {
		ea.log.Warn("NewPayloadV1: inserting block failed", "error", err)
		// TODO not remembering the payload as invalid
		return ea.invalid(err, parent.Header()), nil
	}
	hash := l2common.Hash(block.Hash())
	return &eth.PayloadStatusV1{Status: eth.ExecutionValid, LatestValidHash: &hash}, nil
}

func (ea *L2EngineAPI) invalid(err error, latestValid *types.Header) *eth.PayloadStatusV1 {
	currentHash := l2common.Hash(ea.backend.CurrentHeader().Hash())
	if latestValid != nil {
		// Set latest valid hash to 0x0 if parent is PoW block
		currentHash = l2common.Hash{}
		if latestValid.Difficulty.BitLen() == 0 {
			// Otherwise set latest valid hash to parent hash
			currentHash = l2common.Hash(latestValid.Hash())
		}
	}
	errorMsg := err.Error()
	return &eth.PayloadStatusV1{Status: eth.ExecutionInvalid, LatestValidHash: &currentHash, ValidationError: &errorMsg}
}

func BlockAsPayload(bl *types.Block) (*eth.ExecutionPayload, error) {
	opaqueTxs := make([]eth.Data, len(bl.Transactions()))
	for i, tx := range bl.Transactions() {
		buffer := bytes.NewBuffer(nil)
		if err := tx.EncodeRLP(buffer); err != nil {
			return nil, fmt.Errorf("tx %d failed to marshal: %w", i, err)
		}
		opaqueTxs[i] = buffer.Bytes()
	}

	payload := &eth.ExecutionPayload{
		ParentHash:   l2common.Hash(bl.ParentHash()),
		FeeRecipient: l2common.Address(bl.Coinbase()),
		StateRoot:    eth.Bytes32(bl.Root()),
		ReceiptsRoot: eth.Bytes32(bl.ReceiptHash()),
		LogsBloom:    eth.Bytes256(bl.Bloom()),
		PrevRandao:   eth.Bytes32(bl.MixDigest()),
		BlockNumber:  eth.Uint64Quantity(bl.NumberU64()),
		GasLimit:     eth.Uint64Quantity(bl.GasLimit()),
		GasUsed:      eth.Uint64Quantity(bl.GasUsed()),
		Timestamp:    eth.Uint64Quantity(bl.Time()),
		ExtraData:    bl.Extra(),
		BlockHash:    l2common.Hash(bl.Hash()),
		Transactions: opaqueTxs,
	}

	return payload, nil
}
