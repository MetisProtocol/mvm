// SPDX-License-Identifier: MIT
pragma solidity ^0.8.9;

/* Library Imports */
import { Lib_PredeployAddresses } from "../../libraries/constants/Lib_PredeployAddresses.sol";

/* Contract Imports */
import { L2StandardBridge } from "../messaging/L2StandardBridge.sol";
import { CrossDomainEnabled } from "../../libraries/bridge/CrossDomainEnabled.sol";

/**
 * @title OVM_SequencerFeeVault
 * @dev Simple holding contract for fees paid to the Sequencer. Likely to be replaced in the future
 * but "good enough for now".
 */
interface iOVM_SequencerFeeVault {
    /*************
     * Constants *
     *************/

    event ChainSwitch (address l1Wallet, address l2Manager);
    event ConfigChange(bytes config);

    /********************
     * Public Functions *
     ********************/

    function withdraw(uint256 amount) external payable;
    
    function finalizeChainSwitch(address _FeeWallet, address _L2Manager) external;
    
    function finalizeChainConfig(bytes calldata config) external;
    function send(address payable to, uint256 amount) external;
    
    function sendBatch(address payable[] calldata tos, uint256[] calldata amounts) external;
    function getL2Manager() view external returns(address);
}
