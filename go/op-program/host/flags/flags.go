package flags

import (
	"fmt"

	"github.com/urfave/cli/v2"

	service "github.com/ethereum-optimism/optimism/op-service"
	oplog "github.com/ethereum-optimism/optimism/op-service/log"
)

const EnvVarPrefix = "OP_PROGRAM"

func prefixEnvVars(name string) []string {
	return service.PrefixEnvVar(EnvVarPrefix, name)
}

var (
	DataDir = &cli.StringFlag{
		Name:    "datadir",
		Usage:   "Directory to use for preimage data storage. Default uses in-memory storage",
		EnvVars: prefixEnvVars("DATADIR"),
	}
	L2NodeAddr = &cli.StringFlag{
		Name:    "l2",
		Usage:   "Address of L2 JSON-RPC endpoint to use (eth and debug namespace required)",
		EnvVars: prefixEnvVars("L2_RPC"),
	}
	L2Head = &cli.StringFlag{
		Name:    "l2.head",
		Usage:   "Hash of the L2 block at l2.outputroot",
		EnvVars: prefixEnvVars("L2_HEAD"),
	}
	L2Claim = &cli.StringFlag{
		Name:    "l2.claim",
		Usage:   "Claimed L2 output root to validate",
		EnvVars: prefixEnvVars("L2_CLAIM"),
	}
	L2BlockNumber = &cli.Uint64Flag{
		Name:    "l2.blocknumber",
		Usage:   "Number of the L2 block that the claim is from",
		EnvVars: prefixEnvVars("L2_BLOCK_NUM"),
	}
	L2GenesisPath = &cli.StringFlag{
		Name:    "l2.genesis",
		Usage:   "Path to the op-geth genesis file",
		EnvVars: prefixEnvVars("L2_GENESIS"),
	}
	Exec = &cli.StringFlag{
		Name:    "exec",
		Usage:   "Run the specified client program as a separate process detached from the host. Default is to run the client program in the host process.",
		EnvVars: prefixEnvVars("EXEC"),
	}
	Server = &cli.BoolFlag{
		Name:    "server",
		Usage:   "Run in pre-image server mode without executing any client program.",
		EnvVars: prefixEnvVars("SERVER"),
	}
	// metis flags
	RollupClientHttpFlag = &cli.StringFlag{
		Name:    "rollup.clienthttp",
		Usage:   "HTTP endpoint for the rollup client",
		Value:   "http://localhost:7878",
		EnvVars: prefixEnvVars("ROLLUP_CLIENT_HTTP"),
	}
	PosClientHttpFlag = &cli.StringFlag{
		Name:    "pos.clienthttp",
		Usage:   "HTTP endpoint for the pos layer client",
		Value:   "http://localhost:1337",
		EnvVars: prefixEnvVars("POS_CLIENT_HTTP"),
	}
)

// Flags contains the list of configuration options available to the binary.
var Flags []cli.Flag

var requiredFlags = []cli.Flag{
	L2Head,
	L2Claim,
	L2BlockNumber,
	RollupClientHttpFlag,
}

var programFlags = []cli.Flag{
	PosClientHttpFlag,
	DataDir,
	L2NodeAddr,
	L2GenesisPath,
	Exec,
	Server,
}

func init() {
	Flags = append(Flags, oplog.CLIFlags(EnvVarPrefix)...)
	Flags = append(Flags, requiredFlags...)
	Flags = append(Flags, programFlags...)
}

func CheckRequired(ctx *cli.Context) error {
	for _, flag := range requiredFlags {
		if !ctx.IsSet(flag.Names()[0]) {
			return fmt.Errorf("flag %s is required", flag.Names()[0])
		}
	}
	return nil
}
