package eth

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/MetisProtocol/mvm/l2geth/eth/downloader"
	"github.com/MetisProtocol/mvm/l2geth/p2p/enode"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/naoina/toml"
)

func writeBlockWhitelist(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "block-peers.json")
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBlockPeerWhitelist(t *testing.T) {
	key1, _ := crypto.GenerateKey()
	key2, _ := crypto.GenerateKey()
	pub1 := fmt.Sprintf("%x", crypto.FromECDSAPub(&key1.PublicKey)[1:])
	pub2 := fmt.Sprintf("%x", crypto.FromECDSAPub(&key2.PublicKey)[1:])
	id1, id2 := enode.PubkeyToIDV4(&key1.PublicKey), enode.PubkeyToIDV4(&key2.PublicKey)
	path := writeBlockWhitelist(t, fmt.Sprintf(`{"networks":[{"networkId":1088,"peers":[
 "enode://%s@192.0.2.1:30303?discport=30301",
 "enode://%s@[2001:db8::1]:30304",
 "enode://%s@192.0.2.2:30305"]}, {"networkId":59902,"peers":[]}]}`, pub1, pub1, pub2))
	policy, err := LoadBlockPeerWhitelist(path, 1088, downloader.FullSync)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		id   enode.ID
		addr net.Addr
		want bool
	}{
		{"paired", id1, &net.TCPAddr{IP: net.ParseIP("192.0.2.1"), Port: 12345}, true},
		{"mapped IPv4", id1, &net.TCPAddr{IP: net.ParseIP("::ffff:192.0.2.1")}, true},
		{"second IP", id1, &net.TCPAddr{IP: net.ParseIP("2001:0db8:0:0:0:0:0:1")}, true},
		{"second key", id2, &net.TCPAddr{IP: net.ParseIP("192.0.2.2")}, true},
		{"crossed pairs", id1, &net.TCPAddr{IP: net.ParseIP("192.0.2.2")}, false},
		{"wrong key", enode.ID{}, &net.TCPAddr{IP: net.ParseIP("192.0.2.1")}, false},
		{"wrong IP", id1, &net.TCPAddr{IP: net.ParseIP("192.0.2.3")}, false},
		{"missing address", id1, nil, false},
		{"missing IP", id1, &net.TCPAddr{}, false},
		{"non TCP", id1, &net.UDPAddr{IP: net.ParseIP("192.0.2.1")}, false},
	}
	// Sharing the ETH short ID must not grant the identity's remaining bytes.
	collision := id1
	collision[31] ^= 1
	tests = append(tests, struct {
		name string
		id   enode.ID
		addr net.Addr
		want bool
	}{"short ID collision", collision, &net.TCPAddr{IP: net.ParseIP("192.0.2.1")}, false})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := policy.allows(tt.id, tt.addr); got != tt.want {
				t.Fatalf("allows = %v, want %v", got, tt.want)
			}
		})
	}
	for _, mode := range []downloader.SyncMode{downloader.FullSync, downloader.FastSync} {
		empty, err := LoadBlockPeerWhitelist(path, 59902, mode)
		if err != nil || empty == nil || empty.allows(id1, tests[0].addr) {
			t.Fatalf("empty policy: %v, %v", empty, err)
		}
		missing, err := LoadBlockPeerWhitelist(path, 42, mode)
		if err != nil || missing != nil || !missing.allows(id1, nil) {
			t.Fatalf("missing network: %v, %v", missing, err)
		}
	}
	if p, err := LoadBlockPeerWhitelist("", 1088, downloader.LightSync); err != nil || p != nil {
		t.Fatalf("disabled policy: %v", err)
	}
	if _, err := LoadBlockPeerWhitelist(path, 1088, downloader.LightSync); err == nil {
		t.Fatal("light mode accepted active policy")
	}
	if _, err := LoadBlockPeerWhitelist(path, 59902, downloader.LightSync); err == nil {
		t.Fatal("light mode accepted empty active policy")
	}
	if _, err := LoadBlockPeerWhitelist(path, 42, downloader.LightSync); err != nil {
		t.Fatal(err)
	}
	// A loaded policy is a startup snapshot, independent of later file edits.
	if err := os.WriteFile(path, []byte(`{"networks":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if !policy.allows(id1, tests[0].addr) {
		t.Fatal("policy changed after editing file")
	}
}

func TestBlockPeerWhitelistInvalid(t *testing.T) {
	key := fmt.Sprintf("%x", crypto.FromECDSAPub(&testBankKey.PublicKey)[1:])
	cases := []string{
		``, `{`, `null`, `{}`, `{"networks":null}`,
		`{"networks":[],"typo":true}`, `{"networks":[]} {}`, `{"networks":[]} garbage`,
		`{"networks":[{"peers":[]}]}`, `{"networks":[{"networkId":1}]}`,
		`{"networks":[{"networkId":1,"peers":null}]}`,
		`{"networks":[{"networkId":-1,"peers":[]}]}`,
		`{"networks":[{"networkId":18446744073709551616,"peers":[]}]}`,
		`{"networks":[{"networkId":1,"peers":[]},{"networkId":1,"peers":[]}]}`,
		`{"networks":[{"networkId":1,"peers":[{}]}]}`,
	}
	// Reject the old object format rather than maintaining a second parser.
	cases = append(cases, fmt.Sprintf(`{"networks":[{"networkId":1,"peers":[{"ip":"192.0.2.1","publicKey":%q}]}]}`, key))
	invalidEnodes := []string{
		"", key, "enode://" + key,
		"enode://" + key + "@localhost:30303",
		"enode://" + key + "@example.invalid:30303",
		"enode://" + key + "@192.0.2.1/32:30303",
		"enode://" + key + "@192.0.2.1",
		"enode://" + key + "@192.0.2.1:65536",
		"enode://" + key + "@192.0.2.1:abc",
		"enode://" + key + "@192.0.2.1:30303?discport=invalid",
		"https://" + key + "@192.0.2.1:30303",
	}
	for _, pub := range []string{"", key[:40], key[:64], "0x" + key, strings.Repeat("0", 128), strings.Repeat("z", 128)} {
		invalidEnodes = append(invalidEnodes, "enode://"+pub+"@192.0.2.1:30303")
	}
	for _, entry := range invalidEnodes {
		cases = append(cases, fmt.Sprintf(`{"networks":[{"networkId":1,"peers":[%q]}]}`, entry))
	}
	for i, content := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			// Invalid entries in other networks must also stop startup.
			if _, err := LoadBlockPeerWhitelist(writeBlockWhitelist(t, content), 1088, downloader.FullSync); err == nil {
				t.Fatalf("accepted %s", content)
			}
		})
	}
	if _, err := LoadBlockPeerWhitelist(filepath.Join(t.TempDir(), "missing"), 1088, downloader.FullSync); err == nil {
		t.Fatal("accepted missing file")
	}
	// Network zero is valid and distinct from a missing field.
	if p, err := LoadBlockPeerWhitelist(writeBlockWhitelist(t, `{"networks":[{"networkId":0,"peers":[]}]}`), 0, downloader.FullSync); err != nil || p == nil {
		t.Fatalf("network zero: %v", err)
	}
}

func TestBlockPeerWhitelistTOML(t *testing.T) {
	cfg := DefaultConfig
	cfg.BlockPeerWhitelistFile = "relative/peers.json"
	settings := toml.Config{NormFieldName: func(_ reflect.Type, key string) string { return key }, FieldToKey: func(_ reflect.Type, key string) string { return key }}
	wrapped := struct{ Eth Config }{Eth: cfg}
	encoded, err := settings.Marshal(&wrapped)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct{ Eth Config }
	if err := settings.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Eth.BlockPeerWhitelistFile != cfg.BlockPeerWhitelistFile {
		t.Fatalf("path lost in TOML round trip: %s", encoded)
	}
}

func TestBlockPeerWhitelistRejectsInvalidStartup(t *testing.T) {
	for _, mode := range []downloader.SyncMode{downloader.FullSync, downloader.FastSync} {
		cfg := DefaultConfig
		cfg.SyncMode = mode
		cfg.BlockPeerWhitelistFile = writeBlockWhitelist(t, `{"networks":[{"networkId":1088,"peers":["invalid-enode"]}]}`)
		// Invalid files must stop startup before any database/network initialization.
		if _, err := New(nil, &cfg); err == nil || !strings.Contains(err.Error(), "P2P block whitelist") {
			t.Fatalf("startup error=%v", err)
		}
	}
}
