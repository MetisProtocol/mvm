// Package sources exports a number of clients used to access ethereum chain data.
//
// There are a number of these exported clients used by the op-node:
// [L1Client] wraps an RPC client to retrieve L1 ethereum data.
// [L2Client] wraps an RPC client to retrieve L2 ethereum data.
// [RollupClient] wraps an RPC client to retrieve rollup data.
// [EngineClient] extends the [L2Client] providing engine API bindings.
//
// Internally, the listed clients wrap an [EthClient] which itself wraps a specified RPC client.
package l2sources

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"time"

	"github.com/ethereum-optimism/optimism/op-service/client"
	"github.com/ethereum-optimism/optimism/op-service/sources"
	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"

	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum-optimism/optimism/op-service/sources/caching"

	l2common "github.com/MetisProtocol/mvm/l2geth/common"
	l2types "github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/ethereum-optimism/optimism/go/op-program/derive"
)

type EthClientConfig struct {
	// Maximum number of requests to make per batch
	MaxRequestsPerBatch int

	// limit concurrent requests, applies to the source as a whole
	MaxConcurrentRequests int

	// cache sizes

	// Number of blocks worth of receipts to cache
	ReceiptsCacheSize int
	// Number of blocks worth of transactions to cache
	TransactionsCacheSize int
	// Number of block headers to cache
	HeadersCacheSize int
	// Number of payloads to cache
	PayloadsCacheSize int

	// If the RPC is untrusted, then we should not use cached information from responses,
	// and instead verify against the block-hash.
	// Of real L1 blocks no deposits can be missed/faked, no batches can be missed/faked,
	// only the wrong L1 blocks can be retrieved.
	TrustRPC bool

	// If the RPC must ensure that the results fit the ExecutionPayload(Header) format.
	// If this is not checked, disabled header fields like the nonce or difficulty
	// may be used to get a different block-hash.
	MustBePostMerge bool

	// RPCProviderKind is a hint at what type of RPC provider we are dealing with
	RPCProviderKind sources.RPCProviderKind

	// Method reset duration defines how long we stick to available RPC methods,
	// till we re-attempt the user-preferred methods.
	// If this is 0 then the client does not fall back to less optimal but available methods.
	MethodResetDuration time.Duration

	// [OPTIONAL] The reth DB path to fetch receipts from.
	// If it is specified, the rethdb receipts fetcher will be used
	// and the RPC configuration parameters don't need to be set.
	RethDBPath string
}

func (c *EthClientConfig) Check() error {
	if c.ReceiptsCacheSize < 0 {
		return fmt.Errorf("invalid receipts cache size: %d", c.ReceiptsCacheSize)
	}
	if c.TransactionsCacheSize < 0 {
		return fmt.Errorf("invalid transactions cache size: %d", c.TransactionsCacheSize)
	}
	if c.HeadersCacheSize < 0 {
		return fmt.Errorf("invalid headers cache size: %d", c.HeadersCacheSize)
	}
	if c.PayloadsCacheSize < 0 {
		return fmt.Errorf("invalid payloads cache size: %d", c.PayloadsCacheSize)
	}
	if c.MaxConcurrentRequests < 1 {
		return fmt.Errorf("expected at least 1 concurrent request, but max is %d", c.MaxConcurrentRequests)
	}
	if c.MaxRequestsPerBatch < 1 {
		return fmt.Errorf("expected at least 1 request per batch, but max is: %d", c.MaxRequestsPerBatch)
	}
	if !sources.ValidRPCProviderKind(c.RPCProviderKind) {
		return fmt.Errorf("unknown rpc provider kind: %s", c.RPCProviderKind)
	}
	return nil
}

// EthClient retrieves ethereum data with optimized batch requests, cached results, and flag to not trust the RPC.
type EthClient struct {
	client client.RPC

	trustRPC bool

	mustBePostMerge bool

	log log.Logger

	// cache transactions in bundles per block hash
	// common.Hash -> types.Transactions
	transactionsCache *caching.LRUCache[l2common.Hash, l2types.Transactions]

	// cache block headers of blocks by hash
	// common.Hash -> *HeaderInfo
	headersCache *caching.LRUCache[l2common.Hash, eth.BlockInfo]

	// cache payloads by hash
	// common.Hash -> *eth.ExecutionPayload
	payloadsCache *caching.LRUCache[l2common.Hash, *eth.ExecutionPayloadEnvelope]
}

// NewEthClient returns an [EthClient], wrapping an RPC with bindings to fetch ethereum data with added error logging,
// metric tracking, and caching. The [EthClient] uses a [LimitRPC] wrapper to limit the number of concurrent RPC requests.
func NewEthClient(client client.RPC, log log.Logger, metrics caching.Metrics, config *EthClientConfig) (*EthClient, error) {
	if err := config.Check(); err != nil {
		return nil, fmt.Errorf("bad config, cannot create L1 source: %w", err)
	}

	client = sources.LimitRPC(client, config.MaxConcurrentRequests)
	return &EthClient{
		client:            client,
		trustRPC:          config.TrustRPC,
		mustBePostMerge:   config.MustBePostMerge,
		log:               log,
		transactionsCache: caching.NewLRUCache[l2common.Hash, l2types.Transactions](metrics, "txs", config.TransactionsCacheSize),
		headersCache:      caching.NewLRUCache[l2common.Hash, eth.BlockInfo](metrics, "headers", config.HeadersCacheSize),
		payloadsCache:     caching.NewLRUCache[l2common.Hash, *eth.ExecutionPayloadEnvelope](metrics, "payloads", config.PayloadsCacheSize),
	}, nil
}

// SubscribeNewHead subscribes to notifications about the current blockchain head on the given channel.
func (s *EthClient) SubscribeNewHead(ctx context.Context, ch chan<- *types.Header) (ethereum.Subscription, error) {
	// Note that *types.Header does not cache the block hash unlike *HeaderInfo, it always recomputes.
	// Inefficient if used poorly, but no trust issue.
	return s.client.EthSubscribe(ctx, ch, "newHeads")
}

// rpcBlockID is an internal type to enforce header and block call results match the requested identifier
type rpcBlockID interface {
	// Arg translates the object into an RPC argument
	Arg() any
	// CheckID verifies a block/header result matches the requested block identifier
	CheckID(id eth.BlockID) error
}

// hashID implements rpcBlockID for safe block-by-hash fetching
type hashID l2common.Hash

func (h hashID) Arg() any { return l2common.Hash(h) }
func (h hashID) CheckID(id eth.BlockID) error {
	if l2common.Hash(h) != l2common.Hash(id.Hash) {
		return fmt.Errorf("expected block hash %s but got block %s", l2common.Hash(h), id)
	}
	return nil
}

// numberID implements rpcBlockID for safe block-by-number fetching
type numberID uint64

func (n numberID) Arg() any { return hexutil.EncodeUint64(uint64(n)) }
func (n numberID) CheckID(id eth.BlockID) error {
	if uint64(n) != id.Number {
		return fmt.Errorf("expected block number %d but got block %s", uint64(n), id)
	}
	return nil
}

func (s *EthClient) headerCall(ctx context.Context, method string, id rpcBlockID) (eth.BlockInfo, error) {
	var header *l2types.Header
	err := s.client.CallContext(ctx, &header, method, id.Arg(), false) // headers are just blocks without txs
	if err != nil {
		return nil, err
	}
	if header == nil {
		return nil, ethereum.NotFound
	}
	info := derive.ToHeaderInfo(header)
	if err := id.CheckID(eth.ToBlockID(info)); err != nil {
		return nil, fmt.Errorf("fetched block header does not match requested ID: %w", err)
	}
	s.headersCache.Add(l2common.Hash(info.Hash()), info)
	return info, nil
}

func (s *EthClient) blockCall(ctx context.Context, method string, id rpcBlockID) (eth.BlockInfo, l2types.Transactions, error) {
	var block *l2types.Block
	err := s.client.CallContext(ctx, &block, method, id.Arg(), true)
	if err != nil {
		return nil, nil, err
	}
	if block == nil {
		return nil, nil, ethereum.NotFound
	}
	info := derive.ToHeaderInfo(block.Header())
	if err := id.CheckID(eth.ToBlockID(info)); err != nil {
		return nil, nil, fmt.Errorf("fetched block data does not match requested ID: %w", err)
	}
	s.headersCache.Add(l2common.Hash(info.Hash()), info)
	s.transactionsCache.Add(l2common.Hash(info.Hash()), block.Transactions())
	return info, block.Transactions(), nil
}

func (s *EthClient) payloadCall(ctx context.Context, method string, id rpcBlockID) (*eth.ExecutionPayloadEnvelope, error) {
	var block *l2types.Block
	err := s.client.CallContext(ctx, &block, method, id.Arg(), true)
	if err != nil {
		return nil, err
	}
	if block == nil {
		return nil, ethereum.NotFound
	}
	envelope, err := derive.ToExecutionPayloadEnvelope(block)
	if err != nil {
		return nil, err
	}
	if err := id.CheckID(envelope.ExecutionPayload.ID()); err != nil {
		return nil, fmt.Errorf("fetched payload does not match requested ID: %w", err)
	}
	s.payloadsCache.Add(l2common.Hash(envelope.ExecutionPayload.BlockHash), envelope)
	return envelope, nil
}

// ChainID fetches the chain id of the internal RPC.
func (s *EthClient) ChainID(ctx context.Context) (*big.Int, error) {
	var id hexutil.Big
	err := s.client.CallContext(ctx, &id, "eth_chainId")
	if err != nil {
		return nil, err
	}
	return (*big.Int)(&id), nil
}

func (s *EthClient) InfoByHash(ctx context.Context, hash common.Hash) (eth.BlockInfo, error) {
	if header, ok := s.headersCache.Get(l2common.Hash(hash)); ok {
		return header, nil
	}
	return s.headerCall(ctx, "eth_getBlockByHash", hashID(hash))
}

func (s *EthClient) InfoByNumber(ctx context.Context, number uint64) (eth.BlockInfo, error) {
	// can't hit the cache when querying by number due to reorgs.
	return s.headerCall(ctx, "eth_getBlockByNumber", numberID(number))
}

func (s *EthClient) InfoByLabel(ctx context.Context, label eth.BlockLabel) (eth.BlockInfo, error) {
	// can't hit the cache when querying the head due to reorgs / changes.
	return s.headerCall(ctx, "eth_getBlockByNumber", label)
}

func (s *EthClient) InfoAndTxsByHash(ctx context.Context, hash l2common.Hash) (eth.BlockInfo, l2types.Transactions, error) {
	if header, ok := s.headersCache.Get(hash); ok {
		if txs, ok := s.transactionsCache.Get(hash); ok {
			return header, txs, nil
		}
	}
	return s.blockCall(ctx, "eth_getBlockByHash", hashID(hash))
}

func (s *EthClient) InfoAndTxsByNumber(ctx context.Context, number uint64) (eth.BlockInfo, l2types.Transactions, error) {
	// can't hit the cache when querying by number due to reorgs.
	return s.blockCall(ctx, "eth_getBlockByNumber", numberID(number))
}

func (s *EthClient) InfoAndTxsByLabel(ctx context.Context, label eth.BlockLabel) (eth.BlockInfo, l2types.Transactions, error) {
	// can't hit the cache when querying the head due to reorgs / changes.
	return s.blockCall(ctx, "eth_getBlockByNumber", label)
}

func (s *EthClient) PayloadByHash(ctx context.Context, hash common.Hash) (*eth.ExecutionPayloadEnvelope, error) {
	if payload, ok := s.payloadsCache.Get(l2common.Hash(hash)); ok {
		return payload, nil
	}
	return s.payloadCall(ctx, "eth_getBlockByHash", hashID(hash))
}

func (s *EthClient) PayloadByNumber(ctx context.Context, number uint64) (*eth.ExecutionPayloadEnvelope, error) {
	return s.payloadCall(ctx, "eth_getBlockByNumber", numberID(number))
}

func (s *EthClient) PayloadByLabel(ctx context.Context, label eth.BlockLabel) (*eth.ExecutionPayloadEnvelope, error) {
	return s.payloadCall(ctx, "eth_getBlockByNumber", label)
}

// FetchReceipts returns a block info and all of the receipts associated with transactions in the block.
// It verifies the receipt hash in the block header against the receipt hash of the fetched receipts
// to ensure that the execution engine did not fail to return any receipts.
func (s *EthClient) FetchReceipts(ctx context.Context, blockHash common.Hash) (eth.BlockInfo, types.Receipts, error) {
	info, txs, err := s.InfoAndTxsByHash(ctx, l2common.Hash(blockHash))
	if err != nil {
		return nil, nil, fmt.Errorf("querying block: %w", err)
	}

	receiptsCh := make(chan *types.Receipt, 1)
	errCh := make(chan error, 1)

	// TODO: we might need to control the concurrency here,
	//       as the number of transactions in a block can be very large,
	//       and the rpc provider might have a limit for request frequency
	for _, tx := range txs {
		go func(txHash l2common.Hash) {
			var receipt types.Receipt
			if err := s.client.CallContext(ctx, &receipt, "eth_getTransactionReceipt"); err != nil {
				select {
				case errCh <- err:
				default:
					s.log.Warn("Error channel of eth client is blocked")
				}
			}
		}(tx.Hash())
	}

	var receipts types.Receipts
	for {
		select {
		case receipt := <-receiptsCh:
			receipts = append(receipts, receipt)
		case err := <-errCh:
			return nil, nil, fmt.Errorf("querying receipt: %w", err)
		}
	}

	// sort the receipts by transaction index, ascending
	sort.Slice(receipts, func(i, j int) bool {
		return receipts[i].TransactionIndex < receipts[j].TransactionIndex
	})

	return info, receipts, nil
}

// GetProof returns an account proof result, with any optional requested storage proofs.
// The retrieval does sanity-check that storage proofs for the expected keys are present in the response,
// but does not verify the result. Call accountResult.Verify(stateRoot) to verify the result.
func (s *EthClient) GetProof(ctx context.Context, address common.Address, storage []common.Hash, blockTag string) (*eth.AccountResult, error) {
	var getProofResponse *eth.AccountResult
	err := s.client.CallContext(ctx, &getProofResponse, "eth_getProof", address, storage, blockTag)
	if err != nil {
		return nil, err
	}
	if getProofResponse == nil {
		return nil, ethereum.NotFound
	}
	if len(getProofResponse.StorageProof) != len(storage) {
		return nil, fmt.Errorf("missing storage proof data, got %d proof entries but requested %d storage keys", len(getProofResponse.StorageProof), len(storage))
	}
	for i, key := range storage {
		if key != getProofResponse.StorageProof[i].Key {
			return nil, fmt.Errorf("unexpected storage proof key difference for entry %d: got %s but requested %s", i, getProofResponse.StorageProof[i].Key, key)
		}
	}
	return getProofResponse, nil
}

// GetStorageAt returns the storage value at the given address and storage slot, **without verifying the correctness of the result**.
// This should only ever be used as alternative to GetProof when the user opts in.
// E.g. Erigon L1 node users may have to use this, since Erigon does not support eth_getProof, see https://github.com/ledgerwatch/erigon/issues/1349
func (s *EthClient) GetStorageAt(ctx context.Context, address common.Address, storageSlot common.Hash, blockTag string) (common.Hash, error) {
	var out common.Hash
	err := s.client.CallContext(ctx, &out, "eth_getStorageAt", address, storageSlot, blockTag)
	return out, err
}

// ReadStorageAt is a convenience method to read a single storage value at the given slot in the given account.
// The storage slot value is verified against the state-root of the given block if we do not trust the RPC provider, or directly retrieved without proof if we do trust the RPC.
func (s *EthClient) ReadStorageAt(ctx context.Context, address common.Address, storageSlot common.Hash, blockHash common.Hash) (common.Hash, error) {
	if s.trustRPC {
		return s.GetStorageAt(ctx, address, storageSlot, blockHash.String())
	}
	block, err := s.InfoByHash(ctx, blockHash)
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to retrieve state root of block %s: %w", blockHash, err)
	}

	result, err := s.GetProof(ctx, address, []common.Hash{storageSlot}, blockHash.String())
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to fetch proof of storage slot %s at block %s: %w", storageSlot, blockHash, err)
	}

	if err := result.Verify(block.Root()); err != nil {
		return common.Hash{}, fmt.Errorf("failed to verify retrieved proof against state root: %w", err)
	}
	value := result.StorageProof[0].Value.ToInt()
	return common.BytesToHash(value.Bytes()), nil
}

func (s *EthClient) Close() {
	s.client.Close()
}
