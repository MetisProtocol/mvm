module github.com/ethereum-optimism/optimism/go/cannon

go 1.22.0

toolchain go1.22.7

replace (
	github.com/MetisProtocol/mvm/l2geth => ../../l2geth
	github.com/ethereum-optimism/optimism/go/op-preimage => ../op-preimage
	github.com/ethereum/go-ethereum => github.com/ethereum-optimism/op-geth v1.101503.0
)

require (
	github.com/MetisProtocol/mvm/l2geth v0.0.0-00010101000000-000000000000
	github.com/ethereum-optimism/optimism v1.12.0
	github.com/ethereum-optimism/optimism/go/op-preimage v0.0.0-00010101000000-000000000000
	github.com/ethereum/go-ethereum v1.15.3
	github.com/pkg/profile v1.7.0
	github.com/stretchr/testify v1.10.0
	github.com/urfave/cli/v2 v2.27.5
	golang.org/x/term v0.28.0
)

require (
	github.com/BurntSushi/toml v1.4.0 // indirect
	github.com/VictoriaMetrics/fastcache v1.12.2 // indirect
	github.com/aristanetworks/goarista v0.0.0-20170210015632-ea17b1a17847 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.5 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/deckarep/golang-set v1.8.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.3.0 // indirect
	github.com/edsrzf/mmap-go v1.1.0 // indirect
	github.com/elastic/gosigar v0.14.3 // indirect
	github.com/felixge/fgprof v0.9.5 // indirect
	github.com/go-stack/stack v1.8.1 // indirect
	github.com/golang/snappy v0.0.5-0.20220116011046-fa5810519dcb // indirect
	github.com/google/pprof v0.0.0-20241009165004-a3522334989c // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/hashicorp/golang-lru v0.5.5-0.20210104140557-80c98217689d // indirect
	github.com/holiman/uint256 v1.3.2 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/olekukonko/tablewriter v0.0.5 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/tsdb v0.10.0 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/rs/cors v1.11.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/steakknife/bloomfilter v0.0.0-20180922174646-6819c0d2a570 // indirect
	github.com/steakknife/hamming v0.0.0-20180906055917-c99c65617cd3 // indirect
	github.com/syndtr/goleveldb v1.0.1-0.20220614013038-64ee5596c38a // indirect
	github.com/xrash/smetrics v0.0.0-20240521201337-686a1a2994c1 // indirect
	golang.org/x/crypto v0.32.0 // indirect
	golang.org/x/sys v0.29.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/natefinch/npipe.v2 v2.0.0-20160621034901-c1b8fa8bdcce // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
