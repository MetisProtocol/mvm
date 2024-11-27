package main

import (
	"github.com/ethereum/go-ethereum/log"

	"github.com/ethereum-optimism/optimism/go/op-program/client"
)

func main() {
	client.Main(log.New())
}
