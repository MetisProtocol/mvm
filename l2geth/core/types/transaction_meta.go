/**
 * Optimism 2020 Copyright
 */

package types

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math/big"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/rlp"
	"github.com/MetisProtocol/mvm/l2geth/rollup/rcfg"
)

type QueueOrigin uint8

const (
	// Possible `queue_origin` values
	QueueOriginSequencer QueueOrigin = 0
	QueueOriginL1ToL2    QueueOrigin = 1
)

func (q QueueOrigin) String() string {
	switch q {
	case QueueOriginSequencer:
		return "sequencer"
	case QueueOriginL1ToL2:
		return "l1"
	default:
		return ""
	}
}

func (q *QueueOrigin) UnmarshalJSON(b []byte) error {
	switch string(b) {
	case "\"sequencer\"":
		*q = QueueOriginSequencer
		return nil
	case "\"l1\"":
		*q = QueueOriginL1ToL2
		return nil
	default:
		return fmt.Errorf("Unknown QueueOrigin: %q", b)
	}
}

//go:generate gencodec -type TransactionMeta -out gen_tx_meta_json.go

type TransactionMeta struct {
	L1BlockNumber   *big.Int        `json:"l1BlockNumber"`
	L1Timestamp     uint64          `json:"l1Timestamp"`
	L1MessageSender *common.Address `json:"l1MessageSender" gencodec:"required"`
	QueueOrigin     QueueOrigin     `json:"queueOrigin" gencodec:"required"`
	// The canonical transaction chain index
	Index *uint64 `json:"index" gencodec:"required"`
	// The queue index, nil for queue origin sequencer transactions
	QueueIndex     *uint64 `json:"queueIndex" gencodec:"required"`
	RawTransaction []byte  `json:"rawTransaction" gencodec:"required"`

	R *big.Int `json:"seqR"`
	S *big.Int `json:"seqS"`
	V *big.Int `json:"seqV"`
}

// TransactionMeta version before MPC, as V1
// EncodeRLP of blocks V1 will use this
// TransactionMeta.R is nil
type TransactionMetaV1 struct {
	L1BlockNumber   *big.Int        `json:"l1BlockNumber"`
	L1Timestamp     uint64          `json:"l1Timestamp"`
	L1MessageSender *common.Address `json:"l1MessageSender" gencodec:"required"`
	QueueOrigin     QueueOrigin     `json:"queueOrigin" gencodec:"required"`
	// The canonical transaction chain index
	Index *uint64 `json:"index" gencodec:"required"`
	// The queue index, nil for queue origin sequencer transactions
	QueueIndex     *uint64 `json:"queueIndex" gencodec:"required"`
	RawTransaction []byte  `json:"rawTransaction" gencodec:"required"`
}

// TransactionMeta version from MPC, as V2
// EncodeRLP of blocks V2 will use this
// TransactionMeta.R is not nil, *R should be 0 or other big.Int
type TransactionMetaV2 struct {
	L1BlockNumber   *big.Int        `json:"l1BlockNumber"`
	L1Timestamp     uint64          `json:"l1Timestamp"`
	L1MessageSender *common.Address `json:"l1MessageSender" gencodec:"required"`
	QueueOrigin     QueueOrigin     `json:"queueOrigin" gencodec:"required"`
	// The canonical transaction chain index
	Index *uint64 `json:"index" gencodec:"required"`
	// The queue index, nil for queue origin sequencer transactions
	QueueIndex     *uint64 `json:"queueIndex" gencodec:"required"`
	RawTransaction []byte  `json:"rawTransaction" gencodec:"required"`

	R *big.Int `json:"seqR"`
	S *big.Int `json:"seqS"`
	V *big.Int `json:"seqV"`
}

// DecodeRLP implements rlp.Decoder
func (tm *TransactionMeta) DecodeRLP(s *rlp.Stream) error {
	return s.DecodeTxMeta(tm)
}

// EncodeRLP implements rlp.Encoder
func (tm *TransactionMeta) EncodeRLP(w io.Writer) error {
	// V1, before MPC upgrade
	if tm.R == nil {
		err := rlp.Encode(w, &TransactionMetaV1{
			L1BlockNumber:   tm.L1BlockNumber,
			L1Timestamp:     tm.L1Timestamp,
			L1MessageSender: tm.L1MessageSender,
			QueueOrigin:     tm.QueueOrigin,
			Index:           tm.Index,
			QueueIndex:      tm.QueueIndex,
			RawTransaction:  tm.RawTransaction,
		})
		return err
	}
	// V2, from MPC upgrade
	err := rlp.Encode(w, &TransactionMetaV2{
		L1BlockNumber:   tm.L1BlockNumber,
		L1Timestamp:     tm.L1Timestamp,
		L1MessageSender: tm.L1MessageSender,
		QueueOrigin:     tm.QueueOrigin,
		Index:           tm.Index,
		QueueIndex:      tm.QueueIndex,
		RawTransaction:  tm.RawTransaction,
		R:               tm.R,
		S:               tm.S,
		V:               tm.V,
	})
	return err
}

// NewTransactionMeta creates a TransactionMeta
func NewTransactionMeta(l1BlockNumber *big.Int, l1timestamp uint64, l1MessageSender *common.Address, queueOrigin QueueOrigin, index *uint64, queueIndex *uint64, rawTransaction []byte) *TransactionMeta {
	return &TransactionMeta{
		L1BlockNumber:   l1BlockNumber,
		L1Timestamp:     l1timestamp,
		L1MessageSender: l1MessageSender,
		QueueOrigin:     queueOrigin,
		Index:           index,
		QueueIndex:      queueIndex,
		RawTransaction:  rawTransaction,
	}
}

// TxMetaDecode deserializes bytes as a TransactionMeta struct.
// The schema is:
//
//	varbytes(L1BlockNumber) ||
//	varbytes(L1MessageSender) ||
//	varbytes(QueueOrigin) ||
//	varbytes(L1Timestamp)
func TxMetaDecode(input []byte) (*TransactionMeta, error) {
	var err error
	meta := TransactionMeta{}
	b := bytes.NewReader(input)

	lb, err := common.ReadVarBytes(b, 0, 1024, "l1BlockNumber")
	if err != nil {
		return nil, err
	}
	if !isNullValue(lb) {
		l1BlockNumber := new(big.Int).SetBytes(lb)
		meta.L1BlockNumber = l1BlockNumber
	}

	mb, err := common.ReadVarBytes(b, 0, 1024, "L1MessageSender")
	if err != nil {
		return nil, err
	}
	if !isNullValue(mb) {
		var l1MessageSender common.Address
		binary.Read(bytes.NewReader(mb), binary.LittleEndian, &l1MessageSender)
		meta.L1MessageSender = &l1MessageSender
	}

	qo, err := common.ReadVarBytes(b, 0, 1024, "QueueOrigin")
	if err != nil {
		return nil, err
	}
	if !isNullValue(qo) {
		queueOrigin := new(big.Int).SetBytes(qo)
		meta.QueueOrigin = QueueOrigin(queueOrigin.Uint64())
	}

	l, err := common.ReadVarBytes(b, 0, 1024, "L1Timestamp")
	if err != nil {
		return nil, err
	}
	var l1Timestamp uint64
	binary.Read(bytes.NewReader(l), binary.LittleEndian, &l1Timestamp)
	meta.L1Timestamp = l1Timestamp

	i, err := common.ReadVarBytes(b, 0, 1024, "Index")
	if err != nil {
		return nil, err
	}
	if !isNullValue(i) {
		index := new(big.Int).SetBytes(i).Uint64()
		meta.Index = &index
	}

	qi, err := common.ReadVarBytes(b, 0, 1024, "QueueIndex")
	if err != nil {
		return nil, err
	}
	if !isNullValue(qi) {
		queueIndex := new(big.Int).SetBytes(qi).Uint64()
		meta.QueueIndex = &queueIndex
	}

	raw, err := common.ReadVarBytes(b, 0, 130000, "RawTransaction")
	if err != nil {
		return nil, err
	}
	if !isNullValue(raw) {
		meta.RawTransaction = raw
	}

	if rcfg.SeqValidHeight > 0 && *meta.Index+1 >= rcfg.SeqValidHeight {
		// sequencer sign after mpc enabled height
		r, err := common.ReadVarBytes(b, 0, 1024, "R")
		if err != nil {
			return nil, err
		}
		if !isNullValue(r) {
			rSeq := new(big.Int).SetBytes(r)
			meta.R = rSeq
		}

		s, err := common.ReadVarBytes(b, 0, 1024, "S")
		if err != nil {
			return nil, err
		}
		if !isNullValue(r) {
			sSeq := new(big.Int).SetBytes(s)
			meta.S = sSeq
		}

		v, err := common.ReadVarBytes(b, 0, 1024, "V")
		if err != nil {
			return nil, err
		}
		if !isNullValue(v) {
			vSeq := new(big.Int).SetBytes(v)
			meta.V = vSeq
		}
	}

	return &meta, nil
}

// TxMetaEncode serializes the TransactionMeta as bytes.
func TxMetaEncode(meta *TransactionMeta) []byte {
	b := new(bytes.Buffer)

	L1BlockNumber := meta.L1BlockNumber
	if L1BlockNumber == nil {
		common.WriteVarBytes(b, 0, getNullValue())
	} else {
		l := new(bytes.Buffer)
		binary.Write(l, binary.LittleEndian, L1BlockNumber.Bytes())
		common.WriteVarBytes(b, 0, l.Bytes())
	}

	L1MessageSender := meta.L1MessageSender
	if L1MessageSender == nil {
		common.WriteVarBytes(b, 0, getNullValue())
	} else {
		l := new(bytes.Buffer)
		binary.Write(l, binary.LittleEndian, *L1MessageSender)
		common.WriteVarBytes(b, 0, l.Bytes())
	}

	queueOrigin := meta.QueueOrigin
	q := new(bytes.Buffer)
	binary.Write(q, binary.LittleEndian, queueOrigin)
	common.WriteVarBytes(b, 0, q.Bytes())

	l := new(bytes.Buffer)
	binary.Write(l, binary.LittleEndian, &meta.L1Timestamp)
	common.WriteVarBytes(b, 0, l.Bytes())

	index := meta.Index
	if index == nil {
		common.WriteVarBytes(b, 0, getNullValue())
	} else {
		i := new(bytes.Buffer)
		binary.Write(i, binary.LittleEndian, new(big.Int).SetUint64(*index).Bytes())
		common.WriteVarBytes(b, 0, i.Bytes())
	}

	queueIndex := meta.QueueIndex
	if queueIndex == nil {
		common.WriteVarBytes(b, 0, getNullValue())
	} else {
		qi := new(bytes.Buffer)
		binary.Write(qi, binary.LittleEndian, new(big.Int).SetUint64(*queueIndex).Bytes())
		common.WriteVarBytes(b, 0, qi.Bytes())
	}

	rawTransaction := meta.RawTransaction
	if rawTransaction == nil {
		common.WriteVarBytes(b, 0, getNullValue())
	} else {
		common.WriteVarBytes(b, 0, rawTransaction)
	}

	if rcfg.SeqValidHeight > 0 && *meta.Index+1 >= rcfg.SeqValidHeight {
		// sequencer sign after mpc enabled height
		rSeq := meta.R
		if rSeq == nil {
			common.WriteVarBytes(b, 0, getNullValue())
		} else {
			r := new(bytes.Buffer)
			binary.Write(r, binary.LittleEndian, rSeq.Bytes())
			common.WriteVarBytes(b, 0, r.Bytes())
		}

		sSeq := meta.S
		if sSeq == nil {
			common.WriteVarBytes(b, 0, getNullValue())
		} else {
			s := new(bytes.Buffer)
			binary.Write(s, binary.LittleEndian, sSeq.Bytes())
			common.WriteVarBytes(b, 0, s.Bytes())
		}

		vSeq := meta.V
		if vSeq == nil {
			common.WriteVarBytes(b, 0, getNullValue())
		} else {
			v := new(bytes.Buffer)
			binary.Write(v, binary.LittleEndian, vSeq.Bytes())
			common.WriteVarBytes(b, 0, v.Bytes())
		}
	}

	return b.Bytes()
}

// This may collide with a uint8
func isNullValue(b []byte) bool {
	nullValue := []byte{0x00}
	return bytes.Equal(b, nullValue)
}

func getNullValue() []byte {
	return []byte{0x00}
}
