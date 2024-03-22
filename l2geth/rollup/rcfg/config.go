package rcfg

import (
	"fmt"
	"math/big"
	"os"
	"strconv"

	"github.com/ethereum-optimism/optimism/l2geth/common"
)

// UsingOVM is used to enable or disable functionality necessary for the OVM.
var (
	UsingOVM               bool
	PeerHealthCheckSeconds int64
	ChainID                uint64
	DeSeqBlock             uint64
	SeqValidHeight         uint64
)

var (
	// l2GasPriceSlot refers to the storage slot that the L2 gas price is stored
	// in in the OVM_GasPriceOracle predeploy
	L2GasPriceSlot = common.BigToHash(big.NewInt(1))
	// l1GasPriceSlot refers to the storage slot that the L1 gas price is stored
	// in in the OVM_GasPriceOracle predeploy
	L1GasPriceSlot = common.BigToHash(big.NewInt(2))
	// l2GasPriceOracleOwnerSlot refers to the storage slot that the owner of
	// the OVM_GasPriceOracle is stored in
	L2GasPriceOracleOwnerSlot = common.BigToHash(big.NewInt(0))
	// l2GasPriceOracleAddress is the address of the OVM_GasPriceOracle
	// predeploy
	L2GasPriceOracleAddress = common.HexToAddress("0x420000000000000000000000000000000000000F")
	// OverheadSlot refers to the storage slot in the OVM_GasPriceOracle that
	// holds the per transaction overhead. This is added to the L1 cost portion
	// of the fee
	OverheadSlot = common.BigToHash(big.NewInt(3))
	// ScalarSlot refers to the storage slot in the OVM_GasPriceOracle that
	// holds the transaction fee scalar. This value is scaled upwards by
	// the number of decimals
	ScalarSlot = common.BigToHash(big.NewInt(4))
	// DecimalsSlot refers to the storage slot in the OVM_GasPriceOracle that
	// holds the number of decimals in the fee scalar
	DecimalsSlot = common.BigToHash(big.NewInt(5))
	// DefaultSeqAdderss refers to the sequencer address before MPC enabled
	DefaultSeqAdderss = common.HexToAddress("0x3525fdb496c612e4cDe817A2567081470b7a2Ecb")
)

func init() {
	UsingOVM = os.Getenv("USING_OVM") == "true"

	deseqHeight := os.Getenv("DESEQBLOCK")
	if deseqHeight == "" {
		DeSeqBlock = ^uint64(0)
	} else {
		parsed, err := strconv.ParseUint(deseqHeight, 0, 64)
		if err != nil {
			panic(err)
		}
		DeSeqBlock = parsed
	}

	peerHealthCheck := os.Getenv("PEER_HEALTH_CHECK")
	if peerHealthCheck == "" {
		PeerHealthCheckSeconds = ^int64(0)
	} else {
		parsed, err := strconv.ParseInt(peerHealthCheck, 10, 64)
		if err != nil {
			panic(err)
		}
		PeerHealthCheckSeconds = parsed
	}

	envChainID := os.Getenv("CHAIN_ID")
	if envChainID == "" {
		ChainID = ^uint64(0)
	} else {
		parsed, err := strconv.ParseUint(envChainID, 0, 64)
		if err != nil {
			panic(err)
		}
		ChainID = parsed
	}

	envSvh := os.Getenv("SEQSET_VALID_HEIGHT")
	if envSvh == "" {
		SeqValidHeight = ^uint64(0)
	} else {
		parsed, err := strconv.ParseUint(envSvh, 0, 64)
		if err != nil {
			panic(err)
		}
		SeqValidHeight = parsed
	}

	// for testing
	if defSeqAddr := os.Getenv("SEQSET_FIRST_SEQUENCER"); defSeqAddr != "" {
		if addr := common.HexToAddress(defSeqAddr); !addr.IsZero() {
			DefaultSeqAdderss = addr
		}
	}

	fmt.Println("rcfg UsingOVM ", UsingOVM, " envChainID ", envChainID, "envSeqValidHeight", envSvh, "defaultSeqAdderss", DefaultSeqAdderss.Hex())
}
