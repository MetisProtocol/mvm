package main

import (
	"github.com/ethereum-optimism/optimism/go/op-program/client"
	"github.com/ethereum-optimism/optimism/l2geth/log"
)

func main() {
	client.Main(log.New())
}
