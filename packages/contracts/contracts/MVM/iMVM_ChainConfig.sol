// SPDX-License-Identifier: MIT
pragma solidity ^0.8.9;
/* Contract Imports */
/* External Imports */
interface iMVM_ChainConfig {
    event NewChainConfig (address sender, bytes32 key, bytes value);
    event NewAccountConfig (address sender, address key, bytes value);
   
    function setConfig(bytes32 key, bytes calldata values) external;
        
    function setConfig(bytes calldata values) external;
    
    function getConfig(bytes32 key) view external returns (bytes memory);
    
    function getConfig() view external returns (bytes memory);
    
}