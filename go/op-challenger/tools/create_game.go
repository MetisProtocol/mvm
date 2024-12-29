package tools

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum-optimism/optimism/op-service/txmgr"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/ethereum-optimism/optimism/go/op-challenger/game/fault/contracts"
)

type GameCreator struct {
	contract *contracts.DisputeGameFactoryContract
	txMgr    txmgr.TxManager
}

func NewGameCreator(contract *contracts.DisputeGameFactoryContract, txMgr txmgr.TxManager) *GameCreator {
	return &GameCreator{
		contract: contract,
		txMgr:    txMgr,
	}
}

func (g *GameCreator) CreateGame(ctx context.Context, outputRoot common.Hash, traceType uint64, l2BlockNum uint64) (common.Address, error) {
	txCandidate, err := g.contract.CreateTx(ctx, uint32(traceType), outputRoot, l2BlockNum)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to create tx: %w", err)
	}

	rct, err := g.txMgr.Send(ctx, txCandidate)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to send tx: %w", err)
	}
	if rct.Status != types.ReceiptStatusSuccessful {
		return common.Address{}, fmt.Errorf("game creation transaction (%v) reverted", rct.TxHash.Hex())
	}

	gameAddr, _, _, err := g.contract.DecodeDisputeGameCreatedLog(rct)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to decode game created: %w", err)
	}
	return gameAddr, nil
}

func (g *GameCreator) CreateDispute(ctx context.Context, traceType uint64, l2BlockNum uint64) (uint32, *big.Int, *big.Int, error) {
	txCandidate, err := g.contract.CreateDisputeTx(ctx, uint32(traceType), l2BlockNum)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("failed to create tx: %w", err)
	}

	rct, err := g.txMgr.Send(ctx, txCandidate)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("failed to send tx: %w", err)
	}
	if rct.Status != types.ReceiptStatusSuccessful {
		return 0, nil, nil, fmt.Errorf("game creation transaction (%v) reverted", rct.TxHash.Hex())
	}

	_, gameType, bond, extraData, err := g.contract.DecodeDisputeGameRequestedLog(rct)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("failed to decode game created: %w", err)
	}
	return gameType, bond, new(big.Int).SetBytes(extraData), nil
}
