package merkletrie

import (
	"bytes"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/MetisProtocol/mvm/l2geth/crypto"
)

// defaultHashes: same as the array used by Solidity's getMerkleRoot.
// If a level has odd node count, the last leftover is paired with defaultHashes[depth].
var defaultHashes = []common.Hash{
	common.HexToHash("0x290decd9548b62a8d60345a988386fc84ba6bc95484008f6362f93160ef3e563"),
	common.HexToHash("0x633dc4d7da7256660a892f8f1604a44b5432649cc8ec5cb3ced4c4e6ac94dd1d"),
	common.HexToHash("0x890740a8eb06ce9be422cb8da5cdafc2b58c0a5e24036c578de2a433c828ff7d"),
	common.HexToHash("0x3b8ec09e026fdc305365dfc94e189a81b38c7597b3d941c279f042e8206e0bd8"),
	common.HexToHash("0xecd50eee38e386bd62be9bedb990706951b65fe053bd9d8a521af753d139e2da"),
	common.HexToHash("0xdefff6d330bb5403f63b14f33b578274160de3a50df4efecf0e0db73bcdd3da5"),
	common.HexToHash("0x617bdd11f7c0a11f49db22f629387a12da7596f9d1704d7465177c63d88ec7d7"),
	common.HexToHash("0x292c23a9aa1d8bea7e2435e555a4a60e379a5a35f3f452bae60121073fb6eead"),
	common.HexToHash("0xe1cea92ed99acdcb045a6726b2f87107e8a61620a232cf4d7d5b5766b3952e10"),
	common.HexToHash("0x7ad66c0a68c72cb89e4fb4303841966e4062a76ab97451e3b9fb526a5ceb7f82"),
	common.HexToHash("0xe026cc5a4aed3c22a58cbd3d2ac754c9352c5436f638042dca99034e83636516"),
	common.HexToHash("0x3d04cffd8b46a874edf5cfae63077de85f849a660426697b06a829c70dd1409c"),
	common.HexToHash("0xad676aa337a485e4728a0b240d92b3ef7b3c372d06d189322bfd5f61f1e7203e"),
	common.HexToHash("0xa2fca4a49658f9fab7aa63289c91b7c7b6c832a6d0e69334ff5b0a3483d09dab"),
	common.HexToHash("0x4ebfd9cd7bca2505f7bef59cc1c12ecc708fff26ae4af19abe852afe9e20c862"),
	common.HexToHash("0x2def10d13dd169f550f578bda343d9717a138562e0093b380a1120789d53cf10"),
}

// WriteTrie builds a Merkle tree in the same way as the Solidity getMerkleRoot,
// and returns (root, preimages).
// We assume each `values[i]` is already a 32-byte leaf. We do not store any extra info.
func WriteTrie(values []hexutil.Bytes) (common.Hash, []hexutil.Bytes) {
	var preimages []hexutil.Bytes

	// putNode merges left||right => newHash = keccak256(left||right).
	// Then we store that 64 bytes in db, and record it as a preimage.
	putNode := func(left, right common.Hash) common.Hash {
		var buf bytes.Buffer
		buf.Write(left[:])
		buf.Write(right[:])
		raw := buf.Bytes() // 64 bytes
		newHash := common.BytesToHash(crypto.Keccak256(raw))

		// keep as a preimage
		cp := make([]byte, len(raw))
		copy(cp, raw)
		preimages = append(preimages, cp)
		return newHash
	}

	// 1) Collect leaves
	n := len(values)
	leaves := make([]common.Hash, n)
	for i, val := range values {
		if len(val) != 32 {
			panic(fmt.Errorf("expected 32-byte leaf, got len=%d", len(val)))
		}
		copy(leaves[i][:], val)
	}

	// 2) Pairwise merges, same as Solidity
	row := leaves
	size := n
	depth := 0
	for size > 1 {
		half := size / 2
		odd := size%2 == 1
		idx := 0
		for i := 0; i < half; i++ {
			left := row[2*i]
			right := row[2*i+1]
			newH := putNode(left, right)
			row[idx] = newH
			idx++
		}
		if odd {
			leftover := row[size-1]
			def := defaultHashes[depth]
			newH := putNode(leftover, def)
			row[idx] = newH
			idx++
		}
		size = idx
		depth++
	}

	var root common.Hash
	if size == 1 {
		root = row[0]
	}

	// Also consider each leaf(32 bytes) as a "preimage" if you want them in the second return
	// This is optional, but usually we might want it.
	for _, lf := range leaves {
		cp := make([]byte, 32)
		copy(cp, lf[:])
		preimages = append(preimages, cp)
	}

	return root, preimages
}

// ReadTrie traverses from the root to collect all leaves in BFS order (left to right).
// This order matches the original input order if we built the tree the same way.
func ReadTrie(root common.Hash, getPreimage func(key common.Hash) []byte) []hexutil.Bytes {
	// default set for skipping
	skip := make(map[common.Hash]bool)
	for _, dh := range defaultHashes {
		skip[dh] = true
	}

	var result []common.Hash
	if (root == common.Hash{}) {
		return nil
	}

	// We'll use BFS (queue). This ensures we scan from left to right at each level,
	// which reproduces the original leaf order.
	queue := []common.Hash{root}

	for len(queue) > 0 {
		h := queue[0]
		queue = queue[1:]

		// skip default
		if skip[h] {
			continue
		}
		raw := getPreimage(h)
		if raw == nil {
			// not found => it's presumably a leaf
			// store it in result
			result = append(result, h)
			continue
		}
		// If we find 64 bytes, it's an internal node => left||right
		if len(raw) != 64 {
			// this shouldn't happen if all internal nodes store 64 bytes
			panic(fmt.Errorf("invalid node data length=%d for hash=%s", len(raw), h.Hex()))
		}
		left := common.BytesToHash(raw[:32])
		right := common.BytesToHash(raw[32:])
		// enqueue left->right
		queue = append(queue, left, right)
	}

	// Now 'result' contains the leaves in BFS order,
	// which matches the left->right layering from the original merges.
	out := make([]hexutil.Bytes, len(result))
	for i, leafH := range result {
		cp := make([]byte, 32)
		copy(cp, leafH[:])
		out[i] = cp
	}
	return out
}
