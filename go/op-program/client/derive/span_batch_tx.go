package derive

import (
	"fmt"
	"math/big"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/rlp"
)

type spanBatchTxData interface {
	txType() byte // returns the type ID
}

type spanBatchTx struct {
	inner spanBatchTxData
}

type spanBatchLegacyTxData struct {
	Value    *big.Int // wei amount
	GasPrice *big.Int // wei per gas
	Data     []byte
}

func (txData *spanBatchLegacyTxData) txType() byte { return 0 }

// Type returns the transaction type.
func (tx *spanBatchTx) Type() uint8 {
	return tx.inner.txType()
}

// setDecoded sets the inner transaction after decoding.
func (tx *spanBatchTx) setDecoded(inner spanBatchTxData, size uint64) {
	tx.inner = inner
}

// UnmarshalBinary decodes the canonical encoding of transactions.
// It supports legacy RLP transactions and EIP2718 typed transactions.
func (tx *spanBatchTx) UnmarshalBinary(b []byte) error {
	if len(b) > 0 && b[0] > 0x7f {
		// It's a legacy transaction.
		var data spanBatchLegacyTxData
		err := rlp.DecodeBytes(b, &data)
		if err != nil {
			return fmt.Errorf("failed to decode spanBatchLegacyTxData: %w", err)
		}
		tx.setDecoded(&data, uint64(len(b)))
		return nil
	}

	return fmt.Errorf("unsupported tx type: %d", b[0])
}

// convertToFullTx takes values and convert spanBatchTx to types.Transaction
func (tx *spanBatchTx) convertToFullTx(nonce, gas uint64, to *common.Address, chainID, R, S *big.Int, yParityBit byte) (*types.Transaction, error) {
	batchTxInner, ok := tx.inner.(*spanBatchLegacyTxData)
	if !ok {
		return nil, fmt.Errorf("invalid tx type: %d", tx.Type())
	}

	signer := types.NewEIP155Signer(chainID)

	var fullTx *types.Transaction
	if to == nil {
		fullTx = types.NewContractCreation(nonce, batchTxInner.Value, gas, big.NewInt(0), batchTxInner.Data)
	} else {
		fullTx = types.NewTransaction(nonce, *to, batchTxInner.Value, gas, big.NewInt(0), batchTxInner.Data)
	}

	rawSig := make([]byte, 65)
	copy(rawSig[:32], R.Bytes())
	copy(rawSig[32:64], S.Bytes())

	// the last byte of the sig should be 0 or 1, we need to use the yParity bit here
	rawSig[64] = yParityBit

	return fullTx.WithSignature(signer, rawSig)
}
