package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/MetisProtocol/mvm/l2geth/eth"
	cli "gopkg.in/urfave/cli.v1"
)

func TestBlockPeerWhitelistConfig(t *testing.T) {
	for _, tt := range []struct {
		name    string
		flags   []string
		want    uint64
		enabled bool
		cliPath bool
	}{
		{"TOML", nil, 1088, true, false},
		{"CLI network", []string{"--networkid", "42"}, 42, false, false},
		{"network selector", []string{"--goerli"}, 5, false, false},
		{"explicit network wins", []string{"--goerli", "--networkid", "1088"}, 1088, true, false},
		{"CLI path", nil, 1088, true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			whitelistPath := filepath.Join(dir, "peers.json")
			if err := os.WriteFile(whitelistPath, []byte(`{"networks":[{"networkId":1088,"peers":[]}]}`), 0600); err != nil {
				t.Fatal(err)
			}
			configPath := filepath.Join(dir, "config.toml")
			configuredPath := whitelistPath
			if tt.cliPath {
				configuredPath = filepath.Join(dir, "missing.json")
			}
			config := fmt.Sprintf("[Eth]\nNetworkId = 1088\nBlockPeerWhitelistFile = %q\n", configuredPath)
			if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
				t.Fatal(err)
			}
			command := cli.NewApp()
			command.Flags = app.Flags
			command.Action = func(ctx *cli.Context) error {
				stack, cfg := makeConfigNode(ctx)
				defer stack.Close()
				if cfg.Eth.NetworkId != tt.want {
					t.Fatalf("network=%d, want %d", cfg.Eth.NetworkId, tt.want)
				}
				if cfg.Eth.BlockPeerWhitelistFile != whitelistPath {
					t.Fatalf("wrong path %s", cfg.Eth.BlockPeerWhitelistFile)
				}
				policy, err := eth.LoadBlockPeerWhitelist(cfg.Eth.BlockPeerWhitelistFile, cfg.Eth.NetworkId, cfg.Eth.SyncMode)
				if err != nil {
					return err
				}
				if (policy != nil) != tt.enabled {
					t.Fatalf("policy enabled=%v, want %v", policy != nil, tt.enabled)
				}
				return nil
			}
			args := append([]string{"geth", "--datadir", filepath.Join(dir, "data"), "--config", configPath}, tt.flags...)
			if tt.cliPath {
				args = append(args, "--p2p.whitelist", whitelistPath)
			}
			if err := command.Run(args); err != nil {
				t.Fatal(err)
			}
		})
	}
}
