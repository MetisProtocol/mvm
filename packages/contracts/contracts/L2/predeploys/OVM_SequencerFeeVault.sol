// SPDX-License-Identifier: MIT
pragma solidity ^0.8.9;

/* Library Imports */
import { Lib_PredeployAddresses } from "../../libraries/constants/Lib_PredeployAddresses.sol";

/* Contract Imports */
import { L2StandardBridge } from "../messaging/L2StandardBridge.sol";
import { CrossDomainEnabled } from "../../libraries/bridge/CrossDomainEnabled.sol";
import { iOVM_SequencerFeeVault } from "./iOVM_SequencerFeeVault.sol";
import { iMVM_ChainConfig } from "../../MVM/iMVM_ChainConfig.sol";

/**
 * @title OVM_SequencerFeeVault
 * @dev Simple holding contract for fees paid to the Sequencer. Likely to be replaced in the future
 * but "good enough for now".
 */
contract OVM_SequencerFeeVault is iOVM_SequencerFeeVault, CrossDomainEnabled{
    
    /*************
     * Variables *
     *************/

    // Address on L1 that will hold the fees once withdrawn. Dynamically initialized within l2geth.
    address public l1FeeWallet;
    address public l1Manager;
    address public l2Manager;

    /***************
     * Constructor *
     ***************/

    /**
     * @param _l1FeeWallet Initial address for the L1 wallet that will hold fees once withdrawn.
     * Currently HAS NO EFFECT in production because l2geth will mutate this storage slot during
     * the genesis block. This is ONLY for testing purposes.
     */
    constructor(address _l2CrossDomainMessenger, 
                address _l1FeeWallet,
                address _l1Manager) CrossDomainEnabled(_l2CrossDomainMessenger){
        l1FeeWallet = _l1FeeWallet;
        l1Manager = _l1Manager;
    }
    
    modifier onlyManager {
        require(msg.sender == l2Manager, "not allowed");
        _;
    }

    /************
     * Fallback *
     ************/

    receive() external payable {}
    // Fallback function is called when msg.data is not empty
    fallback() external payable {}

    /********************
     * Public Functions *
     ********************/

    function withdraw(uint256 amount) public onlyManager payable{
        require(
            address(this).balance >= amount,
            "not enough balance to withraw"
        );
        
        if (amount == 0) {
            amount = address(this).balance;
        }

        L2StandardBridge(Lib_PredeployAddresses.L2_STANDARD_BRIDGE).withdrawTo{value:msg.value}(
            Lib_PredeployAddresses.MVM_COINBASE,
            l1FeeWallet,
            amount,
            0,
            bytes("")
        );
    }
    
    function finalizeChainSwitch(
        address _FeeWallet,
        address _L2Manager
    ) external virtual onlyFromCrossDomainAccount(l1Manager) {
    
        l1FeeWallet = _FeeWallet;
        l2Manager = _L2Manager;
        emit ChainSwitch(l1FeeWallet, l2Manager);
    }
    
    function finalizeChainConfig(bytes calldata values) external virtual onlyFromCrossDomainAccount(l1Manager) {
        iMVM_ChainConfig(Lib_PredeployAddresses.MVM_CHAIN_CONFIG).setConfig(values);
        emit ConfigChange(values);
    }
    
    function send(address payable to, uint256 amount) public onlyManager{
        (bool sent, ) = to.call{value: amount}("");
        require(sent, "Failed to send metis");
    }
    
    function sendBatch(address payable[] calldata tos, uint256[] calldata amounts) public onlyManager {
        require(tos.length == amounts.length, "lengths of the parameters do not match");
        for (uint i = 0; i < tos.length; i++) {
            send(tos[i], amounts[i]);
        }
    }
    
    function getL2Manager() view public returns(address) {
        return l2Manager;
    }
}
