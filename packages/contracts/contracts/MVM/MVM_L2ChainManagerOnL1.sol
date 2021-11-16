// SPDX-License-Identifier: MIT
pragma solidity ^0.8.9;
pragma experimental ABIEncoderV2;

/* Library Imports */
import { CrossDomainEnabled } from "../libraries/bridge/CrossDomainEnabled.sol";
import { Lib_PredeployAddresses } from "../libraries/constants/Lib_PredeployAddresses.sol";
import { Lib_AddressManager } from "../libraries/resolver/Lib_AddressManager.sol";
import { iOVM_SequencerFeeVault } from "../L2/predeploys/iOVM_SequencerFeeVault.sol";
import { iMVM_L2ChainManagerOnL1 } from "./iMVM_L2ChainManagerOnL1.sol";
import { iMVM_DiscountOracle } from "./iMVM_DiscountOracle.sol";
/* Interface Imports */

/* External Imports */

/**
 * @title MVM_L2ChainManagerOnL1
 * @dev if want support multi l2 chain on l1,it should add a manager to desc 
 * how many l2 chain now ,and dispatch the l2 chain id to make it is unique.
 *
 * Compiler used: solc
 * Runtime target: EVM
 */
contract MVM_L2ChainManagerOnL1 is iMVM_L2ChainManagerOnL1, CrossDomainEnabled {
 
    /*************
     * Constants *
     *************/
    string constant public CONFIG_OWNER_KEY = "METIS_MANAGER";
    
    /*************
     * Variables *
     *************/
    address public addressmgr;
    // chainid => sequencer
    mapping (uint256 => address) squencers;
    
    // chainid => configs (unused for now);
    mapping (uint256 => bytes) configs;
    
    /***************
     * Constructor *
     ***************/
    // This contract lives behind a proxy, so the constructor parameters will go unused.
    constructor() CrossDomainEnabled(address(0)) {}

    
    /**********************
     * Function Modifiers *
     **********************/

    modifier onlyManager() {
        require(
            msg.sender == Lib_AddressManager(addressmgr).getAddress(CONFIG_OWNER_KEY),
            "MVM_L2ChainManagerOnL1: Function can only be called by the METIS_MANAGER."
        );
        _;
    }

    /********************
     * Public Functions *
     ********************/
    function switchSequencer(uint256 _chainId, address wallet, address manager) public onlyManager payable {
            
        bytes memory message =
            abi.encodeWithSelector(
                iOVM_SequencerFeeVault.finalizeChainSwitch.selector,
                wallet,
                manager
            );
        
        // Send calldata into L2
        sendCrossDomainMessageViaChainId(
            _chainId,
            Lib_PredeployAddresses.SEQUENCER_FEE_WALLET,
            uint32(1_000_000_000),
            message,
            msg.value
        );

        emit SwitchSeq(_chainId, wallet, manager);
    }
    
    function pushConfig(uint256 _chainId, bytes calldata _configs) public payable {
        bytes memory message =
            abi.encodeWithSelector(
                iOVM_SequencerFeeVault.finalizeChainConfig.selector,
                _configs
            );
            
        // Send calldata into L2
        sendCrossDomainMessageViaChainId(
            _chainId,
            Lib_PredeployAddresses.SEQUENCER_FEE_WALLET,
            uint32(1_000_000_000),
            message,
            msg.value
        );
        
        emit PushConfig(_chainId, _configs);
    }
}
