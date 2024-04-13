#!/bin/sh

# FIXME: Cannot use set -e since bash is not installed in Dockerfile
# set -e

RETRIES=${RETRIES:-40}
VERBOSITY=${VERBOSITY:-6}
EXTERNAL_IP=${EXTERNAL_IP:-default-ip}
PORT=${PORT:-30303}
NAT_SET="any"
MAX_PEER=${MAX_PEER:-50}
BOOTNODES=${BOOTNODES:-}
if [ "$EXTERNAL_IP" != "default-ip" ]; then
    NAT_SET="extip:$EXTERNAL_IP"
fi
echo "Nat set is $NAT_SET"
echo "P2P port is $PORT"
echo "BOOTNODES is $BOOTNODES"

# get the genesis file from the deployer
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
curl \
    --fail \
    --show-error \
    --silent \
    --output /dev/null \
    --retry-connrefused \
    --retry $RETRIES \
    --retry-delay 1 \
    $ROLLUP_CLIENT_HTTP

# import the key that will be used to locally sign blocks
# this key does not have to be kept secret in order to be secure
# we use an insecure password ("pwd") to lock/unlock the password
echo "Importing private key"
echo $BLOCK_SIGNER_KEY > key.prv
echo "pwd" > password
geth account import --password ./password ./key.prv

# initialize the geth node with the genesis file
echo "Initializing Geth node"
geth --verbosity="$VERBOSITY" "$@" init genesis.json

# remove static-node
# rm $(echo $DATADIR)/static-nodes.json

# start the geth node
echo "Starting Geth node"
exec geth \
  --verbosity="$VERBOSITY" \
  --password ./password \
  --allow-insecure-unlock \
  --unlock $BLOCK_SIGNER_ADDRESS \
  --mine \
  --miner.etherbase $BLOCK_SIGNER_ADDRESS \
  --miner.recommit 2s\
  --maxpeers $MAX_PEER \
  --nat $NAT_SET \
  --port $PORT \
  --bootnodes="${BOOTNODES}" \
  --syncmode full \
  --txpool.nolocals="1" \
  "$@"
