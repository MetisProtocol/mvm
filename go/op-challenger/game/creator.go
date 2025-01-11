package game

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ethereum-optimism/optimism/op-service/sources/batching"
	"github.com/ethereum-optimism/optimism/op-service/sources/batching/rpcblock"
	"github.com/ethereum-optimism/optimism/op-service/txmgr"
	ethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	l1client "github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/syndtr/goleveldb/leveldb"

	l2client "github.com/MetisProtocol/mvm/l2geth/ethclient"
	"github.com/MetisProtocol/mvm/l2geth/rollup"
	"github.com/ethereum-optimism/optimism/go/op-challenger/abi"
	"github.com/ethereum-optimism/optimism/go/op-challenger/config"
	"github.com/ethereum-optimism/optimism/go/op-challenger/game/fault/contracts"
	"github.com/ethereum-optimism/optimism/go/op-challenger/game/fault/types"
	merkletrie "github.com/ethereum-optimism/optimism/go/op-program/client/merkel"
)

const (
	HighestSyncedGameFactoryL1Block = "highest_factory_l1_block"
	HighestSyncedSCCL1Block         = "highest_scc_l1_block"
)

const (
	maxBatchSize = 2000
)

type DisputeGameRequest struct {
	Requestor common.Address
	GameType  uint32
	Bond      *big.Int
	ExtraData []byte
}

type StateBatchAppended struct {
	ChainID           *big.Int
	BatchIndex        *big.Int
	BatchRoot         common.Hash
	BatchSize         *big.Int
	PrevTotalElements *big.Int
	ExtraData         []byte
}

type EventBlocks struct {
	Header   *ethtypes.Header
	Receipts []*ethtypes.Receipt
}

type gameCreator struct {
	ctx    context.Context
	cancel context.CancelFunc

	cfg *config.Config

	logger          log.Logger
	factoryContract *contracts.DisputeGameFactoryContract
	sccContract     *batching.BoundContract

	l1Source  *l1client.Client
	l2Source  *l2client.Client
	dtlSource rollup.RollupClient
	txMgr     txmgr.TxManager

	multiCaller *batching.MultiCaller

	sccABI *ethabi.ABI

	isCreator bool
	db        *leveldb.DB
}

func newCreator(logger log.Logger,
	multiCaller *batching.MultiCaller,
	factoryContract *contracts.DisputeGameFactoryContract,
	l1Source *l1client.Client,
	dtlSource rollup.RollupClient,
	txMgr txmgr.TxManager,
	cfg *config.Config) *gameCreator {
	var (
		l2Source *l2client.Client
		err      error
	)

	ctx, cancel := context.WithCancel(context.Background())

	if !cfg.GameCreatorMode {
		// only disputer needs l2 client
		if l2Source, err = l2client.DialContext(ctx, cfg.L2Rpc); err != nil {
			logger.Crit("failed to dial L2 client", "err", err)
			return nil
		}
	}

	datadir := path.Join(cfg.Datadir, "game-creator")
	// check data dir existence
	if _, err := os.Stat(datadir); os.IsNotExist(err) {
		if err := os.MkdirAll(datadir, os.ModePerm); err != nil {
			logger.Crit("failed to create game creator data dir", "err", err)
			return nil
		}
	}

	db, err := leveldb.OpenFile(datadir, nil)
	if err != nil {
		logger.Crit("failed to open db", "err", err)
		return nil
	}

	sccABI := abi.LoadSCCABI()

	return &gameCreator{
		ctx:             ctx,
		cancel:          cancel,
		multiCaller:     multiCaller,
		logger:          logger.With("module", "game-creator"),
		factoryContract: factoryContract,
		sccContract:     batching.NewBoundContract(sccABI, cfg.SCCAddress),
		l1Source:        l1Source,
		l2Source:        l2Source,
		dtlSource:       dtlSource,
		isCreator:       cfg.GameCreatorMode,
		txMgr:           txMgr,
		db:              db,
		cfg:             cfg,
		sccABI:          sccABI,
	}
}

// blockProcessor is a function type that processes a single block and its events
type blockProcessor func(*EventBlocks) error

// monitorEvents is the main event monitoring loop that periodically checks for new events
// eventID: the event signature to monitor
// processor: the function to process each block containing the event
func (m *gameCreator) monitorEvents(eventID []byte, processor blockProcessor) {
	ticker := time.NewTicker(m.cfg.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			m.logger.Info("monitoring exited")
			return
		case <-ticker.C:
			if err := m.processNewBlocks(eventID, processor); err != nil {
				m.logger.Error("failed to process blocks", "err", err)
			}
		}
	}
}

// processNewBlocks handles the synchronization of new blocks from L1
// It fetches blocks from the last synced block to the current highest block
func (m *gameCreator) processNewBlocks(eventID []byte, processor blockProcessor) error {
	highestSynced, err := m.getHighestSyncedL1()
	if err != nil {
		m.logger.Error("failed to get highest synced block", "err", err)
		return err
	}

	nextToSync := highestSynced + 1
	if highestSynced == 0 {
		nextToSync = m.cfg.L1StartBlock
	}

	currentHighest, err := m.l1Source.BlockNumber(m.ctx)
	if err != nil {
		m.logger.Error("failed to get current highest block", "err", err)
		return err
	}

	if nextToSync >= currentHighest {
		m.logger.Debug("no new blocks to sync")
		return nil
	}

	target := currentHighest
	if target-nextToSync > maxBatchSize {
		target = nextToSync + maxBatchSize
	}

	m.logger.Info("syncing blocks", "from", nextToSync, "to", target)

	blocks, err := m.fetchBlocksInRange(nextToSync, target, eventID, "l1")
	if err != nil {
		m.logger.Error("failed to fetch blocks", "err", err)
		return err
	}

	for _, block := range blocks {
		if err := processor(block); err != nil {
			m.logger.Error("failed to process block", "err", err)
			return err
		}

		if err := m.setHighestSyncedL1(block.Header.Number.Uint64()); err != nil {
			m.logger.Error("failed to set highest synced block", "err", err)
			return err
		}

		m.logger.Debug("synced block", "block", block.Header.Number.Uint64())
	}

	m.logger.Info("synced blocks", "from", nextToSync, "to", target)

	return nil
}

// sendErrorOrDiscard attempts to send an error to the error channel
// If the channel is full, the error is discarded
func sendErrorOrDiscard(errCh chan<- error, err error) {
	select {
	case errCh <- err:
	default:
	}
}

// checkErrorChannel checks if there's any error in the error channel
// Returns the first error found or nil if the channel is empty
func checkErrorChannel(errCh <-chan error) error {
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

// fetchResult represents the result of a fetch operation
type fetchResult struct {
	blockNum uint64
	data     interface{}
}

// fetchFunc is a function type that performs the actual fetch operation
type fetchFunc func(context.Context, uint64) (interface{}, error)

// fetchInRange performs parallel fetching of data within a specified range
// start: starting block number
// end: ending block number
// fetch: the function to fetch data for each block
func (m *gameCreator) fetchInRange(start, end uint64, fetch fetchFunc, src string) ([]interface{}, error) {
	count := end - start + 1
	results := make(chan *fetchResult, count)
	errs := make(chan error, count)

	// Create a semaphore to limit concurrent goroutines
	sem := make(chan struct{}, m.cfg.MaxConcurrency)
	var wg sync.WaitGroup

	// Launch fetchers
	for blockNum := start; blockNum <= end; blockNum++ {
		wg.Add(1)
		go func(num uint64) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			data, err := fetch(m.ctx, num)
			if err != nil {
				sendErrorOrDiscard(errs, fmt.Errorf("failed to fetch data for block %d: %w", num, err))
				return
			}

			select {
			case results <- &fetchResult{blockNum: num, data: data}:
				m.logger.Debug("fetched block", "src", src, "block", num)
			case <-m.ctx.Done():
				sendErrorOrDiscard(errs, m.ctx.Err())
			}
		}(blockNum)
	}

	// Wait for all fetchers to complete
	wg.Wait()

	// Check for errors
	if err := checkErrorChannel(errs); err != nil {
		return nil, err
	}

	// Collect all results
	fetchedResults := make([]*fetchResult, 0, count)
	for i := uint64(0); i < count; i++ {
		select {
		case result := <-results:
			fetchedResults = append(fetchedResults, result)
		default:
			return nil, fmt.Errorf("missing data, expected %d items but got %d", count, len(fetchedResults))
		}
	}

	// Sort results by block number
	slices.SortFunc(fetchedResults, func(i, j *fetchResult) int {
		return new(big.Int).SetUint64(i.blockNum).Cmp(new(big.Int).SetUint64(j.blockNum))
	})

	// Extract sorted data
	sortedData := make([]interface{}, len(fetchedResults))
	for i, result := range fetchedResults {
		sortedData[i] = result.data
	}

	return sortedData, nil
}

// fetchBlocksInRange fetches blocks in parallel within the specified range
func (m *gameCreator) fetchBlocksInRange(start, end uint64, eventID []byte, src string) ([]*EventBlocks, error) {
	fetch := func(ctx context.Context, num uint64) (interface{}, error) {
		header, err := m.l1Source.HeaderByNumber(ctx, new(big.Int).SetUint64(num))
		if err != nil {
			return nil, fmt.Errorf("failed to fetch block header: %w", err)
		}

		var receipts []*ethtypes.Receipt
		if header.Bloom.Test(eventID) {
			receipts, err = m.l1Source.BlockReceipts(ctx, rpc.BlockNumberOrHashWithNumber(rpc.BlockNumber(num)))
			if err != nil {
				return nil, fmt.Errorf("failed to fetch receipts: %w", err)
			}
		}

		return &EventBlocks{Header: header, Receipts: receipts}, nil
	}

	results, err := m.fetchInRange(start, end, fetch, src)
	if err != nil {
		return nil, err
	}

	blocks := make([]*EventBlocks, len(results))
	for i, result := range results {
		blocks[i] = result.(*EventBlocks)
	}

	return blocks, nil
}

// fetchL2HeadersInRange fetches L2 block headers in parallel within the specified range
func (m *gameCreator) fetchL2HeadersInRange(start, end uint64) ([]hexutil.Bytes, error) {
	fetch := func(ctx context.Context, num uint64) (interface{}, error) {
		header, err := m.l2Source.HeaderByNumber(ctx, new(big.Int).SetUint64(num))
		if err != nil {
			return nil, fmt.Errorf("failed to fetch L2 header: %w", err)
		}
		return header.Root.Bytes(), nil
	}

	results, err := m.fetchInRange(start, end, fetch, "l2")
	if err != nil {
		return nil, err
	}

	stateRoots := make([]hexutil.Bytes, len(results))
	for i, result := range results {
		stateRoots[i] = result.([]byte)
	}

	return stateRoots, nil
}

// minitorStateDisputed monitors the DisputeGameRequested events
func (m *gameCreator) minitorStateDisputed() {
	eventID := m.sccABI.Events["DisputeGameRequested"].ID.Bytes()
	m.monitorEvents(eventID, m.processDisputeGameRequest)
}

// processDisputeGameRequest processes a block for dispute game requests
func (m *gameCreator) processDisputeGameRequest(block *EventBlocks) error {
	if block.Receipts == nil {
		m.logger.Debug("no events in block", "block", block.Header.Number.Uint64())
		return nil
	}

	for _, receipt := range block.Receipts {
		event := m.decodeStateDisputedEvent(receipt)
		if event == nil {
			continue
		}

		m.logger.Info("dispute game request found", "event", event.Requestor, "gameType", event.GameType,
			"bond", event.Bond.Uint64(), "extraData", hexutil.Encode(event.ExtraData))

		if err := m.handleDisputeGameRequest(event, block.Header.Time); err != nil {
			return err
		}
	}
	return nil
}

func (m *gameCreator) handleDisputeGameRequest(event *DisputeGameRequest, timestamp uint64) error {
	// check if the request has expired
	res, err := m.multiCaller.SingleCall(m.ctx, rpcblock.Latest, m.sccContract.Call("FRAUD_PROOF_WINDOW"))
	if err != nil {
		return err
	}

	fraudProofWindow := res.GetBigInt(0).Uint64()
	requestUUID, err := abi.EncodePacked(event.GameType, event.ExtraData)
	if err != nil {
		return err
	}

	if uint64(time.Now().UTC().Unix())-fraudProofWindow > timestamp {
		m.logger.Info("dispute game request has expired", "request", crypto.Keccak256Hash(requestUUID).Hex())
		return nil
	}

	// check if request has already been processed
	if _, err := m.factoryContract.GetDisputeGameRequest(m.ctx, types.GameType(event.GameType), event.ExtraData); err != nil {
		if strings.Contains(err.Error(), "dispute game request not found") {
			m.logger.Warn("dispute game request not exist, probably already processed")
			return nil
		}
		return err
	}

	// check if same game has already been created
	l2BlockNumber := new(big.Int).SetBytes(event.ExtraData).Uint64()
	resp, err := m.dtlSource.GetRawStateRoot(l2BlockNumber - 1)
	if err != nil {
		return err
	}

	batchHeader := &rollup.BatchHeader{
		BatchRoot:         resp.Batch.Root,
		BatchSize:         new(big.Int).SetUint64(uint64(resp.Batch.Size)),
		PrevTotalElements: new(big.Int).SetUint64(uint64(resp.Batch.PrevTotalElements)),
		ExtraData:         rollup.ExtraData(resp.Batch.ExtraData),
	}
	batchHeaderHash := common.Hash(batchHeader.Hash())

	gameProxy, err := m.factoryContract.GetGameFromParameters(m.ctx, event.GameType, batchHeaderHash, l2BlockNumber)
	if err != nil {
		return err
	}
	if gameProxy != (common.Address{}) {
		m.logger.Info("game with the same claim has already been created",
			"game", gameProxy.Hex(), "type", types.GameType(event.GameType).String(), "request", crypto.Keccak256Hash(requestUUID).Hex())
		return nil
	}

	// create game
	txCandidate, err := m.factoryContract.CreateTx(m.ctx, event.GameType, batchHeaderHash, l2BlockNumber)
	if err != nil {
		return err
	}

	receipt, err := m.txMgr.Send(m.ctx, txCandidate)
	if err != nil {
		return err
	}

	if receipt.Status != ethtypes.ReceiptStatusSuccessful {
		return errors.New("game creation transaction reverted")
	}

	gameProxy, _, _, err = m.factoryContract.DecodeDisputeGameCreatedLog(receipt)
	m.logger.Info("game created", "game", gameProxy.Hex(), "type", types.GameType(event.GameType).String())
	return err
}

func (m *gameCreator) monitorStateValidity() {
	eventID := m.sccABI.Events["StateBatchAppended"].ID.Bytes()
	m.monitorEvents(eventID, m.processStateBatch)
}

func (m *gameCreator) processStateBatch(block *EventBlocks) error {
	if block.Receipts == nil {
		m.logger.Debug("no events in block", "block", block.Header.Number.Uint64())
		return nil
	}

	for i, receipt := range block.Receipts {
		event := m.decodeStateBatchAppendedEvent(receipt)
		if event == nil {
			continue
		}

		m.logger.Info("state batch found", "block", block.Header.Number.Uint64(), "tx", i)
		if err := m.handleStateBatch(event, block.Header.Number.Uint64()); err != nil {
			return err
		}
	}
	return nil
}

// handleStateBatch processes a state batch event and verifies its validity
// If the batch root doesn't match our local state, it creates a dispute game
func (m *gameCreator) handleStateBatch(event *StateBatchAppended, blockNumber uint64) error {
	startBlock := event.PrevTotalElements.Uint64() + 1
	endBlock := startBlock + event.BatchSize.Uint64() - 1
	m.logger.Debug("verifying state batch", "start", startBlock, "end", endBlock, "index", event.BatchIndex)
	stateRoots, err := m.fetchL2HeadersInRange(startBlock, endBlock)
	if err != nil {
		m.logger.Error("failed to fetch L2 headers", "err", err)
		return fmt.Errorf("failed to fetch L2 headers: %w", err)
	}

	root, _ := merkletrie.WriteTrie(stateRoots)
	if root != event.BatchRoot {
		m.logger.Info("invalid state batch found, creating game request", "block", blockNumber,
			"local", hexutil.Encode(root[:]), "remote", hexutil.Encode(event.BatchRoot[:]))
		return m.createDisputeGameRequest(event)
	}

	m.logger.Info("valid state batch", "start", event.PrevTotalElements.Uint64(),
		"end", event.PrevTotalElements.Uint64()+event.BatchSize.Uint64(),
		"index", event.BatchIndex,
		"root", hexutil.Encode(root[:]))
	return nil
}

// createDisputeGameRequest creates a new dispute game request for a mismatched state batch
// It generates a unique identifier for the game and submits the transaction
func (m *gameCreator) createDisputeGameRequest(event *StateBatchAppended) error {
	l2Block := rollup.ExtraData(event.ExtraData).L2BlockNumber()
	gameType := uint32(m.cfg.GameCreationTraceType.GameType())

	txCandidate, err := m.factoryContract.CreateDisputeTx(m.ctx, gameType, l2Block.Uint64())
	if err != nil {
		return fmt.Errorf("failed to create dispute transaction: %w", err)
	}

	receipt, err := m.txMgr.Send(m.ctx, txCandidate)
	if err != nil {
		return fmt.Errorf("failed to send dispute transaction: %w", err)
	}

	if receipt.Status != ethtypes.ReceiptStatusSuccessful {
		return errors.New("dispute transaction reverted")
	}

	requestUUID, err := abi.EncodePacked(gameType, common.BigToHash(l2Block).Bytes())
	if err != nil {
		return fmt.Errorf("failed to pack request UUID: %w", err)
	}

	m.logger.Info("dispute game requested", "tx", receipt.TxHash.Hex(),
		"request", crypto.Keccak256Hash(requestUUID).Hex(),
		"game_type", types.GameType(gameType).String(),
		"l2_block", l2Block.Uint64())
	return nil
}

func (m *gameCreator) getHighestSyncedL1() (uint64, error) {
	if m.isCreator {
		return m.getNumber(HighestSyncedGameFactoryL1Block)
	}

	return m.getNumber(HighestSyncedSCCL1Block)
}

func (m *gameCreator) setHighestSyncedL1(number uint64) error {
	if m.isCreator {
		return m.setNumber(HighestSyncedGameFactoryL1Block, number)
	}

	return m.setNumber(HighestSyncedSCCL1Block, number)
}

func (m *gameCreator) setNumber(key string, number uint64) error {
	return m.db.Put([]byte(key), new(big.Int).SetUint64(number).Bytes(), nil)
}

func (m *gameCreator) getNumber(key string) (uint64, error) {
	raw, err := m.db.Get([]byte(key), nil)
	if err != nil {
		if errors.Is(err, leveldb.ErrNotFound) {
			return 0, nil
		}

		return 0, err
	}

	return new(big.Int).SetBytes(raw).Uint64(), nil
}

func (m *gameCreator) decodeStateDisputedEvent(receipt *ethtypes.Receipt) *DisputeGameRequest {
	sender, gameType, bond, extraData, err := m.factoryContract.DecodeDisputeGameRequestedLog(receipt)
	if err != nil {
		return nil
	}

	return &DisputeGameRequest{
		Requestor: sender,
		GameType:  gameType,
		Bond:      bond,
		ExtraData: extraData,
	}
}

func (m *gameCreator) Mode() string {
	if m.isCreator {
		return "creator"
	}

	return "verifier"
}

func (m *gameCreator) decodeStateBatchAppendedEvent(receipt *ethtypes.Receipt) *StateBatchAppended {
	for _, log := range receipt.Logs {
		if log.Address != m.sccContract.Addr() {
			continue
		}

		name, result, err := m.sccContract.DecodeEvent(log)
		if err != nil {
			continue
		}

		if name != "StateBatchAppended" {
			continue
		}

		return &StateBatchAppended{
			ChainID:           result.GetBigInt(0),
			BatchIndex:        result.GetBigInt(1),
			BatchRoot:         result.GetHash(2),
			BatchSize:         result.GetBigInt(3),
			PrevTotalElements: result.GetBigInt(4),
			ExtraData:         result.GetBytes(5),
		}
	}

	return nil
}

func (m *gameCreator) StartMonitoring() {
	if m.isCreator {
		go m.minitorStateDisputed()
	} else {
		go m.monitorStateValidity()
	}
}

func (m *gameCreator) StopMonitoring() {
	m.cancel()
}
