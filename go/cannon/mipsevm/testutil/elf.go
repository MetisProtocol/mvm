package testutil

import (
	"debug/elf"

	"github.com/stretchr/testify/require"

	"github.com/ethereum-optimism/optimism/go/cannon/mipsevm"
	"github.com/ethereum-optimism/optimism/go/cannon/mipsevm/program"
)

func LoadELFProgram[T mipsevm.FPVMState](t require.TestingT, name string, initState program.CreateInitialFPVMState[T], doPatchGo bool) (T, *program.Metadata) {
	elfProgram, err := elf.Open(name)
	require.NoError(t, err, "open ELF file")
	meta, err := program.MakeMetadata(elfProgram)
	require.NoError(t, err, "load metadata")

	state, err := program.LoadELF(elfProgram, initState)
	require.NoError(t, err, "load ELF into state")

	if doPatchGo {
		err = program.PatchGo(elfProgram, state)
		require.NoError(t, err, "apply Go runtime patches")
	}

	require.NoError(t, program.PatchStack(state), "add initial stack")
	return state, meta
}