package types

import "github.com/ethereum-optimism/optimism/l2geth/common"

type L1BlockRef struct {
	Hash       common.Hash `json:"hash"`
	Number     uint64      `json:"number"`
	ParentHash common.Hash `json:"parentHash"`
	Time       uint64      `json:"timestamp"`
}

type L2BlockRef struct {
	Hash           common.Hash `json:"hash"`
	Number         uint64      `json:"number"`
	ParentHash     common.Hash `json:"parentHash"`
	Time           uint64      `json:"timestamp"`
	L1Origin       BlockID     `json:"l1origin"`
	SequenceNumber uint64      `json:"sequenceNumber"` // distance to first block of epoch
}

type BlockID struct {
	Hash   common.Hash `json:"hash"`
	Number uint64      `json:"number"`
}

// SyncStatus is a snapshot of the driver. (migrated from optimism v1.9.1)
// Values may be zeroed if not yet initialized.
type SyncStatus struct {
	// CurrentL1 is the L1 block that the derivation process is last idled at.
	// This may not be fully derived into L2 data yet.
	// The safe L2 blocks were produced/included fully from the L1 chain up to and including this L1 block.
	// If the node is synced, this matches the HeadL1, minus the verifier confirmation distance.
	CurrentL1 L1BlockRef `json:"currentL1"`
	// CurrentL1Finalized is a legacy sync-status attribute. This is deprecated.
	// A previous version of the L1 finalization-signal was updated only after the block was retrieved by number.
	// This attribute just matches FinalizedL1 now.
	CurrentL1Finalized L1BlockRef `json:"currentL1Finalized"`
	// HeadL1 is the perceived head of the L1 chain, no confirmation distance.
	// The head is not guaranteed to build on the other L1 sync status fields,
	// as the node may be in progress of resetting to adapt to a L1 reorg.
	HeadL1      L1BlockRef `json:"headL1"`
	SafeL1      L1BlockRef `json:"safeL1"`
	FinalizedL1 L1BlockRef `json:"finalizedL1"`
	// UnsafeL2 is the absolute tip of the L2 chain,
	// pointing to block data that has not been submitted to L1 yet.
	// The sequencer is building this, and verifiers may also be ahead of the
	// SafeL2 block if they sync blocks via p2p or other offchain sources.
	UnsafeL2 L2BlockRef `json:"unsafeL2"`
	// SafeL2 points to the L2 block that was derived from the L1 chain.
	// This point may still reorg if the L1 chain reorgs.
	SafeL2 L2BlockRef `json:"safeL2"`
	// FinalizedL2 points to the L2 block that was derived fully from
	// finalized L1 information, thus irreversible.
	FinalizedL2 L2BlockRef `json:"finalizedL2"`
	// PendingSafeL2 points to the L2 block processed from the batch, but not consolidated to the safe block yet.
	PendingSafeL2 L2BlockRef `json:"pendingSafeL2"`
}
