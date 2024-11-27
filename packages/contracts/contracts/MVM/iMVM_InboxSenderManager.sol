// SPDX-License-Identifier: MIT
pragma solidity ^0.8.9;

interface iMVM_InboxSenderManager {
    enum InboxSenderType {
        InboxSender,
        InboxBlobSender
    }

    struct InboxSender {
        InboxSenderType senderType;
        address sender;
    }

    event InboxSenderSet(uint256 indexed blockNumber, address indexed inboxSender, InboxSenderType indexed inboxSenderType);

    function defaultInboxSender(InboxSenderType senderType) external view returns (address);

    function setInboxSenders(uint256 blockNumber, InboxSender[] calldata _inboxSenders) external;

    function overwriteLastInboxSenders(uint256 blockNumber, InboxSender[] calldata _inboxSenders) external;

    function getInboxSender(uint256 blockNumber, InboxSenderType inboxSenderType) external view returns (address);
}
