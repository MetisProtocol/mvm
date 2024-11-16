// SPDX-License-Identifier: MIT
pragma solidity ^0.8.9;

/* Library Imports */
import { Lib_AddressResolver } from "../libraries/resolver/Lib_AddressResolver.sol";
import { iMVM_InboxSenderManager } from "./iMVM_InboxSenderManager.sol";

contract MVM_InboxSenderManager is iMVM_InboxSenderManager, Lib_AddressResolver {
    /*************
     * Constants *
     *************/
    string public constant CONFIG_OWNER_KEY = "METIS_MANAGER";

    /*************
     * Variables *
     *************/
    // blockNumber => inboxSender
    mapping(uint256 => InboxSender[]) public inboxSenders;

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
        if (blockNumbers.length > 0) {
            require(
                blockNumber > blockNumbers[blockNumbers.length - 1],
                "Block number must be greater than the previous block number."
            );
        }

        for (uint256 i = 0; i < _inboxSenders.length; ++i) {
            inboxSenders[blockNumber].push(_inboxSenders[i]);
        }
        blockNumbers.push(blockNumber);
        for (uint256 i = 0; i < _inboxSenders.length; ++i) {
            emit InboxSenderSet(blockNumber, _inboxSenders[i].sender, _inboxSenders[i].senderType);
        }
    }

    function getInboxSender(uint256 blockNumber, InboxSenderType inboxSenderType) external view override returns (address) {
        for (int256 i = int256(blockNumbers.length) - 1; i >= 0; i--) {
            if (blockNumbers[uint256(i)] <= blockNumber) {
                InboxSender[] storage inboxSendersAtBlock = inboxSenders[blockNumbers[uint256(i)]];
                for (uint256 j = 0; j < inboxSendersAtBlock.length; ++j) {
                    InboxSender memory inboxSender = inboxSendersAtBlock[j];
                    if (inboxSender.senderType == inboxSenderType) {
                        return inboxSender.sender;
                    }
                }
            }
        }

        return defaultInboxSender[inboxSenderType];
    }
}
