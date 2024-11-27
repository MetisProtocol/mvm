package testutil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/big"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/ethereum-optimism/optimism/op-chain-ops/foundry"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/consensus"
	"github.com/MetisProtocol/mvm/l2geth/consensus/ethash"
	"github.com/MetisProtocol/mvm/l2geth/core"
	"github.com/MetisProtocol/mvm/l2geth/core/rawdb"
	"github.com/MetisProtocol/mvm/l2geth/core/state"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/core/vm"
	"github.com/MetisProtocol/mvm/l2geth/params"
)

type Artifacts struct {
	MIPS   *foundry.Artifact
	Oracle *foundry.Artifact
}

type Addresses struct {
	MIPS         common.Address
	Oracle       common.Address
	Sender       common.Address
	FeeRecipient common.Address
}

type ContractMetadata struct {
	Artifacts *Artifacts
	Addresses *Addresses
}

func TestContractsSetup(t require.TestingT, version MipsVersion) *ContractMetadata {
	artifacts, err := loadArtifacts(version)
	require.NoError(t, err)

	addrs := &Addresses{
		MIPS:         common.Address{0: 0xff, 19: 1},
		Oracle:       common.Address{0: 0xff, 19: 2},
		Sender:       common.Address{0x13, 0x37},
		FeeRecipient: common.Address{0xaa},
	}

	return &ContractMetadata{Artifacts: artifacts, Addresses: addrs}
}

// loadArtifacts loads the Cannon contracts, from the contracts package.
func loadArtifacts(version MipsVersion) (*Artifacts, error) {
	artifactFS := foundry.OpenArtifactsDir("../../../packages/contracts-bedrock/forge-artifacts")
	var mips *foundry.Artifact
	var err error
	switch version {
	case MipsSingleThreaded:
		mips, err = artifactFS.ReadArtifact("MIPS.sol", "MIPS")
	case MipsMultithreaded:
		mips, err = artifactFS.ReadArtifact("MIPS2.sol", "MIPS2")
	default:
		return nil, fmt.Errorf("Unknown MipsVersion supplied: %v", version)
	}
	if err != nil {
		return nil, err
	}

	oracle, err := artifactFS.ReadArtifact("PreimageOracle.sol", "PreimageOracle")
	if err != nil {
		return nil, err
	}

	return &Artifacts{
		MIPS:   mips,
		Oracle: oracle,
	}, nil
}

func NewEVMEnv(contracts *ContractMetadata) (*vm.EVM, *state.StateDB) {
	// Temporary hack until Cancun is activated on mainnet
	cpy := *params.MainnetChainConfig
	chainCfg := &cpy             // don't modify the global chain config
	offsetBlocks := uint64(1000) // blocks after cancun fork
	bc := &testChain{startTime: uint64(time.Now().Unix()) + offsetBlocks*12}
	header := bc.GetHeader(common.Hash{}, 17034870+offsetBlocks)
	db := rawdb.NewMemoryDatabase()
	statedb := state.NewDatabase(db)
	state, err := state.New(types.EmptyRootHash, statedb)
	if err != nil {
		panic(fmt.Errorf("failed to create memory state db: %w", err))
	}
	evmContext := core.NewEVMContext(types.NewMessage(
		contracts.Addresses.Sender,
		nil,
		0,
		big.NewInt(0),
		0,
		big.NewInt(0),
		nil,
		false,
		big.NewInt(0),
		0,
		types.QueueOriginSequencer,
	), header, bc, nil)
	vmCfg := vm.Config{}

	env := vm.NewEVM(evmContext, state, chainCfg, vmCfg)
	// pre-deploy the contracts
	env.StateDB.SetCode(contracts.Addresses.Oracle, contracts.Artifacts.Oracle.DeployedBytecode.Object)

	var mipsCtorArgs [32]byte
	copy(mipsCtorArgs[12:], contracts.Addresses.Oracle[:])
	mipsDeploy := append(bytes.Clone(contracts.Artifacts.MIPS.Bytecode.Object), mipsCtorArgs[:]...)
	startingGas := uint64(30_000_000)
	_, deployedMipsAddr, leftOverGas, err := env.Create(vm.AccountRef(contracts.Addresses.Sender), mipsDeploy, startingGas, big.NewInt(0))
	if err != nil {
		panic(fmt.Errorf("failed to deploy MIPS contract: %w. took %d gas", err, startingGas-leftOverGas))
	}
	contracts.Addresses.MIPS = deployedMipsAddr

	return env, state
}

type testChain struct {
	startTime uint64
}

func (d *testChain) Engine() consensus.Engine {
	return ethash.NewFullFaker()
}

func (d *testChain) GetHeader(h common.Hash, n uint64) *types.Header {
	parentHash := common.Hash{0: 0xff}
	binary.BigEndian.PutUint64(parentHash[1:], n-1)
	return &types.Header{
		ParentHash:  parentHash,
		UncleHash:   types.EmptyUncleHash,
		Coinbase:    common.Address{},
		Root:        common.Hash{},
		TxHash:      types.EmptyTxsHash,
		ReceiptHash: types.EmptyReceiptsHash,
		Bloom:       types.Bloom{},
		Difficulty:  big.NewInt(0),
		Number:      new(big.Int).SetUint64(n),
		GasLimit:    30_000_000,
		GasUsed:     0,
		Time:        d.startTime + n*12,
		Extra:       nil,
		MixDigest:   common.Hash{},
		Nonce:       types.BlockNonce{},
	}
}

func MarkdownTracer() vm.Tracer {
	return vm.NewStructLogger(&vm.LogConfig{})
}

// TODO: unfortunately, op-code level tracing is not yet supported by the l2geth tracer,
//       so we can't use it here.
//func SourceMapTracer(t require.TestingT, version MipsVersion, mips *foundry.Artifact, oracle *foundry.Artifact, addrs *Addresses) vm.Tracer {
//	srcFS := foundry.NewSourceMapFS(os.DirFS("../../../packages/contracts-bedrock"))
//	var mipsSrcMap *srcmap.SourceMap
//	var err error
//	switch version {
//	case MipsSingleThreaded:
//		mipsSrcMap, err = srcFS.SourceMap(mips, "MIPS")
//	case MipsMultithreaded:
//		mipsSrcMap, err = srcFS.SourceMap(mips, "MIPS2")
//	default:
//		require.Fail(t, "invalid mips version")
//	}
//	require.NoError(t, err)
//	oracleSrcMap, err := srcFS.SourceMap(oracle, "PreimageOracle")
//	require.NoError(t, err)
//
//	return srcmap.NewSourceMapTracer(map[common.Address]*srcmap.SourceMap{
//		addrs.MIPS:   mipsSrcMap,
//		addrs.Oracle: oracleSrcMap,
//	}, os.Stdout).Hooks()
//}
