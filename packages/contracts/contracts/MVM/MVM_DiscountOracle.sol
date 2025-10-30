// SPDX-License-Identifier: MIT
pragma solidity ^0.8.9;
/* Contract Imports */
/* External Imports */

import { iMVM_DiscountOracle } from "./iMVM_DiscountOracle.sol";
import { Lib_AddressResolver } from "../libraries/resolver/Lib_AddressResolver.sol";
import { Lib_Uint } from "../libraries/utils/Lib_Uint.sol";

contract MVM_DiscountOracle is iMVM_DiscountOracle, Lib_AddressResolver {
    // Current l2 gas price
    uint256 public discount;
    uint256 public minL2Gas;
    mapping(address => bool) public xDomainWL;
    mapping(uint256 => uint256) public l2ChainSeqGas;
    bool public allowAllXDomainSenders;
    string public constant CONFIG_OWNER_KEY = "METIS_MANAGER";

    /**********************
     * Function Modifiers *
     **********************/

    modifier onlyManager() {
        require(
            msg.sender == resolve(CONFIG_OWNER_KEY),
            "MVM_DiscountOracle: Function can only be called by the METIS_MANAGER."
        );
        _;
    }

    constructor(
        address _addressManager,
        uint256 _initialDiscount
    ) Lib_AddressResolver(_addressManager) {
        discount = _initialDiscount;
        minL2Gas = 200_000;
        allowAllXDomainSenders = false;
    }

    function getMinL2Gas() public view override returns (uint256) {
        return minL2Gas;
    }

    function getDiscount() public view override returns (uint256) {
        return discount;
    }

    function setDiscount(uint256 _discount) public override onlyManager {
        discount = _discount;
    }

    function setMinL2Gas(uint256 _minL2Gas) public override onlyManager {
        minL2Gas = _minL2Gas;
    }

    function setWhitelistedXDomainSender(
        address _sender,
        bool _isWhitelisted
    ) external override onlyManager {
        xDomainWL[_sender] = _isWhitelisted;
    }

    function isXDomainSenderAllowed(address _sender) public view override returns (bool) {
        return (allowAllXDomainSenders || xDomainWL[_sender]);
    }

    function setAllowAllXDomainSenders(bool _allowAllXDomainSenders) public override onlyManager {
        allowAllXDomainSenders = _allowAllXDomainSenders;
    }

    function processL2SeqGas(address sender, uint256 _chainId) public payable override {
        require(isXDomainSenderAllowed(sender), "sender is not whitelisted");
        require(_chainId > 0, "incorrect chainId");
        l2ChainSeqGas[_chainId] += msg.value;
    }

    function withdrawToSeq(uint256 _amount, uint256 _chainId) public override onlyManager {
        require(_amount > 0, "incorrect amount");
        require(_amount <= address(this).balance, "insufficient balance");
        require(_chainId > 0, "incorrect chainId");
        require(_amount <= l2ChainSeqGas[_chainId], "this chain sequencer gas is not enough");
        address _to = resolve(
            string(abi.encodePacked(Lib_Uint.uint2str(_chainId), "_MVM_Sequencer_Wrapper"))
        );
        require(_to != address(0) && _to != address(this), "unknown sequencer address");
        l2ChainSeqGas[_chainId] -= _amount;
        (bool success, ) = _to.call{ value: _amount }("");
        require(success, "transfer failed");
    }
}
