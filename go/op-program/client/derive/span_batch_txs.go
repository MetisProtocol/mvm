package derive

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/common/hexutil"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/rlp"

	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/holiman/uint256"
)

type spanBatchTxs struct {
	// this field must be manually set
	totalBlockTxCount uint64

	// 8 fields
	contractCreationBits *big.Int // standard span-batch bitlist
	yParityBits          *big.Int // standard span-batch bitlist
	txSigs               []spanBatchSignature
	txNonces             []uint64
	txGases              []uint64
	txTos                []common.Address
	txDatas              []hexutil.Bytes
	protectedBits        *big.Int // standard span-batch bitlist

	// metis fields
	queueOriginBits *big.Int
	l1TxOrigins     []common.Address
	txSeqSigs       []spanBatchSignature
	seqYParityBits  *big.Int

	l1BlockNumbers []uint64
	l1Timestamps   []uint64
	blockTxCounts  []uint64

	totalEnqueueTxCount uint64

	// intermediate variables which can be recovered
	txTypes            []int
	totalLegacyTxCount uint64
}

type spanBatchSignature struct {
	v uint64
	r *uint256.Int
	s *uint256.Int
}

func (btx *spanBatchTxs) decodeContractCreationBits(r *bytes.Reader) error {
	if btx.totalBlockTxCount > derive.MaxSpanBatchElementCount {
		return derive.ErrTooBigSpanBatchSize
	}
	bits, err := decodeSpanBatchBits(r, btx.totalBlockTxCount)
	if err != nil {
		return fmt.Errorf("failed to decode contract creation bits: %w", err)
	}
	btx.contractCreationBits = bits
	return nil
}

func (btx *spanBatchTxs) decodeProtectedBits(r *bytes.Reader) error {
	if btx.totalLegacyTxCount > derive.MaxSpanBatchElementCount {
		return derive.ErrTooBigSpanBatchSize
	}
	bits, err := decodeSpanBatchBits(r, btx.totalLegacyTxCount)
	if err != nil {
		return fmt.Errorf("failed to decode protected bits: %w", err)
	}
	btx.protectedBits = bits
	return nil
}

func (btx *spanBatchTxs) decodeQueueOriginBits(r *bytes.Reader) error {
	if btx.totalLegacyTxCount > derive.MaxSpanBatchElementCount {
		return derive.ErrTooBigSpanBatchSize
	}
	bits, err := decodeSpanBatchBits(r, btx.totalBlockTxCount)
	if err != nil {
		return fmt.Errorf("failed to decode queue origin bits: %w", err)
	}
	btx.queueOriginBits = bits
	return nil
}

func (btx *spanBatchTxs) decodeSeqYParityBits(r *bytes.Reader) error {
	if btx.totalLegacyTxCount > derive.MaxSpanBatchElementCount {
		return derive.ErrTooBigSpanBatchSize
	}
	bits, err := decodeSpanBatchBits(r, btx.totalBlockTxCount)
	if err != nil {
		return fmt.Errorf("failed to decode seq y parity bits: %w", err)
	}
	btx.seqYParityBits = bits
	return nil
}

func (btx *spanBatchTxs) contractCreationCount() (uint64, error) {
	if btx.contractCreationBits == nil {
		return 0, errors.New("dev error: contract creation bits not set")
	}
	var result uint64 = 0
	for i := 0; i < int(btx.totalBlockTxCount); i++ {
		bit := btx.contractCreationBits.Bit(i)
		if bit == 1 {
			result++
		}
	}
	return result, nil
}

func (btx *spanBatchTxs) enqueueTxCount() (uint64, error) {
	if btx.queueOriginBits == nil {
		return 0, errors.New("dev error: contract creation bits not set")
	}
	var result uint64 = 0
	for i := 0; i < int(btx.totalBlockTxCount); i++ {
		bit := btx.queueOriginBits.Bit(i)
		if bit == 1 {
			result++
		}
	}
	return result, nil
}

func (btx *spanBatchTxs) decodeYParityBits(r *bytes.Reader) error {
	bits, err := decodeSpanBatchBits(r, btx.totalBlockTxCount)
	if err != nil {
		return fmt.Errorf("failed to decode y-parity bits: %w", err)
	}
	btx.yParityBits = bits
	return nil
}

func (btx *spanBatchTxs) decodeTxSigsRS(r *bytes.Reader) error {
	var txSigs []spanBatchSignature
	var sigBuffer [32]byte
	for i := 0; i < int(btx.totalBlockTxCount); i++ {
		var txSig spanBatchSignature
		_, err := io.ReadFull(r, sigBuffer[:])
		if err != nil {
			return fmt.Errorf("failed to read tx sig r: %w", err)
		}
		txSig.r, _ = uint256.FromBig(new(big.Int).SetBytes(sigBuffer[:]))
		_, err = io.ReadFull(r, sigBuffer[:])
		if err != nil {
			return fmt.Errorf("failed to read tx sig s: %w", err)
		}
		txSig.s, _ = uint256.FromBig(new(big.Int).SetBytes(sigBuffer[:]))
		txSigs = append(txSigs, txSig)
	}
	btx.txSigs = txSigs
	return nil
}

func (btx *spanBatchTxs) decodeTxSeqSigsRS(r *bytes.Reader) error {
	var txSeqSigs []spanBatchSignature
	var sigBuffer [32]byte
	for i := 0; i < int(btx.totalBlockTxCount); i++ {
		var txSig spanBatchSignature
		_, err := io.ReadFull(r, sigBuffer[:])
		if err != nil {
			return fmt.Errorf("failed to read tx sig r: %w", err)
		}
		txSig.r, _ = uint256.FromBig(new(big.Int).SetBytes(sigBuffer[:]))
		_, err = io.ReadFull(r, sigBuffer[:])
		if err != nil {
			return fmt.Errorf("failed to read tx sig s: %w", err)
		}
		txSig.s, _ = uint256.FromBig(new(big.Int).SetBytes(sigBuffer[:]))
		txSeqSigs = append(txSeqSigs, txSig)
	}
	btx.txSeqSigs = txSeqSigs
	return nil
}

func (btx *spanBatchTxs) decodeTxNonces(r *bytes.Reader) error {
	var txNonces []uint64
	for i := 0; i < int(btx.totalBlockTxCount); i++ {
		txNonce, err := binary.ReadUvarint(r)
		if err != nil {
			return fmt.Errorf("failed to read tx nonce: %w", err)
		}
		txNonces = append(txNonces, txNonce)
	}
	btx.txNonces = txNonces
	return nil
}

func (btx *spanBatchTxs) decodeTxGases(r *bytes.Reader) error {
	var txGases []uint64
	for i := 0; i < int(btx.totalBlockTxCount); i++ {
		txGas, err := binary.ReadUvarint(r)
		if err != nil {
			return fmt.Errorf("failed to read tx gas: %w", err)
		}
		txGases = append(txGases, txGas)
	}
	btx.txGases = txGases
	return nil
}

func (btx *spanBatchTxs) decodeTxTos(r *bytes.Reader) error {
	var txTos []common.Address
	txToBuffer := make([]byte, common.AddressLength)
	contractCreationCount, err := btx.contractCreationCount()
	if err != nil {
		return err
	}
	for i := 0; i < int(btx.totalBlockTxCount-contractCreationCount); i++ {
		_, err := io.ReadFull(r, txToBuffer)
		if err != nil {
			return fmt.Errorf("failed to read tx to address: %w", err)
		}
		txTos = append(txTos, common.BytesToAddress(txToBuffer))
	}
	btx.txTos = txTos
	return nil
}

func (btx *spanBatchTxs) decodeL1TxOrigins(r *bytes.Reader) error {
	var txOrigins []common.Address
	l1TxOriginsBuffer := make([]byte, common.AddressLength)
	enqueueTxCount, err := btx.enqueueTxCount()
	if err != nil {
		return err
	}
	for i := 0; i < int(enqueueTxCount); i++ {
		_, err := io.ReadFull(r, l1TxOriginsBuffer)
		if err != nil {
			return fmt.Errorf("failed to read tx to address: %w", err)
		}
		txOrigins = append(txOrigins, common.BytesToAddress(l1TxOriginsBuffer))
	}
	btx.l1TxOrigins = txOrigins
	return nil
}

func (btx *spanBatchTxs) decodeTxDatas(r *bytes.Reader) error {
	var txDatas []hexutil.Bytes
	var txTypes []int
	// Do not need txDataHeader because RLP byte stream already includes length info
	for i := 0; i < int(btx.totalBlockTxCount); i++ {
		txData, txType, err := derive.ReadTxData(r)
		if err != nil {
			return err
		}
		txDatas = append(txDatas, txData)
		txTypes = append(txTypes, txType)
		if txType == 0 {
			btx.totalLegacyTxCount++
		}
	}
	btx.txDatas = txDatas
	btx.txTypes = txTypes
	return nil
}

func (btx *spanBatchTxs) recoverV(chainID *big.Int) error {
	if len(btx.txTypes) != len(btx.txSigs) {
		return errors.New("tx type length and tx sigs length mismatch")
	}
	if btx.protectedBits == nil {
		return errors.New("dev error: protected bits not set")
	}
	protectedBitsIdx := 0
	for idx, txType := range btx.txTypes {
		bit := uint64(btx.yParityBits.Bit(idx))
		var v uint64
		switch txType {
		case 0:
			protectedBit := btx.protectedBits.Bit(protectedBitsIdx)
			protectedBitsIdx++
			if protectedBit == 0 {
				v = 27 + bit
			} else {
				// EIP-155
				v = chainID.Uint64()*2 + 35 + bit
			}
		default:
			return fmt.Errorf("invalid tx type: %d", txType)
		}
		btx.txSigs[idx].v = v
	}
	return nil
}

func (btx *spanBatchTxs) decode(r *bytes.Reader) error {
	if err := btx.decodeContractCreationBits(r); err != nil {
		return err
	}
	if err := btx.decodeYParityBits(r); err != nil {
		return err
	}
	if err := btx.decodeTxSigsRS(r); err != nil {
		return err
	}
	if err := btx.decodeTxTos(r); err != nil {
		return err
	}
	if err := btx.decodeTxDatas(r); err != nil {
		return err
	}
	if err := btx.decodeTxNonces(r); err != nil {
		return err
	}
	if err := btx.decodeTxGases(r); err != nil {
		return err
	}
	if err := btx.decodeProtectedBits(r); err != nil {
		return err
	}
	if err := btx.decodeQueueOriginBits(r); err != nil {
		return err
	}
	if err := btx.decodeSeqYParityBits(r); err != nil {
		return err
	}
	if err := btx.decodeTxSeqSigsRS(r); err != nil {
		return err
	}
	if err := btx.decodeL1TxOrigins(r); err != nil {
		return err
	}
	return nil
}

func (btx *spanBatchTxs) fullTxs(chainID *big.Int, startBlock uint64) (types.Transactions, error) {
	var txs types.Transactions
	toIdx := 0
	enqueueTxIdx := 0
	blockCounter := uint64(0)
	for idx := 0; idx < int(btx.totalBlockTxCount); idx++ {
		idxBlockUpperBound := 0
		for blockTxCountIdx := range btx.blockTxCounts {
			idxBlockUpperBound += int(btx.blockTxCounts[blockTxCountIdx])
			if uint64(blockTxCountIdx) >= blockCounter {
				break
			}
		}

		if idx >= idxBlockUpperBound {
			blockCounter++
		}

		var stx spanBatchTx
		if err := stx.UnmarshalBinary(btx.txDatas[idx]); err != nil {
			return nil, err
		}
		nonce := btx.txNonces[idx]
		gas := btx.txGases[idx]
		var to *common.Address = nil
		bit := btx.contractCreationBits.Bit(idx)
		if bit == 0 {
			if len(btx.txTos) <= toIdx {
				return nil, errors.New("tx to not enough")
			}
			to = &btx.txTos[toIdx]
			toIdx++
		}

		var l1TxOrigin common.Address
		queueIndex := uint64(0)
		queueOrigin := types.QueueOrigin(btx.queueOriginBits.Bit(idx))
		if queueOrigin == types.QueueOriginL1ToL2 {
			if len(btx.l1TxOrigins) <= enqueueTxIdx {
				return nil, errors.New("l1 tx origins not enough")
			}
			l1TxOrigin = btx.l1TxOrigins[enqueueTxIdx]
			queueIndex = nonce
			enqueueTxIdx++
		}

		r := btx.txSigs[idx].r.ToBig()
		s := btx.txSigs[idx].s.ToBig()
		tx, err := stx.convertToFullTx(nonce, gas, to, chainID, r, s, byte(getBit(btx.yParityBits, idx)))
		if err != nil {
			return nil, err
		}

		// Note: set l2 tx to encode only the data part
		tx.SetL2Tx(2)
		encodedTxData, err := rlp.EncodeToBytes(tx)
		if err != nil {
			return nil, err
		}

		tx.SetL2Tx(0)
		batchBlockIndex := startBlock + blockCounter - 1
		txMeta := &types.TransactionMeta{
			L1BlockNumber:   new(big.Int).SetUint64(btx.l1BlockNumbers[blockCounter]),
			L1Timestamp:     btx.l1Timestamps[blockCounter],
			L1MessageSender: &l1TxOrigin,
			QueueOrigin:     queueOrigin,
			Index:           &batchBlockIndex,
			QueueIndex:      &queueIndex,
			RawTransaction:  encodedTxData,
			R:               btx.txSeqSigs[idx].r.ToBig(),
			S:               btx.txSeqSigs[idx].s.ToBig(),
			V:               big.NewInt(int64(btx.seqYParityBits.Bit(idx))),
		}
		tx.SetTransactionMeta(txMeta)

		// set l2 tx back
		tx.SetL2Tx(0)

		txs = append(txs, tx)
	}
	return txs, nil
}

func getBit(bits *big.Int, index int) uint {
	return bits.Bit(index)
}

func isProtectedV(v uint64, txType int) bool {
	if txType == 0 {
		// if EIP-155 applied, v = 2 * chainID + 35 + yParity
		return v != 27 && v != 28
	}
	// every non legacy tx are protected
	return true
}
