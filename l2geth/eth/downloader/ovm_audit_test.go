package downloader

import (
	"errors"
	"math/big"
	"testing"

	"github.com/MetisProtocol/mvm/l2geth/core"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
)

type auditErrorChain struct {
	BlockChain
	err error
}

func (c auditErrorChain) InsertChain(types.Blocks) (int, error) { return 0, c.err }

func TestOVMAuditImportError(t *testing.T) {
	for _, cause := range []error{nil, errors.New("disk full")} {
		stop := &core.OVMAuditControl{Err: cause}
		d := &Downloader{blockchain: auditErrorChain{err: stop}}
		err := d.importBlockResults([]*fetchResult{{Header: &types.Header{Number: big.NewInt(1), Difficulty: big.NewInt(1)}}})
		if err != stop || err == errInvalidChain {
			t.Fatalf("local stop changed into peer failure: %v", err)
		}
	}
}
