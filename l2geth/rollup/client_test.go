package rollup

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/jarcoal/httpmock"

	"github.com/MetisProtocol/mvm/l2geth/common"
	"github.com/MetisProtocol/mvm/l2geth/common/hexutil"
)

const url = "http://localhost:9999"

func TestBatchHeaderPackUnpackHash(t *testing.T) {
	header := &BatchHeader{
		BatchRoot:         common.HexToHash("0xceee1e2f2476f1f88dc4189211f1a04447b3d3b0d8df15d2d5d846d1be69549a"),
		BatchSize:         big.NewInt(0x77),
		PrevTotalElements: big.NewInt(0x5bc),
		ExtraData:         hexutil.MustDecode("0x00000000000000000000000000000000000000000000000000000000676ab5f6000000000000000000000000879cf65ca2fda7fe7152b0ec6c19282a30120921bdc1eab0f7830415ee8f92aa33ee8f735365ab52bf317c2fc64ac359f5f250db0000000000000000000000000000000000000000000000000000000000000633"),
	}

	packed, err := header.Pack()
	if err != nil {
		t.Errorf("Pack failed: %v", err)
	}

	newHeader := new(BatchHeader)
	if err := newHeader.Unpack(packed); err != nil {
		t.Errorf("Unpack failed: %v", err)
	}

	if header.BatchRoot != newHeader.BatchRoot {
		t.Errorf("BatchRoot mismatch, expected: %s, got: %s", header.BatchRoot.Hex(), newHeader.BatchRoot.Hex())
	}
	if header.BatchSize.Cmp(newHeader.BatchSize) != 0 {
		t.Errorf("BatchSize mismatch, expected: %s, got: %s", header.BatchSize.String(), newHeader.BatchSize.String())
	}
	if header.PrevTotalElements.Cmp(newHeader.PrevTotalElements) != 0 {
		t.Errorf("PrevTotalElements mismatch, expected: %s, got: %s", header.PrevTotalElements.String(), newHeader.PrevTotalElements.String())
	}
	if fmt.Sprintf("%x", header.ExtraData) != fmt.Sprintf("%x", newHeader.ExtraData) {
		t.Errorf("ExtraData mismatch, expected: %x, got: %x", header.ExtraData, newHeader.ExtraData)
	}

	if header.Hash() != common.HexToHash("0x7eb1d7286be43da5e6a089c253f5dfd5d2308dd5a3ae6cb4d80ac7f74e4258aa") {
		t.Errorf("Hash mismatch, header hash is: %s", header.Hash().Hex())
	}
}

func TestRollupClientCannotConnect(t *testing.T) {
	endpoint := fmt.Sprintf("%s/eth/context/latest", url)
	client := NewClient(url, big.NewInt(1))

	httpmock.ActivateNonDefault(client.client.GetClient())

	response, _ := httpmock.NewJsonResponder(
		400,
		map[string]interface{}{},
	)
	httpmock.RegisterResponder(
		"GET",
		endpoint,
		response,
	)

	context, err := client.GetLatestEthContext()
	if context != nil {
		t.Fatal("returned value is not nil")
	}
	if !errors.Is(err, errHTTPError) {
		t.Fatalf("Incorrect error returned: %s", err)
	}
}
func TestDecodedJSON(t *testing.T) {
	str := []byte(`
	{
		"index": 643116,
		"batchIndex": 21083,
		"blockNumber": 25954867,
		"timestamp": 1625605288,
		"gasLimit": "11000000",
		"target": "0x4200000000000000000000000000000000000005",
		"origin": null,
		"data": "0xf86d0283e4e1c08343eab8941a5245ea5210c3b57b7cfdf965990e63534a7b528901a055690d9db800008081aea019f7c6719f1718475f39fb9e5a6a897c3bd5057488a014666e5ad573ec71cf0fa008836030e686f3175dd7beb8350809b47791c23a19092a8c2fab1f0b4211a466",
		"queueOrigin": "sequencer",
		"value": "0x1a055690d9db80000",
		"queueIndex": null,
		"decoded": {
			"nonce": "2",
			"gasPrice": "15000000",
			"gasLimit": "4451000",
			"value": "0x1a055690d9db80000",
			"target": "0x1a5245ea5210c3b57b7cfdf965990e63534a7b52",
			"data": "0x",
			"sig": {
				"v": 1,
				"r": "0x19f7c6719f1718475f39fb9e5a6a897c3bd5057488a014666e5ad573ec71cf0f",
				"s": "0x08836030e686f3175dd7beb8350809b47791c23a19092a8c2fab1f0b4211a466"
			}
		},
		"confirmed": true
	}`)

	tx := new(Transaction)
	json.Unmarshal(str, tx)
	cmp, _ := new(big.Int).SetString("1a055690d9db80000", 16)
	if tx.Value.ToInt().Cmp(cmp) != 0 {
		t.Fatal("Cannot decode")
	}
}
