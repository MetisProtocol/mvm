// SPDX-License-Identifier: MIT
pragma solidity ^0.8.9;

import {Lib_AddressResolver} from "../libraries/resolver/Lib_AddressResolver.sol";
import {iMVM_InboxSenderManager} from "./iMVM_InboxSenderManager.sol";

/* Library Imports */

contract MVM_InboxSenderManager is iMVM_InboxSenderManager, Lib_AddressResolver {
    /*************
     * Constants *
     *************/
    string public constant CONFIG_OWNER_KEY = "METIS_MANAGER";

    /*************
     * Variables *
     *************/
    // blockNumber => InboxSenderType => inboxSender
    mapping(uint256 => mapping(InboxSenderType => InboxSender)) public inboxSenders;

    uint256[] public blockNumbers;
    mapping(InboxSenderType => address) public defaultInboxSender;

    /***************
     * Constructor *
     ***************/
    constructor(address _libAddressManager, InboxSender[] memory _defaultInboxSenders)
        Lib_AddressResolver(_libAddressManager)
    {
        for (uint256 i = 0; i < _defaultInboxSenders.length; ++i) {
            defaultInboxSender[_defaultInboxSenders[i].senderType] = _defaultInboxSenders[i].sender;
        }
    }

    /**********************
     * Function Modifiers *
     **********************/
    modifier onlyManager() {
        require(
            msg.sender == resolve("METIS_MANAGER"),
            "MVM_InboxSenderManager: Function can only be called by the METIS_MANAGER."
        );
        _;
    }

    /********************
     * Public Functions *
     ********************/
    function setInboxSenders(uint256 blockNumber, InboxSender[] calldata _inboxSenders)
        external
        override
        onlyManager
    {
        _setInboxSenders(blockNumber, _inboxSenders);
    }

    // allow us to overwrite the last block number and its senders, just in case if we made any mistake
    function overwriteLastInboxSenders(uint256 blockNumber, InboxSender[] calldata _inboxSenders)
        external
        override
        onlyManager
    {
        require(blockNumbers.length > 0, "MVM_InboxSenderManager: No block to update.");

        // pop the last block
        uint256 lastBlockNumber = blockNumbers[blockNumbers.length - 1];
        blockNumbers.pop();

        // clean up the last block senders
        mapping(InboxSenderType => InboxSender) storage lastInboxSenders = inboxSenders[lastBlockNumber];
        delete lastInboxSenders[InboxSenderType.InboxSender];
        delete lastInboxSenders[InboxSenderType.InboxBlobSender];

        // write the new senders
        _setInboxSenders(blockNumber, _inboxSenders);
    }

    function getInboxSender(uint256 blockNumber, InboxSenderType inboxSenderType) external view override returns (address) {
        uint256 blockNumerCounts = blockNumbers.length;
        if (blockNumerCounts == 0) {
            return defaultInboxSender[inboxSenderType];
        }

        for (int256 i = int256(blockNumerCounts) - 1; i >= 0; i--) {
            if (blockNumbers[uint256(i)] <= blockNumber) {
                address sender = inboxSenders[blockNumbers[uint256(i)]][inboxSenderType].sender;
                if (sender != address(0)) {
                    return sender;
                }
            }
        }

        return defaultInboxSender[inboxSenderType];
    }

    /********************
     * Internal Functions *
     ********************/
    function _setInboxSenders(uint256 blockNumber, InboxSender[] calldata _inboxSenders)
    private
    {
        require(_inboxSenders.length > 0, "MVM_InboxSenderManager: Inbox senders cannot be empty.");

        if (blockNumbers.length > 0) {
            require(
                blockNumber > blockNumbers[blockNumbers.length - 1],
                "MVM_InboxSenderManager: Block number must be greater than the previous block number."
            );
        }

        for (uint256 i = 0; i < _inboxSenders.length; ++i) {
            require(
                _inboxSenders[i].sender != address(0),
                "MVM_InboxSenderManager: Inbox sender cannot be empty."
            );
            inboxSenders[blockNumber][_inboxSenders[i].senderType] = _inboxSenders[i];
            emit InboxSenderSet(blockNumber, _inboxSenders[i].sender, _inboxSenders[i].senderType);
        }

        blockNumbers.push(blockNumber);
    }
}
