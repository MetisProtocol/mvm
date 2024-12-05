package rollup

import (
	"encoding/hex"

	preimage "github.com/ethereum-optimism/optimism/go/op-preimage"
)

// Keccak256Key wraps a keccak256 hash to use it as a typed pre-image key.
type Keccak256Key [32]byte

func (k Keccak256Key) PreimageKey() (out [32]byte) {
	out = k                                      // copy the keccak hash
	out[0] = byte(preimage.GlobalGenericKeyType) // apply prefix
	return
}

func (k Keccak256Key) String() string {
	return "0x" + hex.EncodeToString(k[:])
}

func (k Keccak256Key) TerminalString() string {
	return "0x" + hex.EncodeToString(k[:])
}
