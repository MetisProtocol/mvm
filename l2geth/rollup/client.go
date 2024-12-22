package rollup

import (
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/go-resty/resty/v2"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/common/hexutil"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/crypto"
)

// Constants that are used to compare against values in the deserialized JSON
// fetched by the RollupClient
const (
	sequencer = "sequencer"
	l1        = "l1"
)

// errElementNotFound represents the error case of the remote element not being
// found. It applies to transactions, queue elements and batches
var errElementNotFound = errors.New("element not found")

// errHttpError represents the error case of when the remote server
// returns a 400 or greater error
var errHTTPError = errors.New("http error")

var (
	bytes32Type, _ = abi.NewType("bytes32", "", nil)
	uint256Type, _ = abi.NewType("uint256", "", nil)
	bytesType, _   = abi.NewType("bytes", "", nil)

	BatchHeaderABI = abi.Arguments{
		{
			Name: "BatchRoot",
			Type: bytes32Type,
		},
		{
			Name: "BatchSize",
			Type: uint256Type,
		},
		{
			Name: "PrevTotalElements",
			Type: uint256Type,
		},
		{
			Name: "ExtraData",
			Type: bytesType,
		},
	}
)

type ExtraData []byte

func (e ExtraData) L1Timestamp() uint64 {
	if len(e) < 32 {
		return 0
	}

	return big.NewInt(0).SetBytes(e[0:32]).Uint64()
}

func (e ExtraData) Submitter() common.Address {
	if len(e) < 64 {
		return common.Address{}
	}

	return common.BytesToAddress(e[32:64])
}

func (e ExtraData) L2Head() common.Hash {
	if len(e) < 96 {
		return common.Hash{}
	}

	return common.BytesToHash(e[64:96])
}

type BatchHeader struct {
	BatchRoot         common.Hash
	BatchSize         *big.Int
	PrevTotalElements *big.Int
	ExtraData         ExtraData
}

func (b *BatchHeader) Unpack(data []byte) error {
	values, err := BatchHeaderABI.UnpackValues(data)
	if err != nil {
		return err
	}

	var ok bool
	b.BatchRoot, ok = values[0].(common.Hash)
	b.BatchSize, ok = values[1].(*big.Int)
	b.PrevTotalElements, ok = values[2].(*big.Int)
	b.ExtraData, ok = values[3].([]byte)
	if !ok {
		return errors.New("invalid batch header")
	}

	return nil
}

func (b *BatchHeader) Pack() ([]byte, error) {
	if b.BatchSize == nil || b.PrevTotalElements == nil {
		return nil, errors.New("nil batch size or prev total elements")
	}

	return BatchHeaderABI.Pack(
		b.BatchRoot,
		b.BatchSize,
		b.PrevTotalElements,
		b.ExtraData,
	)
}

func (b BatchHeader) Hash() common.Hash {
	data, err := b.Pack()
	if err != nil {
		return common.Hash{}
	}
	return crypto.Keccak256Hash(data)
}

// Batch represents the data structure that is submitted with
// a series of transactions to layer one
type Batch struct {
	Index             uint64         `json:"index"`
	Root              common.Hash    `json:"root,omitempty"`
	Size              uint32         `json:"size,omitempty"`
	PrevTotalElements uint32         `json:"prevTotalElements,omitempty"`
	ExtraData         hexutil.Bytes  `json:"extraData,omitempty"`
	BlockNumber       uint64         `json:"blockNumber"`
	Timestamp         uint64         `json:"timestamp"`
	Submitter         common.Address `json:"submitter"`
}

// EthContext represents the L1 EVM context that is injected into
// the OVM at runtime. It is updated with each `enqueue` Transaction
// and needs to be fetched from a remote server to be updated when
// too much time has passed between `enqueue` transactions.
type EthContext struct {
	BlockNumber uint64      `json:"blockNumber"`
	BlockHash   common.Hash `json:"blockHash"`
	Timestamp   uint64      `json:"timestamp"`
}

// SyncStatus represents the state of the remote server. The SyncService
// does not want to begin syncing until the remote server has fully synced.
type SyncStatus struct {
	Syncing                      bool   `json:"syncing"`
	HighestKnownTransactionIndex uint64 `json:"highestKnownTransactionIndex"`
	CurrentTransactionIndex      uint64 `json:"currentTransactionIndex"`
}

// L1GasPrice represents the gas price of L1. It is used as part of the gas
// estimatation logic.
type L1GasPrice struct {
	GasPrice string `json:"gasPrice"`
}

// Transaction represents the return result of the remote server.
// It either came from a batch or was replicated from the sequencer.
type Transaction struct {
	Index       uint64          `json:"index"`
	BatchIndex  uint64          `json:"batchIndex"`
	BlockNumber uint64          `json:"blockNumber"`
	Timestamp   uint64          `json:"timestamp"`
	Value       *hexutil.Big    `json:"value"`
	GasLimit    uint64          `json:"gasLimit,string"`
	Target      common.Address  `json:"target"`
	Origin      *common.Address `json:"origin"`
	Data        hexutil.Bytes   `json:"data"`
	QueueOrigin string          `json:"queueOrigin"`
	QueueIndex  *uint64         `json:"queueIndex"`
	Decoded     *Decoded        `json:"Decoded"`
	SeqSign     string          `json:"seqSign"`
}

// Enqueue represents an `enqueue` Transaction or a L1 to L2 Transaction.
type Enqueue struct {
	Index       *uint64         `json:"ctcIndex"`
	Target      *common.Address `json:"target"`
	Data        *hexutil.Bytes  `json:"data"`
	GasLimit    *uint64         `json:"gasLimit,string"`
	Origin      *common.Address `json:"origin"`
	BlockNumber *uint64         `json:"blockNumber"`
	Timestamp   *uint64         `json:"timestamp"`
	QueueIndex  *uint64         `json:"index"`
}

// Signature represents a secp256k1 ECDSA Signature
type Signature struct {
	R hexutil.Bytes `json:"r"`
	S hexutil.Bytes `json:"s"`
	V uint          `json:"v"`
}

// Decoded represents the Decoded Transaction from the batch.
// When this struct exists in other structs and is set to `nil`,
// it means that the decoding failed.
type Decoded struct {
	Signature Signature       `json:"sig"`
	Value     *hexutil.Big    `json:"value"`
	GasLimit  uint64          `json:"gasLimit,string"`
	GasPrice  uint64          `json:"gasPrice,string"`
	Nonce     uint64          `json:"nonce,string"`
	Target    *common.Address `json:"target"`
	Data      hexutil.Bytes   `json:"data"`
}

// block represents the return result of the remote server.
// It either came from a batch or was replicated from the sequencer.
// It is used after DeSeqBlock
type Block struct {
	Index        uint64         `json:"index"`
	BatchIndex   uint64         `json:"batchIndex"`
	Timestamp    uint64         `json:"timestamp"`
	Transactions []*Transaction `json:"transactions"`
	Confirmed    bool           `json:"confirmed"`
}

// RollupClient is able to query for information
// that is required by the SyncService
type RollupClient interface {
	GetEnqueue(index uint64) (*types.Transaction, error)
	GetLatestEnqueue() (*types.Transaction, error)
	GetLatestEnqueueIndex() (*uint64, error)
	GetRawTransaction(uint64, Backend) (*TransactionResponse, error)
	GetTransaction(uint64, Backend) (*types.Transaction, error)
	GetLatestTransaction(Backend) (*types.Transaction, error)
	GetLatestTransactionIndex(Backend) (*uint64, error)
	GetEthContext(uint64) (*EthContext, error)
	GetLatestEthContext() (*EthContext, error)
	GetLastConfirmedEnqueue() (*types.Transaction, error)
	GetLatestTransactionBatch() (*Batch, []*types.Transaction, error)
	GetLatestTransactionBatchIndex() (*uint64, error)
	GetTransactionBatch(uint64) (*Batch, []*types.Transaction, error)
	SyncStatus(Backend) (*SyncStatus, error)
	GetStateRoot(index uint64) (common.Hash, error)
	GetStateBatchHeader(batchHeaderHash common.Hash) (*BatchHeader, []common.Hash, error)
	SetLastVerifier(index uint64, stateRoot string, verifierRoot string, success bool) error
	GetRawBlock(uint64, Backend) (*BlockResponse, error)
	GetBlock(uint64, Backend) (*types.Block, error)
	GetLatestBlock(Backend) (*types.Block, error)
	GetLatestBlockIndex(Backend) (*uint64, error)
	GetLatestBlockBatch() (*Batch, []*types.Block, error)
	GetLatestBlockBatchIndex() (*uint64, error)
	GetBlockBatch(uint64) (*Batch, []*types.Block, error)

	Signer() types.Signer
}

// Client is an HTTP based RollupClient
type Client struct {
	client  *resty.Client
	signer  *types.EIP155Signer
	chainID string

	useInbox bool
}

// TransactionResponse represents the response from the remote server when
// querying transactions.
type TransactionResponse struct {
	Transaction *Transaction `json:"Transaction"`
	Batch       *Batch       `json:"batch"`
}

// TransactionBatchResponse represents the response from the remote server
// when querying batches.
type TransactionBatchResponse struct {
	Batch        *Batch         `json:"batch"`
	Transactions []*Transaction `json:"transactions"`
}

// TransactionResponse represents the response from the remote server when
// querying transactions.
type BlockResponse struct {
	Block *Block `json:"block"`
	Batch *Batch `json:"batch"`
}

// TransactionBatchResponse represents the response from the remote server
// when querying batches.
type BlockBatchResponse struct {
	Batch  *Batch   `json:"batch"`
	Blocks []*Block `json:"blocks"`
}

type stateRoot struct {
	Index      uint64      `json:"index"`
	BatchIndex uint64      `json:"batchIndex"`
	Value      common.Hash `json:"value"`
	Confirmed  bool        `json:"confirmed"`
}

// StateRootResponse represents the response from the remote server
// when querying stateRoot
type StateRootResponse struct {
	Batch     *Batch     `json:"batch"`
	StateRoot *stateRoot `json:"stateRoot"`
}

type StateRootBatchResponse struct {
	Batch      *Batch       `json:"batch"`
	StateRoots []*stateRoot `json:"stateRoots"`
}

// NewClient create a new Client given a remote HTTP url and a chain id
func NewClient(url string, chainID *big.Int) *Client {
	client := resty.New()
	client.SetHostURL(url)
	client.SetHeader("User-Agent", "sequencer")
	client.OnAfterResponse(func(c *resty.Client, r *resty.Response) error {
		statusCode := r.StatusCode()
		if statusCode >= 400 {
			method := r.Request.Method
			url := r.Request.URL
			return fmt.Errorf("%d cannot %s %s: %w", statusCode, method, url, errHTTPError)
		}
		return nil
	})
	signer := types.NewEIP155Signer(chainID)

	return &Client{
		client:  client,
		signer:  &signer,
		chainID: chainID.String(),
	}
}

// GetEnqueue fetches an `enqueue` Transaction by queue index
func (c *Client) GetEnqueue(index uint64) (*types.Transaction, error) {
	str := strconv.FormatUint(index, 10)
	response, err := c.client.R().
		SetPathParams(map[string]string{
			"index":   str,
			"chainId": c.chainID,
		}).
		SetResult(&Enqueue{}).
		Get("/enqueue/index/{index}/{chainId}")

	if err != nil {
		return nil, fmt.Errorf("cannot fetch enqueue: %w", err)
	}
	enqueue, ok := response.Result().(*Enqueue)
	if !ok {
		return nil, fmt.Errorf("Cannot fetch enqueue %d", index)
	}
	if enqueue == nil {
		return nil, fmt.Errorf("Cannot deserialize enqueue %d", index)
	}
	tx, err := enqueueToTransaction(enqueue)
	if err != nil {
		return nil, err
	}
	return tx, nil
}

// enqueueToTransaction turns an Enqueue into a types.Transaction
// so that it can be consumed by the SyncService
func enqueueToTransaction(enqueue *Enqueue) (*types.Transaction, error) {
	if enqueue == nil {
		return nil, errElementNotFound
	}
	// When the queue index is nil, is means that the enqueue'd Transaction
	// does not exist.
	if enqueue.QueueIndex == nil {
		return nil, errElementNotFound
	}
	// The queue index is the nonce
	nonce := *enqueue.QueueIndex

	if enqueue.Target == nil {
		return nil, errors.New("Target not found for enqueue tx")
	}
	target := *enqueue.Target

	if enqueue.GasLimit == nil {
		return nil, errors.New("Gas limit not found for enqueue tx")
	}
	gasLimit := *enqueue.GasLimit
	if enqueue.Origin == nil {
		return nil, errors.New("Origin not found for enqueue tx")
	}
	origin := *enqueue.Origin
	if enqueue.BlockNumber == nil {
		return nil, errors.New("Blocknumber not found for enqueue tx")
	}
	blockNumber := new(big.Int).SetUint64(*enqueue.BlockNumber)
	if enqueue.Timestamp == nil {
		return nil, errors.New("Timestamp not found for enqueue tx")
	}
	timestamp := *enqueue.Timestamp

	if enqueue.Data == nil {
		return nil, errors.New("Data not found for enqueue tx")
	}
	data := *enqueue.Data

	// enqueue transactions have no value
	value := big.NewInt(0)
	tx := types.NewTransaction(nonce, target, value, gasLimit, big.NewInt(0), data)

	// The index does not get a check as it is allowed to be nil in the context
	// of an enqueue Transaction that has yet to be included into the CTC
	txMeta := types.NewTransactionMeta(
		blockNumber,
		timestamp,
		&origin,
		types.QueueOriginL1ToL2,
		enqueue.Index,
		enqueue.QueueIndex,
		data,
	)
	tx.SetTransactionMeta(txMeta)

	return tx, nil
}

// GetLatestEnqueue fetches the latest `enqueue`, meaning the `enqueue`
// Transaction with the greatest queue index.
func (c *Client) GetLatestEnqueue() (*types.Transaction, error) {
	response, err := c.client.R().
		SetPathParams(map[string]string{
			"chainId": c.chainID,
		}).
		SetResult(&Enqueue{}).
		Get("/enqueue/latest/{chainId}")

	if err != nil {
		return nil, fmt.Errorf("cannot fetch latest enqueue: %w", err)
	}
	enqueue, ok := response.Result().(*Enqueue)
	if !ok {
		return nil, errors.New("Cannot fetch latest enqueue")
	}
	tx, err := enqueueToTransaction(enqueue)
	if err != nil {
		return nil, fmt.Errorf("Cannot parse enqueue tx: %w", err)
	}
	return tx, nil
}

// GetLatestEnqueueIndex returns the latest `enqueue()` index
func (c *Client) GetLatestEnqueueIndex() (*uint64, error) {
	tx, err := c.GetLatestEnqueue()
	if err != nil {
		return nil, err
	}
	index := tx.GetMeta().QueueIndex
	if index == nil {
		return nil, errors.New("Latest queue index is nil")
	}
	return index, nil
}

// GetLatestTransactionIndex returns the latest CTC index that has been batch
// submitted or not, depending on the backend
func (c *Client) GetLatestTransactionIndex(backend Backend) (*uint64, error) {
	tx, err := c.GetLatestTransaction(backend)
	if err != nil {
		return nil, err
	}
	index := tx.GetMeta().Index
	if index == nil {
		return nil, errors.New("Latest index is nil")
	}
	return index, nil
}

// GetLatestTransactionBatchIndex returns the latest Transaction batch index
func (c *Client) GetLatestTransactionBatchIndex() (*uint64, error) {
	var batch *Batch
	var err error
	if !c.useInbox {
		batch, _, err = c.GetLatestTransactionBatch()
	} else {
		batch, _, err = c.GetLatestBlockBatch()
	}
	if err != nil {
		if strings.Contains(err.Error(), "USE_INBOX_BATCH_INDEX") {
			batch, _, err = c.GetLatestBlockBatch()
			if err != nil {
				return nil, err
			}
			c.useInbox = true
		} else {
			return nil, err
		}
	}
	index := batch.Index
	return &index, nil
}

// BatchedBlockToBlock converts a block into a types.Block
func BatchedBlockToBlock(res *Block, signerChain *types.EIP155Signer) (*types.Block, error) {
	if res == nil || len(res.Transactions) == 0 {
		return nil, errElementNotFound
	}
	header := &types.Header{
		Time:   res.Timestamp,
		Number: big.NewInt(int64(res.Index + 1)),
	}
	var transactions []*types.Transaction
	for _, tx := range res.Transactions {
		transaction, err := BatchedTransactionToTransaction(tx, signerChain)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, transaction)
	}
	block := types.NewBlock(header, transactions, nil, nil)
	return block, nil
}

// BatchedTransactionToTransaction converts a Transaction into a
// types.Transaction that can be consumed by the SyncService
func BatchedTransactionToTransaction(res *Transaction, signerChain *types.EIP155Signer) (*types.Transaction, error) {
	// `nil` transactions are not found
	if res == nil {
		return nil, errElementNotFound
	}
	// The queue origin must be either sequencer of l1, otherwise
	// it is considered an unknown queue origin and will not be processed
	var queueOrigin types.QueueOrigin
	switch res.QueueOrigin {
	case sequencer:
		queueOrigin = types.QueueOriginSequencer
	case l1:
		queueOrigin = types.QueueOriginL1ToL2
	default:
		return nil, fmt.Errorf("Unknown queue origin: %s", res.QueueOrigin)
	}
	// Transactions that have been Decoded are
	// Queue Origin Sequencer transactions
	if res.Decoded != nil {
		nonce := res.Decoded.Nonce
		to := res.Decoded.Target
		value := (*big.Int)(res.Decoded.Value)
		// Note: there are two gas limits, one top level and
		// another on the raw Transaction itself. Maybe maxGasLimit
		// for the top level?
		gasLimit := res.Decoded.GasLimit
		gasPrice := new(big.Int).SetUint64(res.Decoded.GasPrice)
		data := res.Decoded.Data

		var tx *types.Transaction
		if to == nil {
			tx = types.NewContractCreation(nonce, value, gasLimit, gasPrice, data)
		} else {
			tx = types.NewTransaction(nonce, *to, value, gasLimit, gasPrice, data)
		}

		txMeta := types.NewTransactionMeta(
			new(big.Int).SetUint64(res.BlockNumber),
			res.Timestamp,
			res.Origin,
			queueOrigin,
			&res.Index,
			res.QueueIndex,
			res.Data,
		)
		tx.SetTransactionMeta(txMeta)

		r, s := res.Decoded.Signature.R, res.Decoded.Signature.S
		sig := make([]byte, crypto.SignatureLength)
		copy(sig[32-len(r):32], r)
		copy(sig[64-len(s):64], s)

		var signer types.Signer
		if res.Decoded.Signature.V == 27 || res.Decoded.Signature.V == 28 {
			signer = types.HomesteadSigner{}
			sig[64] = byte(res.Decoded.Signature.V - 27)
		} else {
			signer = signerChain
			sig[64] = byte(res.Decoded.Signature.V)
		}

		tx, err := tx.WithSignature(signer, sig[:])
		if err != nil {
			return nil, fmt.Errorf("Cannot add Signature to Transaction: %w", err)
		}

		// restore sequencer sign
		if len(res.SeqSign) > 0 {
			signResult := strings.Split(res.SeqSign, ",")
			seqSign := &types.SeqSign{
				R: big.NewInt(0),
				S: big.NewInt(0),
				V: big.NewInt(0),
			}
			if len(signResult) == 3 {
				seqR, _ := hexutil.DecodeBig(signResult[0])
				seqS, _ := hexutil.DecodeBig(signResult[1])
				seqV, _ := hexutil.DecodeBig(signResult[2])
				seqSign = &types.SeqSign{
					R: seqR,
					S: seqS,
					V: seqV,
				}
			}
			tx.SetSeqSign(seqSign)
		}

		return tx, nil
	}

	// The Transaction is  either an L1 to L2 Transaction or it does not have a
	// known deserialization
	nonce := uint64(0)
	if res.QueueOrigin == l1 {
		if res.QueueIndex == nil {
			return nil, errors.New("Queue origin L1 to L2 without a queue index")
		}
		nonce = *res.QueueIndex
	}
	target := res.Target
	gasLimit := res.GasLimit
	data := res.Data
	origin := res.Origin
	value := (*big.Int)(res.Value)
	tx := types.NewTransaction(nonce, target, value, gasLimit, big.NewInt(0), data)
	txMeta := types.NewTransactionMeta(
		new(big.Int).SetUint64(res.BlockNumber),
		res.Timestamp,
		origin,
		queueOrigin,
		&res.Index,
		res.QueueIndex,
		res.Data,
	)
	tx.SetTransactionMeta(txMeta)
	return tx, nil
}

// GetTransaction will get a Transaction by Canonical Transaction Chain index
func (c *Client) GetRawTransaction(index uint64, backend Backend) (*TransactionResponse, error) {
	str := strconv.FormatUint(index, 10)
	response, err := c.client.R().
		SetPathParams(map[string]string{
			"index":   str,
			"chainId": c.chainID,
		}).
		SetQueryParams(map[string]string{
			"backend": backend.String(),
		}).
		SetResult(&TransactionResponse{}).
		Get("/Transaction/index/{index}/{chainId}")

	if err != nil {
		return nil, fmt.Errorf("cannot fetch Transaction: %w", err)
	}
	res, ok := response.Result().(*TransactionResponse)
	if !ok {
		return nil, fmt.Errorf("could not get tx with index %d", index)
	}
	return res, nil
}

// GetTransaction will get a Transaction by Canonical Transaction Chain index
func (c *Client) GetTransaction(index uint64, backend Backend) (*types.Transaction, error) {
	res, err := c.GetRawTransaction(index, backend)
	if err != nil {
		return nil, err
	}
	return BatchedTransactionToTransaction(res.Transaction, c.signer)
}

// GetLatestTransaction will get the latest Transaction, meaning the Transaction
// with the greatest Canonical Transaction Chain index
func (c *Client) GetLatestTransaction(backend Backend) (*types.Transaction, error) {
	response, err := c.client.R().
		SetPathParams(map[string]string{
			"chainId": c.chainID,
		}).
		SetResult(&TransactionResponse{}).
		SetQueryParams(map[string]string{
			"backend": backend.String(),
		}).
		Get("/Transaction/latest/{chainId}")

	if err != nil {
		return nil, fmt.Errorf("cannot fetch latest transactions: %w", err)
	}
	res, ok := response.Result().(*TransactionResponse)
	if !ok {
		return nil, errors.New("Cannot get latest Transaction")
	}

	return BatchedTransactionToTransaction(res.Transaction, c.signer)
}

// GetEthContext will return the EthContext by block number
func (c *Client) GetEthContext(blockNumber uint64) (*EthContext, error) {
	str := strconv.FormatUint(blockNumber, 10)
	response, err := c.client.R().
		SetPathParams(map[string]string{
			"blocknumber": str,
			"chainId":     c.chainID,
		}).
		SetResult(&EthContext{}).
		Get("/eth/context/blocknumber/{blocknumber}")

	if err != nil {
		return nil, err
	}

	context, ok := response.Result().(*EthContext)
	if !ok {
		return nil, errors.New("Cannot parse EthContext")
	}
	return context, nil
}

// GetLatestEthContext will return the latest EthContext
func (c *Client) GetLatestEthContext() (*EthContext, error) {
	response, err := c.client.R().
		SetResult(&EthContext{}).
		Get("/eth/context/latest")

	if err != nil {
		return nil, fmt.Errorf("Cannot fetch eth context: %w", err)
	}

	context, ok := response.Result().(*EthContext)
	if !ok {
		return nil, errors.New("Cannot parse EthContext")
	}

	return context, nil
}

// GetLastConfirmedEnqueue will get the last `enqueue` Transaction that has been
// batched up
func (c *Client) GetLastConfirmedEnqueue() (*types.Transaction, error) {
	enqueue, err := c.GetLatestEnqueue()
	if err != nil {
		return nil, fmt.Errorf("Cannot get latest enqueue: %w", err)
	}
	// This should only happen if there are no L1 to L2 transactions yet
	if enqueue == nil {
		return nil, errElementNotFound
	}
	// Work backwards looking for the first enqueue
	// to have an index, which means it has been included
	// in the canonical Transaction chain.
	for {
		meta := enqueue.GetMeta()
		// The enqueue has an index so it has been confirmed
		if meta.Index != nil {
			return enqueue, nil
		}
		// There is no queue index so this is a bug
		if meta.QueueIndex == nil {
			return nil, fmt.Errorf("queue index is nil")
		}
		// No enqueue transactions have been confirmed yet
		if *meta.QueueIndex == uint64(0) {
			return nil, errElementNotFound
		}
		next, err := c.GetEnqueue(*meta.QueueIndex - 1)
		if err != nil {
			return nil, fmt.Errorf("cannot get enqueue %d: %w", *meta.Index, err)
		}
		enqueue = next
	}
}

// SyncStatus will query the remote server to determine if it is still syncing
func (c *Client) SyncStatus(backend Backend) (*SyncStatus, error) {
	response, err := c.client.R().
		SetPathParams(map[string]string{
			"chainId": c.chainID,
		}).
		SetResult(&SyncStatus{}).
		SetQueryParams(map[string]string{
			"backend": backend.String(),
		}).
		Get("/eth/syncing/{chainId}")

	if err != nil {
		return nil, fmt.Errorf("Cannot fetch sync status: %w", err)
	}

	status, ok := response.Result().(*SyncStatus)
	if !ok {
		return nil, fmt.Errorf("Cannot parse sync status")
	}

	return status, nil
}

// GetLatestTransactionBatch will return the latest Transaction batch
func (c *Client) GetLatestTransactionBatch() (*Batch, []*types.Transaction, error) {
	response, err := c.client.R().
		SetPathParams(map[string]string{
			"chainId": c.chainID,
		}).
		SetResult(&TransactionBatchResponse{}).
		Get("/batch/Transaction/latest/{chainId}")

	if err != nil {
		errStr := err.Error()
		if response != nil {
			errStr = response.String()
		}
		return nil, nil, fmt.Errorf("Cannot get latest Transaction batch: %s", errStr)
	}
	txBatch, ok := response.Result().(*TransactionBatchResponse)
	if !ok {
		return nil, nil, fmt.Errorf("Cannot parse Transaction batch response")
	}
	return parseTransactionBatchResponse(txBatch, c.signer)
}

// GetTransactionBatch will return the Transaction batch by batch index
func (c *Client) GetTransactionBatch(index uint64) (*Batch, []*types.Transaction, error) {
	str := strconv.FormatUint(index, 10)
	response, err := c.client.R().
		SetResult(&TransactionBatchResponse{}).
		SetPathParams(map[string]string{
			"index":   str,
			"chainId": c.chainID,
		}).
		Get("/batch/Transaction/index/{index}/{chainId}")

	if err != nil {
		errStr := err.Error()
		if response != nil {
			errStr = response.String()
		}
		return nil, nil, fmt.Errorf("Cannot get Transaction batch %d: %s", index, errStr)
	}
	txBatch, ok := response.Result().(*TransactionBatchResponse)
	if !ok {
		return nil, nil, fmt.Errorf("Cannot parse Transaction batch response")
	}
	return parseTransactionBatchResponse(txBatch, c.signer)
}

// parseTransactionBatchResponse will turn a TransactionBatchResponse into a
// Batch and its corresponding types.Transactions
func parseTransactionBatchResponse(txBatch *TransactionBatchResponse, signer *types.EIP155Signer) (*Batch, []*types.Transaction, error) {
	if txBatch == nil || txBatch.Batch == nil {
		return nil, nil, errElementNotFound
	}
	batch := txBatch.Batch
	txs := make([]*types.Transaction, len(txBatch.Transactions))
	for i, tx := range txBatch.Transactions {
		transaction, err := BatchedTransactionToTransaction(tx, signer)
		if err != nil {
			return nil, nil, fmt.Errorf("Cannot parse Transaction batch: %w", err)
		}
		txs[i] = transaction
	}
	return batch, txs, nil
}

// GetStateRoot will return the stateroot by batch index
func (c *Client) GetStateRoot(index uint64) (common.Hash, error) {
	str := strconv.FormatUint(index, 10)
	response, err := c.client.R().
		SetResult(&StateRootResponse{}).
		SetPathParams(map[string]string{
			"index":   str,
			"chainId": c.chainID,
		}).
		Get("/stateroot/index/{index}/{chainId}")

	if err != nil {
		return common.Hash{}, fmt.Errorf("Cannot get stateroot %d: %w", index, err)
	}
	stateRootResp, ok := response.Result().(*StateRootResponse)
	if !ok {
		return common.Hash{}, fmt.Errorf("Cannot parse stateroot response")
	}
	if stateRootResp.StateRoot == nil {
		return common.Hash{}, nil
	}
	return stateRootResp.StateRoot.Value, nil
}

// GetStateBatchHeader will return the state batch header preimage by batch header hash
func (c *Client) GetStateBatchHeader(batchHeaderHash common.Hash) (*BatchHeader, []common.Hash, error) {
	response, err := c.client.R().
		SetResult(&StateRootBatchResponse{}).
		SetPathParams(map[string]string{
			"hash":    batchHeaderHash.Hex(),
			"chainId": c.chainID,
		}).
		Get("/batch/stateroot/hash/{hash}/{chainId}")

	if err != nil {
		return nil, nil, fmt.Errorf("Cannot get state batch header %s: %w", batchHeaderHash.Hex(), err)
	}

	batchResp, ok := response.Result().(*StateRootBatchResponse)
	if !ok {
		return nil, nil, fmt.Errorf("Cannot parse state batch header response")
	}

	stateRoots := make([]common.Hash, len(batchResp.StateRoots))
	for i, stateRoot := range batchResp.StateRoots {
		stateRoots[i] = stateRoot.Value
	}

	return &BatchHeader{
		BatchRoot:         batchResp.Batch.Root,
		BatchSize:         big.NewInt(int64(batchResp.Batch.Size)),
		PrevTotalElements: big.NewInt(int64(batchResp.Batch.PrevTotalElements)),
		ExtraData:         ExtraData(batchResp.Batch.ExtraData),
	}, stateRoots, nil
}

// GetStateRoot will return the stateroot by batch index
func (c *Client) SetLastVerifier(index uint64, stateRoot string, verifierRoot string, success bool) error {
	str := strconv.FormatUint(index, 10)
	_, err := c.client.R().
		SetResult(&StateRootResponse{}).
		SetPathParams(map[string]string{
			"index":        str,
			"chainId":      c.chainID,
			"success":      strconv.FormatBool(success),
			"stateRoot":    stateRoot,
			"verifierRoot": verifierRoot,
		}).
		Get("/verifier/set/{success}/{chainId}/{index}/{stateRoot}/{verifierRoot}")

	if err != nil {
		return fmt.Errorf("Cannot set last verifier %d: %w", index, err)
	}
	return nil
}

// GetRawBlock will get a block with transactions by inbox batch index
func (c *Client) GetRawBlock(index uint64, backend Backend) (*BlockResponse, error) {
	str := strconv.FormatUint(index, 10)
	response, err := c.client.R().
		SetPathParams(map[string]string{
			"index":   str,
			"chainId": c.chainID,
		}).
		SetQueryParams(map[string]string{
			"backend": backend.String(),
		}).
		SetResult(&BlockResponse{}).
		Get("/block/index/{index}/{chainId}")

	if err != nil {
		return nil, fmt.Errorf("cannot fetch block: %w", err)
	}
	res, ok := response.Result().(*BlockResponse)
	if !ok {
		return nil, fmt.Errorf("could not get tx with index %d", index)
	}
	return res, nil
}

// GetBlock will get a block with transactions by inbox batch index
func (c *Client) GetBlock(index uint64, backend Backend) (*types.Block, error) {
	res, err := c.GetRawBlock(index, backend)
	if err != nil {
		return nil, err
	}
	return BatchedBlockToBlock(res.Block, c.signer)
}

func (c *Client) Signer() types.Signer {
	return c.signer
}

// GetLatestBlock will get the latest Transaction, meaning the Transaction
// with the greatest Canonical Transaction Chain index
func (c *Client) GetLatestBlock(backend Backend) (*types.Block, error) {
	response, err := c.client.R().
		SetPathParams(map[string]string{
			"chainId": c.chainID,
		}).
		SetResult(&BlockResponse{}).
		SetQueryParams(map[string]string{
			"backend": backend.String(),
		}).
		Get("/block/latest/{chainId}")

	if err != nil {
		// return nil, fmt.Errorf("cannot fetch latest block: %w", err)
		return nil, errElementNotFound
	}
	res, ok := response.Result().(*BlockResponse)
	if !ok {
		return nil, errors.New("Cannot get latest block")
	}

	return BatchedBlockToBlock(res.Block, c.signer)
}

// GetLatestBlockIndex returns the latest inbox batch
// submitted or not, depending on the backend
// the index keeps the same rule as CTC tx index (blockNumber - 1)
func (c *Client) GetLatestBlockIndex(backend Backend) (*uint64, error) {
	block, err := c.GetLatestBlock(backend)
	if err != nil {
		return nil, err
	}

	index := block.NumberU64() - 1
	return &index, nil
}

// GetLatestBlockBatch will return the latest block batch
func (c *Client) GetLatestBlockBatch() (*Batch, []*types.Block, error) {
	response, err := c.client.R().
		SetPathParams(map[string]string{
			"chainId": c.chainID,
		}).
		SetResult(&BlockBatchResponse{}).
		Get("/batch/block/latest/{chainId}")

	if err != nil {
		return nil, nil, errors.New("Cannot get latest block batch")
	}
	txBatch, ok := response.Result().(*BlockBatchResponse)
	if !ok {
		return nil, nil, fmt.Errorf("Cannot parse block batch response")
	}
	return parseBlockBatchResponse(txBatch, c.signer)
}

// GetBlockBatch will return the block batch by batch index
func (c *Client) GetBlockBatch(index uint64) (*Batch, []*types.Block, error) {
	str := strconv.FormatUint(index, 10)
	response, err := c.client.R().
		SetResult(&BlockBatchResponse{}).
		SetPathParams(map[string]string{
			"index":   str,
			"chainId": c.chainID,
		}).
		Get("/batch/block/index/{index}/{chainId}")

	if err != nil {
		return nil, nil, fmt.Errorf("Cannot get block batch %d: %w", index, err)
	}
	blockBatch, ok := response.Result().(*BlockBatchResponse)
	if !ok {
		return nil, nil, fmt.Errorf("Cannot parse block batch response")
	}
	return parseBlockBatchResponse(blockBatch, c.signer)
}

// parseBlockBatchResponse will turn a BlockBatchResponse into a
// Batch and its corresponding types.Transactions
func parseBlockBatchResponse(blockBatch *BlockBatchResponse, signer *types.EIP155Signer) (*Batch, []*types.Block, error) {
	if blockBatch == nil || blockBatch.Batch == nil {
		return nil, nil, errElementNotFound
	}
	batch := blockBatch.Batch
	blocks := make([]*types.Block, len(blockBatch.Blocks))
	for i, block := range blockBatch.Blocks {
		blockNew, err := BatchedBlockToBlock(block, signer)
		if err != nil {
			return nil, nil, fmt.Errorf("Cannot parse block batch: %w", err)
		}
		blocks[i] = blockNew
	}
	return batch, blocks, nil
}

// GetLatestBlockBatchIndex returns the latest block batch index
func (c *Client) GetLatestBlockBatchIndex() (*uint64, error) {
	batch, _, err := c.GetLatestBlockBatch()
	if err != nil {
		return nil, err
	}
	index := batch.Index
	return &index, nil
}
