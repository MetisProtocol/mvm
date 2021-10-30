package vm

import (
	"fmt"
	"os"
)

// UsingOVM is used to enable or disable functionality necessary for the OVM.
var UsingOVM bool

func init() {
	UsingOVM = os.Getenv("USING_OVM") == "true"
	value := os.Getenv("ROLLUP_ENABLE_ARBITRARY_CONTRACT_DEPLOYMENT")
	if value != "" {
		switch value {
		case "true":
			EnableArbitraryContractDeployment = &EnableArbitraryContractDeploymentTrue
		case "false":
			EnableArbitraryContractDeployment = &EnableArbitraryContractDeploymentFalse
		default:
			panic(fmt.Sprintf("Unknown ROLLUP_ENABLE_ARBITRARY_CONTRACT_DEPLOYMENT value: %s", value))
		}
	}
}
