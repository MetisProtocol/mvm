// SPDX-License-Identifier: MIT
pragma solidity ^0.8.9;

interface iMVM_InboxSenderManager {
    event InboxSenderSet(uint256 indexed blockNumber, address indexed inboxSender);

    function defaultInboxSender() external view returns (address);

    function setInboxSender(uint256 blockNumber, address inboxSender) external;

    function getInboxSender(uint256 blockNumber) external view returns (address);
}
