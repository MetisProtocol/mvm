#!/bin/bash

URL="https://metis-us-east-2-dev-json.s3.us-east-2.amazonaws.com/addresses.json"
RETRIES=2
ADDRESSES=$(curl --fail --show-error --silent --retry-connrefused --retry $RETRIES --retry-delay 5 $URL)
   
function envSet() {
    VAR=$1
    export $VAR=$(echo $ADDRESSES | jq -r ".$2")
}

# set the address to the proxy gateway if possible
envSet L1_STANDARD_BRIDGE_ADDRESS Proxy__OVM_L1StandardBridge

envSet L1_CROSS_DOMAIN_MESSENGER_ADDRESS Proxy__OVM_L1CrossDomainMessenger

envSet L1_METIS_MANAGER_ADDRESS Proxy__MVM_ChainManager

export L2_BLOCK_GAS_LIMIT=1100000000
export L2_CHAIN_ID=666
export BLOCK_SIGNER_ADDRESS=0x00000398232E2064F896018496b4b44b3D62751F
export L1_FEE_WALLET_ADDRESS=0x67bE001A254B81Dc44EaB9F5a7C66889a5c009AE
export WHITELIST_OWNER=0x67bE001A254B81Dc44EaB9F5a7C66889a5c009AE
export GAS_PRICE_ORACLE_OWNER=0x67bE001A254B81Dc44EaB9F5a7C66889a5c009AE
export GAS_PRICE_ORACLE_OVERHEAD=2750
export GAS_PRICE_ORACLE_SCALAR=55000000
export GAS_PRICE_ORACLE_L1_BASE_FEE=150000000000
export GAS_PRICE_ORACLE_GAS_PRICE=60000000000
export GAS_PRICE_ORACLE_DECIMALS=6
export METIS_ADDRESS=0xe552Fb52a4F19e44ef5A967632DBc320B0820639
export MIN_L1_ERC20_BRIDGE_COST=400000000000000000

yarn build:dump

cd ./dist/dumps
exec python -c \
            'import BaseHTTPServer as bhs, SimpleHTTPServer as shs; bhs.HTTPServer(("0.0.0.0", 8081), shs.SimpleHTTPRequestHandler).serve_forever()'
