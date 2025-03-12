package contracts

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum-optimism/optimism/op-service/sources/batching"
	"github.com/ethereum-optimism/optimism/op-service/sources/batching/rpcblock"
	"github.com/ethereum-optimism/optimism/op-service/txmgr"
	"github.com/ethereum/go-ethereum/common"

	"github.com/ethereum-optimism/optimism/go/op-challenger/abi"
)

var (
	methodDelayedWMetisDelay        = "delay"
	methodDelayedWMetisWithdrawals  = "withdrawals"
	methodDelayedWMetisDeposit      = "deposit"
	methodDelayedWMetisApprove      = "approve"
	methodDelayedWMetisAllowance    = "allowance"
	methodDelayedWMetisBalanceOf    = "balanceOf"
	methodDelayedWMetisTransfer     = "transfer"
	methodDelayedWMetisTransferFrom = "transferFrom"
	methodDelayedWMetisMetisToken   = "metisToken"
	methodDelayedWMetisHold         = "hold"
	methodDelayedWMetisUnlock       = "unlock"
	methodDelayedWMetisWithdraw     = "withdraw"
)

type WithdrawalRequest struct {
	Amount    *big.Int
	Timestamp *big.Int
}

type WMetisContract interface {
	GetMetis(ctx context.Context) (MetisContract, error)
}

type DelayedWMetisContract struct {
	multiCaller *batching.MultiCaller
	contract    *batching.BoundContract
	addr        common.Address
	from        common.Address
}

func NewDelayedWMetisContract(addr common.Address, caller *batching.MultiCaller, from common.Address) *DelayedWMetisContract {
	contractAbi := abi.LoadDelayedWMetisABI()
	return &DelayedWMetisContract{
		multiCaller: caller,
		contract:    batching.NewBoundContract(contractAbi, addr),
		addr:        addr,
		from:        from,
	}
}

func (d *DelayedWMetisContract) Addr() common.Address {
	return d.addr
}

func (d *DelayedWMetisContract) GetBalanceAndDelay(ctx context.Context, block rpcblock.Block) (*big.Int, time.Duration, error) {
	results, err := d.multiCaller.Call(ctx, block,
		d.contract.Call(methodDelayedWMetisBalanceOf, d.from),
		d.contract.Call(methodDelayedWMetisDelay))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get balance and delay: %w", err)
	}
	if len(results) != 2 {
		return nil, 0, fmt.Errorf("expected 2 results but got %v", len(results))
	}
	balance := results[0].GetBigInt(0)
	delay := time.Duration(results[1].GetUint64(0)) * time.Second
	return balance, delay, nil
}

func (d *DelayedWMetisContract) GetAllowance(ctx context.Context, block rpcblock.Block, spender common.Address) (*big.Int, error) {
	result, err := d.multiCaller.SingleCall(ctx, block,
		d.contract.Call(methodDelayedWMetisAllowance, d.from, spender))
	if err != nil {
		return nil, fmt.Errorf("failed to get allowance: %w", err)
	}
	return result.GetBigInt(0), nil
}

func (d *DelayedWMetisContract) ApproveTx(spender common.Address, amount *big.Int) (txmgr.TxCandidate, error) {
	return d.contract.Call(methodDelayedWMetisApprove, spender, amount).ToTxCandidate()
}

func (d *DelayedWMetisContract) DepositTx(amount *big.Int) (txmgr.TxCandidate, error) {
	return d.contract.Call(methodDelayedWMetisDeposit, amount).ToTxCandidate()
}

func (d *DelayedWMetisContract) TransferTx(to common.Address, amount *big.Int) (txmgr.TxCandidate, error) {
	return d.contract.Call(methodDelayedWMetisTransfer, to, amount).ToTxCandidate()
}

func (d *DelayedWMetisContract) TransferFromTx(from common.Address, to common.Address, amount *big.Int) (txmgr.TxCandidate, error) {
	return d.contract.Call(methodDelayedWMetisTransferFrom, from, to, amount).ToTxCandidate()
}

func (d *DelayedWMetisContract) GetWithdrawalRequest(ctx context.Context, block rpcblock.Block, recipient common.Address) (*WithdrawalRequest, error) {
	result, err := d.multiCaller.SingleCall(ctx, block,
		d.contract.Call(methodDelayedWMetisWithdrawals, d.from, recipient))
	if err != nil {
		return nil, fmt.Errorf("failed to get withdrawal request: %w", err)
	}
	return &WithdrawalRequest{
		Amount:    result.GetBigInt(0),
		Timestamp: result.GetBigInt(1),
	}, nil
}

func (d *DelayedWMetisContract) GetWithdrawals(ctx context.Context, block rpcblock.Block, game common.Address, recipients ...common.Address) ([]*WithdrawalRequest, error) {
	calls := make([]batching.Call, 0, len(recipients))
	for _, recipient := range recipients {
		calls = append(calls, d.contract.Call(methodDelayedWMetisWithdrawals, game, recipient))
	}
	results, err := d.multiCaller.Call(ctx, block, calls...)
	if err != nil {
		return nil, fmt.Errorf("failed to get withdrawals: %w", err)
	}

	withdrawals := make([]*WithdrawalRequest, 0, len(recipients))
	for _, result := range results {
		withdrawals = append(withdrawals, &WithdrawalRequest{
			Amount:    result.GetBigInt(0),
			Timestamp: result.GetBigInt(1),
		})
	}
	return withdrawals, nil
}

func (d *DelayedWMetisContract) GetMetis(ctx context.Context) (MetisContract, error) {
	result, err := d.multiCaller.SingleCall(ctx, rpcblock.Latest,
		d.contract.Call(methodDelayedWMetisMetisToken))
	if err != nil {
		return nil, fmt.Errorf("failed to get Metis token address: %w", err)
	}
	addr := result.GetAddress(0)
	return NewMetisTokenContract(addr, d.multiCaller, d.from), nil
}

func (d *DelayedWMetisContract) HoldTx(guy common.Address, amount *big.Int) (txmgr.TxCandidate, error) {
	return d.contract.Call(methodDelayedWMetisHold, guy, amount).ToTxCandidate()
}

func (d *DelayedWMetisContract) UnlockTx(guy common.Address, amount *big.Int) (txmgr.TxCandidate, error) {
	return d.contract.Call(methodDelayedWMetisUnlock, guy, amount).ToTxCandidate()
}

func (d *DelayedWMetisContract) WithdrawTx(amount *big.Int) (txmgr.TxCandidate, error) {
	return d.contract.Call(methodDelayedWMetisWithdraw, amount).ToTxCandidate()
}

func (d *DelayedWMetisContract) WithdrawToTx(guy common.Address, amount *big.Int) (txmgr.TxCandidate, error) {
	return d.contract.Call(methodDelayedWMetisWithdraw, guy, amount).ToTxCandidate()
}
