package core

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/core/rawdb"
	"github.com/MetisProtocol/mvm/l2geth/ethdb"
	"github.com/MetisProtocol/mvm/l2geth/log"
)

// Inject only into an index apply batch, after canonical publication succeeds.
type publicationFaultDB struct {
	ethdb.Database
	mode string
}

type publicationFaultBatch struct {
	ethdb.Batch
	db    *publicationFaultDB
	index bool
}

func (d *publicationFaultDB) NewBatch() ethdb.Batch {
	return &publicationFaultBatch{Batch: d.Database.NewBatch(), db: d}
}

func (b *publicationFaultBatch) Put(key, value []byte) error {
	if bytes.Equal(key, rawdb.OVMIndexHeadKey) {
		b.index = true
	}
	return b.Batch.Put(key, value)
}

func (b *publicationFaultBatch) Write() error {
	if b.index && b.db.mode == "before" {
		return errIndexInjected
	}
	if err := b.Batch.Write(); err != nil {
		return err
	}
	if b.index && b.db.mode == "after" {
		return errIndexInjected
	}
	return nil
}

func (d *publicationFaultDB) Put(key, value []byte) error {
	if d.mode == "ready" && bytes.Equal(key, rawdb.OVMIndexReadyKey) {
		return errIndexInjected
	}
	return d.Database.Put(key, value)
}

func TestOVMIndexPublicationFailure(t *testing.T) {
	const childDir = "MVM_OVM_INDEX_FAILURE_DIR"
	const childMode = "MVM_OVM_INDEX_FAILURE_MODE"
	open := func(t *testing.T, dir string) ethdb.Database {
		t.Helper()
		db, err := rawdb.NewLevelDBDatabaseWithFreezer(filepath.Join(dir, "chaindata"), 16, 16, filepath.Join(dir, "ancient"), "")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		return db
	}
	if dir := os.Getenv(childDir); dir != "" {
		log.Root().SetHandler(log.StreamHandler(os.Stderr, log.LogfmtFormat()))
		genesis, blocks, _ := ovmPreimageBlocks(t)
		db := &publicationFaultDB{Database: open(t, dir)}
		genesis.MustCommit(db)
		chain := openIndexChain(t, db, genesis)
		if _, err := chain.InsertChain(blocks[:1]); err != nil {
			t.Fatal(err)
		}
		db.mode = os.Getenv(childMode)
		if db.mode == "interrupt" {
			// Simulate Stop's interruption after import has passed its initial
			// checks, but before index reconciliation can finish.
			chain.ovmIndex.interrupted = func() bool {
				atomic.StoreInt32(&chain.procInterrupt, 1)
				return true
			}
		}
		_, err := chain.InsertChain(blocks[1:])
		if db.mode != "interrupt" {
			// A retry must never be possible in this process. Use a distinct
			// exit code so a regression cannot masquerade as the fatal exit.
			os.Exit(42)
		}
		if !errors.Is(err, errInsertionInterrupted) {
			t.Fatalf("shutdown interruption: %v", err)
		}
		chain.Stop()
		return
	}
	for _, mode := range []string{"before", "after", "ready", "interrupt"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestOVMIndexPublicationFailure$")
			cmd.Env = append(os.Environ(), childDir+"="+dir, childMode+"="+mode)
			output, err := cmd.CombinedOutput()
			if bytes.Contains(output, []byte("WARNING: DATA RACE")) {
				t.Fatalf("race in publication failure subprocess:\n%s", output)
			}
			if mode == "interrupt" {
				if err != nil {
					t.Fatalf("shutdown failed: %v\n%s", err, output)
				}
			} else {
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != 1 || !bytes.Contains(output, []byte("Failed to reconcile OVM index after canonical publication")) {
					t.Fatalf("expected fatal publication failure: %v\n%s", err, output)
				}
			}
			// Reopen the real database after process exit, with no injected
			// failure. Startup must reconcile the published block exactly once.
			genesis, blocks, addresses := ovmPreimageBlocks(t)
			db := open(t, dir)
			if rawdb.ReadHeadBlockHash(db) != blocks[1].Hash() {
				t.Fatal("fixture did not publish canonical head before failure")
			}
			chain := openIndexChain(t, db, genesis)
			assertIndexReady(t, db, blocks[1])
			assertIndexMembers(t, db, map[common.Address]rawdb.OVMIndexPosition{addresses[2]: ovmIndexPosition(blocks[1].Header())})
			if _, err := chain.InsertChain(blocks[1:]); err != nil {
				t.Fatal(err)
			}
			assertIndexReady(t, db, blocks[1])
		})
	}
}
