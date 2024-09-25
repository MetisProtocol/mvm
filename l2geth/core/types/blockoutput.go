package types

import (
	"errors"

	"github.com/ethereum-optimism/optimism/l2geth/common"
	"github.com/ethereum-optimism/optimism/l2geth/crypto"
)

type OutputResponse struct {
	Version               common.Hash `json:"version"`
	OutputRoot            common.Hash `json:"outputRoot"`
	BlockRef              L2BlockRef  `json:"blockRef"`
	WithdrawalStorageRoot common.Hash `json:"withdrawalStorageRoot"`
	StateRoot             common.Hash `json:"stateRoot"`
	Status                *SyncStatus `json:"syncStatus"`
}

type SafeHeadResponse struct {
	L1Block  BlockID `json:"l1Block"`
	SafeHead BlockID `json:"safeHead"`
}

var (
	ErrInvalidOutput        = errors.New("invalid output")
	ErrInvalidOutputVersion = errors.New("invalid output version")

	OutputVersionV0 = common.Hash{}
)

type Output interface {
	// Version returns the version of the L2 output
	Version() common.Hash

	// Marshal a L2 output into a byte slice for hashing
	Marshal() []byte
}

type OutputV0 struct {
	StateRoot                common.Hash
	MessagePasserStorageRoot common.Hash
	BlockHash                common.Hash
}

func (o *OutputV0) Version() common.Hash {
	return OutputVersionV0
}

func (o *OutputV0) Marshal() []byte {
	var buf [128]byte
	version := o.Version()
	copy(buf[:32], version[:])
	copy(buf[32:], o.StateRoot[:])
	copy(buf[64:], o.MessagePasserStorageRoot[:])
	copy(buf[96:], o.BlockHash[:])
	return buf[:]
}

// OutputRoot returns the keccak256 hash of the marshaled L2 output
func OutputRoot(output Output) common.Hash {
	marshaled := output.Marshal()
	return crypto.Keccak256Hash(marshaled)
}

func UnmarshalOutput(data []byte) (Output, error) {
	if len(data) < 32 {
		return nil, ErrInvalidOutput
	}
	var ver common.Hash
	copy(ver[:], data[:32])
	switch ver {
	case OutputVersionV0:
		return unmarshalOutputV0(data)
	default:
		return nil, ErrInvalidOutputVersion
	}
}

func unmarshalOutputV0(data []byte) (*OutputV0, error) {
	if len(data) != 128 {
		return nil, ErrInvalidOutput
	}
	var output OutputV0
	// data[:32] is the version
	copy(output.StateRoot[:], data[32:64])
	copy(output.MessagePasserStorageRoot[:], data[64:96])
	copy(output.BlockHash[:], data[96:128])
	return &output, nil
}
