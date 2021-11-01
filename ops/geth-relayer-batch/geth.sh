#!/bin/bash
#set -e
RETRIES=${RETRIES:-40}
VERBOSITY=${VERBOSITY:-6}
echo "RETRIES => $RETRIES">> /app/log/t_geth.log
echo "VERBOSITY => $VERBOSITY">> /app/log/t_geth.log

function envSet() {
    VAR=$1
    export $VAR=$(echo $ADDRESSES | jq -r ".$2")
}

curl \
    --fail \
    --show-error \
    --silent \
    --retry-connrefused \
    --retry $RETRIES \
    --retry-delay 5 \
    $ROLLUP_STATE_DUMP_PATH \
    -o genesis.json

# wait for the dtl to be up, else geth will crash if it cannot connect
echo "ROLLUP_CLIENT_HTTP => $ROLLUP_CLIENT_HTTP">> /app/log/t_geth.log
CMD="$ROLLUP_CLIENT_HTTP/eth/syncing/$CHAIN_ID"
echo "CMD => $CMD">> /app/log/t_geth.log
curl \
    --fail \
    --show-error \
    --silent \
    --output /dev/null \
    --retry-connrefused \
    --retry $RETRIES \
    --retry-delay 1 \
    $CMD


# import the key that will be used to locally sign blocks
# this key does not have to be kept secret in order to be secure
# we use an insecure password ("pwd") to lock/unlock the password
echo "Importing private key" >> /app/log/t_geth.log
echo $BLOCK_SIGNER_KEY > key.prv
echo "pwd" > password
echo $BLOCK_SIGNER_KEY >> /app/log/t_geth.log
cat key.prv >> /app/log/t_geth.log

geth account import --password ./password ./key.prv --datadir /root/.ethereum
    
# initialize the geth node with the genesis file
echo "Initializing Geth node" >> /app/log/t_geth.log
geth --verbosity="$VERBOSITY" "$@" init genesis.json

#exec geth --verbosity="$VERBOSITY" "$@"
nohup geth --verbosity="$VERBOSITY" \
   --password ./password \
   --allow-insecure-unlock \
   --unlock $BLOCK_SIGNER_ADDRESS \
   --mine \
   --miner.etherbase $BLOCK_SIGNER_ADDRESS \
   "$@" >> /app/log/t_geth.log &
