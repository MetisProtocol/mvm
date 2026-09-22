package state

// The observer records auxiliary evidence only. It never journals or mutates
// consensus state. In particular, Copy must not copy an active observer.
import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/core/types"
	"github.com/MetisProtocol/mvm/l2geth/ethdb"
	"github.com/MetisProtocol/mvm/l2geth/rollup/dump"
	"github.com/MetisProtocol/mvm/l2geth/trie"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

var OVMAuditPrefix = []byte("ovmaudit/v1/")

type OVMAuditSlot struct {
	Kind    string         `json:"kind"`
	Address common.Address `json:"address,omitempty"`
}

// OVMAuditIndex uses hashed storage keys, also usable when scanning a secure trie.
type OVMAuditIndex struct {
	DB      ethdb.KeyValueReader
	Pending map[common.Hash]OVMAuditSlot
	Err     error
	Check   func() error
}

func NewOVMAuditIndex(db ethdb.KeyValueReader) *OVMAuditIndex {
	return &OVMAuditIndex{DB: db, Pending: make(map[common.Hash]OVMAuditSlot)}
}

func OVMAuditSlotKey(hash common.Hash) []byte {
	return append(append([]byte{}, OVMAuditPrefix...), append([]byte("slot/"), hash[:]...)...)
}

func (i *OVMAuditIndex) Lookup(hash common.Hash) (OVMAuditSlot, bool) {
	if slot, ok := i.Pending[hash]; ok {
		return slot, true
	}
	key := OVMAuditSlotKey(hash)
	ok, err := i.DB.Has(key)
	if err != nil {
		i.Err = err
		return OVMAuditSlot{}, false
	}
	if !ok {
		return OVMAuditSlot{}, false
	}
	blob, err := i.DB.Get(key)
	var slot OVMAuditSlot
	if err == nil {
		err = json.Unmarshal(blob, &slot)
	}
	if err != nil {
		i.Err = err
		return OVMAuditSlot{}, false
	}
	return slot, true
}

func (i *OVMAuditIndex) Add(raw common.Hash, slot OVMAuditSlot) {
	hash := crypto.Keccak256Hash(raw[:])
	if old, ok := i.Lookup(hash); ok {
		if old != slot {
			i.Err = fmt.Errorf("conflicting OVM slot classification %s", raw.Hex())
		}
		return
	}
	i.Pending[hash] = slot
}

func (i *OVMAuditIndex) Address(addr common.Address) {
	i.Add(GetOVMBalanceKey(addr), OVMAuditSlot{Kind: "balance", Address: addr})
}

func (i *OVMAuditIndex) Allowance(owner, spender common.Address) {
	i.Address(owner)
	i.Address(spender)
	inner := crypto.Keccak256Hash(common.LeftPadBytes(owner[:], 32), common.LeftPadBytes([]byte{1}, 32))
	i.Add(inner, OVMAuditSlot{Kind: "allowance-base", Address: owner})
	outer := crypto.Keccak256Hash(common.LeftPadBytes(spender[:], 32), inner[:])
	i.Add(outer, OVMAuditSlot{Kind: "allowance"})
}

func (i *OVMAuditIndex) Preimage(hash common.Hash, value []byte) {
	if crypto.Keccak256Hash(value) != hash {
		i.Err = fmt.Errorf("invalid preimage %s", hash.Hex())
		return
	}
	if len(value) == 20 {
		i.Address(common.BytesToAddress(value))
		return
	}
	if len(value) != 64 || !bytes.Equal(value[:12], make([]byte, 12)) {
		return
	}
	addr := common.BytesToAddress(value[:32])
	base := common.BytesToHash(value[32:])
	switch base {
	case common.Hash{}:
		i.Address(addr)
	case common.BigToHash(big.NewInt(1)):
		i.Address(addr)
		i.Add(hash, OVMAuditSlot{Kind: "allowance-base", Address: addr})
	default:
		if slot, ok := i.Lookup(crypto.Keccak256Hash(base[:])); ok && slot.Kind == "allowance-base" {
			i.Allowance(slot.Address, addr)
		}
	}
}

func (i *OVMAuditIndex) Log(l *types.Log) {
	if l.Address != dump.OvmEthAddress || len(l.Topics) != 3 || len(l.Data) != 32 {
		return
	}
	if !bytes.Equal(l.Topics[1][:12], make([]byte, 12)) || !bytes.Equal(l.Topics[2][:12], make([]byte, 12)) {
		return
	}
	a, b := common.BytesToAddress(l.Topics[1][:]), common.BytesToAddress(l.Topics[2][:])
	switch l.Topics[0] {
	case ovmTransferTopic:
		i.Address(a)
		i.Address(b)
	case crypto.Keccak256Hash([]byte("Approval(address,address,uint256)")):
		i.Allowance(a, b)
	}
}

func (i *OVMAuditIndex) Write(w ethdb.KeyValueWriter) error {
	if i.Err != nil {
		return i.Err
	}
	for hash, slot := range i.Pending {
		blob, err := json.Marshal(slot)
		if err != nil {
			return err
		}
		if err = w.Put(OVMAuditSlotKey(hash), blob); err != nil {
			return err
		}
	}
	return nil
}

func (i *OVMAuditIndex) Metadata(s *StateDB) error {
	for n := int64(0); n <= 6; n++ {
		raw := common.BigToHash(big.NewInt(n))
		kind := "metadata"
		if n == 2 {
			kind = "supply"
		}
		i.Add(raw, OVMAuditSlot{Kind: kind})
		value := s.GetState(dump.OvmEthAddress, raw)
		if n < 2 && value != (common.Hash{}) {
			return fmt.Errorf("nonzero OVM mapping base slot %d", n)
		}
		if n != 3 && n != 4 {
			continue
		}
		if value[31]&1 == 0 {
			length := int(value[31] / 2)
			if length > 31 || !bytes.Equal(value[length:31], make([]byte, 31-length)) {
				return fmt.Errorf("invalid OVM string slot %d", n)
			}
			continue
		}
		length := new(big.Int).Rsh(value.Big(), 1)
		// Bound auxiliary work on corrupt/unexpected layouts (32 MiB strings).
		if !length.IsUint64() || length.Uint64() < 32 || length.Uint64() > 32<<20 {
			return fmt.Errorf("unsupported OVM string length at slot %d", n)
		}
		base := crypto.Keccak256Hash(raw[:]).Big()
		for k := uint64(0); k < (length.Uint64()+31)/32; k++ {
			i.Add(common.BigToHash(new(big.Int).Add(base, new(big.Int).SetUint64(k))), OVMAuditSlot{Kind: "metadata"})
		}
	}
	if err := s.Error(); err != nil {
		return err
	}
	return i.Err
}

type OVMAuditCause struct {
	Kind     string         `json:"kind"`
	From     common.Address `json:"from"`
	To       common.Address `json:"to"`
	Amount   string         `json:"amountWei"`
	Required string         `json:"requiredWei,omitempty"`
	Debited  string         `json:"debitedWei,omitempty"`
	GasLimit uint64         `json:"gasLimit,omitempty"`
	GasPrice string         `json:"gasPriceWei,omitempty"`
	PC       uint64         `json:"pc,omitempty"`
	Depth    int            `json:"depth,omitempty"`
	SDUpdate bool           `json:"sdUpdate"`
	Shanghai bool           `json:"shanghai"`
	Reverted bool           `json:"reverted"`
}

type OVMAuditDelta struct {
	Balance     string `json:"balanceDeltaWei"`
	Supply      string `json:"supplyDeltaWei"`
	Difference  string `json:"discrepancyDeltaWei"`
	Gas         string `json:"gasShortfallWei"`
	Suicide     string `json:"legacySelfdestructWei"`
	Unexplained string `json:"unexplainedDeltaWei"`
}

func ZeroOVMAuditDelta() OVMAuditDelta { return OVMAuditDelta{"0", "0", "0", "0", "0", "0"} }

func (d *OVMAuditDelta) Add(other OVMAuditDelta, sign int64) error {
	left := []*string{&d.Balance, &d.Supply, &d.Difference, &d.Gas, &d.Suicide, &d.Unexplained}
	right := []string{other.Balance, other.Supply, other.Difference, other.Gas, other.Suicide, other.Unexplained}
	for n, p := range left {
		a, ok := new(big.Int).SetString(*p, 10)
		if !ok {
			return fmt.Errorf("invalid audit integer %q", *p)
		}
		b, ok := new(big.Int).SetString(right[n], 10)
		if !ok {
			return fmt.Errorf("invalid audit integer %q", right[n])
		}
		*p = a.Add(a, b.Mul(b, big.NewInt(sign))).String()
	}
	return nil
}

type OVMAuditEvent struct {
	Scope   string      `json:"scope"`
	TxHash  common.Hash `json:"txHash"`
	TxIndex int         `json:"txIndex"`
	Failed  bool        `json:"failed"`
	OVMAuditDelta
	Causes   []OVMAuditCause         `json:"causes,omitempty"`
	Balances []OVMAuditBalanceChange `json:"balances,omitempty"`
}

type OVMAuditBalanceChange struct {
	Address common.Address `json:"address"`
	Before  string         `json:"beforeWei"`
	After   string         `json:"afterWei"`
}

type OVMAuditObserver struct {
	Index     *OVMAuditIndex
	Total     OVMAuditDelta
	Events    []OVMAuditEvent
	Err       error
	before    map[common.Hash]common.Hash
	snapshots map[int]int
	current   OVMAuditEvent
}

func (s *StateDB) SetOVMAudit(o *OVMAuditObserver) { s.ovmAudit = o }
func (s *StateDB) OVMAudit() *OVMAuditObserver     { return s.ovmAudit }

func (s *StateDB) BeginOVMAuditScope(scope string, hash common.Hash, index int) {
	if o := s.ovmAudit; o != nil {
		o.before = make(map[common.Hash]common.Hash)
		o.snapshots = make(map[int]int)
		o.current = OVMAuditEvent{Scope: scope, TxHash: hash, TxIndex: index}
	}
}

func (s *StateDB) EndOVMAuditScope(failed bool) {
	o := s.ovmAudit
	if o == nil || o.Err != nil || o.before == nil {
		return
	}
	defer func() {
		if o.Err != nil {
			o.Err = fmt.Errorf("OVM audit scope=%s tx=%s index=%d: %w", o.current.Scope, o.current.TxHash, o.current.TxIndex, o.Err)
		}
	}()
	if err := o.Index.Metadata(s); err != nil {
		o.Err = err
		return
	}
	balance, supply, gas, suicide := new(big.Int), new(big.Int), new(big.Int), new(big.Int)
	keys := make([]common.Hash, 0, len(o.before))
	for key := range o.before {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return bytes.Compare(keys[i][:], keys[j][:]) < 0 })
	for _, key := range keys {
		before, after := o.before[key], s.GetState(dump.OvmEthAddress, key)
		if before == after {
			continue
		}
		slot, ok := o.Index.Lookup(crypto.Keccak256Hash(key[:]))
		if !ok {
			o.Err = fmt.Errorf("unclassified changed OVM slot %s (before=%s after=%s)", key.Hex(), before.Big(), after.Big())
			return
		}
		delta := new(big.Int).Sub(after.Big(), before.Big())
		switch slot.Kind {
		case "balance":
			balance.Add(balance, delta)
			o.current.Balances = append(o.current.Balances, OVMAuditBalanceChange{slot.Address, before.Big().String(), after.Big().String()})
		case "supply":
			supply.Add(supply, delta)
		case "metadata", "allowance":
		default:
			o.Err = fmt.Errorf("unexpected OVM slot kind %q", slot.Kind)
			return
		}
	}
	for _, c := range o.current.Causes {
		if c.Reverted {
			continue
		}
		value, _ := new(big.Int).SetString(c.Amount, 10)
		if c.Kind == "gas-shortfall" {
			gas.Add(gas, value)
		}
		if c.Kind == "selfdestruct" && !c.SDUpdate {
			suicide.Add(suicide, value)
		}
	}
	diff := new(big.Int).Sub(balance, supply)
	residual := new(big.Int).Sub(new(big.Int).Sub(new(big.Int).Set(diff), gas), suicide)
	o.current.Failed = failed
	o.current.OVMAuditDelta = OVMAuditDelta{balance.String(), supply.String(), diff.String(), gas.String(), suicide.String(), residual.String()}
	if o.Total.Balance == "" {
		o.Total = ZeroOVMAuditDelta()
	}
	if err := o.Total.Add(o.current.OVMAuditDelta, 1); err != nil {
		o.Err = err
		return
	}
	if diff.Sign() != 0 || len(o.current.Causes) != 0 {
		o.Events = append(o.Events, o.current)
	}
	o.before = nil
	o.snapshots = nil
	if err := s.Error(); err != nil {
		o.Err = err
	} else if o.Index.Err != nil {
		o.Err = o.Index.Err
	}
}

func (s *StateDB) AuditOVMGas(from common.Address, required, debited *big.Int, limit uint64, price *big.Int, shanghai bool) {
	if o := s.ovmAudit; o != nil {
		o.current.Causes = append(o.current.Causes, OVMAuditCause{Kind: "gas-shortfall", From: from, Amount: new(big.Int).Sub(required, debited).String(), Required: required.String(), Debited: debited.String(), GasLimit: limit, GasPrice: price.String(), Shanghai: shanghai})
	}
}

func (s *StateDB) AuditOVMSelfdestruct(from, to common.Address, balance *big.Int, pc uint64, depth int, updated bool) {
	if o := s.ovmAudit; o != nil {
		o.current.Causes = append(o.current.Causes, OVMAuditCause{Kind: "selfdestruct", From: from, To: to, Amount: balance.String(), PC: pc, Depth: depth, SDUpdate: updated})
	}
}

// AuditOVMPreimage is independent of persistent VM preimage recording. Even a
// preimage repeated after a reverted call remains useful classification evidence.
func (s *StateDB) AuditOVMPreimage(hash common.Hash, value []byte) {
	if o := s.ovmAudit; o != nil {
		o.Index.Preimage(hash, value)
	}
}

type OVMAuditScan struct {
	Root         common.Hash `json:"root"`
	CodeHash     common.Hash `json:"ovmCodeHash"`
	BalanceSlots uint64      `json:"balanceSlots"`
	Balances     string      `json:"balancesWei"`
	Supply       string      `json:"supplyWei"`
	Difference   string      `json:"differenceWei"`
}

// ScanOVMAudit reads the secure storage trie directly, not the observer's sums.
func (s *StateDB) ScanOVMAudit(index *OVMAuditIndex, root common.Hash) (OVMAuditScan, error) {
	result := OVMAuditScan{Root: root, CodeHash: s.GetCodeHash(dump.OvmEthAddress)}
	if !s.Exist(dump.OvmEthAddress) || len(s.GetCode(dump.OvmEthAddress)) == 0 {
		return result, fmt.Errorf("OVM_ETH code missing")
	}
	if err := index.Metadata(s); err != nil {
		return result, err
	}
	storage := s.StorageTrie(dump.OvmEthAddress)
	if storage == nil {
		return result, fmt.Errorf("OVM_ETH storage missing: %v", s.Error())
	}
	it := trie.NewIterator(storage.NodeIterator(nil))
	sum := new(big.Int)
	for it.Next() {
		if index.Check != nil {
			if err := index.Check(); err != nil {
				return result, err
			}
		}
		var content []byte
		if err := rlp.DecodeBytes(it.Value, &content); err != nil {
			return result, err
		}
		if len(content) > 32 || (len(content) > 0 && content[0] == 0) {
			return result, fmt.Errorf("invalid OVM storage integer")
		}
		value := new(big.Int).SetBytes(content)
		if value.Sign() == 0 {
			continue
		}
		slot, ok := index.Lookup(common.BytesToHash(it.Key))
		if !ok {
			return result, fmt.Errorf("unclassified nonzero OVM storage leaf %x", it.Key)
		}
		switch slot.Kind {
		case "balance":
			sum.Add(sum, value)
			result.BalanceSlots++
		case "supply", "metadata", "allowance":
		default:
			return result, fmt.Errorf("nonzero OVM %s leaf %x", slot.Kind, it.Key)
		}
	}
	if it.Err != nil {
		return result, it.Err
	}
	if index.Err != nil {
		return result, index.Err
	}
	if s.Error() != nil {
		return result, s.Error()
	}
	supply := s.GetState(dump.OvmEthAddress, common.BigToHash(big.NewInt(2))).Big()
	result.Balances = sum.String()
	result.Supply = supply.String()
	result.Difference = new(big.Int).Sub(sum, supply).String()
	return result, s.Error()
}
