package ethapi

import (
	"errors"
	"fmt"

	"github.com/ethereum-optimism/optimism/l2geth/accounts/abi"
	"github.com/ethereum-optimism/optimism/l2geth/common/hexutil"
)

// revertError is an API error that encompasses an EVM revert with JSON error
// code and a binary data blob.
type revertError struct {
	error
	reason string // revert reason hex encoded
}

// ErrorCode returns the JSON error code for a revert.
// See: https://github.com/ethereum/wiki/wiki/JSON-RPC-Error-Codes-Improvement-Proposal
func (e *revertError) ErrorCode() int {
	return 3
}

// ErrorData returns the hex encoded revert reason.
func (e *revertError) ErrorData() interface{} {
	return e.reason
}

// newRevertError creates a revertError instance with the provided revert data.
func newRevertError(revert []byte) *revertError {
	err := errors.New("execution reverted")

	reason, errUnpack := abi.UnpackRevert(revert)
	if errUnpack == nil {
		err = fmt.Errorf("execution reverted: %s", reason)
	}
	return &revertError{
		error:  err,
		reason: hexutil.Encode(revert),
	}
}
