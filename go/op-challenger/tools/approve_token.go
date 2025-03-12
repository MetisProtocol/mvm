package tools

import (
	"context"
	"fmt"

	"github.com/ethereum-optimism/optimism/op-service/txmgr"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/ethereum-optimism/optimism/go/op-challenger/game/fault/contracts"
)

type TokenApprover struct {
	contract *contracts.MetisTokenContract
	txMgr    txmgr.TxManager
}

func NewTokenApprover(contract *contracts.MetisTokenContract, txMgr txmgr.TxManager) *TokenApprover {
	return &TokenApprover{
		contract: contract,
		txMgr:    txMgr,
	}
}

func (t *TokenApprover) ApproveToken(ctx context.Context, spender common.Address) error {
	txCandidate, err := t.contract.ApproveWithMaxAllowanceTx(spender)
	if err != nil {
		return err
	}

	rct, err := t.txMgr.Send(ctx, txCandidate)
	if err != nil {
		fmt.Errorf("failed to send tx: %w", err)
	}
	if rct.Status != types.ReceiptStatusSuccessful {
		return fmt.Errorf("approve token transaction (%v) reverted", rct.TxHash.Hex())
	}

	return nil
}
