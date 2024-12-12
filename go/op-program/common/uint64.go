package common

import (
	"strconv"

	"github.com/MetisProtocol/mvm/l2geth/common/hexutil"
)

type Uint64 uint64

func (u Uint64) EncodeHex() string {
	enc := make([]byte, 0, 10)
	enc = strconv.AppendUint(enc, uint64(u), 16)
	for len(enc) < 8 {
		enc = append([]byte{'0'}, enc...)
	}
	return string(append([]byte("0x"), enc...))
}

func DecodeUint64Hint(input string) (uint64, error) {
	if len(input) == 0 {
		return 0, hexutil.ErrEmptyString
	}

	// remove leading zeros
	for len(input) > 1 && input[0] == '0' {
		input = input[1:]
	}

	return strconv.ParseUint(input, 16, 64)
}

func has0xPrefix(input string) bool {
	return len(input) >= 2 && input[0] == '0' && (input[1] == 'x' || input[1] == 'X')
}
