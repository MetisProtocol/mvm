#!/bin/bash
set -ex
region='us-east-2'
if [ -n "$1"  ]
then
    region=$1
fi

stack="metis"
if [ -n "$2" ]
then
    stack=$2
fi

if [ -n "$3" ]
then
    account=$3
fi

# --no-cache
echo 'Building metis_l2_geth image'
cp ./efs-utils.conf ./settings/efs-utils.conf
cmd="sed -i s#REGION_VAR_FOR_ENV#$region#g ./settings/efs-utils.conf"
$cmd
# docker images|grep metis_l2_geth|awk '{print $3}'|xargs docker rmi -f
docker build --no-cache -f ./Dockerfile -t metis_l2_geth ../geth-relayer-batch
cmd="sed -i s#$region#REGION_VAR_FOR_ENV#g ./settings/efs-utils.conf"
$cmd

profile="aws --profile default ecr get-login-password --region $region"
login="docker login --username AWS --password-stdin $account.dkr.ecr.$region.amazonaws.com"
$profile | $login

echo 'Pushing metis_l2_geth'
l2geth="docker tag metis_l2_geth:latest $account.dkr.ecr.$region.amazonaws.com/$stack-l2-geth:latest"
$l2geth
l2geth_push="docker push  $account.dkr.ecr.$region.amazonaws.com/$stack-l2-geth:latest"
$l2geth_push

echo 'Pushing data-transport-layer'
dtl="docker tag ethereumoptimism/data-transport-layer:latest $account.dkr.ecr.$region.amazonaws.com/$stack-dtl:latest"
$dtl
dtl_push="docker push $account.dkr.ecr.$region.amazonaws.com/$stack-dtl:latest"
$dtl_push

