package solver

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ethereum-optimism/optimism/go/op-challenger/game/fault/test"
	"github.com/ethereum-optimism/optimism/go/op-challenger/game/fault/types"
)

func TestHonestClaimTracker_RootClaim(t *testing.T) {
	tracker := newHonestClaimTracker()
	builder := test.NewAlphabetClaimBuilder(t, big.NewInt(3), 4)

	claim := builder.Seq().Get()
	require.False(t, tracker.IsHonest(claim))

	tracker.AddHonestClaim(types.Claim{}, claim)
	require.True(t, tracker.IsHonest(claim))
}

func TestHonestClaimTracker_ChildClaim(t *testing.T) {
	tracker := newHonestClaimTracker()
	builder := test.NewAlphabetClaimBuilder(t, big.NewInt(3), 4)

	seq := builder.Seq().Attack().Defend()
	parent := seq.Get()
	child := seq.Attack().Get()
	require.Zero(t, child.ContractIndex, "should work for claims that are not in the game state yet")

	tracker.AddHonestClaim(parent, child)
	require.False(t, tracker.IsHonest(parent))
	require.True(t, tracker.IsHonest(child))
	counter, ok := tracker.HonestCounter(parent)
	require.True(t, ok)
	require.Equal(t, child, counter)
}
