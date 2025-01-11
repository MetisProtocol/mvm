package runner

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum-optimism/optimism/op-service/cliapp"
	"github.com/ethereum-optimism/optimism/op-service/dial"
	"github.com/ethereum-optimism/optimism/op-service/httputil"
	opmetrics "github.com/ethereum-optimism/optimism/op-service/metrics"
	"github.com/ethereum-optimism/optimism/op-service/sources/batching"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"

	l2client "github.com/MetisProtocol/mvm/l2geth/ethclient"
	"github.com/MetisProtocol/mvm/l2geth/rollup"
	"github.com/ethereum-optimism/optimism/go/cannon/mipsevm"

	"github.com/ethereum-optimism/optimism/go/op-challenger/config"
	"github.com/ethereum-optimism/optimism/go/op-challenger/game/fault/contracts"
	contractMetrics "github.com/ethereum-optimism/optimism/go/op-challenger/game/fault/contracts/metrics"
	"github.com/ethereum-optimism/optimism/go/op-challenger/game/fault/trace/utils"
	"github.com/ethereum-optimism/optimism/go/op-challenger/game/fault/trace/vm"
	"github.com/ethereum-optimism/optimism/go/op-challenger/game/fault/types"
)

var (
	ErrUnexpectedStatusCode = errors.New("unexpected status code")
)

type Metricer interface {
	vm.Metricer
	contractMetrics.ContractMetricer

	RecordFailure(vmType types.TraceType)
	RecordInvalid(vmType types.TraceType)
	RecordSuccess(vmType types.TraceType)
}

type Runner struct {
	log log.Logger
	cfg *config.Config
	m   Metricer

	running    atomic.Bool
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	metricsSrv *httputil.HTTPServer
}

func NewRunner(logger log.Logger, cfg *config.Config) *Runner {
	return &Runner{
		log: logger,
		cfg: cfg,
		m:   NewMetrics(),
	}
}

func (r *Runner) Start(ctx context.Context) error {
	if !r.running.CompareAndSwap(false, true) {
		return errors.New("already started")
	}
	ctx, cancel := context.WithCancel(ctx)
	r.ctx = ctx
	r.cancel = cancel
	if err := r.initMetricsServer(&r.cfg.MetricsConfig); err != nil {
		return fmt.Errorf("failed to start metrics: %w", err)
	}

	l2Client, err := l2client.Dial(r.cfg.L2Rpc)
	if err != nil {
		return fmt.Errorf("failed to dial l2 client: %w", err)
	}

	chainId, err := l2Client.ChainID(ctx)
	if err != nil {
		return fmt.Errorf("failed to get chain id: %w", err)
	}

	rollupClient := rollup.NewClient(r.cfg.RollupRpc, chainId)
	l1Client, err := dial.DialRPCClientWithTimeout(ctx, 1*time.Minute, r.log, r.cfg.L1EthRpc)
	if err != nil {
		return fmt.Errorf("failed to dial l1 client: %w", err)
	}
	l1RPCClient := ethclient.NewClient(l1Client)

	caller := batching.NewMultiCaller(l1Client, batching.DefaultBatchSize)

	for _, traceType := range r.cfg.TraceTypes {
		r.wg.Add(1)
		go r.loop(ctx, traceType, l1RPCClient, l2Client, rollupClient, caller)
	}

	r.log.Info("Runners started")
	return nil
}

func (r *Runner) loop(ctx context.Context, traceType types.TraceType, l1RPCClient *ethclient.Client, l2RPCClient *l2client.Client, rollupClient rollup.RollupClient, caller *batching.MultiCaller) {
	defer r.wg.Done()
	t := time.NewTicker(1 * time.Minute)
	defer t.Stop()
	for {
		if err := r.runOnce(ctx, traceType, l1RPCClient, l2RPCClient, rollupClient, caller); errors.Is(err, ErrUnexpectedStatusCode) {
			r.log.Error("Incorrect status code", "type", traceType, "err", err)
			r.m.RecordInvalid(traceType)
		} else if err != nil {
			r.log.Error("Failed to run", "type", traceType, "err", err)
			r.m.RecordFailure(traceType)
		} else {
			r.log.Info("Successfully verified output root", "type", traceType)
			r.m.RecordSuccess(traceType)
		}
		select {
		case <-t.C:
		case <-ctx.Done():
			return
		}
	}
}

func (r *Runner) runOnce(ctx context.Context, traceType types.TraceType, l1RPCClient *ethclient.Client, l2RPCClient *l2client.Client, rollupClient rollup.RollupClient, caller *batching.MultiCaller) error {
	prestateHash, err := r.getPrestateHash(ctx, traceType, caller)
	if err != nil {
		return err
	}

	localInputs, err := r.createGameInputs(ctx, l1RPCClient, l2RPCClient, rollupClient)
	if err != nil {
		return err
	}
	dir, err := r.prepDatadir(traceType)
	if err != nil {
		return err
	}
	logger := r.log.New("l1", localInputs.L1Head, "l2", localInputs.L2Head, "l2Block", localInputs.L2BlockNumber, "claim", localInputs.L2Claim, "type", traceType)
	provider, err := createTraceProvider(logger, r.m, r.cfg, prestateHash, traceType, localInputs, dir)
	if err != nil {
		return fmt.Errorf("failed to create trace provider: %w", err)
	}
	hash, err := provider.Get(ctx, types.RootPosition)
	if err != nil {
		return fmt.Errorf("failed to execute trace provider: %w", err)
	}
	if hash[0] != mipsevm.VMStatusValid {
		return fmt.Errorf("%w: %v", ErrUnexpectedStatusCode, hash)
	}
	return nil
}

func (r *Runner) prepDatadir(traceType types.TraceType) (string, error) {
	dir := filepath.Join(r.cfg.Datadir, traceType.String())
	if err := os.RemoveAll(dir); err != nil {
		return "", fmt.Errorf("failed to remove old dir: %w", err)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create data dir (%v): %w", dir, err)
	}
	return dir, nil
}

func (r *Runner) createGameInputs(ctx context.Context, l1RPCClient *ethclient.Client, l2RPCClient *l2client.Client, rollupClient rollup.RollupClient) (utils.LocalGameInputs, error) {
	latestBatch, err := rollupClient.GetLatestStateBatches()
	if err != nil {
		return utils.LocalGameInputs{}, fmt.Errorf("failed to get latest L2 batch: %w", err)
	}

	// we need to dispute until the last block of the batch,
	// since we are not like op that having a 1:N mapping for l1 -> l2 blocks
	blockNumber := uint64(latestBatch.Batch.PrevTotalElements + latestBatch.Batch.Size)
	prevBatch, err := rollupClient.GetStateBatchByIndex(latestBatch.Batch.Index - 1)

	claimOutput := rollup.BatchHeader{
		BatchRoot:         latestBatch.Batch.Root,
		BatchSize:         big.NewInt(int64(latestBatch.Batch.Size)),
		PrevTotalElements: big.NewInt(int64(latestBatch.Batch.PrevTotalElements)),
		ExtraData:         rollup.ExtraData(latestBatch.Batch.ExtraData),
	}.Hash()

	parentOutput := rollup.BatchHeader{
		BatchRoot:         prevBatch.Batch.Root,
		BatchSize:         big.NewInt(int64(prevBatch.Batch.Size)),
		PrevTotalElements: big.NewInt(int64(prevBatch.Batch.PrevTotalElements)),
		ExtraData:         rollup.ExtraData(prevBatch.Batch.ExtraData),
	}.Hash()

	l1Head, err := l1RPCClient.HeaderByNumber(ctx, new(big.Int).SetUint64(latestBatch.Batch.BlockNumber))
	if err != nil {
		return utils.LocalGameInputs{}, fmt.Errorf("failed to get L1 head: %w", err)
	}

	l2Head, err := l2RPCClient.HeaderByNumber(ctx, new(big.Int).SetUint64(uint64(prevBatch.Batch.PrevTotalElements+prevBatch.Batch.Size)))
	if err != nil {
		return utils.LocalGameInputs{}, fmt.Errorf("failed to get L2 head: %w", err)
	}

	localInputs := utils.LocalGameInputs{
		L1Head:        l1Head.Hash(),
		L2Head:        common.Hash(l2Head.Hash()),
		L2OutputRoot:  common.Hash(parentOutput),
		L2Claim:       common.Hash(claimOutput),
		L2BlockNumber: new(big.Int).SetUint64(blockNumber),
	}
	return localInputs, nil
}

func (r *Runner) findL2BlockNumberToDispute(ctx context.Context, client rollup.RollupClient, batchIndex uint64, l1HeadNum uint64, l2BlockNum uint64) (uint64, error) {
	// Try to find a L1 block prior to the batch that make l2BlockNum safe
	// Limits how far back we search to 64 batches
	var prevBatch *rollup.Batch
	for i := 0; i < 64; i++ {
		batch, blocks, err := client.GetBlockBatch(batchIndex)
		if err != nil {
			return 0, fmt.Errorf("failed to get raw block: %w", err)
		}

		// went too far back, batch is not disputable
		if len(batch.ExtraData) < 128 {
			if prevBatch == nil {
				return 0, fmt.Errorf("no prior batch found")
			}

			// return the beginning of the next batch of the genesis batch
			return uint64(prevBatch.PrevTotalElements + prevBatch.Size + 1), nil
		}

		if batch.BlockNumber < l1HeadNum {
			return blocks[len(blocks)-1].NumberU64() + 1, nil
		}

		prevBatch = batch
	}

	r.log.Warn("Failed to find prior batch", "l2BlockNum", l2BlockNum, "earliestCheckL1Block", l1HeadNum)

	return l2BlockNum, nil
}

func (r *Runner) getPrestateHash(ctx context.Context, traceType types.TraceType, caller *batching.MultiCaller) (common.Hash, error) {
	gameFactory := contracts.NewDisputeGameFactoryContract(r.m, r.cfg.GameFactoryAddress, caller, common.Address{})
	gameImplAddr, err := gameFactory.GetGameImpl(ctx, traceType.GameType())
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to load game impl: %w", err)
	}
	if gameImplAddr == (common.Address{}) {
		return common.Hash{}, nil // No prestate is set, will only work if a single prestate is specified
	}
	gameImpl, err := contracts.NewFaultDisputeGameContract(ctx, r.m, gameImplAddr, caller, common.Address{})
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to create fault dispute game contract bindings for %v: %w", gameImplAddr, err)
	}
	prestateHash, err := gameImpl.GetAbsolutePrestateHash(ctx)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to get absolute prestate hash for %v: %w", gameImplAddr, err)
	}
	return prestateHash, err
}

func (r *Runner) Stop(ctx context.Context) error {
	r.log.Info("Stopping")
	if !r.running.CompareAndSwap(true, false) {
		return errors.New("not started")
	}
	r.cancel()
	r.wg.Wait()

	if r.metricsSrv != nil {
		return r.metricsSrv.Stop(ctx)
	}
	return nil
}

func (r *Runner) Stopped() bool {
	return !r.running.Load()
}

func (r *Runner) initMetricsServer(cfg *opmetrics.CLIConfig) error {
	if !cfg.Enabled {
		return nil
	}
	r.log.Debug("Starting metrics server", "addr", cfg.ListenAddr, "port", cfg.ListenPort)
	m, ok := r.m.(opmetrics.RegistryMetricer)
	if !ok {
		return fmt.Errorf("metrics were enabled, but metricer %T does not expose registry for metrics-server", r.m)
	}
	metricsSrv, err := opmetrics.StartServer(m.Registry(), cfg.ListenAddr, cfg.ListenPort)
	if err != nil {
		return fmt.Errorf("failed to start metrics server: %w", err)
	}
	r.log.Info("started metrics server", "addr", metricsSrv.Addr())
	r.metricsSrv = metricsSrv
	return nil
}

var _ cliapp.Lifecycle = (*Runner)(nil)
