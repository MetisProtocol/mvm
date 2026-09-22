package core

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/core/state"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/ethdb"
	"github.com/MetisProtocol/mvm/l2geth/log"
	"github.com/MetisProtocol/mvm/l2geth/params"
	"github.com/MetisProtocol/mvm/l2geth/rollup/rcfg"
	"github.com/ethereum/go-ethereum/crypto"
	"golang.org/x/crypto/sha3"
)

type OVMAuditConfig struct {
	Enabled bool
	To      uint64
	ToHash  common.Hash
	Dir     string
	Witness string
}

// OVMAuditControl is a local stop, never evidence of an invalid remote block.
type OVMAuditControl struct{ Err error }

func (e *OVMAuditControl) Error() string {
	if e.Err == nil {
		return "OVM audit completed"
	}
	return "OVM audit stopped: " + e.Err.Error()
}
func (e *OVMAuditControl) Unwrap() error { return e.Err }
func IsOVMAuditControl(err error) bool   { var stop *OVMAuditControl; return errors.As(err, &stop) }

type ovmAuditIdentity struct {
	Version        int            `json:"version"`
	Genesis        common.Hash    `json:"genesis"`
	ConfigHash     common.Hash    `json:"configHash"`
	WitnessHash    common.Hash    `json:"witnessHash"`
	To             uint64         `json:"to"`
	ToHash         common.Hash    `json:"toHash"`
	DeSeqBlock     uint64         `json:"deSeqBlock"`
	SeqValidHeight uint64         `json:"seqValidHeight"`
	FirstSequencer common.Address `json:"firstSequencer"`
}

type ovmAuditManifest struct {
	Identity ovmAuditIdentity   `json:"identity"`
	Genesis  state.OVMAuditScan `json:"genesisState"`
}

type ovmAuditBlock struct {
	Hash         common.Hash           `json:"blockHash"`
	Parent       common.Hash           `json:"parentHash"`
	Root         common.Hash           `json:"stateRoot"`
	Number       uint64                `json:"blockNumber"`
	Transactions int                   `json:"transactions"`
	GasComputed  uint64                `json:"gasComputed"`
	GasHeader    uint64                `json:"gasHeader"`
	Delta        state.OVMAuditDelta   `json:"delta"`
	Events       []state.OVMAuditEvent `json:"events,omitempty"`
}

type ovmAuditCursor struct {
	Number uint64              `json:"number"`
	Hash   common.Hash         `json:"hash"`
	Total  state.OVMAuditDelta `json:"total"`
}

type ovmAudit struct {
	bc           *BlockChain
	cfg          OVMAuditConfig
	manifest     ovmAuditManifest
	cursor       ovmAuditCursor
	snapshot     ovmAuditCursor
	lastProgress time.Time
	done         chan struct{}
	mu           sync.Mutex
	stopped      bool
	err          error
	cancelled    atomic.Bool
}

func auditKey(s string) []byte {
	return append(append([]byte{}, state.OVMAuditPrefix...), []byte(s)...)
}
func auditBlockKey(hash common.Hash) []byte { return append(auditKey("block/"), hash[:]...) }

func (bc *BlockChain) OVMAuditEnabled() bool { return bc.ovmAudit != nil }

// Cancel before waiting for networking to stop: a downloader may be waiting for
// the importer's final storage scan/report, while BlockChain.Stop runs later.
func (bc *BlockChain) CancelOVMAudit() {
	if bc.ovmAudit != nil {
		bc.ovmAudit.cancelled.Store(true)
	}
}
func (bc *BlockChain) OVMAuditDone() <-chan struct{} {
	if bc.ovmAudit == nil {
		return nil
	}
	return bc.ovmAudit.done
}
func (bc *BlockChain) OVMAuditError() error {
	if a := bc.ovmAudit; a != nil {
		a.mu.Lock()
		defer a.mu.Unlock()
		return a.err
	}
	return nil
}

func (a *ovmAudit) control() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stopped {
		return &OVMAuditControl{a.err}
	}
	return nil
}

func (a *ovmAudit) stop(err error) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.stopped {
		a.err = err
		a.stopped = true
		if err != nil {
			log.Error("OVM audit stopped", "err", err, "block", a.snapshot.Number, "hash", a.snapshot.Hash)
			// Preserve the original failure if even the diagnostic file cannot be written.
			if writeErr := auditJSONFile(a.cfg.Dir, "failure.json", map[string]interface{}{"error": err.Error(), "cursor": a.snapshot}); writeErr != nil {
				log.Error("Cannot write OVM audit failure report", "err", writeErr)
			}
		}
		close(a.done)
	}
	return &OVMAuditControl{a.err}
}

// EnableOVMAudit runs before networking starts. All subsequent mutable audit
// state is owned by the block import critical section.
func (bc *BlockChain) EnableOVMAudit(cfg OVMAuditConfig) error {
	if !cfg.Enabled {
		return nil
	}
	if !rcfg.UsingOVM || bc.chainConfig.Clique == nil {
		return fmt.Errorf("OVM audit requires OVM/Clique")
	}
	if cfg.To == 0 || cfg.ToHash == (common.Hash{}) || cfg.Dir == "" {
		return fmt.Errorf("OVM audit requires target number, hash and report directory")
	}
	configBlob, err := json.Marshal(bc.chainConfig)
	if err != nil {
		return err
	}
	index := state.NewOVMAuditIndex(bc.db)
	witnessHash, err := loadAuditWitness(cfg.Witness, nil, nil)
	if err != nil {
		return err
	}
	id := ovmAuditIdentity{1, bc.genesisBlock.Hash(), crypto.Keccak256Hash(configBlob), witnessHash, cfg.To, cfg.ToHash, rcfg.DeSeqBlock, rcfg.SeqValidHeight, rcfg.DefaultSeqAdderss}
	a := &ovmAudit{bc: bc, cfg: cfg, done: make(chan struct{}), cursor: ovmAuditCursor{Hash: bc.genesisBlock.Hash(), Total: state.ZeroOVMAuditDelta()}}
	has, err := bc.db.Has(auditKey("manifest"))
	if err != nil {
		return err
	}
	if has {
		blob, err := bc.db.Get(auditKey("manifest"))
		if err != nil {
			return err
		}
		if err = json.Unmarshal(blob, &a.manifest); err != nil {
			return err
		}
		if a.manifest.Identity != id {
			return fmt.Errorf("OVM audit configuration, genesis or witness changed")
		}
	} else {
		if bc.CurrentBlock().NumberU64() != 0 || bc.CurrentFastBlock().NumberU64() != 0 || bc.CurrentHeader().Number.Sign() != 0 {
			return fmt.Errorf("first OVM audit run requires a fresh genesis database")
		}
		loadedHash, err := loadAuditWitness(cfg.Witness, index, func() error {
			batch := bc.db.NewBatch()
			if err := index.Write(batch); err != nil {
				return err
			}
			if err := batch.Write(); err != nil {
				return err
			}
			index.Pending = make(map[common.Hash]state.OVMAuditSlot)
			return nil
		})
		if err != nil {
			return err
		}
		if loadedHash != witnessHash {
			return fmt.Errorf("OVM audit witness changed during startup")
		}
		// Preimages are ordered by hash, not dependency. The first pass learns
		// every allowance base; the second resolves outer slots even when they
		// precede their bases. Keep pending evidence across passes and flush in
		// bounded batches, as for the witness index.
		for pass := 0; pass < 2; pass++ {
			it := bc.db.NewIteratorWithPrefix([]byte("secure-key-"))
			for it.Next() {
				key, value := it.Key(), it.Value()
				if len(key) != len("secure-key-")+32 {
					err = fmt.Errorf("invalid preimage key")
					break
				}
				index.Preimage(common.BytesToHash(key[len("secure-key-"):]), value)
				if index.Err != nil {
					err = index.Err
					break
				}
				if len(index.Pending) >= 4096 {
					batch := bc.db.NewBatch()
					if err = index.Write(batch); err == nil {
						err = batch.Write()
					}
					if err != nil {
						break
					}
					index = state.NewOVMAuditIndex(bc.db)
				}
			}
			if err == nil {
				err = it.Error()
			}
			it.Release()
			if err != nil {
				return err
			}
		}
		s, err := bc.StateAt(bc.genesisBlock.Root())
		if err != nil {
			return err
		}
		scan, err := s.ScanOVMAudit(index, bc.genesisBlock.Root())
		if err != nil {
			return fmt.Errorf("audit genesis: %w", err)
		}
		a.manifest = ovmAuditManifest{Identity: id, Genesis: scan}
		batch := bc.db.NewBatch()
		if err = index.Write(batch); err != nil {
			return err
		}
		blob, _ := json.Marshal(a.manifest)
		if err = batch.Put(auditKey("manifest"), blob); err != nil {
			return err
		}
		if err = batch.Write(); err != nil {
			return err
		}
	}
	if err = os.MkdirAll(cfg.Dir, 0700); err != nil {
		return err
	}
	if blob, err := os.ReadFile(filepath.Join(cfg.Dir, "run.json")); err == nil {
		var previous ovmAuditManifest
		if json.Unmarshal(blob, &previous) != nil || previous.Identity != id {
			return fmt.Errorf("OVM audit report directory belongs to another run")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err = auditJSONFile(cfg.Dir, "run.json", a.manifest); err != nil {
		return err
	}
	if has, err = bc.db.Has(auditKey("cursor")); err != nil {
		return err
	} else if has {
		blob, err := bc.db.Get(auditKey("cursor"))
		if err != nil {
			return err
		}
		if err = json.Unmarshal(blob, &a.cursor); err != nil {
			return err
		}
		if a.cursor.Number == 0 && a.cursor.Hash != bc.genesisBlock.Hash() {
			return fmt.Errorf("invalid audit genesis cursor")
		}
	}
	head, err := a.auditHead(bc.CurrentBlock())
	if err != nil {
		return err
	}
	if err = a.reconcile(head); err != nil {
		return err
	}
	bc.ovmAudit = a
	if head.NumberU64() == cfg.To {
		s, err := bc.StateAt(head.Root())
		if err != nil {
			return err
		}
		if err = a.finish(s, head); err != nil {
			return err
		}
		a.stop(nil)
	}
	return nil
}

func loadAuditWitness(path string, index *state.OVMAuditIndex, flush func() error) (common.Hash, error) {
	if path == "" {
		return crypto.Keccak256Hash(nil), nil
	}
	f, err := os.Open(path)
	if err != nil {
		return common.Hash{}, err
	}
	defer f.Close()
	h := sha3.NewLegacyKeccak256()
	scanner := bufio.NewScanner(io.TeeReader(f, h))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		var record struct {
			Type    string          `json:"type"`
			Address *common.Address `json:"address"`
			Owner   *common.Address `json:"owner"`
			Spender *common.Address `json:"spender"`
		}
		decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&record); err != nil {
			return common.Hash{}, err
		}
		var extra interface{}
		if decoder.Decode(&extra) != io.EOF {
			return common.Hash{}, fmt.Errorf("trailing witness data")
		}
		switch record.Type {
		case "address":
			if record.Address == nil || record.Owner != nil || record.Spender != nil {
				return common.Hash{}, fmt.Errorf("invalid address witness")
			}
			if index != nil {
				index.Address(*record.Address)
			}
		case "allowance":
			if record.Address != nil || record.Owner == nil || record.Spender == nil {
				return common.Hash{}, fmt.Errorf("invalid allowance witness")
			}
			if index != nil {
				index.Allowance(*record.Owner, *record.Spender)
			}
		default:
			return common.Hash{}, fmt.Errorf("unknown witness type %q", record.Type)
		}
		if index != nil && index.Err != nil {
			return common.Hash{}, index.Err
		}
		if index != nil && flush != nil && len(index.Pending) >= 4096 {
			if err = flush(); err != nil {
				return common.Hash{}, err
			}
		}
	}
	if err = scanner.Err(); err != nil {
		return common.Hash{}, err
	}
	var digest common.Hash
	copy(digest[:], h.Sum(nil))
	return digest, nil
}

func (bc *BlockChain) beginOVMAudit(s *state.StateDB) error {
	if a := bc.ovmAudit; a != nil {
		if a.cancelled.Load() {
			return a.stop(fmt.Errorf("audit interrupted; restart with the same options"))
		}
		if err := a.control(); err != nil {
			return err
		}
		s.SetOVMAudit(&state.OVMAuditObserver{Index: state.NewOVMAuditIndex(bc.db), Total: state.ZeroOVMAuditDelta()})
		s.BeginOVMAuditScope("block-context", common.Hash{}, -1)
	}
	return nil
}

// A crash can leave a verified block and its state on disk before canonical
// publication. Resume that publication rather than silently skipping the target.
func (bc *BlockChain) importKnownOVMAudit(block *types.Block) error {
	current := bc.CurrentBlock()
	if block.Hash() != current.Hash() {
		td, local := bc.GetTd(block.Hash(), block.NumberU64()), bc.GetTd(current.Hash(), current.NumberU64())
		if td == nil || local == nil {
			return bc.ovmAudit.stop(fmt.Errorf("missing known block difficulty"))
		}
		if td.Cmp(local) <= 0 {
			return nil
		}
	}
	if _, err := bc.ovmAudit.read(block.Hash()); err != nil {
		return bc.ovmAudit.stop(err)
	}
	if err := bc.writeKnownBlock(block); err != nil {
		return err
	}
	s, err := bc.StateAt(block.Root())
	if err != nil {
		return bc.ovmAudit.stop(err)
	}
	return bc.committedOVMAudit(s)
}

func (bc *BlockChain) checkOVMAudit(s *state.StateDB) error {
	if a := bc.ovmAudit; a != nil {
		if err := s.Error(); err != nil {
			return a.stop(err)
		}
		if s.OVMAudit() == nil {
			return a.stop(fmt.Errorf("missing block execution observer"))
		}
		if err := s.OVMAudit().Err; err != nil {
			return a.stop(err)
		}
		if err := s.OVMAudit().Index.Err; err != nil {
			return a.stop(err)
		}
	}
	return nil
}

func (bc *BlockChain) writeOVMAudit(batch ethdb.KeyValueWriter, block *types.Block, s *state.StateDB, receipts types.Receipts) error {
	a := bc.ovmAudit
	if a == nil {
		return nil
	}
	if err := a.control(); err != nil {
		return err
	}
	if err := bc.checkOVMAudit(s); err != nil {
		return err
	}
	o := s.OVMAudit()
	computed := uint64(0)
	if len(receipts) > 0 {
		computed = receipts[len(receipts)-1].CumulativeGasUsed
	}
	record := ovmAuditBlock{block.Hash(), block.ParentHash(), block.Root(), block.NumberU64(), len(block.Transactions()), computed, block.GasUsed(), o.Total, o.Events}
	blob, err := json.Marshal(record)
	if err != nil {
		return a.stop(err)
	}
	key := auditBlockKey(block.Hash())
	if exists, err := bc.db.Has(key); err != nil {
		return a.stop(err)
	} else if exists {
		old, err := bc.db.Get(key)
		if err != nil {
			return a.stop(err)
		}
		if !bytes.Equal(old, blob) {
			return a.stop(fmt.Errorf("audit re-execution differs at %d %s", block.NumberU64(), block.Hash()))
		}
	}
	if err = batch.Put(key, blob); err != nil {
		return a.stop(err)
	}
	if err = o.Index.Write(batch); err != nil {
		return a.stop(err)
	}
	return nil
}

func (a *ovmAudit) read(hash common.Hash) (ovmAuditBlock, error) {
	var record ovmAuditBlock
	blob, err := a.bc.db.Get(auditBlockKey(hash))
	if err != nil {
		return record, fmt.Errorf("audit coverage missing for %s: %w", hash, err)
	}
	if err = json.Unmarshal(blob, &record); err != nil {
		return record, err
	}
	if record.Hash != hash {
		return record, fmt.Errorf("audit record hash mismatch")
	}
	header := a.bc.GetHeader(hash, record.Number)
	if header != nil && (header.Root != record.Root || header.ParentHash != record.Parent) {
		return record, fmt.Errorf("audit record disagrees with header %s", hash)
	}
	return record, nil
}

// Reconcile the materialized sum against canonical hashes. It also handles the
// node's normal startup state rewind without retaining historical state tries.
func (a *ovmAudit) reconcile(head *types.Block) error {
	// Follow the executed head's parents, not header-sync's possibly newer
	// canonical mappings. Accumulate backwards without retaining a history list.
	hash, number := head.Hash(), head.NumberU64()
	added := state.ZeroOVMAuditDelta()
	for a.cursor.Hash != hash {
		if a.cursor.Number >= number {
			if a.cursor.Number == 0 {
				return fmt.Errorf("audit cursor has no common genesis")
			}
			r, err := a.read(a.cursor.Hash)
			if err != nil {
				return err
			}
			if r.Number != a.cursor.Number {
				return fmt.Errorf("audit cursor number mismatch")
			}
			if err = a.cursor.Total.Add(r.Delta, -1); err != nil {
				return err
			}
			a.cursor.Number--
			a.cursor.Hash = r.Parent
		} else {
			r, err := a.read(hash)
			if err != nil {
				return err
			}
			if r.Number != number {
				return fmt.Errorf("audit coverage number mismatch")
			}
			if err = added.Add(r.Delta, 1); err != nil {
				return err
			}
			number--
			hash = r.Parent
		}
	}
	if a.cursor.Number != number {
		return fmt.Errorf("audit common ancestor number mismatch")
	}
	if err := a.cursor.Total.Add(added, 1); err != nil {
		return err
	}
	a.cursor.Number = head.NumberU64()
	a.cursor.Hash = head.Hash()
	a.mu.Lock()
	a.snapshot = a.cursor
	a.mu.Unlock()
	return nil
}

// auditHead bounds accounting to the target on the executed head's ancestry.
// A heavier branch can become canonical only after its height passes the target;
// header-sync's number mappings are not sufficient to establish that ancestry.
func (a *ovmAudit) auditHead(head *types.Block) (*types.Block, error) {
	if head.NumberU64() > a.cfg.To {
		header := head.Header()
		for header.Number.Uint64() > a.cfg.To {
			header = a.bc.GetHeader(header.ParentHash, header.Number.Uint64()-1)
			if header == nil {
				return nil, fmt.Errorf("missing ancestor while locating audit target")
			}
		}
		head = a.bc.GetBlock(header.Hash(), a.cfg.To)
		if head == nil {
			return nil, fmt.Errorf("audit target block is missing")
		}
	}
	if head.NumberU64() == a.cfg.To && head.Hash() != a.cfg.ToHash {
		return nil, fmt.Errorf("audit target mismatch: have %d %s, want %d %s", head.NumberU64(), head.Hash(), a.cfg.To, a.cfg.ToHash)
	}
	return head, nil
}

func (bc *BlockChain) committedOVMAudit(s *state.StateDB) error {
	a := bc.ovmAudit
	if a == nil {
		return nil
	}
	head := bc.CurrentBlock()
	bound, err := a.auditHead(head)
	if err != nil {
		return a.stop(err)
	}
	if err := a.reconcile(bound); err != nil {
		return a.stop(err)
	}
	if a.cursor.Number == a.cfg.To {
		if bound.Hash() != head.Hash() {
			s, err = bc.StateAt(bound.Root())
			if err != nil {
				return a.stop(fmt.Errorf("open audit target state: %w", err))
			}
		}
		if err := a.finish(s, bound); err != nil {
			return a.stop(err)
		}
		return a.stop(nil)
	}
	if a.cursor.Number%1000 == 0 {
		blob, _ := json.Marshal(a.cursor)
		if err := bc.db.Put(auditKey("cursor"), blob); err != nil {
			return a.stop(err)
		}
		if err := auditJSONFile(a.cfg.Dir, "progress.json", a.cursor); err != nil {
			return a.stop(err)
		}
	}
	if time.Since(a.lastProgress) >= 30*time.Second {
		log.Info("OVM audit progress", "number", a.cursor.Number, "target", a.cfg.To, "difference", a.cursor.Total.Difference, "unexplained", a.cursor.Total.Unexplained)
		a.lastProgress = time.Now()
	}
	return nil
}

func (a *ovmAudit) finish(s *state.StateDB, block *types.Block) error {
	if block.NumberU64() != a.cfg.To || block.Hash() != a.cfg.ToHash {
		return fmt.Errorf("audit target mismatch: have %d %s, want %d %s", block.NumberU64(), block.Hash(), a.cfg.To, a.cfg.ToHash)
	}
	// The import lock pins the canonical target while its state is scanned.
	index := state.NewOVMAuditIndex(a.bc.db)
	index.Check = func() error {
		if a.cancelled.Load() || atomic.LoadInt32(&a.bc.procInterrupt) != 0 {
			return fmt.Errorf("audit interrupted; restart with the same options")
		}
		if time.Since(a.lastProgress) >= 30*time.Second {
			log.Info("OVM audit scanning target state", "number", block.NumberU64())
			a.lastProgress = time.Now()
		}
		return nil
	}
	end, err := s.ScanOVMAudit(index, block.Root())
	if err != nil {
		return err
	}
	start, _ := new(big.Int).SetString(a.manifest.Genesis.Difference, 10)
	last, _ := new(big.Int).SetString(end.Difference, 10)
	if start == nil || last == nil {
		return fmt.Errorf("invalid audit baseline")
	}
	if new(big.Int).Sub(last, start).String() != a.cursor.Total.Difference {
		return fmt.Errorf("OVM audit reconciliation failed: genesis=%s end=%s cumulative=%s", start, last, a.cursor.Total.Difference)
	}
	for _, values := range [][3]string{{a.manifest.Genesis.Balances, end.Balances, a.cursor.Total.Balance}, {a.manifest.Genesis.Supply, end.Supply, a.cursor.Total.Supply}} {
		before, ok1 := new(big.Int).SetString(values[0], 10)
		after, ok2 := new(big.Int).SetString(values[1], 10)
		if !ok1 || !ok2 || new(big.Int).Sub(after, before).String() != values[2] {
			return fmt.Errorf("OVM audit balance/supply component mismatch")
		}
	}
	var unexplainedEvents uint64
	// Regenerate the canonical event view; source records, not log offsets, are truth.
	err = auditFile(a.cfg.Dir, "events.jsonl", func(w io.Writer) error {
		encoder := json.NewEncoder(w)
		previous := a.manifest.Identity.Genesis
		total := state.ZeroOVMAuditDelta()
		for number := uint64(1); number <= a.cfg.To; number++ {
			if a.cancelled.Load() || atomic.LoadInt32(&a.bc.procInterrupt) != 0 {
				return fmt.Errorf("audit interrupted; restart with the same options")
			}
			if time.Since(a.lastProgress) >= 30*time.Second {
				log.Info("OVM audit exporting report", "block", number, "target", a.cfg.To)
				a.lastProgress = time.Now()
			}
			record, err := a.read(a.bc.GetCanonicalHash(number))
			if err != nil {
				return err
			}
			if record.Number != number || record.Parent != previous {
				return fmt.Errorf("audit report coverage gap at %d", number)
			}
			previous = record.Hash
			if err = total.Add(record.Delta, 1); err != nil {
				return err
			}
			for _, event := range record.Events {
				if event.Unexplained != "0" {
					unexplainedEvents++
				}
				row := struct {
					BlockNumber uint64      `json:"blockNumber"`
					BlockHash   common.Hash `json:"blockHash"`
					state.OVMAuditEvent
				}{number, record.Hash, event}
				if err = encoder.Encode(row); err != nil {
					return err
				}
			}
		}
		if total != a.cursor.Total {
			return fmt.Errorf("audit cached totals disagree with canonical records")
		}
		if previous != a.cfg.ToHash {
			return fmt.Errorf("canonical headers changed during audit export")
		}
		return nil
	})
	if err != nil {
		return err
	}
	knownTarget := common.HexToHash("31ccdca9cb607cdde996f7a8a82519bc873ffff68b898a3d8472069fc5724226")
	summary := map[string]interface{}{"version": 1, "identity": a.manifest.Identity, "clientVersion": params.VersionWithMeta, "from": 1, "verifiedBlocks": a.cfg.To, "genesisState": a.manifest.Genesis, "targetState": end, "totals": a.cursor.Total, "executionVerified": true, "accountingReconciled": true, "intervalCausesFullyExplained": unexplainedEvents == 0, "unexplainedEvents": unexplainedEvents, "genesisDifferenceExplained": a.manifest.Genesis.Difference == "0"}
	if a.cfg.To == 23182122 && a.cfg.ToHash == knownTarget {
		summary["previousDiagnosisDifferenceWei"] = "39864711177577535"
		summary["matchesPreviousDiagnosis"] = end.Difference == "39864711177577535"
	}
	if err = auditJSONFile(a.cfg.Dir, "summary.json", summary); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(a.cfg.Dir, "failure.json")); err != nil && !os.IsNotExist(err) {
		return err
	}
	log.Info("OVM audit completed", "number", a.cfg.To, "difference", end.Difference, "unexplained", a.cursor.Total.Unexplained, "report", a.cfg.Dir)
	return nil
}

func auditJSONFile(dir, name string, value interface{}) error {
	return auditFile(dir, name, func(w io.Writer) error { e := json.NewEncoder(w); e.SetIndent("", "  "); return e.Encode(value) })
}

func auditFile(dir, name string, write func(io.Writer) error) (err error) {
	f, err := os.CreateTemp(dir, "."+name+"-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { f.Close(); os.Remove(tmp) }()
	w := bufio.NewWriter(f)
	if err = write(w); err != nil {
		return err
	}
	if err = w.Flush(); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(tmp, filepath.Join(dir, name)); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
