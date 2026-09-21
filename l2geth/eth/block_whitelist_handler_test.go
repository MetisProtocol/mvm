package eth

import (
	"bytes"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/p2p"
	"github.com/MetisProtocol/mvm/l2geth/p2p/enode"
	"github.com/MetisProtocol/mvm/l2geth/rlp"
)

// No fetcher/downloader is installed: forwarding any rejected data would fail
// the test immediately, rather than depending on asynchronous import timing.
type whitelistMessageRW struct{ message p2p.Msg }

func (rw *whitelistMessageRW) ReadMsg() (p2p.Msg, error) { return rw.message, nil }
func (rw *whitelistMessageRW) WriteMsg(p2p.Msg) error {
	return fmt.Errorf("unexpected outbound message")
}

func whitelistHandleMessage(t *testing.T, pm *ProtocolManager, p *peer, code uint64, value interface{}) error {
	t.Helper()
	payload, err := rlp.EncodeToBytes(value)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(payload)
	p.rw = &whitelistMessageRW{p2p.Msg{Code: code, Size: uint32(len(payload)), Payload: reader}}
	err = pm.handleMsg(p)
	if reader.Len() != 0 {
		t.Fatal("message was not fully consumed")
	}
	return err
}

func TestBlockPeerWhitelistDropsImportMessages(t *testing.T) {
	block := types.NewBlock(&types.Header{Number: big.NewInt(1), Difficulty: big.NewInt(1), GasLimit: 10000000}, nil, nil, nil)
	tests := []struct {
		code uint64
		data interface{}
	}{
		{NewBlockMsg, &newBlockData{Block: block, TD: big.NewInt(2)}},
		{NewBlockHashesMsg, newBlockHashesData{{Hash: block.Hash(), Number: 1}}},
		{BlockHeadersMsg, []*types.Header{block.Header()}},
		{BlockHeadersMsg, []*types.Header{block.Header(), block.Header()}},
		{BlockHeadersMsg, []*types.Header{}},
		{BlockBodiesMsg, blockBodiesData{&blockBody{}}},
		{NodeDataMsg, [][]byte{{1, 2, 3}}},
		{ReceiptsMsg, []types.Receipts{{}}},
	}
	for i, tt := range tests {
		t.Run(fmt.Sprintf("%d-%d", tt.code, i), func(t *testing.T) {
			pm := &ProtocolManager{blockPeerWhitelist: &BlockPeerWhitelist{}}
			p := newPeer(eth63, p2p.NewPeer(enode.ID{1}, "not allowed", nil), nil)
			p.td = new(big.Int)
			if err := whitelistHandleMessage(t, pm, p, tt.code, tt.data); err != nil {
				t.Fatal(err)
			}
			if p.knownBlocks.Contains(block.Hash()) {
				t.Fatal("ignored announcement suppressed outbound broadcast")
			}
			if _, td := p.Head(); td.Sign() != 0 {
				t.Fatal("ignored block changed peer head")
			}
		})
	}
	// Synchronisation cannot even inspect the chain when the source is denied.
	pm := &ProtocolManager{blockPeerWhitelist: &BlockPeerWhitelist{}}
	pm.synchronise(newPeer(eth63, p2p.NewPeer(enode.ID{1}, "denied", nil), nil))
}

func TestBlockPeerWhitelistPreservesChallenges(t *testing.T) {
	header := &types.Header{Number: big.NewInt(7), Difficulty: big.NewInt(1)}
	for _, checkpoint := range []bool{false, true} {
		for _, match := range []bool{false, true} {
			t.Run(fmt.Sprintf("checkpoint=%v/match=%v", checkpoint, match), func(t *testing.T) {
				want := header.Hash()
				if !match {
					want = common.Hash{1}
				}
				pm := &ProtocolManager{blockPeerWhitelist: &BlockPeerWhitelist{}}
				p := newPeer(eth63, p2p.NewPeer(enode.ID{1}, "denied", nil), nil)
				if checkpoint {
					pm.checkpointNumber, pm.checkpointHash = 7, want
					p.syncDrop = time.AfterFunc(time.Hour, func() {})
					defer func() {
						if p.syncDrop != nil {
							p.syncDrop.Stop()
						}
					}()
				} else {
					pm.whitelist = map[uint64]common.Hash{7: want}
				}
				err := whitelistHandleMessage(t, pm, p, BlockHeadersMsg, []*types.Header{header})
				if (err == nil) != match {
					t.Fatalf("challenge error=%v, match=%v", err, match)
				}
				if checkpoint && p.syncDrop != nil {
					t.Fatal("challenge timer not cleared")
				}
			})
		}
	}
}

func TestBlockPeerWhitelistPreservesDecodeErrors(t *testing.T) {
	for _, code := range []uint64{NewBlockMsg, NewBlockHashesMsg, BlockHeadersMsg, BlockBodiesMsg, NodeDataMsg, ReceiptsMsg} {
		pm := &ProtocolManager{blockPeerWhitelist: &BlockPeerWhitelist{}}
		p := newPeer(eth63, p2p.NewPeer(enode.ID{1}, "denied", nil), nil)
		if err := whitelistHandleMessage(t, pm, p, code, uint64(1)); err == nil {
			t.Fatalf("invalid message %d accepted", code)
		}
	}
}
