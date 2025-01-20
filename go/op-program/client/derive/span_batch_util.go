package derive

import (
	"bytes"
	"fmt"
	"io"
	"math/big"
)

// decodeSpanBatchBits decodes a standard span-batch bitlist.
// The bitlist is encoded as big-endian integer, left-padded with zeroes to a multiple of 8 bits.
// The encoded bitlist cannot be longer than MaxSpanBatchSize.
func decodeSpanBatchBits(r *bytes.Reader, bitLength uint64) (*big.Int, error) {
	// Round up, ensure enough bytes when number of bits is not a multiple of 8.
	// Alternative of (L+7)/8 is not overflow-safe.
	bufLen := bitLength / 8
	if bitLength%8 != 0 {
		bufLen++
	}
	buf := make([]byte, bufLen)
	_, err := io.ReadFull(r, buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read bits: %w", err)
	}
	out := new(big.Int)
	out.SetBytes(buf)
	// We read the correct number of bytes, but there may still be trailing bits
	if l := uint64(out.BitLen()); l > bitLength {
		return nil, fmt.Errorf("bitfield has %d bits, but expected no more than %d", l, bitLength)
	}
	return out, nil
}
