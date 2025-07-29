# Metis

Metis is a next-generation Layer 2 Ethereum solution, designed to empower decentralized applications and businesses with scalability, low-cost transactions, and a user-centric ecosystem. By leveraging advanced technologies like Optimistic Rollups and Decentralized Sequencers, Metis aims to overcome the limitations of traditional blockchain networks while maintaining the core ethos of decentralization and security.

## Directory Structure

<pre>
├── <a href="./go/op-challenger">op-challenger</a>: Dispute game challenge agent
├── <a href="./go/op-preimage">op-preimage</a>: Go bindings for Preimage Oracle
├── <a href="./go/op-program">op-program</a>: Fault proof program
├── <a href="./l2geth">l2geth</a>: geth fork for Metis execution layer
├── <a href="./packages">packages</a>
│   ├── <a href="./packages/contracts">contracts</a>: Metis L1 and L2 smart contracts
│   ├── <a href="./packages/data-transport-layer">data transport layer</a>: long-running software service designed to reliably index transaction data from Layer 1
│   ├── <a href="./packages/batch-submitter">batch submitter</a>: Contains an executable batch submitter service which watches L1 and a local L2 node and submits data blobs
│   ├── <a href="./packages/message-relayer">message relayer</a>: A service for relaying messages from L2 to L1
</pre>

## Run a node

https://github.com/MetisProtocol/metis-ansible

https://github.com/MetisProtocol/metis-charts

## Solidity example

https://github.com/MetisProtocol/metis-hardhat-template

## Documentation

https://docs.metis.io/andromeda
