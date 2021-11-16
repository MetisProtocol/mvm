#!/bin/bash

### DEPLOYMENT SCRIPT ###
# To be called from root of contracts dir #

# Required env vars
if [[ -z "$CONTRACTS_DEPLOYER_KEY" ]]; then
  echo "Must pass CONTRACTS_DEPLOYER_KEY"
  exit 1
fi
if [[ -z "$CONTRACTS_RPC_URL" ]]; then
  echo "Must pass CONTRACTS_RPC_URL"
  exit 1
fi

export CONTRACTS_TARGET_NETWORK=andromeda 

npx hardhat deploy \
  --ctc-max-transaction-gas-limit 1100000000 \
  --ctc-enqueue-gas-cost 60000 \
  --ctc-l2-gas-discount-divisor 32 \
  --l1-block-time-seconds 15 \
  --ovm-address-manager-owner 0x48fE1f85ff8Ad9D088863A42Af54d06a1328cF21 \
  --ovm-sequencer-address 0xcDf02971871B7736874E20B8487c019D28090019 \
  --ovm-proposer-address 0x9cB01d516D930EF49591a05B09e0D33E6286689D \
  --scc-fraud-proof-window 10 \
  --scc-sequencer-publish-window 12592000 \
  --network andromeda \
  --num-deploy-confirmations 12 \
  --mvm-metis-address 0x9E32b13ce7f2E80A01932B42553652E053D6ed8e \
  --mvm-metis-manager 0x48fE1f85ff8Ad9D088863A42Af54d06a1328cF21
  --l2chain 1088 
  
yarn autogen:markdown