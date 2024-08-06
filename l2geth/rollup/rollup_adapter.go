package rollup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/ethereum-optimism/optimism/l2geth/contracts/checkpointoracle/contract/seqset"
	"github.com/ethereum-optimism/optimism/l2geth/core"

	"github.com/ethereum-optimism/optimism/l2geth/common"
	"github.com/ethereum-optimism/optimism/l2geth/core/types"
	"github.com/ethereum-optimism/optimism/l2geth/ethclient"
	"github.com/ethereum-optimism/optimism/l2geth/log"
	"github.com/ethereum-optimism/optimism/l2geth/rollup/rcfg"
)

// RollupAdapter is the adapter for decentralized sequencers
// that is required by the SyncService
type RollupAdapter interface {
	RecoverSeqAddress(tx *types.Transaction) (string, error)
	// get tx sequencer for checking tx is valid
	GetTxSequencer(tx *types.Transaction, expectIndex uint64) (common.Address, error)
	GetEpochByBlockNumber(expectIndex uint64) (struct {
		Number     *big.Int
		Signer     common.Address
		StartBlock *big.Int
		EndBlock   *big.Int
	}, error)
	// check current sequencer is working
	// CheckSequencerIsWorking() bool
	//
	GetSeqValidHeight() uint64
	GetFinalizedBlock() (uint64, error)
	CheckPosLayerSynced() (bool, error)
	ParseUpdateSeqData(data []byte) (bool, common.Address, *big.Int, *big.Int)
	IsSeqSetContractCall(tx *types.Transaction) (bool, []byte)
	IsRespanCall(tx *types.Transaction) bool
	SetPreRespan(oldAddress common.Address, newAddress common.Address, number uint64) error
	IsPreRespanSequencer(seqAddress string, number uint64) bool
	IsNotNextRespanSequencer(seqAddress string, number uint64) bool
	RemoveCachedSeqEpoch()
}

// Cached seq epoch, if recommit or block number < start | > end, clear cache with status false
type CachedSeqEpoch struct {
	Signer     common.Address
	StartBlock *big.Int
	EndBlock   *big.Int
	RespanArr  []*big.Int
	Status     bool
}

// RreRespan is called by bridge, to notify sequencer to prevent save p2p blocks >= RespanStartBlock
type PreRespan struct {
	PreSigner        common.Address
	NewSigner        common.Address
	RespanStartBlock uint64
}

// SeqAdapter is an adpater used by sequencer based RollupClient
type SeqAdapter struct {
	// posClient  // connnect to pos layer
	// l2client // connect to l2geth client
	l2SeqContract          common.Address // l2 seq contract address
	seqContractValidHeight uint64         // l2 seq contract valid height
	localL2Url             string
	localL2Conn            *ethclient.Client
	metisPosURL            string
	metisPosClient         *http.Client
	bc                     *core.BlockChain
	seqContract            *seqset.Seqset
	cachedSeqEpoch         *CachedSeqEpoch
	cachedSeqMux           sync.Mutex
	preRespan              *PreRespan
}

func NewSeqAdapter(l2SeqContract common.Address, seqContractValidHeight uint64, posClientUrl, localL2Url string, bc *core.BlockChain) *SeqAdapter {
	return &SeqAdapter{
		l2SeqContract:          l2SeqContract,
		seqContractValidHeight: seqContractValidHeight,
		metisPosURL:            posClientUrl,
		localL2Url:             localL2Url,

		metisPosClient: &http.Client{},
		bc:             bc,
		cachedSeqEpoch: &CachedSeqEpoch{
			Signer:     common.HexToAddress("0x0"),
			StartBlock: new(big.Int).SetUint64(0),
			EndBlock:   new(big.Int).SetUint64(0),
			Status:     false,
		},
	}
}

func (s *SeqAdapter) ParseUpdateSeqData(data []byte) (bool, common.Address, *big.Int, *big.Int) {
	return core.DecodeReCommitData(data)
}

func (s *SeqAdapter) insertRespanNumber(arr []*big.Int, value *big.Int) []*big.Int {
	index := sort.Search(len(arr), func(i int) bool {
		return arr[i].Cmp(value) >= 0
	})

	arr = append(arr, nil)
	if index < len(arr)-1 {
		copy(arr[index+1:], arr[index:])
	}
	arr[index] = new(big.Int).Set(value)

	return arr
}

func (s *SeqAdapter) removeFirstRespan(arr []*big.Int) []*big.Int {
	if len(arr) > 0 {
		index := 0
		copy(arr[index:], arr[index+1:])
		arr = arr[:len(arr)-1]
	}

	return arr
}

func (s *SeqAdapter) ensureSeqContract() error {
	var err error
	if s.localL2Conn == nil {
		s.localL2Conn, err = ethclient.Dial(s.localL2Url)
		if err != nil {
			return err
		}
		s.seqContract, err = seqset.NewSeqset(s.l2SeqContract, s.localL2Conn)
		if err != nil {
			log.Error("Connect contract err", "l2SeqContract", s.l2SeqContract, "err", err)
			s.localL2Conn = nil
			return err
		}
	}
	return nil
}

func (s *SeqAdapter) GetEpochByBlockNumber(expectIndex uint64) (struct {
	Number     *big.Int
	Signer     common.Address
	StartBlock *big.Int
	EndBlock   *big.Int
}, error) {
	ret := new(struct {
		Number     *big.Int
		Signer     common.Address
		StartBlock *big.Int
		EndBlock   *big.Int
	})
	err := s.ensureSeqContract()
	if err != nil {
		return *ret, err
	}
	epochNumber, err := s.seqContract.GetEpochByBlock(nil, big.NewInt(int64(expectIndex)))
	if err != nil {
		log.Error("Get sequencer error when GetEpochByBlock", "err", err)
		return *ret, err
	}
	currentEpochNumber, err := s.seqContract.CurrentEpochNumber(nil)
	if err != nil {
		log.Error("Get sequencer error when CurrentEpochNumber", "err", err)
		return *ret, err
	}
	if epochNumber.Uint64() > currentEpochNumber.Uint64() {
		log.Error("Get sequencer incorrect epoch number", "epoch", epochNumber.Uint64(), "current", currentEpochNumber.Uint64())
		return *ret, errors.New("get sequencer incorrect epoch number")
	}
	epoch, err := s.seqContract.Epochs(nil, epochNumber)
	if err != nil {
		log.Error("Get sequencer error when Epochs", "err", err)
		return *ret, err
	}
	return epoch, nil
}

func (s *SeqAdapter) getSequencer(expectIndex uint64) (common.Address, error) {
	err := s.ensureSeqContract()
	if err != nil {
		return common.Address{}, err
	}
	if s.cachedSeqEpoch.Status && (s.cachedSeqEpoch.StartBlock.Uint64() > expectIndex || s.cachedSeqEpoch.EndBlock.Uint64() < expectIndex) {
		s.cachedSeqEpoch.Status = false
	}
	// check RespanArr[0]
	blockNumber := uint64(0)
	block := s.bc.CurrentBlock()
	if block != nil {
		blockNumber = block.Number().Uint64()
	}
	// clear preRespan when block >= respanStart
	if s.preRespan != nil && s.preRespan.RespanStartBlock <= blockNumber {
		log.Info("clear pre respan")
		s.preRespan = nil
	}
	if len(s.cachedSeqEpoch.RespanArr) > 0 {
		// at this time, blockChain has not reach the respan height, pause sequencer with an error
		respanStart := s.cachedSeqEpoch.RespanArr[0].Uint64()
		if expectIndex > respanStart && blockNumber < respanStart {
			log.Error("Get sequencer error when check respan", "expectIndex", expectIndex, "blockNumber", blockNumber, "respanStart", respanStart)
			return common.Address{}, errors.New("get sequencer error when check respan")
		}
		if blockNumber >= respanStart {
			// remove [0], reload cache
			s.cachedSeqEpoch.RespanArr = s.removeFirstRespan(s.cachedSeqEpoch.RespanArr)
			s.cachedSeqEpoch.Status = false
		}
	}
	if !s.cachedSeqEpoch.Status {
		// start loading cache
		log.Info("get tx seqeuencer start epoch cache")
		// re-cache the epoch info
		epoch, err := s.GetEpochByBlockNumber(expectIndex)
		if err != nil {
			log.Error("Get sequencer error when Epochs", "err", err)
			return common.Address{}, err
		}
		s.cachedSeqEpoch.Signer = epoch.Signer
		s.cachedSeqEpoch.StartBlock = epoch.StartBlock
		s.cachedSeqEpoch.EndBlock = epoch.EndBlock
		s.cachedSeqEpoch.Status = true
		// loaded epoch cache
		log.Info("get tx seqeuencer loaded epoch cache", "status", s.cachedSeqEpoch.Status, "start", s.cachedSeqEpoch.StartBlock.Uint64(), "end", s.cachedSeqEpoch.EndBlock.Uint64(), "signer", s.cachedSeqEpoch.Signer.String())
	}
	return s.cachedSeqEpoch.Signer, nil
}

func (s *SeqAdapter) GetSeqValidHeight() uint64 {
	return s.seqContractValidHeight
}

func (s *SeqAdapter) GetFinalizedBlock() (uint64, error) {
	blockNumber := uint64(0)
	block := s.bc.CurrentBlock()
	if block != nil {
		blockNumber = block.Number().Uint64()
	}
	if blockNumber < s.seqContractValidHeight {
		return blockNumber, nil
	}
	err := s.ensureSeqContract()
	if err != nil {
		return 0, err
	}
	finalizedBlock, err := s.seqContract.FinalizedBlock(nil)
	if err != nil {
		log.Error("Get finalizedBlock err", "l2SeqContract", s.l2SeqContract, "err", err)
		return 0, err
	}
	return finalizedBlock.Uint64(), nil
}

func (s *SeqAdapter) RecoverSeqAddress(tx *types.Transaction) (string, error) {
	addr, err := core.RecoverSeqAddress(tx)
	if err != nil {
		return "", err
	}
	return addr.String(), nil
}

func (s *SeqAdapter) IsSeqSetContractCall(tx *types.Transaction) (bool, []byte) {
	if (tx.To() == nil || s.l2SeqContract == common.Address{}) {
		return false, nil
	}
	// from equals MPC
	err := s.ensureSeqContract()
	if err != nil {
		return false, nil
	}
	sender, err := types.Sender(types.NewEIP155Signer(s.bc.Config().ChainID), tx)
	if err != nil {
		return false, nil
	}
	mpcAddress, err := s.seqContract.MpcAddress(nil)
	if err != nil {
		log.Error("Get sequencer error when query mpc address from contract", "err", err)
		return false, nil
	}
	if sender == mpcAddress && strings.EqualFold(tx.To().String(), s.l2SeqContract.String()) {
		return true, tx.Data()
	}
	return false, nil
}

func (s *SeqAdapter) IsRespanCall(tx *types.Transaction) bool {
	if tx == nil {
		return false
	}
	seqOper, data := s.IsSeqSetContractCall(tx)
	if !seqOper {
		return false
	}
	isRespan, _, _, _ := s.ParseUpdateSeqData(data)
	return isRespan
}

func (s *SeqAdapter) SetPreRespan(oldAddress common.Address, newAddress common.Address, number uint64) error {
	// s.preRespan = &PreRespan{
	// 	PreSigner:        oldAddress,
	// 	NewSigner:        newAddress,
	// 	RespanStartBlock: number,
	// }
	log.Info("set pre respan, log", "preSigner", oldAddress.Hex(), "newSigner", newAddress.Hex(), "startBlock", number)
	return nil
}

func (s *SeqAdapter) IsPreRespanSequencer(seqAddress string, number uint64) bool {
	if s.preRespan == nil || s.preRespan.RespanStartBlock == 0 || (s.preRespan.PreSigner == common.Address{}) {
		return false
	}
	if number >= s.preRespan.RespanStartBlock && strings.EqualFold(s.preRespan.PreSigner.Hex(), seqAddress) {
		return true
	}
	return false
}

func (s *SeqAdapter) IsNotNextRespanSequencer(seqAddress string, number uint64) bool {
	if s.preRespan == nil || s.preRespan.RespanStartBlock == 0 || (s.preRespan.NewSigner == common.Address{}) {
		return false
	}
	if number >= s.preRespan.RespanStartBlock && !strings.EqualFold(s.preRespan.NewSigner.Hex(), seqAddress) {
		return true
	}
	return false
}

func (s *SeqAdapter) GetTxSequencer(tx *types.Transaction, expectIndex uint64) (common.Address, error) {
	// check is update sequencer operate
	if expectIndex <= s.seqContractValidHeight {
		// return default address
		return rcfg.DefaultSeqAdderss, nil
	}

	s.cachedSeqMux.Lock()
	defer s.cachedSeqMux.Unlock()

	if tx != nil {
		seqOper, data := s.IsSeqSetContractCall(tx)
		if seqOper {
			updateSeq, newSeq, startBlock, endBlock := s.ParseUpdateSeqData(data)
			if updateSeq {
				// respan
				log.Info("get tx seqeuencer respan", "respan-start", startBlock.Uint64(), "respan-end", endBlock.Uint64(), "respan-signer", newSeq.String())
				// cache the respan start block
				s.cachedSeqEpoch.RespanArr = s.insertRespanNumber(s.cachedSeqEpoch.RespanArr, startBlock)
				// return the respan sequencer, it will mine. it can success or fail
				return newSeq, nil
			}
		}
	}

	// log.Debug("Will get sequencer info from seq contract on L2")
	// get status from contract on height expectIndex - 1
	// return result ,err
	address, err := s.getSequencer(expectIndex)
	log.Debug("GetTxSequencer ", "getSequencer address", address, "expectIndex", expectIndex, "contract address ", s.l2SeqContract, "err", err)

	// a special case, when error with "get sequencer incorrect epoch number",
	// allows transfers to mpcAddress, and the block is generated by the sequencer of the previous epoch.
	if tx != nil && err != nil && err.Error() == "get sequencer incorrect epoch number" {
		if s.localL2Conn == nil {
			log.Error("Get sequencer error when local l2conn nil")
			return address, err
		}
		mpcAddress, err2 := s.seqContract.MpcAddress(nil)
		if err2 != nil {
			log.Error("Get sequencer error when query mpc address from contract", "err", err2)
			return address, err
		}
		if (mpcAddress == common.Address{}) {
			log.Error("Get sequencer error when mpc address nil")
			return address, err
		}
		// min value = 10 METIS
		minValue := new(big.Int)
		minValue.SetString("10000000000000000000", 10)
		if tx.To() == nil || tx.To().Hex() != mpcAddress.Hex() || tx.Value() == nil || tx.Value().Cmp(minValue) <= 0 {
			// log.Error("Get sequencer error when compare tx to and value", "mpc address", mpcAddress.Hex())
			return address, err
		}
		// check mpcAddress balance <= 0.5 METIS
		bgCtx := context.Background()
		rawBalance, err2 := s.localL2Conn.BalanceAt(bgCtx, mpcAddress, nil)
		if err2 != nil {
			log.Error("Get sequencer error when query mpc address balance", "err", err2)
			return address, err
		}
		wantBalance := new(big.Int)
		wantBalance.SetString("500000000000000000", 10)
		if rawBalance.Cmp(wantBalance) > 0 {
			log.Error("Mpc address has enough balance", "got", rawBalance, "want below", wantBalance)
			return address, err
		}
		// get latest sequencer
		currentEpochNumber, err2 := s.seqContract.CurrentEpochNumber(nil)
		if err2 != nil {
			log.Error("Get sequencer error when CurrentEpochNumber 2", "err", err2)
			return address, err
		}
		epoch, err2 := s.seqContract.Epochs(nil, currentEpochNumber)
		if err2 != nil {
			log.Error("Get sequencer error when Epochs 2", "err", err2)
			return address, err
		}
		return epoch.Signer, nil
	}

	return address, err
}

func (s *SeqAdapter) RemoveCachedSeqEpoch() {
	s.cachedSeqMux.Lock()
	defer s.cachedSeqMux.Unlock()
	s.cachedSeqEpoch.Status = false
	log.Info("Removed cachedSeqEpoch")
}

func (s *SeqAdapter) CheckPosLayerSynced() (bool, error) {
	// get pos layer synced
	if s == nil {
		return false, errors.New("client is null")
	}
	path := fmt.Sprintf("%v/checkPosIsSynced", s.metisPosURL)
	resp, err := s.metisPosClient.Get(path)

	if err != nil {
		fmt.Println("error:", err)
		return false, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}
	//fmt.Println(string(body))

	return strconv.ParseBool(string(body))

}
