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
    // blockNumber => inboxSende
    mapping(uint256 => address) public inboxSenders;
    uint256[] public blockNumbers;
    address public defaultInboxSender;

    /***************
     * Constructor *
     ***************/
    constructor(address _libAddressManager, address _defaultInboxSender)
        Lib_AddressResolver(_libAddressManager)
    {
        defaultInboxSender = _defaultInboxSender;
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
    function setInboxSender(uint256 blockNumber, address inboxSender)
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

        inboxSenders[blockNumber] = inboxSender;
        blockNumbers.push(blockNumber);

        emit InboxSenderSet(blockNumber, inboxSender);
    }

    function getInboxSender(uint256 blockNumber) external view override returns (address) {
        for (int256 i = int256(blockNumbers.length) - 1; i >= 0; i--) {
            if (blockNumbers[uint256(i)] <= blockNumber) {
                return inboxSenders[blockNumbers[uint256(i)]];
            }
        }

        return defaultInboxSender;
    }
}
