#!/bin/bash
URL=https://gist.githubusercontent.com/akriventsev/bdae760708d7b5ce106b62b07d72a61c/raw/af8c701aa73fc17ee07966ca0b850c72847ac04b/testnet-gton-network-addresses.json
ADDRESSES=$(curl --fail --show-error --silent --retry-connrefused --retry 2 --retry-delay 5 $URL)

function envSet() {
    VAR=$1
    export $VAR=$(echo $ADDRESSES | jq -r ".$2")
}

# set the address to the proxy gateway if possible
envSet L1_STANDARD_BRIDGE_ADDRESS Proxy__OVM_L1StandardBridge

envSet L1_CROSS_DOMAIN_MESSENGER_ADDRESS Proxy__OVM_L1CrossDomainMessenger

envSet L1_GCD_MANAGER_ADDRESS Proxy__MVM_ChainManager

echo $L1_CROSS_DOMAIN_MESSENGER_ADDRESS

export L2_BLOCK_GAS_LIMIT=1100000000
export L2_CHAIN_ID=50021
export BLOCK_SIGNER_ADDRESS=0x68CCe680B6b1d4Bce7D5Da2cF6d3637A43312433
export L1_FEE_WALLET_ADDRESS=0xAFe4d2a5972651A9DBC8BC60213540CF70dBFD35
export WHITELIST_OWNER=0xAFe4d2a5972651A9DBC8BC60213540CF70dBFD35
export GAS_PRICE_ORACLE_OWNER=0xAFe4d2a5972651A9DBC8BC60213540CF70dBFD35

export GAS_PRICE_ORACLE_OVERHEAD=2750
export GAS_PRICE_ORACLE_SCALAR=40000000
export GAS_PRICE_ORACLE_L1_BASE_FEE=150000000000
export GAS_PRICE_ORACLE_GAS_PRICE=40000000000
export GAS_PRICE_ORACLE_DECIMALS=6

export GCD_ADDRESS=0x1EF834d6D3694a932A2082678EDd543E3Eb3412b
#export MIN_L1_ERC20_BRIDGE_COST=400000000000000000
export MIN_L1_ERC20_BRIDGE_COST=1000000

yarn build:dump


cd ./dist/dumps
exec python -c \
            'import BaseHTTPServer as bhs, SimpleHTTPServer as shs; bhs.HTTPServer(("0.0.0.0", 8081), shs.SimpleHTTPRequestHandler).serve_forever()'
