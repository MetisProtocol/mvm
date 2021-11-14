// SPDX-License-Identifier: MIT
pragma solidity ^0.8.9;

import { iOVM_SequencerFeeVault } from "./iOVM_SequencerFeeVault.sol";
import { Lib_PredeployAddresses } from "../../libraries/constants/Lib_PredeployAddresses.sol";

/**
 * @title OVM_DeployerWhitelist
 * @dev The Deployer Whitelist is a temporary predeploy used to provide additional safety during the
 * initial phases of our mainnet roll out. It is owned by the Optimism team, and defines accounts
 * which are allowed to deploy contracts on Layer2. The Execution Manager will only allow an
 * ovmCREATE or ovmCREATE2 operation to proceed if the deployer's address whitelisted.
 */
contract OVM_DeployerWhitelist {
    /**********
     * Events *
     **********/

    event OwnerChanged(address oldOwner, address newOwner);
    event WhitelistStatusChanged(address deployer, bool whitelisted);
    event WhitelistDisabled(address oldOwner);
    mapping (address => bool) public xDomainWL;

    /**********************
     * Contract Constants *
     **********************/

    // WARNING: When owner is set to address(0), the whitelist is disabled.
    address public owner;
    mapping(address => bool) public whitelist;
    bool public allowArbitraryDeployment;
    bool public allowAllXDomainSenders;

    /**********************
     * Function Modifiers *
     **********************/

    /**
     * Blocks functions to anyone except the contract owner.
     */
    modifier onlyOwner() {
        require(msg.sender == owner, "Function can only be called by the owner of this contract.");
        _;
    }
    
    modifier onlyManager() {
        require(msg.sender == iOVM_SequencerFeeVault(Lib_PredeployAddresses.SEQUENCER_FEE_WALLET).getL2Manager(),
                "Function can only be called by the l2manager.");
        _;
    }

    /********************
     * Public Functions *
     ********************/

    /**
     * Adds or removes an address from the deployment whitelist.
     * @param _deployer Address to update permissions for.
     * @param _isWhitelisted Whether or not the address is whitelisted.
     */
    function setWhitelistedDeployer(address _deployer, bool _isWhitelisted) external onlyOwner {
        whitelist[_deployer] = _isWhitelisted;
        allowArbitraryDeployment = false;
        emit WhitelistStatusChanged(_deployer, _isWhitelisted);
    }
    
    function setWhitelistedXDomainSender(
        address _sender,
        bool _isWhitelisted
    )
        external
        onlyOwner
    {
        xDomainWL[_sender] = _isWhitelisted;
        allowAllXDomainSenders = false;
        emit WhitelistStatusChanged(_sender, _isWhitelisted);
    }

    /**
     * Updates the owner of this contract.
     * @param _owner Address of the new owner.
     */
    function setOwner(address _owner) public onlyOwner {
        emit OwnerChanged(owner, _owner);
        owner = _owner;
    }

    /**
     * Permanently enables arbitrary contract deployment and deletes the owner.
     */
    function enableArbitraryContractDeployment() external onlyOwner {
        emit WhitelistDisabled(owner);
        allowArbitraryDeployment = true;
    }
    
    function enableAllXDomainSenders()
        external
        onlyOwner
    {
        emit WhitelistDisabled(owner);
        allowAllXDomainSenders = true;
    }

    /**
     * Checks whether an address is allowed to deploy contracts.
     * @param _deployer Address to check.
     * @return _allowed Whether or not the address can deploy contracts.
     */
    function isDeployerAllowed(address _deployer) external view returns (bool) {
        return (owner == address(0) || allowArbitraryDeployment || whitelist[_deployer]);
    }
    
    function isXDomainSenderAllowed(
        address _sender
    )
        external
        view
        returns (
            bool
        )
    {
        return (
            owner == address(0)
            || allowAllXDomainSenders == true
            || xDomainWL[_sender]
        );
    }
}
