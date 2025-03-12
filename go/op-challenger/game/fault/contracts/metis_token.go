package contracts

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum-optimism/optimism/op-service/sources/batching"
	"github.com/ethereum-optimism/optimism/op-service/sources/batching/rpcblock"
	"github.com/ethereum-optimism/optimism/op-service/txmgr"
	"github.com/ethereum/go-ethereum/common"

	"github.com/ethereum-optimism/optimism/go/op-challenger/abi"
)

const (
	methodMetisAllowance    = "allowance"
	methodMetisApprove      = "approve"
	methodMetisBalanceOf    = "balanceOf"
	methodMetisTransfer     = "transfer"
	methodMetisTransferFrom = "transferFrom"
	methodMetisDecimals     = "decimals"
	methodMetisName         = "name"
	methodMetisSymbol       = "symbol"
	methodMetisTotalSupply  = "totalSupply"
)

var (
	maxUint256 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
)

type MetisContract interface {
	Addr() common.Address
	GetAllowanceAndBalance(ctx context.Context, block rpcblock.Block, owner common.Address, spender common.Address) (*big.Int, *big.Int, error)
	ApproveWithMaxAllowanceTx(spender common.Address) (txmgr.TxCandidate, error)
}

type MetisTokenContract struct {
	multiCaller *batching.MultiCaller
	contract    *batching.BoundContract
	addr        common.Address
	from        common.Address
}

func NewMetisTokenContract(addr common.Address, caller *batching.MultiCaller, from common.Address) *MetisTokenContract {
	contractAbi := abi.LoadMetisTokenABI()
	return &MetisTokenContract{
		multiCaller: caller,
		contract:    batching.NewBoundContract(contractAbi, addr),
		addr:        addr,
		from:        from,
	}
}

func (m *MetisTokenContract) Addr() common.Address {
	return m.addr
}

func (m *MetisTokenContract) GetAllowanceAndBalance(ctx context.Context, block rpcblock.Block, owner common.Address, spender common.Address) (*big.Int, *big.Int, error) {
	results, err := m.multiCaller.Call(ctx, block,
		m.contract.Call(methodMetisAllowance, owner, spender),
		m.contract.Call(methodMetisBalanceOf, owner))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get allowance: %w", err)
	}
	return results[0].GetBigInt(0), results[1].GetBigInt(0), nil
}

func (m *MetisTokenContract) GetAllowance(ctx context.Context, block rpcblock.Block, owner common.Address, spender common.Address) (*big.Int, error) {
	result, err := m.multiCaller.SingleCall(ctx, block,
		m.contract.Call(methodMetisAllowance, owner, spender))
	if err != nil {
		return nil, fmt.Errorf("failed to get allowance: %w", err)
	}
	return result.GetBigInt(0), nil
}

func (m *MetisTokenContract) GetBalanceOf(ctx context.Context, block rpcblock.Block, account common.Address) (*big.Int, error) {
	result, err := m.multiCaller.SingleCall(ctx, block,
		m.contract.Call(methodMetisBalanceOf, account))
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}
	return result.GetBigInt(0), nil
}

func (m *MetisTokenContract) ApproveTx(spender common.Address, amount *big.Int) (txmgr.TxCandidate, error) {
	return m.contract.Call(methodMetisApprove, spender, amount).ToTxCandidate()
}

func (m *MetisTokenContract) ApproveWithMaxAllowanceTx(spender common.Address) (txmgr.TxCandidate, error) {
	return m.contract.Call(methodMetisApprove, spender, maxUint256).ToTxCandidate()
}

func (m *MetisTokenContract) TransferTx(to common.Address, amount *big.Int) (txmgr.TxCandidate, error) {
	return m.contract.Call(methodMetisTransfer, to, amount).ToTxCandidate()
}

func (m *MetisTokenContract) TransferFromTx(from common.Address, to common.Address, amount *big.Int) (txmgr.TxCandidate, error) {
	return m.contract.Call(methodMetisTransferFrom, from, to, amount).ToTxCandidate()
}

func (m *MetisTokenContract) GetDecimals(ctx context.Context) (uint8, error) {
	result, err := m.multiCaller.SingleCall(ctx, rpcblock.Latest,
		m.contract.Call(methodMetisDecimals))
	if err != nil {
		return 0, fmt.Errorf("failed to get decimals: %w", err)
	}
	return result.GetUint8(0), nil
}

func (m *MetisTokenContract) GetName(ctx context.Context) (string, error) {
	result, err := m.multiCaller.SingleCall(ctx, rpcblock.Latest,
		m.contract.Call(methodMetisName))
	if err != nil {
		return "", fmt.Errorf("failed to get name: %w", err)
	}
	return result.GetString(0), nil
}

func (m *MetisTokenContract) GetSymbol(ctx context.Context) (string, error) {
	result, err := m.multiCaller.SingleCall(ctx, rpcblock.Latest,
		m.contract.Call(methodMetisSymbol))
	if err != nil {
		return "", fmt.Errorf("failed to get symbol: %w", err)
	}
	return result.GetString(0), nil
}

func (m *MetisTokenContract) GetTotalSupply(ctx context.Context) (*big.Int, error) {
	result, err := m.multiCaller.SingleCall(ctx, rpcblock.Latest,
		m.contract.Call(methodMetisTotalSupply))
	if err != nil {
		return nil, fmt.Errorf("failed to get total supply: %w", err)
	}
	return result.GetBigInt(0), nil
}
