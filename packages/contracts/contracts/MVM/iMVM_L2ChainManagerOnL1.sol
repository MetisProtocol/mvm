// SPDX-License-Identifier: MIT
pragma solidity ^0.8.9;
pragma experimental ABIEncoderV2;

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
interface iMVM_L2ChainManagerOnL1 {

    event SwitchSeq (uint256 chainid, address wallet, address manager);
    event PushConfig (uint256 chainid, bytes configs);
   
    
    /********************
     * Public Functions *
     ********************/
    function switchSequencer(uint256 _chainId, address wallet, address manager) external payable;
    function pushConfig(uint256 _chainId, bytes calldata configs) external payable;
}
