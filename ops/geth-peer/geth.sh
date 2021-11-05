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

echo "get the genesis => $ROLLUP_STATE_DUMP_PATH">> /app/log/t_geth.log
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

echo "wait for dtl => $ROLLUP_CLIENT_HTTP">> /app/log/t_geth.log
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
echo "Importing private key">> /app/log/t_geth.log
echo $BLOCK_SIGNER_KEY > key.prv
echo "pwd" > password
geth account import --password ./password ./key.prv

# initialize the geth node with the genesis file
echo "Initializing Geth node">> /app/log/t_geth.log
geth --verbosity="$VERBOSITY" "$@" init genesis.json

echo "get main node enode info">> /app/log/t_geth.log
JSON='{"jsonrpc":"2.0","id":0,"method":"admin_nodeInfo","params":[]}'
NODE_INFO=$(curl --silent --fail --show-error -H "Content-Type: application/json" --retry-connrefused --retry $RETRIES --retry-delay 3  -d $JSON $L2_URL)
echo "NODE_INFO => $NODE_INFO">> /app/log/t_geth.log

NODE_ENODE=$(echo $NODE_INFO | jq -r '.result.enode')
NODE_IP=$(echo $NODE_INFO | jq -r '.result.ip')

# if [ "$NODE_IP" = "127.0.0.1" ];then
#     HOST_IP=$(/sbin/ip route | awk '/default/ { print $3 }')
#     NODE_ENODE=${NODE_ENODE//127.0.0.1/$HOST_IP}
# fi
IP=$(echo "$L2_URL"|awk -F'[/:]' '{print $4}')
NODE_ENODE=$(echo "$NODE_ENODE"|sed "s/$NODE_IP/$IP/g")

# mkdir $(echo $DATADIR)
touch $(echo $DATADIR)/static-nodes.json

echo "[\"$NODE_ENODE\"]" > $(echo $DATADIR)/static-nodes.json

echo "starting Geth peer node">> /app/log/t_geth.log
#exec geth --verbosity="$VERBOSITY" "$@"
nohup geth \
  --verbosity="$VERBOSITY" \
  --password ./password \
  --allow-insecure-unlock \
  --unlock $BLOCK_SIGNER_ADDRESS \
  --mine \
  --miner.etherbase $BLOCK_SIGNER_ADDRESS \
  "$@" >> /app/log/t_geth.log &
