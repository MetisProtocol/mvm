// SPDX-License-Identifier: MIT
pragma solidity ^0.8.9;

import { Ownable } from "@openzeppelin/contracts/access/Ownable.sol";
import { iMVM_ProposerRegistry } from "./iMVM_ProposerRegistry.sol";

/**
 * @title MVM_ProposerRegistry
 * @dev This contract manages proposers for different chain IDs.
 *
 * Compiler used: solc
 * Runtime target: EVM
 */
contract MVM_ProposerRegistry is Ownable, iMVM_ProposerRegistry {
    mapping(uint256 => address) public proposers;

    // Initialize the contract with the deployer as the owner
    // The owner is the security council minority multisig
    constructor(address _owner) {
        transferOwnership(_owner);
    }

    // Set the proposer for a specific chain ID
    // Note: the proposer can be a zero address to disable proposals from that chain
    function setProposer(uint256 _chainId, address _proposer) external override onlyOwner {
        proposers[_chainId] = _proposer;
    }

    function getProposer(uint256 _chainId) external view override returns (address) {
        return proposers[_chainId];
    }
}
