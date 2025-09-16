// SPDX-License-Identifier: MIT
pragma solidity ^0.8.9;

interface iMVM_ProposerRegistry {
    function getProposer(uint256 _chainId) external view returns (address);
    function setProposer(uint256 _chainId, address _proposer) external;
}
