package les

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MetisProtocol/mvm/l2geth/eth"
	"github.com/MetisProtocol/mvm/l2geth/eth/downloader"
)

func TestBlockPeerWhitelistRejectsLightStartup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peers.json")
	if err := os.WriteFile(path, []byte(`{"networks":[{"networkId":1088,"peers":[]}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := eth.DefaultConfig
	cfg.NetworkId = 1088
	cfg.SyncMode = downloader.LightSync
	cfg.BlockPeerWhitelistFile = path
	// No service context: rejection must happen before opening the database or
	// constructing a light client that could silently bypass the policy.
	if _, err := New(nil, &cfg); err == nil || !strings.Contains(err.Error(), "not supported in light sync mode") {
		t.Fatalf("startup error=%v", err)
	}
	cfg.BlockPeerWhitelistFile = path + ".missing"
	if _, err := New(nil, &cfg); err == nil || !strings.Contains(err.Error(), "P2P block whitelist") {
		t.Fatalf("startup error=%v", err)
	}
}
