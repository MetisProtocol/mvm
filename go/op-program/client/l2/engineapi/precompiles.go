// This file contains code of the upstream go-ethereum kzgPointEvaluation implementation.
// Modifications have been made, primarily to substitute kzgPointEvaluation, ecrecover, and runBn256Pairing
// functions to interact with the preimage oracle.
//
// Original copyright disclaimer, applicable only to this file:
// -------------------------------------------------------------------
// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package engineapi

import (
	"bytes"
	"errors"
	"math/big"

	ethcommon "github.com/ethereum/go-ethereum/common"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/core/vm"
	"github.com/MetisProtocol/mvm/l2geth/crypto"
	"github.com/MetisProtocol/mvm/l2geth/params"
)

var (
	ecrecoverPrecompileAddress          = common.BytesToAddress([]byte{0x1})
	bn256PairingPrecompileAddress       = common.BytesToAddress([]byte{0x8})
	kzgPointEvaluationPrecompileAddress = common.BytesToAddress([]byte{0xa})
)

// PrecompileOracle defines the high-level API used to retrieve the result of a precompile call
// The caller is expected to validate the input to the precompile call
type PrecompileOracle interface {
	Precompile(address ethcommon.Address, input []byte, requiredGas uint64) ([]byte, bool)
}

func CreatePrecompileOverrides(precompileOracle PrecompileOracle) vm.PrecompileOverrides {
	if precompileOracle == nil {
		return nil
	}

	return func(rules params.Rules, orig vm.PrecompiledContract, address common.Address) vm.PrecompiledContract {
		if orig == nil { // Only override existing contracts. Never introduce a precompile that is not there.
			return nil
		}
		// NOTE: Ignoring chain rules for now. We assume that precompile behavior won't change for the foreseeable future
		switch address {
		case ecrecoverPrecompileAddress:
			return &ecrecoverOracle{Orig: orig, Oracle: precompileOracle}
		case bn256PairingPrecompileAddress:
			return &bn256PairingOracle{Orig: orig, Oracle: precompileOracle}
			// NOTE: we don't support the KZG precompile yet
			// case kzgPointEvaluationPrecompileAddress: return &kzgPointEvaluationOracle{Orig: orig, Oracle: precompileOracle}
		default:
			return orig
		}
	}
}

type ecrecoverOracle struct {
	Orig   vm.PrecompiledContract
	Oracle PrecompileOracle
}

func (c *ecrecoverOracle) RequiredGas(input []byte) uint64 {
	return c.Orig.RequiredGas(input)
}

func (c *ecrecoverOracle) Run(input []byte) ([]byte, error) {
	// Modification note: the L1 precompile behavior may change, but not in incompatible ways.
	// We want to enforce the subset that represents the EVM behavior activated in L2.
	// Below is a copy of the Cancun behavior. L1 might expand on that at a later point.

	const ecRecoverInputLength = 128

	input = common.RightPadBytes(input, ecRecoverInputLength)
	// "input" is (hash, v, r, s), each 32 bytes
	// but for ecrecover we want (r, s, v)

	r := new(big.Int).SetBytes(input[64:96])
	s := new(big.Int).SetBytes(input[96:128])
	v := input[63] - 27

	// tighter sig s values input homestead only apply to tx sigs
	if !allZero(input[32:63]) || !crypto.ValidateSignatureValues(v, r, s, false) {
		return nil, nil
	}
	// We must make sure not to modify the 'input', so placing the 'v' along with
	// the signature needs to be done on a new allocation
	sig := make([]byte, 65)
	copy(sig, input[64:128])
	sig[64] = v
	// v needs to be at the end for libsecp256k1

	// Modification note: below replaces the crypto.Ecrecover call
	result, ok := c.Oracle.Precompile(ethcommon.Address(ecrecoverPrecompileAddress), input, c.RequiredGas(input))
	if !ok {
		return nil, errors.New("invalid ecrecover input")
	}
	return result, nil
}

func allZero(b []byte) bool {
	for _, byte := range b {
		if byte != 0 {
			return false
		}
	}
	return true
}

type bn256PairingOracle struct {
	Orig   vm.PrecompiledContract
	Oracle PrecompileOracle
}

func (b *bn256PairingOracle) RequiredGas(input []byte) uint64 {
	return b.Orig.RequiredGas(input)
}

var (
	// true32Byte is returned if the bn256 pairing check succeeds.
	true32Byte = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}

	// false32Byte is returned if the bn256 pairing check fails.
	false32Byte = make([]byte, 32)

	// errBadPairingInput is returned if the bn256 pairing input is invalid.
	errBadPairingInput = errors.New("bad elliptic curve pairing size")
)

func (b *bn256PairingOracle) Run(input []byte) ([]byte, error) {
	// Handle some corner cases cheaply
	if len(input)%192 > 0 {
		return nil, errBadPairingInput
	}
	// Modification note: below replaces point verification and pairing checks
	// Assumes both L2 and the L1 oracle have an identical range of valid points
	result, ok := b.Oracle.Precompile(ethcommon.Address(bn256PairingPrecompileAddress), input, b.RequiredGas(input))
	if !ok {
		return nil, errors.New("invalid bn256Pairing check")
	}
	if !bytes.Equal(result, true32Byte) && !bytes.Equal(result, false32Byte) {
		panic("unexpected result from bn256Pairing check")
	}
	return result, nil
}
