#!/bin/bash

ADDRESSES=$(curl --fail --show-error --silent --retry-connrefused --retry $RETRIES --retry-delay 5 $1)
   
function envSet() {
    VAR=$1
    export $VAR=$(echo $ADDRESSES | jq -r ".$2")
}

# set the address to the proxy gateway if possible
envSet L1_STANDARD_BRIDGE_ADDRESS Proxy__OVM_L1StandardBridge
if [ $L1_STANDARD_BRIDGE_ADDRESS == null ]; then
    envSet L1_STANDARD_BRIDGE_ADDRESS L1StandardBridge
fi

envSet L1_CROSS_DOMAIN_MESSENGER_ADDRESS Proxy__OVM_L1CrossDomainMessenger
if [ $L1_CROSS_DOMAIN_MESSENGER_ADDRESS == null ]; then
    envSet L1_CROSS_DOMAIN_MESSENGER_ADDRESS L1CrossDomainMessenger
fi

envSet L1_METIS_MANAGER_ADDRESS Proxy__MVM_ChainManager
if [ $L1_METIS_MANAGER_ADDRESS == null ]; then
    envSet L1_METIS_MANAGER_ADDRESS MVM_ChainManager
fi

export L2_BLOCK_GAS_LIMIT=1100000000
export L2_CHAIN_ID=666
export BLOCK_SIGNER_ADDRESS=0x00000398232E2064F896018496b4b44b3D62751F
export L1_FEE_WALLET_ADDRESS=0x67bE001A254B81Dc44EaB9F5a7C66889a5c009AE
export WHITELIST_OWNER=0x67bE001A254B81Dc44EaB9F5a7C66889a5c009AE
export GAS_PRICE_ORACLE_OWNER=0x67bE001A254B81Dc44EaB9F5a7C66889a5c009AE
export GAS_PRICE_ORACLE_OVERHEAD=2750
export GAS_PRICE_ORACLE_SCALAR=1500000
export GAS_PRICE_ORACLE_L1_BASE_FEE=1200000000
export GAS_PRICE_ORACLE_GAS_PRICE=60000000
export GAS_PRICE_ORACLE_DECIMALS=6

yarn build:dump

cd ./dist/dumps
exec python -c \
            'import BaseHTTPServer as bhs, SimpleHTTPServer as shs; bhs.HTTPServer(("0.0.0.0", 8081), shs.SimpleHTTPRequestHandler).serve_forever()'