package eth

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/consensus/ethash"
	"github.com/MetisProtocol/mvm/l2geth/core"
	"github.com/MetisProtocol/mvm/l2geth/core/forkid"
	"github.com/MetisProtocol/mvm/l2geth/core/rawdb"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/core/vm"
	"github.com/MetisProtocol/mvm/l2geth/eth/downloader"
	"github.com/MetisProtocol/mvm/l2geth/ethdb"
	"github.com/MetisProtocol/mvm/l2geth/event"
	"github.com/MetisProtocol/mvm/l2geth/p2p"
	"github.com/MetisProtocol/mvm/l2geth/params"
	"github.com/MetisProtocol/mvm/l2geth/rlp"
	"github.com/MetisProtocol/mvm/l2geth/rollup/dump"
	"github.com/MetisProtocol/mvm/l2geth/rollup/rcfg"
	"github.com/ethereum/go-ethereum/crypto"
)

func newWhitelistManager(t *testing.T, mode downloader.SyncMode, blocks int, policy *BlockPeerWhitelist) (*ProtocolManager, ethdb.Database) {
	t.Helper()
	db := rawdb.NewMemoryDatabase()
	genesisSpec := &core.Genesis{Config: params.TestChainConfig, Alloc: core.GenesisAlloc{
		testBank: {Balance: big.NewInt(1000000)},
		// Keep the OVM balance storage account alive across EIP-158 commits.
		dump.OvmEthAddress: {Balance: new(big.Int), Code: []byte{0x00}},
	}}
	genesis := genesisSpec.MustCommit(db)
	engine := ethash.NewFaker()
	chain, err := core.NewBlockChain(db, nil, genesisSpec.Config, engine, vm.Config{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	generated, _ := core.GenerateChain(genesisSpec.Config, genesis, engine, db, blocks, func(i int, block *core.BlockGen) {
		if i < 2 {
			// Non-empty bodies/receipts exercise both download pipelines. A
			// nonzero gas price is required by the OVM fee calculation.
			tx := types.NewTransaction(block.TxNonce(testBank), common.Address{1}, big.NewInt(1), params.TxGas, big.NewInt(1), nil)
			tx.SetL1BlockNumber(1)
			signed, err := types.SignTx(tx, types.NewEIP155Signer(genesisSpec.Config.ChainID), testBankKey)
			if err != nil {
				t.Fatal(err)
			}
			block.AddTx(signed)
		}
	})
	if len(generated) > 0 {
		if _, err := chain.InsertChain(generated); err != nil {
			t.Fatal(err)
		}
	}
	// Production drains this channel in the rollup sync service. Keep enough room
	// for all test imports without modifying production rollup behavior.
	pm, err := NewProtocolManager(genesisSpec.Config, nil, mode, 1088, new(event.TypeMux), new(testTxPool), engine, chain, db, 1, nil, nil, make(chan *types.Block, 4096), nil)
	if err != nil {
		t.Fatal(err)
	}
	pm.blockPeerWhitelist = policy
	pm.Start(20)
	t.Cleanup(func() { pm.Stop(); chain.Stop(); db.Close(); engine.Close() })
	return pm, db
}

func whitelistKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func whitelistPolicyFor(t *testing.T, key *ecdsa.PrivateKey, ip string) *BlockPeerWhitelist {
	t.Helper()
	path := writeBlockWhitelist(t, fmt.Sprintf(`{"networks":[{"networkId":1088,"peers":["enode://%x@%s"]}]}`, crypto.FromECDSAPub(&key.PublicKey)[1:], net.JoinHostPort(ip, "30303")))
	policy, err := LoadBlockPeerWhitelist(path, 1088, downloader.FullSync)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func whitelistServer(t *testing.T, key *ecdsa.PrivateKey, protocols []p2p.Protocol) *p2p.Server {
	t.Helper()
	srv := &p2p.Server{Config: p2p.Config{PrivateKey: key, MaxPeers: 20, NoDiscovery: true, ListenAddr: "127.0.0.1:0", Protocols: protocols}}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Stop)
	return srv
}

func waitWhitelist(t *testing.T, reason string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", reason)
}

func TestBlockPeerWhitelistNetworkSync(t *testing.T) {
	for _, mode := range []downloader.SyncMode{downloader.FullSync, downloader.FastSync} {
		t.Run(mode.String(), func(t *testing.T) {
			allowedKey, deniedKey := whitelistKey(t), whitelistKey(t)
			source, _ := newWhitelistManager(t, downloader.FullSync, 128, &BlockPeerWhitelist{})
			outsider, _ := newWhitelistManager(t, downloader.FullSync, 160, &BlockPeerWhitelist{})
			target, _ := newWhitelistManager(t, mode, 0, whitelistPolicyFor(t, allowedKey, "127.0.0.1"))
			sourceServer := whitelistServer(t, allowedKey, []p2p.Protocol{source.makeProtocol(eth63)})
			outsiderServer := whitelistServer(t, deniedKey, []p2p.Protocol{outsider.makeProtocol(eth63)})
			targetServer := whitelistServer(t, whitelistKey(t), []p2p.Protocol{target.makeProtocol(eth63)})
			targetServer.AddTrustedPeer(outsiderServer.Self())
			targetServer.AddPeer(outsiderServer.Self())
			waitWhitelist(t, "denied static/trusted peer", func() bool { return target.peers.Len() == 1 })
			if best, count := target.blockSyncPeers(); best != nil || count != 0 {
				t.Fatal("denied peer is a sync candidate")
			}
			outsiderID := outsiderServer.Self().ID()
			denied := target.peers.Peer(fmt.Sprintf("%x", outsiderID[:8]))
			target.synchronise(denied)
			if target.blockchain.CurrentBlock().NumberU64() != 0 {
				t.Fatal("imported from denied source")
			}
			targetServer.AddPeer(sourceServer.Self())
			waitWhitelist(t, "both peers", func() bool { return target.peers.Len() == 2 })
			best, count := target.blockSyncPeers()
			if best == nil || count != 1 || best.ID() != sourceServer.Self().ID() {
				t.Fatal("highest TD outsider displaced allowed source")
			}
			done := make(chan struct{})
			go func() { target.synchronise(best); close(done) }()
			select {
			case <-done:
			case <-time.After(30 * time.Second):
				t.Fatal("sync timeout")
			}
			if got := target.blockchain.CurrentBlock().NumberU64(); got != 128 {
				t.Fatalf("imported height %d, want 128", got)
			}
			first := target.blockchain.GetBlockByNumber(1)
			if len(first.Transactions()) != 1 || len(target.blockchain.GetReceiptsByHash(first.Hash())) != 1 {
				t.Fatal("non-empty body or receipts missing after sync")
			}
			if target.peers.Len() != 2 {
				t.Fatal("non-whitelisted peer disconnected")
			}
		})
	}
}

func TestBlockPeerWhitelistNetworkFetcher(t *testing.T) {
	key := whitelistKey(t)
	source, _ := newWhitelistManager(t, downloader.FullSync, 2, &BlockPeerWhitelist{})
	target, _ := newWhitelistManager(t, downloader.FullSync, 0, whitelistPolicyFor(t, key, "127.0.0.1"))
	src := whitelistServer(t, key, []p2p.Protocol{source.makeProtocol(eth63)})
	dst := whitelistServer(t, whitelistKey(t), []p2p.Protocol{target.makeProtocol(eth63)})
	// Let the source dial: the same policy applies to inbound connections.
	src.AddPeer(dst.Self())
	waitWhitelist(t, "fetcher peers", func() bool { return source.peers.Len() == 1 && target.peers.Len() == 1 })
	source.BroadcastBlock(source.blockchain.GetBlockByNumber(1), true)
	waitWhitelist(t, "broadcast block import", func() bool { return target.blockchain.CurrentBlock().NumberU64() == 1 })
	source.BroadcastBlock(source.blockchain.GetBlockByNumber(2), false)
	waitWhitelist(t, "announced block fetch/import", func() bool { return target.blockchain.CurrentBlock().NumberU64() == 2 })
}

// A real RLPx peer sends disallowed data, then requests data and receives a
// broadcast over the same connection. Request/reply exchanges act as barriers.
func TestBlockPeerWhitelistNetworkOutbound(t *testing.T) {
	for _, reason := range []string{"empty", "wrong IP", "wrong key"} {
		t.Run(reason, func(t *testing.T) {
			remoteKey := whitelistKey(t)
			policy := &BlockPeerWhitelist{}
			if reason == "wrong IP" {
				policy = whitelistPolicyFor(t, remoteKey, "192.0.2.1")
			}
			if reason == "wrong key" {
				policy = whitelistPolicyFor(t, whitelistKey(t), "127.0.0.1")
			}
			target, db := newWhitelistManager(t, downloader.FullSync, 0, policy)
			genesis := target.blockchain.Genesis()
			blocks, _ := core.GenerateChain(params.TestChainConfig, genesis, ethash.NewFaker(), db, 1, nil)
			block := blocks[0]
			td := new(big.Int).Add(target.blockchain.GetTd(genesis.Hash(), 0), block.Difficulty())
			srv := whitelistServer(t, whitelistKey(t), []p2p.Protocol{target.makeProtocol(eth63)})
			done := make(chan error, 1)
			var stage atomic.Value
			stage.Store("connect")
			protocol := target.makeProtocol(eth63)
			protocol.Run = func(raw *p2p.Peer, rw p2p.MsgReadWriter) error {
				err := func() error {
					stage.Store("handshake")
					p := newPeer(eth63, raw, rw)
					if err := p.Handshake(1088, td, block.Hash(), genesis.Hash(), forkid.NewID(target.blockchain), forkid.NewFilter(target.blockchain)); err != nil {
						return err
					}
					stage.Store("send block")
					if err := p.SendNewBlock(block, td); err != nil {
						return err
					}
					if err := p.SendNewBlockHashes([]common.Hash{block.Hash()}, []uint64{1}); err != nil {
						return err
					}
					// Unsolicited download responses must also be consumed harmlessly.
					if err := p.SendBlockHeaders([]*types.Header{block.Header()}); err != nil {
						return err
					}
					body, _ := rlp.EncodeToBytes(block.Body())
					if err := p.SendBlockBodiesRLP([]rlp.RawValue{body}); err != nil {
						return err
					}
					if err := p.SendNodeData([][]byte{{1, 2}}); err != nil {
						return err
					}
					if err := p2p.Send(rw, ReceiptsMsg, []types.Receipts{{}}); err != nil {
						return err
					}
					stage.Store("headers")
					if err := p.RequestHeadersByNumber(0, 1, 0, false); err != nil {
						return err
					}
					if err := p2p.ExpectMsg(rw, BlockHeadersMsg, []*types.Header{genesis.Header()}); err != nil {
						return err
					}
					if target.blockchain.CurrentBlock().NumberU64() != 0 {
						return fmt.Errorf("imported denied block")
					}
					if best, count := target.blockSyncPeers(); best != nil || count != 0 {
						return fmt.Errorf("denied source selected")
					}
					stage.Store("bodies")
					if err := p.RequestBodies([]common.Hash{genesis.Hash()}); err != nil {
						return err
					}
					if err := p2p.ExpectMsg(rw, BlockBodiesMsg, []*types.Body{genesis.Body()}); err != nil {
						return err
					}
					stage.Store("receipts")
					if err := p.RequestReceipts([]common.Hash{genesis.Hash()}); err != nil {
						return err
					}
					if err := p2p.ExpectMsg(rw, ReceiptsMsg, []types.Receipts{{}}); err != nil {
						return err
					}
					stage.Store("state")
					nodeData, err := target.blockchain.TrieNode(genesis.Root())
					if err != nil {
						return err
					}
					if err := p.RequestNodeData([]common.Hash{genesis.Root()}); err != nil {
						return err
					}
					if err := p2p.ExpectMsg(rw, NodeDataMsg, [][]byte{nodeData}); err != nil {
						return err
					}
					// Import locally, then broadcast the exact block the peer tried to send.
					stage.Store("local import")
					if _, err := target.blockchain.InsertChain(blocks); err != nil {
						return err
					}
					stage.Store("block broadcast")
					target.BroadcastBlock(block, true)
					if err := p2p.ExpectMsg(rw, NewBlockMsg, &newBlockData{Block: block, TD: td}); err != nil {
						return err
					}
					tx := newTestTransaction(testBankKey, 0, 0)
					stage.Store("transaction broadcast")
					target.BroadcastTxs(types.Transactions{tx})
					if !rcfg.UsingOVM {
						return p2p.ExpectMsg(rw, TxMsg, types.Transactions{tx})
					}
					// OVM already disables P2P transaction propagation. A reply
					// to a subsequent request must arrive without a Tx message.
					if err := p.RequestHeadersByNumber(0, 1, 0, false); err != nil {
						return err
					}
					return p2p.ExpectMsg(rw, BlockHeadersMsg, []*types.Header{genesis.Header()})
				}()
				done <- err
				return err
			}
			remote := whitelistServer(t, remoteKey, []p2p.Protocol{protocol})
			srv.AddTrustedPeer(remote.Self())
			srv.AddPeer(remote.Self())
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(10 * time.Second):
				t.Fatalf("outbound test timeout at %s", stage.Load())
			}
		})
	}
}
