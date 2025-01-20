package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"time"

	"github.com/MetisProtocol/mvm/l2geth/ethclient"
)

type LocalState struct {
	Number   uint64
	LastSeen time.Time
}

func GetLocalState(file string) (*LocalState, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var block LocalState
	if err := json.Unmarshal(data, &block); err != nil {
		return nil, err
	}
	return &block, nil
}

func SaveLocalState(block *LocalState, file string) error {
	fd, err := os.Create(file)
	if err != nil {
		return err
	}
	defer fd.Close()

	return json.NewEncoder(fd).Encode(block)
}

func main() {
	var (
		FilePath string
		RPC      string
		TxPool   bool

		Timeout         time.Duration
		StalledDuration time.Duration
	)

	flag.StringVar(&FilePath, "file", "/tmp/is-l2geth-stalled.json", "an ephemeral file path")
	flag.DurationVar(&Timeout, "timeout", time.Second*3, "the timeout to send rpc request")
	flag.StringVar(&RPC, "rpc", "/root/.ethereum/geth.ipc", "geth rpc endpoint")
	flag.DurationVar(&StalledDuration, "duration", 120*time.Second, "duration to check if the l2geth is stalled")
	flag.BoolVar(&TxPool, "txpool", false, "check if txpool status")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	client, err := ethclient.DialContext(ctx, RPC)
	if err != nil {
		log.Fatalln("failed to dial rpc", err)
	}
	defer client.Close()

	local, err := GetLocalState(FilePath)
	if err != nil {
		log.Fatalln("failed to get local file", err)
	}

	latest, err := client.HeaderByNumber(ctx, nil)
	if err != nil {
		log.Fatalln("failed to get block from rpc", err)
	}

	var pending uint64 = 0
	if TxPool {
		pending, _, err = client.TxPoolStatus(ctx)
		if err != nil {
			log.Fatalln("failed to get txpool status from rpc", err)
		}
	}

	var stuck bool
	var now = time.Now().UTC()
	switch {
	case pending == 0:
		stuck = local != nil && latest.Number.Uint64() == local.Number && now.Sub(local.LastSeen) > StalledDuration
	case pending > 0:
		stuck = now.Sub(time.Unix(int64(latest.Time), 0)) > StalledDuration
	}

	if stuck {
		log.Fatalf("geth is stalled at %s in the past %s", latest.Number, StalledDuration)
	}

	if local == nil || latest.Number.Uint64() != local.Number {
		local = &LocalState{Number: latest.Number.Uint64(), LastSeen: now}
		if err := SaveLocalState(local, FilePath); err != nil {
			log.Fatalln("failed to save the block file", err)
		}
	}
}
