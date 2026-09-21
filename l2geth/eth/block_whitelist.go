package eth

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/url"
	"os"

	"github.com/MetisProtocol/mvm/l2geth/eth/downloader"
	"github.com/MetisProtocol/mvm/l2geth/log"
	"github.com/MetisProtocol/mvm/l2geth/p2p/enode"
)

type blockPeerRule struct {
	id enode.ID
	ip string
}

// BlockPeerWhitelist is an immutable set of block sources for one network.
// A nil policy allows all sources; an allocated empty policy allows none.
type BlockPeerWhitelist struct {
	rules map[blockPeerRule]struct{}
}

// LoadBlockPeerWhitelist validates the entire file, then selects networkID.
// Paths are relative to the process working directory. Call before starting
// networking and retain the returned policy for the lifetime of the service.
func LoadBlockPeerWhitelist(path string, networkID uint64, mode downloader.SyncMode) (*BlockPeerWhitelist, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("P2P block whitelist: %w", err)
	}
	defer f.Close()
	var file struct {
		Networks []struct {
			NetworkID *uint64  `json:"networkId"`
			Peers     []string `json:"peers"`
		} `json:"networks"`
	}
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&file); err != nil {
		return nil, fmt.Errorf("P2P block whitelist %q: %w", path, err)
	}
	if err := dec.Decode(new(interface{})); err != io.EOF {
		return nil, fmt.Errorf("P2P block whitelist %q: expected exactly one JSON document", path)
	}
	if file.Networks == nil {
		return nil, fmt.Errorf("P2P block whitelist %q: networks must be an array", path)
	}
	seen := make(map[uint64]bool)
	var selected *BlockPeerWhitelist
	for i, network := range file.Networks {
		if network.NetworkID == nil || network.Peers == nil {
			return nil, fmt.Errorf("P2P block whitelist %q: networks[%d] requires networkId and peers array", path, i)
		}
		id := *network.NetworkID
		if seen[id] {
			return nil, fmt.Errorf("P2P block whitelist %q: duplicate networkId %d", path, id)
		}
		seen[id] = true
		policy := &BlockPeerWhitelist{rules: make(map[blockPeerRule]struct{})}
		for j, entry := range network.Peers {
			// ParseV4 also resolves hostnames. Require a literal IP first so
			// whitelist loading cannot use DNS or accept key-only enodes.
			u, err := url.Parse(entry)
			if err != nil || net.ParseIP(u.Hostname()) == nil {
				return nil, fmt.Errorf("P2P block whitelist %q: network %d peer %d requires a complete enode with a literal IP", path, id, j)
			}
			node, err := enode.ParseV4(entry)
			if err != nil {
				return nil, fmt.Errorf("P2P block whitelist %q: network %d peer %d has invalid enode: %w", path, id, j, err)
			}
			policy.rules[blockPeerRule{node.ID(), node.IP().String()}] = struct{}{}
		}
		if id == networkID {
			selected = policy
		}
	}
	if selected != nil && mode == downloader.LightSync {
		return nil, fmt.Errorf("P2P block whitelist is not supported in light sync mode for network %d", networkID)
	}
	count := 0
	if selected != nil {
		count = len(selected.rules)
	}
	log.Info("P2P block source policy", "network", networkID, "enabled", selected != nil, "rules", count)
	return selected, nil
}

func (w *BlockPeerWhitelist) allows(id enode.ID, addr net.Addr) bool {
	if w == nil {
		return true
	}
	// Use the authenticated full node identity and the connection's actual
	// remote IP, never the advertised enode address or the short ETH peer ID.
	tcp, ok := addr.(*net.TCPAddr)
	if !ok || tcp == nil || tcp.IP == nil {
		return false
	}
	_, ok = w.rules[blockPeerRule{id, tcp.IP.String()}]
	return ok
}

func (pm *ProtocolManager) allowBlockSource(p *peer) bool {
	return pm.blockPeerWhitelist.allows(p.ID(), p.RemoteAddr())
}

func (pm *ProtocolManager) ignoreBlockSource(p *peer, code uint64) bool {
	if pm.allowBlockSource(p) {
		return false
	}
	p.Log().Trace("Ignored data from non-whitelisted block source", "code", code)
	return true
}

// blockSyncPeers selects only eligible sources without removing other peers
// from the ETH peer set used to serve requests and broadcast data.
func (pm *ProtocolManager) blockSyncPeers() (*peer, int) {
	pm.peers.lock.RLock()
	defer pm.peers.lock.RUnlock()
	var best *peer
	var bestTD *big.Int
	count := 0
	for _, p := range pm.peers.peers {
		if !pm.allowBlockSource(p) {
			continue
		}
		count++
		if _, td := p.Head(); best == nil || td.Cmp(bestTD) > 0 {
			best, bestTD = p, td
		}
	}
	return best, count
}
