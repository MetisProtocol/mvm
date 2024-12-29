package main

import (
	"context"
	"fmt"

	oplog "github.com/ethereum-optimism/optimism/op-service/log"
	"github.com/ethereum-optimism/optimism/op-service/sources/batching"
	"github.com/ethereum-optimism/optimism/op-service/txmgr"
	"github.com/ethereum/go-ethereum/common"
	"github.com/urfave/cli/v2"

	"github.com/ethereum-optimism/optimism/go/op-challenger/flags"
	"github.com/ethereum-optimism/optimism/go/op-challenger/game/fault/contracts"
	contractMetrics "github.com/ethereum-optimism/optimism/go/op-challenger/game/fault/contracts/metrics"
	"github.com/ethereum-optimism/optimism/go/op-challenger/game/fault/types"
	"github.com/ethereum-optimism/optimism/go/op-challenger/tools"
)

func CreateDispute(ctx *cli.Context) error {
	traceType := ctx.Uint64(TraceTypeFlag.Name)
	l2BlockNum := ctx.Uint64(L2BlockNumFlag.Name)

	contract, txMgr, err := NewContractWithTxMgr[*contracts.DisputeGameFactoryContract](ctx, flags.FactoryAddress,
		func(ctx context.Context, metricer contractMetrics.ContractMetricer, address common.Address, caller *batching.MultiCaller) (*contracts.DisputeGameFactoryContract, error) {
			return contracts.NewDisputeGameFactoryContract(metricer, address, caller), nil
		})
	if err != nil {
		return fmt.Errorf("failed to create dispute game factory bindings: %w", err)
	}

	creator := tools.NewGameCreator(contract, txMgr)
	gameType, bond, l2Block, err := creator.CreateDispute(ctx.Context, traceType, l2BlockNum)
	if err != nil {
		return fmt.Errorf("failed to create game: %w", err)
	}

	fmt.Printf("Created Dispute Game %s Request with: { Bond: %s, L2BlockNumber: %s }\n",
		types.GameType(gameType).String(), bond.String(), l2Block.String())
	return nil
}

func createDisputeFlags() []cli.Flag {
	cliFlags := []cli.Flag{
		flags.L1EthRpcFlag,
		flags.NetworkFlag,
		flags.FactoryAddressFlag,
		TraceTypeFlag,
		L2BlockNumFlag,
	}
	cliFlags = append(cliFlags, txmgr.CLIFlagsWithDefaults(flags.EnvVarPrefix, txmgr.DefaultChallengerFlagValues)...)
	cliFlags = append(cliFlags, oplog.CLIFlags(flags.EnvVarPrefix)...)
	return cliFlags
}

var CreateDisputeCommand = &cli.Command{
	Name:        "create-dispute",
	Usage:       "Creates a dispute game request via the factory",
	Description: "Creates a dispute game request via the factory",
	Action:      Interruptible(CreateDispute),
	Flags:       createDisputeFlags(),
}
