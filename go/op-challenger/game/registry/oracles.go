package registry

import (
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/exp/maps"

	keccakTypes "github.com/ethereum-optimism/optimism/go/op-challenger/game/keccak/types"
)

type OracleRegistry struct {
	l       sync.Mutex
	oracles map[common.Address]keccakTypes.LargePreimageOracle
}

func NewOracleRegistry() *OracleRegistry {
	return &OracleRegistry{
		oracles: make(map[common.Address]keccakTypes.LargePreimageOracle),
	}
}

func (r *OracleRegistry) RegisterOracle(oracle keccakTypes.LargePreimageOracle) {
	r.l.Lock()
	defer r.l.Unlock()
	r.oracles[oracle.Addr()] = oracle
}

func (r *OracleRegistry) Oracles() []keccakTypes.LargePreimageOracle {
	r.l.Lock()
	defer r.l.Unlock()
	return maps.Values(r.oracles)
}
