#syntax=docker/dockerfile:1
FROM node:20-bullseye-slim AS builder
RUN apt update -y \
    && apt install -y git python3 python3-dev pkg-config libusb-1.0-0-dev make g++ gcc build-essential libudev-dev
WORKDIR /optimism

# Install dependencies
COPY . .
RUN corepack prepare && corepack enable
RUN yarn install --immutable
RUN yarn build

FROM node:20-alpine AS deployer
RUN apk add --update --no-cache git curl python3 bash jq ca-certificates

WORKDIR /opt/optimism/

COPY --from=builder /optimism/*.json /optimism/yarn.lock ./
COPY --from=builder /optimism/node_modules ./node_modules

# copy deps (would have been nice if docker followed the symlinks required)
COPY --from=builder /optimism/packages/core-utils/package.json ./packages/core-utils/package.json
COPY --from=builder /optimism/packages/core-utils/dist ./packages/core-utils/dist

# get the needed built artifacts
WORKDIR /opt/optimism/packages/contracts
COPY --from=builder /optimism/packages/contracts/dist ./dist
COPY --from=builder /optimism/packages/contracts/*.json ./
COPY --from=builder /optimism/packages/contracts/node_modules ./node_modules
COPY --from=builder /optimism/packages/contracts/artifacts ./artifacts
COPY --from=builder /optimism/packages/contracts/src ./src

# get non-build artifacts from the host
COPY packages/contracts/bin ./bin
COPY packages/contracts/contracts ./contracts
COPY packages/contracts/hardhat.config.ts ./
COPY packages/contracts/deploy ./deploy
COPY packages/contracts/tasks ./tasks
COPY packages/contracts/test/helpers/constants.ts ./test/helpers/constants.ts
COPY packages/contracts/scripts ./scripts

COPY --chmod=775 ./ops/scripts/deployer.sh .
ENTRYPOINT yarn run deploy


FROM node:20-alpine AS dtl
RUN apk add --no-cache curl bash jq ca-certificates

WORKDIR /opt/optimism

# copy top level files
COPY --from=builder /optimism/*.json ./
COPY --from=builder /optimism/yarn.lock .
COPY --from=builder /optimism/node_modules ./node_modules

# copy deps (would have been nice if docker followed the symlinks required)
COPY --from=builder /optimism/packages/core-utils/package.json ./packages/core-utils/package.json
COPY --from=builder /optimism/packages/core-utils/dist ./packages/core-utils/dist
COPY --from=builder /optimism/packages/common-ts/package.json ./packages/common-ts/package.json
COPY --from=builder /optimism/packages/common-ts/dist ./packages/common-ts/dist

COPY --from=builder /optimism/packages/contracts/package.json ./packages/contracts/package.json
COPY --from=builder /optimism/packages/contracts/deployments ./packages/contracts/deployments
COPY --from=builder /optimism/packages/contracts/dist ./packages/contracts/dist
COPY --from=builder /optimism/packages/contracts/artifacts ./packages/contracts/artifacts

# copy the service
WORKDIR /opt/optimism/packages/data-transport-layer
COPY --from=builder /optimism/packages/data-transport-layer/dist ./dist
COPY --from=builder /optimism/packages/data-transport-layer/package.json .
COPY --from=builder /optimism/packages/data-transport-layer/node_modules ./node_modules
COPY --chmod=775 ./ops/scripts/dtl.sh .
VOLUME [ "/data" ]
ENTRYPOINT ["./dtl.sh"]

FROM node:20-alpine AS relayer
RUN apk add --no-cache curl bash jq ca-certificates

WORKDIR /opt/optimism

# copy top level files
COPY --from=builder /optimism/*.json ./
COPY --from=builder /optimism/yarn.lock .
COPY --from=builder /optimism/node_modules ./node_modules

# copy deps (would have been nice if docker followed the symlinks required)
COPY --from=builder /optimism/packages/core-utils/package.json ./packages/core-utils/package.json
COPY --from=builder /optimism/packages/core-utils/dist ./packages/core-utils/dist
COPY --from=builder /optimism/packages/common-ts/package.json ./packages/common-ts/package.json
COPY --from=builder /optimism/packages/common-ts/dist ./packages/common-ts/dist

COPY --from=builder /optimism/packages/contracts/package.json ./packages/contracts/package.json
COPY --from=builder /optimism/packages/contracts/deployments ./packages/contracts/deployments
COPY --from=builder /optimism/packages/contracts/dist ./packages/contracts/dist
COPY --from=builder /optimism/packages/contracts/artifacts ./packages/contracts/artifacts

# copy the service
WORKDIR /opt/optimism/packages/message-relayer
COPY --from=builder /optimism/packages/message-relayer/dist ./dist
COPY --from=builder /optimism/packages/message-relayer/package.json .
COPY --from=builder /optimism/packages/message-relayer/exec ./exec
COPY --from=builder /optimism/packages/message-relayer/node_modules ./node_modules
COPY  --chmod=775 ./ops/scripts/relayer.sh .
ENTRYPOINT ["./relayer.sh"]

FROM node:20-alpine AS batch-submitter
RUN apk add --no-cache curl bash jq ca-certificates

WORKDIR /opt/optimism

# copy top level files
COPY --from=builder /optimism/*.json /optimism/yarn.lock ./
COPY --from=builder /optimism/node_modules ./node_modules

# copy deps (would have been nice if docker followed the symlinks required)
COPY --from=builder /optimism/packages/core-utils/package.json ./packages/core-utils/package.json
COPY --from=builder /optimism/packages/core-utils/dist ./packages/core-utils/dist
COPY --from=builder /optimism/packages/common-ts/package.json ./packages/common-ts/package.json
COPY --from=builder /optimism/packages/common-ts/dist ./packages/common-ts/dist

COPY --from=builder /optimism/packages/contracts/package.json ./packages/contracts/package.json
COPY --from=builder /optimism/packages/contracts/deployments ./packages/contracts/deployments
COPY --from=builder /optimism/packages/contracts/dist ./packages/contracts/dist
COPY --from=builder /optimism/packages/contracts/artifacts ./packages/contracts/artifacts

# copy the service
WORKDIR /opt/optimism/packages/batch-submitter
COPY --from=builder /optimism/packages/batch-submitter/package.json ./
COPY --from=builder /optimism/packages/batch-submitter/dist ./dist
COPY --from=builder /optimism/packages/batch-submitter/exec ./exec
COPY --from=builder /optimism/packages/batch-submitter/node_modules ./node_modules
COPY --chmod=775 ./ops/scripts/batches.sh .
VOLUME [ "/data" ]
ENTRYPOINT ["./batches.sh"]

