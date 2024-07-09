package core

import (
	"encoding/hex"
	"errors"
	"math/big"

	"github.com/ethereum-optimism/optimism/l2geth/common"
	"github.com/ethereum-optimism/optimism/l2geth/core/state"
	"github.com/ethereum-optimism/optimism/l2geth/core/types"
	"github.com/ethereum-optimism/optimism/l2geth/crypto"
	"github.com/ethereum-optimism/optimism/l2geth/log"
)

const (
	seqsetRecommitMethod  = ("2c91c679")               // RecommitEpoch is a paid mutator transaction binding the contract method 0x2c91c679, when input data does not has prefix 0x
	seqsetRecommitDataLen = 4 + 32 + 32 + 32 + 32 + 32 // uint256 oldEpochId,uint256 newEpochId, uint256 startBlock,uint256 endBlock, address newSigner
)

type Epoch struct {
	Number     *big.Int       // uint256
	Signer     common.Address // address
	StartBlock *big.Int       // uint256
	EndBlock   *big.Int       // uint256
}

var epochCache []*Epoch = make([]*Epoch, 0, 10)

func DecodeReCommitData(data []byte) (bool, common.Address, *big.Int, *big.Int) {
	zeroBigInt := new(big.Int).SetUint64(0)
	if len(data) < seqsetRecommitDataLen {
		return false, common.HexToAddress("0x0"), zeroBigInt, zeroBigInt
	}
	method := hex.EncodeToString(data[0:4])
	startBlock := new(big.Int).SetBytes(data[2*32+4 : 3*32+4])
	endBlock := new(big.Int).SetBytes(data[3*32+4 : 4*32+4])
	signer := common.BytesToAddress(data[4*32+16 : 4*32+36])
	if method == seqsetRecommitMethod {
		return true, signer, startBlock, endBlock
	}
	return false, common.HexToAddress("0x0"), zeroBigInt, zeroBigInt
}

func RecoverSeqAddress(tx *types.Transaction) (common.Address, error) {
	// enqueue tx no sign
	if tx.QueueOrigin() == types.QueueOriginL1ToL2 {
		return common.Address{}, errors.New("enqueue seq sign is null")
	}
	seqSign := tx.GetSeqSign()
	if seqSign == nil {
		return common.Address{}, errors.New("seq sign is null")
	}

	var signBytes []byte
	signBytes = append(signBytes, seqSign.R.FillBytes(make([]byte, 32))...)
	signBytes = append(signBytes, seqSign.S.FillBytes(make([]byte, 32))...)
	signBytes = append(signBytes, byte(seqSign.V.Int64()))

	signer, err := crypto.SigToPub(tx.Hash().Bytes(), signBytes)
	if err != nil {
		return common.Address{}, err
	}
	return crypto.PubkeyToAddress(*signer), nil
}

func updateEpochCache(currentEpochId *big.Int, epoch *Epoch, prependBeginning bool) {
	if prependBeginning {
		if len(epochCache) > 0 && epochCache[0].Number.Cmp(currentEpochId) == 0 {
			if epochCache[0].Signer != epoch.Signer || epochCache[0].StartBlock.Cmp(epoch.StartBlock) != 0 || epochCache[0].EndBlock.Cmp(epoch.EndBlock) != 0 {
				epochCache[0] = epoch
			}
		} else {
			epochCache = append([]*Epoch{epoch}, epochCache...)
		}
	} else {
		epochCache = append(epochCache, epoch)
	}
	if len(epochCache) > 10 {
		epochCache = epochCache[:10]
	}
}

func processSeqSetBlock(bc *BlockChain, statedb *state.StateDB, block *types.Block, parent *types.Header) error {
	currentBN := new(big.Int).Add(big.NewInt(1), parent.Number)
	if !bc.Config().IsSeqSetPeerEnabled(currentBN) {
		return nil
	}
	// check seqset
	seqsetAddr := bc.Config().MetisSeqSetContract()
	if seqsetAddr.IsZero() {
		return ErrNoSeqSetAddress
	}

	// recommit and l1 not validate
	if len(block.Transactions()) == 1 {
		currentTx := block.Transactions()[0]
		if currentTx.QueueOrigin() == types.QueueOriginL1ToL2 {
			return nil
		}
		// check recommit method
		to := currentTx.To()
		if to != nil && *to == seqsetAddr {
			// decode tx data
			isRecommit, newSequencer, _, _ := DecodeReCommitData(currentTx.Data())
			recoverSigner, err := RecoverSeqAddress(currentTx)
			if err != nil {
				return err
			}
			if isRecommit {
				if newSequencer != recoverSigner {
					return ErrIncorrectSequencer
				}
				return nil
			}
		}
	}

	// get currentEpochId, slot index is 104
	currentEpochIdSlot := big.NewInt(104)
	currentEpochId := statedb.GetState(seqsetAddr, common.BytesToHash(currentEpochIdSlot.Bytes())).Big()

	// epoch id slice
	prependBeginning := true
	epochIds := []*big.Int{currentEpochId}
	if len(epochCache) == 0 {
		prependBeginning = false
		for i := 1; i <= 10; i++ {
			epochId := new(big.Int).Sub(currentEpochId, big.NewInt(int64(i)))
			if epochId.Cmp(big.NewInt(0)) < 0 {
				break
			}
			epochIds = append(epochIds, epochId)
		}
	}

	// get epoch, base slot index is 103
	for _, epochId := range epochIds {
		epochsSlot := big.NewInt(103)
		numberSlot := crypto.Keccak256Hash(append(common.LeftPadBytes(epochId.Bytes(), 32), common.LeftPadBytes(epochsSlot.Bytes(), 32)...)).Big()
		signerSlot := new(big.Int).Add(numberSlot, common.Big1)
		startBlockSlot := new(big.Int).Add(signerSlot, common.Big1)
		endBlockSlot := new(big.Int).Add(startBlockSlot, common.Big1)

		numberData := statedb.GetState(seqsetAddr, common.BytesToHash(numberSlot.Bytes())).Bytes()
		signerData := statedb.GetState(seqsetAddr, common.BytesToHash(signerSlot.Bytes())).Bytes()
		startBlockData := statedb.GetState(seqsetAddr, common.BytesToHash(startBlockSlot.Bytes())).Bytes()
		endBlockData := statedb.GetState(seqsetAddr, common.BytesToHash(endBlockSlot.Bytes())).Bytes()

		epoch := &Epoch{
			Number:     new(big.Int).SetBytes(numberData),
			Signer:     common.BytesToAddress(signerData),
			StartBlock: new(big.Int).SetBytes(startBlockData),
			EndBlock:   new(big.Int).SetBytes(endBlockData),
		}

		log.Debug("Read epoch from slot", "epoch", epoch)

		updateEpochCache(currentEpochId, epoch, prependBeginning)
	}

	// check first tx is enough
	currentTx := block.Transactions()[0]
	recoverSigner, err := RecoverSeqAddress(currentTx)
	if err != nil {
		return err
	}
	found := false
	for _, epoch := range epochCache {
		if epoch.Signer == recoverSigner {
			found = true
			break
		}
	}
	if !found {
		return ErrIncorrectSequencer
	}
	return nil
}
