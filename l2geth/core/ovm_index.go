package core

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/core/rawdb"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/ethdb"
	"github.com/MetisProtocol/mvm/l2geth/log"
	"github.com/MetisProtocol/mvm/l2geth/rlp"
	"github.com/MetisProtocol/mvm/l2geth/rollup/dump"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	ovmIndexTransferTopic = common.BytesToHash(crypto.Keccak256([]byte("Transfer(address,address,uint256)")))
	ovmIndexApprovalTopic = common.BytesToHash(crypto.Keccak256([]byte("Approval(address,address,uint256)")))
)

// ovmMigrationIndex is serialized by BlockChain.chainmu. It tracks an independent
// exact head so interrupted reorgs and startup state repair can be reconciled.
// No background writer or state-sized in-memory address set is used.
type ovmMigrationIndex struct {
	db          ethdb.Database
	genesis     *types.Header
	interrupted func() bool
	disabled    bool
}

// The immutable metadata binds the two namespaces to this chain and layout.
// A missing ready position means building; ready == head == LastBlock is the
// necessary completion condition for consumers, not a proof of event history.
type ovmIndexMetadata struct {
	Version       uint64
	Genesis       common.Hash
	Contract      common.Address
	AllowanceSlot uint64
	TransferTopic common.Hash
	ApprovalTopic common.Hash
	FirstBlock    uint64
}

func ovmIndexPosition(header *types.Header) rawdb.OVMIndexPosition {
	return rawdb.OVMIndexPosition{Number: header.Number.Uint64(), Hash: header.Hash()}
}

func (i *ovmMigrationIndex) initialize() (*rawdb.OVMIndexPosition, error) {
	expected, err := rlp.EncodeToBytes(ovmIndexMetadata{1, i.genesis.Hash(), dump.OvmEthAddress, 1, ovmIndexTransferTopic, ovmIndexApprovalTopic, 0})
	if err != nil {
		return nil, err
	}
	meta, err := rawdb.ReadOVMIndexRecord(i.db, rawdb.OVMIndexMetadataKey)
	if err != nil {
		return nil, err
	}
	if meta != nil {
		if !bytes.Equal(meta, expected) {
			return nil, errors.New("OVM index metadata disagrees with genesis or rules")
		}
		head, err := rawdb.ReadOVMIndexPosition(i.db, rawdb.OVMIndexHeadKey)
		if err != nil {
			return nil, err
		}
		if head == nil {
			return nil, errors.New("OVM index metadata exists without head")
		}
		ready, err := rawdb.ReadOVMIndexPosition(i.db, rawdb.OVMIndexReadyKey)
		if err != nil {
			return nil, err
		}
		if ready != nil && *ready != *head {
			return nil, errors.New("OVM index ready marker disagrees with head")
		}
		return head, nil
	}
	fresh, err := rawdb.OVMIndexCanInitialize(i.db, i.genesis.Hash())
	if err != nil {
		return nil, err
	}
	if !fresh {
		return nil, errors.New("OVM indexes require synchronization from genesis; historical backfill is disabled")
	}
	// Never adopt partially populated or foreign namespaces as an empty index.
	for _, prefix := range [][]byte{rawdb.OVMAllowancePrefix, []byte("ovm-transfer-v1/")} {
		it := i.db.NewIteratorWithPrefix(prefix)
		exists := it.Next()
		err := it.Error()
		it.Release()
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, errors.New("OVM index records exist without metadata")
		}
	}
	genesis := ovmIndexPosition(i.genesis)
	canonical, err := rawdb.ReadOVMCanonicalHash(i.db, 0)
	if err != nil {
		return nil, err
	}
	if genesis.Number != 0 || genesis.Hash != canonical || i.genesis.ReceiptHash != types.EmptyRootHash || i.genesis.TxHash != types.EmptyRootHash || i.genesis.GasUsed != 0 || i.genesis.Bloom != (types.Bloom{}) {
		return nil, errors.New("invalid OVM index genesis")
	}
	batch := i.db.NewBatch()
	if err := batch.Put(rawdb.OVMIndexMetadataKey, expected); err != nil {
		return nil, err
	}
	if err := batch.Put(rawdb.OVMIndexHeadKey, rawdb.EncodeOVMIndexPosition(genesis)); err != nil {
		return nil, err
	}
	if err := rawdb.WriteOVMTransferUndo(batch, genesis.Hash, nil); err != nil {
		return nil, err
	}
	if err := batch.Write(); err != nil {
		return nil, err
	}
	return &genesis, nil
}

// rewindToCanonical only removes indexed blocks. In particular, header-only
// reorgs and destructive rewinds must never replay an unexecuted branch.
// It leaves ready absent until align reaches an executed canonical target.
func (i *ovmMigrationIndex) rewindToCanonical(maxHeight uint64) (*rawdb.OVMIndexPosition, error) {
	head, err := i.initialize()
	if err != nil {
		return nil, err
	}
	if err := i.db.Delete(rawdb.OVMIndexReadyKey); err != nil {
		return nil, err
	}
	reported := time.Now()
	for {
		if i.interrupted() {
			return nil, errInsertionInterrupted
		}
		if head.Number <= maxHeight {
			canonical, err := rawdb.ReadOVMCanonicalHash(i.db, head.Number)
			if err != nil {
				return nil, err
			}
			if head.Hash == canonical {
				break
			}
		}
		if head.Number == 0 {
			return nil, errors.New("OVM index has no common genesis")
		}
		head, err = i.undo(*head)
		if err != nil {
			return nil, fmt.Errorf("rewind OVM index: %w", err)
		}
		if time.Since(reported) >= 8*time.Second {
			log.Info("Rewinding OVM migration indexes", "number", head.Number, "target", maxHeight)
			reported = time.Now()
		}
	}
	// Validate the current ancestor even if no forward/reverse work is needed.
	if _, err := rawdb.ReadOVMIndexHeader(i.db, *head); err != nil {
		return nil, err
	}
	if _, err := rawdb.ReadOVMTransferUndo(i.db, head.Hash); err != nil {
		return nil, err
	}
	return head, nil
}

// align consumes a stable canonical view ending at an executed target.
// tipReceipts is used only for that target; nil reads authenticated history.
func (i *ovmMigrationIndex) align(target *types.Header, tipReceipts types.Receipts) error {
	if target == nil || target.Number == nil || !target.Number.IsUint64() {
		return errors.New("invalid OVM index target")
	}
	targetPos := ovmIndexPosition(target)
	canonical, err := rawdb.ReadOVMCanonicalHash(i.db, targetPos.Number)
	if err != nil {
		return err
	}
	if canonical != targetPos.Hash {
		return errors.New("OVM index target is not canonical")
	}
	head, err := i.rewindToCanonical(targetPos.Number)
	if err != nil {
		return err
	}
	reported := time.Now()
	if head.Number < targetPos.Number && targetPos.Number-head.Number > 1 {
		log.Info("Building OVM migration indexes", "from", head.Number, "target", targetPos.Number)
	}
	for head.Number < targetPos.Number {
		if i.interrupted() {
			return errInsertionInterrupted
		}
		number := head.Number + 1
		hash, err := rawdb.ReadOVMCanonicalHash(i.db, number)
		if err != nil {
			return err
		}
		p := rawdb.OVMIndexPosition{Number: number, Hash: hash}
		header, err := rawdb.ReadOVMIndexHeader(i.db, p)
		if err != nil {
			return err
		}
		if header.ParentHash != head.Hash {
			return errors.New("OVM index canonical parent mismatch")
		}
		receipts := tipReceipts
		if p != targetPos || receipts == nil {
			receipts, err = rawdb.ReadOVMIndexReceipts(i.db, p)
			if err != nil {
				return err
			}
		}
		if err := i.apply(header, receipts); err != nil {
			return fmt.Errorf("index OVM block %d (%s): %w", number, hash, err)
		}
		head = &p
		if time.Since(reported) >= 8*time.Second {
			log.Info("Building OVM migration indexes", "number", number, "target", targetPos.Number)
			reported = time.Now()
		}
	}
	if *head != targetPos {
		return errors.New("OVM index did not reach target")
	}
	return i.db.Put(rawdb.OVMIndexReadyKey, rawdb.EncodeOVMIndexPosition(*head))
}

func (i *ovmMigrationIndex) undo(head rawdb.OVMIndexPosition) (*rawdb.OVMIndexPosition, error) {
	header, err := rawdb.ReadOVMIndexHeader(i.db, head)
	if err != nil {
		return nil, err
	}
	parent := rawdb.OVMIndexPosition{Number: head.Number - 1, Hash: header.ParentHash}
	if _, err := rawdb.ReadOVMIndexHeader(i.db, parent); err != nil {
		return nil, err
	}
	addresses, err := rawdb.ReadOVMTransferUndo(i.db, head.Hash)
	if err != nil {
		return nil, err
	}
	batch := i.db.NewBatch()
	for _, address := range addresses {
		if i.interrupted() {
			return nil, errInsertionInterrupted
		}
		member, err := rawdb.ReadOVMTransferMember(i.db, address)
		if err != nil {
			return nil, err
		}
		if member == nil || *member != head {
			return nil, errors.New("OVM undo disagrees with first occurrence")
		}
		if err := rawdb.DeleteOVMTransferMember(batch, address); err != nil {
			return nil, err
		}
	}
	if err := batch.Put(rawdb.OVMIndexHeadKey, rawdb.EncodeOVMIndexPosition(parent)); err != nil {
		return nil, err
	}
	if err := batch.Write(); err != nil {
		return nil, err
	}
	return &parent, nil
}

func validateOVMIndexReceipts(header *types.Header, receipts types.Receipts) error {
	var gas uint64
	for _, receipt := range receipts {
		if receipt == nil {
			return errors.New("nil OVM receipt")
		}
		if receipt.CumulativeGasUsed < gas {
			return errors.New("decreasing OVM cumulative gas")
		}
		gas = receipt.CumulativeGasUsed
		if len(receipt.PostState) == 0 && (receipt.Status > 1 || (receipt.Status == 0 && len(receipt.Logs) != 0)) {
			return errors.New("invalid OVM receipt status/logs")
		}
		if len(receipt.PostState) != 0 && len(receipt.PostState) != 32 {
			return errors.New("invalid OVM receipt post state")
		}
		for _, event := range receipt.Logs {
			if event == nil {
				return errors.New("nil OVM log")
			}
		}
		if types.CreateBloom(types.Receipts{receipt}) != receipt.Bloom {
			return errors.New("OVM receipt bloom disagrees with logs")
		}
	}
	if types.DeriveSha(receipts) != header.ReceiptHash || types.CreateBloom(receipts) != header.Bloom || gas != header.GasUsed || (len(receipts) == 0) != (header.TxHash == types.EmptyRootHash) {
		return errors.New("OVM receipts disagree with header commitments")
	}
	return nil
}

func (i *ovmMigrationIndex) apply(header *types.Header, receipts types.Receipts) error {
	if err := validateOVMIndexReceipts(header, receipts); err != nil {
		return err
	}
	position := ovmIndexPosition(header)
	batch := i.db.NewBatch()
	senders := make(map[common.Address]struct{})
	// Deduplicate uncommitted allowance writes too, and reject conflicting pairs.
	allowances := make(map[common.Hash][40]byte)
	for _, receipt := range receipts {
		for _, event := range receipt.Logs {
			if i.interrupted() {
				return errInsertionInterrupted
			}
			if event.Address != dump.OvmEthAddress || len(event.Topics) == 0 {
				continue
			}
			topic := event.Topics[0]
			if topic != ovmIndexTransferTopic && topic != ovmIndexApprovalTopic {
				continue
			}
			if len(event.Topics) != 3 || len(event.Data) != 32 || !bytes.Equal(event.Topics[1][:12], make([]byte, 12)) || !bytes.Equal(event.Topics[2][:12], make([]byte, 12)) {
				return errors.New("malformed OVM Transfer/Approval")
			}
			from, to := common.BytesToAddress(event.Topics[1][12:]), common.BytesToAddress(event.Topics[2][12:])
			if topic == ovmIndexTransferTopic {
				if common.BytesToHash(event.Data) != (common.Hash{}) {
					senders[from] = struct{}{}
				}
				continue
			}
			key := rawdb.OVMAllowanceTrieKey(from, to)
			var pair [40]byte
			copy(pair[:20], from[:])
			copy(pair[20:], to[:])
			if old, ok := allowances[key]; ok {
				if old != pair {
					return errors.New("conflicting OVM allowance in block")
				}
				continue
			}
			allowances[key] = pair
			if err := rawdb.WriteOVMAllowance(i.db, batch, from, to); err != nil {
				return err
			}
		}
	}
	added := make([]common.Address, 0, len(senders))
	for address := range senders {
		if i.interrupted() {
			return errInsertionInterrupted
		}
		member, err := rawdb.ReadOVMTransferMember(i.db, address)
		if err != nil {
			return err
		}
		if member != nil {
			if member.Number >= position.Number {
				return errors.New("OVM member is ahead of index prefix")
			}
			canonical, err := rawdb.ReadOVMCanonicalHash(i.db, member.Number)
			if err != nil {
				return err
			}
			if member.Hash != canonical {
				return errors.New("OVM member belongs to noncanonical history")
			}
			continue
		}
		added = append(added, address)
		if err := rawdb.WriteOVMTransferMember(batch, address, position); err != nil {
			return err
		}
	}
	sort.Slice(added, func(a, b int) bool { return bytes.Compare(added[a][:], added[b][:]) < 0 })
	if err := rawdb.WriteOVMTransferUndo(batch, position.Hash, added); err != nil {
		return err
	}
	if err := batch.Put(rawdb.OVMIndexHeadKey, rawdb.EncodeOVMIndexPosition(position)); err != nil {
		return err
	}
	if i.interrupted() {
		return errInsertionInterrupted
	}
	return batch.Write()
}

// syncOVMIndex requires chainmu (or exclusive constructor ownership).
func (bc *BlockChain) syncOVMIndex(header *types.Header, receipts types.Receipts) error {
	if bc.ovmIndex == nil || bc.ovmIndex.disabled {
		return nil
	}
	// Header sync can select a fork before its blocks have been executed.
	// LastBlock is then temporarily outside the canonical header chain. Keep
	// only the indexed common prefix, without rejecting startup or consuming
	// receipts from the unexecuted fork.
	canonical, err := rawdb.ReadOVMCanonicalHashIfPresent(bc.db, header.Number.Uint64())
	if err != nil {
		return err
	}
	if canonical == header.Hash() {
		if err := bc.ovmIndex.align(header, receipts); err != nil {
			return fmt.Errorf("OVM migration index: %w", err)
		}
		return nil
	}
	limit := bc.CurrentHeader().Number.Uint64()
	if canonical == (common.Hash{}) && header.Number.Uint64() <= limit {
		return errors.New("missing OVM canonical hash within header chain")
	}
	if header.Number.Uint64() < limit {
		limit = header.Number.Uint64()
	}
	_, err = bc.ovmIndex.rewindToCanonical(limit)
	return err
}

// prepareOVMIndex runs before loadLastState can repair the executed head. An old
// unindexed database remains usable, but never receives a fabricated full index.
func (bc *BlockChain) prepareOVMIndex() error {
	disabled, err := rawdb.ReadOVMIndexRecord(bc.db, rawdb.OVMIndexDisabledKey)
	if err != nil {
		return err
	}
	if disabled != nil {
		log.Warn("OVM migration indexes disabled; resync from genesis with a fresh database to build them", "reason", string(disabled))
		return nil
	}
	meta, err := rawdb.ReadOVMIndexRecord(bc.db, rawdb.OVMIndexMetadataKey)
	if err != nil {
		return err
	}
	if meta == nil {
		fresh, err := rawdb.OVMIndexCanInitialize(bc.db, bc.genesisBlock.Hash())
		if err != nil {
			return err
		}
		if !fresh {
			log.Warn("OVM migration indexes unavailable; continuing without indexing or historical backfill; resync from genesis with a fresh database to build them")
			return nil
		}
	}
	bc.ovmIndex = &ovmMigrationIndex{db: bc.db, genesis: bc.genesisBlock.Header(), interrupted: bc.getProcInterrupt}
	// Commit the genesis prefix before any chain-head changes can occur.
	_, err = bc.ovmIndex.initialize()
	return err
}

// disableOVMIndex requires chainmu. Fast sync may continue, but it cannot claim
// complete execution-derived indexes or backfill the skipped historical prefix.
func (bc *BlockChain) disableOVMIndex(reason string) error {
	batch := bc.db.NewBatch()
	if err := batch.Put(rawdb.OVMIndexDisabledKey, []byte(reason)); err != nil {
		return err
	}
	if err := batch.Delete(rawdb.OVMIndexReadyKey); err != nil {
		return err
	}
	if err := batch.Write(); err != nil {
		return err
	}
	bc.ovmIndex.disabled = true
	log.Warn("OVM migration indexes disabled; continuing without historical backfill; resync from genesis with a fresh database to build them", "reason", reason)
	return nil
}
